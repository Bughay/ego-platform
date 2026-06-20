package deepseek

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	maxAgentIterations = 100
	iterationDelay     = 2 * time.Second
	retryDelay         = 5 * time.Second
	maxRetries         = 3
	memoryWindowSize   = 20
)

type React struct {
	Conversation []AgentResponse
}

type AgentResponse struct {
	Reasoning string `json:"reasoning"`
	Act       string `json:"act"`
}

type Agent struct {
	Model        string
	SystemPrompt string
	UserPrompt   string
	Memory       []Message
	Thinking     bool // run the text-generation call as a DeepSeek reasoning ("thinking") model
	Tools        []Tool
	Registry     map[string]func(string) (string, error)
	SchemaData   []byte // embedded tool schema JSON (preferred over Path)
	Path         string // fallback: file path to tool schema JSON
	MaxTokens    int
}

// sleepCtx pauses for d but returns ctx.Err() early if the context is cancelled,
// so the agent's retry/iteration backoffs can be interrupted promptly instead of
// blocking for the full delay.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// trimMemory keeps the system prompt and first user message then slides a
// window over the most recent iterations to cap memory growth.
func (a *Agent) trimMemory() {
	const head = 2
	if len(a.Memory) <= head+memoryWindowSize {
		return
	}
	trimmed := make([]Message, head+memoryWindowSize)
	copy(trimmed[:head], a.Memory[:head])
	copy(trimmed[head:], a.Memory[len(a.Memory)-memoryWindowSize:])
	a.Memory = trimmed
}

// convertTextToJSON repairs a non-JSON agent message into the strict
// {reasoning, act} shape using the embedded tool schema. It is only used when
// the model's own text output did not already parse as JSON, so it is a repair
// step rather than the primary path.
func (a *Agent) convertTextToJSON(ctx context.Context, text string) (string, error) {
	slog.Info("agent text not valid JSON, converting via schema")

	toolsStr, err := ToolsToLLMStringFromData(a.SchemaData)
	if err != nil {
		return "", fmt.Errorf("load tools for conversion: %w", err)
	}

	schema := `{
    "reasoning": "your step-by-step thinking about what to do",
    "act": "tool_name|arg1,arg2 OR finish|your_final_answer"
}`
	conversionContext := "You convert another agent's plain-text answer into a single JSON object that matches the schema and tools described below. Read the text you are given and express it as the JSON object — do not add information that is not in the text. "
	jsonMessages := []Message{
		{Role: "system", Content: conversionContext + "\nYou must respond in this exact JSON format:\n" + schema + "\nHere are the tools schema: \n" + toolsStr},
		{Role: "user", Content: text},
	}

	result, err := DeepseekOneshotJSON(ctx, a.Model, jsonMessages, 0.1, a.MaxTokens)
	if err != nil {
		return "", fmt.Errorf("convert to JSON: %w", err)
	}

	slog.Info("converted to JSON successfully")
	return result, nil
}

// stripCodeFence removes markdown code fences (```json ... ``` or ``` ... ```)
// that the LLM sometimes wraps around its JSON response.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx != -1 {
			s = s[:idx]
		}
	}
	return strings.TrimSpace(s)
}

// parseAgentResponse strips any markdown code fence and unmarshals the raw model
// output into an AgentResponse.
func parseAgentResponse(raw string) (*AgentResponse, error) {
	var resp AgentResponse
	if err := json.Unmarshal([]byte(stripCodeFence(raw)), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// toolCallToResponse maps a native tool-calling reply onto the {reasoning, act}
// shape the ReAct loop already understands, so Run stays unchanged:
//
//   - a tool_call becomes act "name|<json-args>" (content is the reasoning). The
//     args JSON is preserved verbatim — SplitN(act,"|",2) in Run only splits on
//     the first "|", so any "|" inside the arguments survives and the tool
//     handler receives the exact arguments object.
//   - no tool_call, but content that itself parses as a {reasoning, act} object
//     (the model occasionally still follows that format) is used as-is.
//   - no tool_call and plain content is treated as the final answer (finish).
//   - nothing usable (no tool_call, blank content) returns nil so the caller can
//     fall back to the text path.
func toolCallToResponse(content string, toolCalls []ToolCall) *AgentResponse {
	if len(toolCalls) > 0 {
		tc := toolCalls[0] // one tool per ReAct step; ignore any extras
		return &AgentResponse{
			Reasoning: content,
			Act:       tc.Function.Name + "|" + tc.Function.Arguments,
		}
	}
	if resp, err := parseAgentResponse(content); err == nil {
		return resp
	}
	if strings.TrimSpace(content) != "" {
		return &AgentResponse{Reasoning: content, Act: "finish|" + content}
	}
	return nil
}

// attempt makes one reasoning step. v2 tries the structured tools path FIRST:
// it calls the model with the official tools parameter and maps the resulting
// content (reasoning) + tool_calls (act) onto an AgentResponse. Only when that
// response is unusable (no tool_calls and no parseable/non-empty content, or a
// non-context error) does it fall back to the v1 behavior — generate plain text,
// parse it as the {reasoning, act} JSON, and repair via convertTextToJSON if
// needed. Transport/API/context errors propagate so oneloop can retry the step.
func (a *Agent) attempt(ctx context.Context, messages []Message) (*AgentResponse, error) {
	content, toolCalls, finish, err := deepseekToolCall(ctx, a.Model, messages, 0.1, a.MaxTokens, a.Tools, a.Thinking)
	if err == nil {
		_ = finish // available for logging if needed
		if resp := toolCallToResponse(content, toolCalls); resp != nil {
			return resp, nil
		}
		slog.Info("tools response unusable, falling back to text mode")
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err // never burn the text fallback on a dead context
	} else {
		slog.Warn("tools call failed, falling back to text mode", "err", err)
	}

	text, terr := DeepseekOneshotMemory(ctx, a.Model, messages, 0.1, a.MaxTokens, a.Thinking)
	if terr != nil {
		return nil, fmt.Errorf("text generation: %w", terr)
	}

	if resp, perr := parseAgentResponse(text); perr == nil {
		return resp, nil // text was already valid JSON — use it as-is
	}

	if len(a.SchemaData) == 0 {
		return nil, fmt.Errorf("agent text not parseable and no schema for conversion")
	}
	converted, cerr := a.convertTextToJSON(ctx, text)
	if cerr != nil {
		return nil, cerr
	}
	return parseAgentResponse(converted)
}

func (a *Agent) oneloop(ctx context.Context, messages []Message) (*AgentResponse, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		resp, err := a.attempt(ctx, messages)
		if err == nil {
			return resp, nil
		}
		// Don't burn retries on a cancelled/expired context — bail immediately.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		lastErr = err
		slog.Error("agent attempt failed", "retry", i, "err", err)
		if serr := sleepCtx(ctx, retryDelay); serr != nil {
			return nil, serr
		}
	}

	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// FinishAnswer returns the text after the "finish|" prefix of a finished
// agent response.
func (r *AgentResponse) FinishAnswer() string {
	return strings.TrimPrefix(r.Act, "finish|")
}

func (a *Agent) Run(ctx context.Context) (*AgentResponse, error) {
	var (
		toolsDesc string
		err       error
	)
	if len(a.SchemaData) > 0 {
		toolsDesc, err = ToolsToLLMStringFromData(a.SchemaData)
	} else {
		toolsDesc, err = ToolsToLLMString(a.Path)
	}
	if err != nil {
		return nil, fmt.Errorf("load tools: %w", err)
	}

	// Send the tools via the official `tools` parameter. Callers normally set
	// a.Tools already; load it from the embedded schema if they didn't so the
	// structured path always has tool definitions to offer.
	if len(a.Tools) == 0 && len(a.SchemaData) > 0 {
		if loaded, lerr := LoadToolsFromData(a.SchemaData); lerr == nil {
			a.Tools = loaded
		}
	}

	fullSystemPrompt := fmt.Sprintf(`You are a ReAct agent that solves problems through reasoning and tool use.
The Task you will be solving:
%s

Available tools:
%s

You can call the tools above as functions. To take an action, CALL the appropriate
tool with its arguments — do not describe the call in plain text. Put your
step-by-step thinking for the current step in the message content.
When you have the final answer and need no more tools, reply with the answer as
plain message content and do NOT call any tool.`, a.SystemPrompt, toolsDesc)

	// Memory may already hold prior conversation turns seeded by the caller;
	// wrap the system prompt around the front and the current user prompt at the
	// end. A fresh agent (Memory == nil) collapses to the usual [system, user].
	a.Memory = append([]Message{{Role: "system", Content: fullSystemPrompt}}, a.Memory...)
	a.Memory = append(a.Memory, Message{Role: "user", Content: a.UserPrompt})

	slog.Info("agent starting", "model", a.Model, "maxIterations", maxAgentIterations)

	for i := 0; i < maxAgentIterations; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		slog.Info("agent iteration", "step", i+1, "memoryMessages", len(a.Memory))

		resp, err := a.oneloop(ctx, a.Memory)
		if err != nil {
			return nil, err
		}

		if strings.HasPrefix(resp.Act, "finish|") {
			// Never hand back an empty final reply: if the answer is blank, fall
			// back to the reasoning text so the user always sees something.
			if strings.TrimSpace(resp.FinishAnswer()) == "" {
				resp.Act = "finish|" + strings.TrimSpace(resp.Reasoning)
			}
			slog.Info("agent finished", "steps", i+1)
			return resp, nil
		}

		parts := strings.SplitN(resp.Act, "|", 2)
		if len(parts) != 2 {
			slog.Warn("invalid act format, feeding back to agent", "act", resp.Act)
			a.Memory = append(a.Memory,
				Message{Role: "assistant", Content: fmt.Sprintf("Reasoning: %s\nAct: %s", resp.Reasoning, resp.Act)},
				Message{Role: "user", Content: fmt.Sprintf("Observation: Invalid action format %q — use 'tool_name|args' or 'finish|your answer'.", resp.Act)},
			)
			a.trimMemory()
			if err := sleepCtx(ctx, iterationDelay); err != nil {
				return nil, err
			}
			continue
		}
		toolName, toolArgs := parts[0], parts[1]
		slog.Info("tool call", "tool", toolName)

		observation := fmt.Sprintf("Tool not found: %s", toolName)
		if executeFunc, exists := a.Registry[toolName]; exists {
			result, err := executeFunc(toolArgs)
			if err != nil {
				observation = fmt.Sprintf("Error: %v", err)
				slog.Error("tool error", "tool", toolName, "err", err)
			} else {
				observation = result
			}
		}

		a.Memory = append(a.Memory,
			Message{Role: "assistant", Content: fmt.Sprintf("Reasoning: %s\nAct: %s", resp.Reasoning, resp.Act)},
			Message{Role: "user", Content: "Observation: " + observation},
		)
		a.trimMemory()
		if err := sleepCtx(ctx, iterationDelay); err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("max iterations (%d) reached", maxAgentIterations)
}

func (a *Agent) PrintConversation() {
	fmt.Println("=== Conversation History ===")
	for _, msg := range a.Memory {
		fmt.Println("============================")
		fmt.Printf("[%s]: %s\n", strings.ToUpper(msg.Role), msg.Content)
		fmt.Println("============================")
	}
	fmt.Println("============================")
}

func (a *Agent) PrintMemory() {
	fmt.Println("=== Memory History ===")
	for i, chat := range a.Memory {
		fmt.Printf("Chat number %d\n", i)
		fmt.Println(chat)
	}
	fmt.Println("============================")
}
