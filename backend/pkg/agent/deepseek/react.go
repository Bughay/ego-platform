package deepseek

import (
	"context"
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
	Thinking     bool // run the model call as a DeepSeek reasoning ("thinking") model
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
//
// The window must not begin with orphaned role:"tool" results whose assistant
// (tool_calls) parent was trimmed away — the API rejects a tool message that
// doesn't follow an assistant tool-call turn — so any leading tool messages in
// the window are dropped.
func (a *Agent) trimMemory() {
	const head = 2
	if len(a.Memory) <= head+memoryWindowSize {
		return
	}
	tail := a.Memory[len(a.Memory)-memoryWindowSize:]
	for len(tail) > 0 && tail[0].Role == "tool" {
		tail = tail[1:]
	}
	trimmed := make([]Message, 0, head+len(tail))
	trimmed = append(trimmed, a.Memory[:head]...)
	trimmed = append(trimmed, tail...)
	a.Memory = trimmed
}

// FinishAnswer returns the text after the "finish|" prefix of a finished
// agent response.
func (r *AgentResponse) FinishAnswer() string {
	return strings.TrimPrefix(r.Act, "finish|")
}

// callModel makes one tools-enabled model call with retries. It returns the
// assistant content (reasoning), any tool_calls, and the finish_reason. A
// cancelled/expired context bails immediately instead of burning retries; other
// transport/API errors are retried up to maxRetries with retryDelay backoff.
func (a *Agent) callModel(ctx context.Context, messages []Message) (string, []ToolCall, string, error) {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		content, toolCalls, finish, err := deepseekToolCall(ctx, a.Model, messages, 0.1, a.MaxTokens, a.Tools, a.Thinking)
		if err == nil {
			return content, toolCalls, finish, nil
		}
		// Don't burn retries on a cancelled/expired context — bail immediately.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", nil, "", err
		}
		lastErr = err
		slog.Error("agent model call failed", "retry", i, "err", err)
		if serr := sleepCtx(ctx, retryDelay); serr != nil {
			return "", nil, "", serr
		}
	}
	return "", nil, "", fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// Run drives the native tool-calling loop. Each iteration makes one tools-enabled
// model call: if the model returns no tool_calls it has answered, so we finish; if
// it returns tool_calls we echo the assistant turn (with its tool_calls), run each
// tool, append a role:"tool" result keyed by tool_call_id, and loop. This is the
// standard DeepSeek/OpenAI function-calling protocol end-to-end — no custom
// {reasoning, act} text parsing or JSON repair.
func (a *Agent) Run(ctx context.Context) (*AgentResponse, error) {
	// Send the tools via the official `tools` parameter. Callers normally set
	// a.Tools already; load it from the embedded schema if they didn't so the
	// structured path always has tool definitions to offer.
	if len(a.Tools) == 0 && len(a.SchemaData) > 0 {
		loaded, err := LoadToolsFromData(a.SchemaData)
		if err != nil {
			return nil, fmt.Errorf("load tools: %w", err)
		}
		a.Tools = loaded
	}

	// The tools are advertised to the model through the native `tools` parameter,
	// so the system prompt only needs to describe the task and the protocol — no
	// inline tool catalog.
	fullSystemPrompt := fmt.Sprintf(`You are an agent that solves problems through reasoning and tool use.
The Task you will be solving:
%s

You can call the available tools as functions. To take an action, CALL the
appropriate tool with its arguments — do not describe the call in plain text. Put
your step-by-step thinking for the current step in the message content. When you
have the final answer and need no more tools, reply with the answer as plain
message content and do NOT call any tool.`, a.SystemPrompt)

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

		content, toolCalls, _, err := a.callModel(ctx, a.Memory)
		if err != nil {
			return nil, err
		}

		// No tool calls -> the model has produced the final answer.
		if len(toolCalls) == 0 {
			answer := strings.TrimSpace(content)
			slog.Info("agent finished", "steps", i+1)
			return &AgentResponse{Reasoning: content, Act: "finish|" + answer}, nil
		}

		// Echo the assistant turn WITH its tool_calls: the follow-up role:"tool"
		// results are only valid when they trail an assistant message that asked
		// for those calls.
		a.Memory = append(a.Memory, Message{Role: "assistant", Content: content, ToolCalls: toolCalls})

		// Run every requested tool and append its result as a role:"tool" message
		// linked back to the call by tool_call_id.
		for _, tc := range toolCalls {
			slog.Info("tool call", "tool", tc.Function.Name)
			result := fmt.Sprintf("Tool not found: %s", tc.Function.Name)
			if executeFunc, exists := a.Registry[tc.Function.Name]; exists {
				out, ferr := executeFunc(tc.Function.Arguments)
				if ferr != nil {
					result = fmt.Sprintf("Error: %v", ferr)
					slog.Error("tool error", "tool", tc.Function.Name, "err", ferr)
				} else {
					result = out
				}
			}
			a.Memory = append(a.Memory, Message{Role: "tool", ToolCallID: tc.ID, Content: result})
		}

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
