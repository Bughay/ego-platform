package deepseek

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Bughay/egolifter/pkg/agent/helper"
)

// errEmptyContent is returned by DeepseekOneshotJSON when the model sends an
// empty body. Callers that have a fallback strategy (e.g. the ReAct agent)
// can detect this with errors.Is and retry via text mode.
var errEmptyContent = errors.New("LLM returned empty JSON content")

const (
	deepseekURL = "https://api.deepseek.com/beta/v1/chat/completions"
	apiTimeout  = 300 * time.Second
)

// httpClient is shared across all requests so the underlying TCP connections
// are reused (keep-alive) across a multi-iteration agent loop instead of
// dialing a fresh connection on every call.
var httpClient = &http.Client{Timeout: apiTimeout}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatTemplate struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
	// Temperature is a pointer so an explicit 0 (deterministic output) is still
	// sent on the wire; a plain float64 with omitempty would drop 0 and let the
	// API fall back to its default. nil omits the field entirely.
	Temperature     *float64               `json:"temperature,omitempty"`
	MaxTokens       int                    `json:"max_tokens,omitempty"`
	ResponseFormat  *ResponseFormat        `json:"response_format,omitempty"`
	ReasoningEffort string                 `json:"reasoning_effort,omitempty"`
	ExtraBody       map[string]interface{} `json:"extra_body,omitempty"`
	// Tools carries the official function-calling tool definitions. When set,
	// the model may answer with structured tool_calls instead of (or alongside)
	// plain content. omitempty keeps it off the wire for plain chat calls.
	Tools []Tool `json:"tools,omitempty"`
	// ToolChoice steers tool use ("auto" / "none" / "required"). Only sent when
	// Tools is set; omitempty leaves it out otherwise.
	ToolChoice string `json:"tool_choice,omitempty"`
}

type ResponseFormat struct {
	Type string `json:"type"`
}

// ToolCall is one function call the model asked for in its response. Arguments
// is a JSON object string (e.g. `{"id":"..."}`), passed verbatim to the matching
// tool handler.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// EnsureAPIKey loads the .env file (once) and verifies DEEPSEEKAPIKEY is present.
func EnsureAPIKey() error { return helper.EnsureAPIKey("DEEPSEEKAPIKEY") }

// doRequest performs a single DeepSeek chat-completion HTTP call and returns the
// decoded response. It centralizes the boilerplate that used to be copy-pasted
// across DeepseekOneshot / DeepseekOneshotJSON / DeepseekOneshotMemory, so each
// of those is now just "build a ChatTemplate, hand it here, post-process".
//
// The call is bound to ctx: cancelling ctx (timeout or manual cancel) aborts the
// in-flight HTTP request.
//
// Step-by-step (pseudocode):
//
//  1. key  := loadAPIKey()                  // read DEEPSEEKAPIKEY (.env or env)
//     if key missing -> return error
//  2. body := json.Marshal(chat)            // serialize the request payload
//     if marshal fails -> return error
//  3. req  := POST deepseekURL with body    // request is bound to ctx
//     set header  Content-Type: application/json
//     set header  Authorization: Bearer <key>
//  4. resp := httpClient.Do(req)            // shared client -> connection reuse
//     if transport error / ctx cancelled -> return error
//     defer close(resp.Body)
//  5. if resp.StatusCode != 200             // surface the API error body verbatim
//     -> return error(status, body)
//  6. raw  := readAll(resp.Body)            // read the whole body once
//     if raw is blank -> return error("empty response body")
//  7. out  := json.Unmarshal(raw)           // decode into ChatResponse
//     if decode fails -> return error
//  8. if out has no choices -> return error
//  9. return &out                           // caller pulls content / finish_reason
func doRequest(ctx context.Context, chat *ChatTemplate) (*ChatResponse, error) {
	apiKey, err := helper.LoadAPIKey("DEEPSEEKAPIKEY")
	if err != nil {
		return nil, err
	}

	jsonData, err := json.Marshal(chat)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deepseekURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, helper.ReadErrorBody(resp.Body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if strings.TrimSpace(string(bodyBytes)) == "" {
		return nil, fmt.Errorf("empty response body")
	}

	var response ChatResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}
	return &response, nil
}

// applyThinking turns the request into a reasoning ("thinking") call by setting
// the reasoning_effort + extra_body.thinking fields, or leaves it as a plain
// non-thinking call when thinking is false. Centralizes the single definition of
// "thinking" so callers just pass a bool.
func applyThinking(chat *ChatTemplate, thinking bool) {
	if thinking {
		chat.ReasoningEffort = "max"
		chat.ExtraBody = map[string]interface{}{"thinking": map[string]string{"type": "enabled"}}
	}
}

func DeepseekOneshot(ctx context.Context, model string, systemMessage string, userMessage string, temperature float64, maxTokens int, thinking bool) (string, error) {
	chat := &ChatTemplate{
		Model: model,
		Messages: []Message{
			{Role: "system", Content: systemMessage},
			{Role: "user", Content: userMessage},
		},
		Stream:      false,
		Temperature: &temperature,
		MaxTokens:   maxTokens,
	}
	applyThinking(chat, thinking)

	resp, err := doRequest(ctx, chat)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

func DeepseekOneshotJSON(ctx context.Context, model string, messages []Message, temperature float64, maxTokens int) (string, error) {
	chat := &ChatTemplate{
		Model:          model,
		Messages:       messages,
		Stream:         false,
		Temperature:    &temperature,
		MaxTokens:      maxTokens,
		ResponseFormat: &ResponseFormat{Type: "json_object"},
		// No ReasoningEffort and no ExtraBody/thinking: JSON mode must NOT run as a
		// reasoning model, or the model routes output into reasoning_content (which
		// we don't decode) and stresses constrained decoding -> empty/broken JSON.
		// This matches the documented minimal json_object request.
	}

	resp, err := doRequest(ctx, chat)
	if err != nil {
		return "", err
	}

	if resp.Choices[0].FinishReason == "length" {
		return "", fmt.Errorf("output truncated due to max_tokens limit (current: %d)", maxTokens)
	}

	content := resp.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		return "", errEmptyContent
	}
	return content, nil
}

// deepseekToolCall makes a tools-enabled chat call and returns the assistant
// message's content (the model's reasoning / brief note), any structured
// tool_calls it asked for, and the finish_reason. It is the v2 primary path: the
// model is given the official tools parameter (tool_choice "auto") and answers
// with content + tool_calls together instead of the custom {reasoning, act} JSON.
//
// Tools/ToolChoice are only set when at least one tool is supplied, so a
// tool-less agent degrades to a plain content call. No response_format is set —
// we want natural content alongside tool_calls, not a JSON object.
func deepseekToolCall(ctx context.Context, model string, messages []Message, temperature float64, maxTokens int, tools []Tool, thinking bool) (string, []ToolCall, string, error) {
	chat := &ChatTemplate{
		Model:       model,
		Messages:    messages,
		Stream:      false,
		Temperature: &temperature,
		MaxTokens:   maxTokens,
	}
	if len(tools) > 0 {
		chat.Tools = tools
		chat.ToolChoice = "auto"
	}
	applyThinking(chat, thinking)

	resp, err := doRequest(ctx, chat)
	if err != nil {
		return "", nil, "", err
	}

	choice := resp.Choices[0]
	if choice.FinishReason == "length" {
		return "", nil, "length", fmt.Errorf("output truncated due to max_tokens limit (current: %d)", maxTokens)
	}
	return choice.Message.Content, choice.Message.ToolCalls, choice.FinishReason, nil
}

func DeepseekOneshotMemory(ctx context.Context, model string, memory []Message, temperature float64, maxTokens int, thinking bool) (string, error) {
	chat := &ChatTemplate{
		Model:       model,
		Messages:    memory,
		Stream:      false,
		Temperature: &temperature,
		MaxTokens:   maxTokens,
	}
	applyThinking(chat, thinking)

	resp, err := doRequest(ctx, chat)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

func DeepseekMemoryLoop(ctx context.Context, systemMessage string, temperature float64, maxTokens int, thinking bool) error {
	memory := []Message{
		{Role: "system", Content: systemMessage},
	}

	fmt.Println("I am your Deepseek assistant with memory, how may I help you?")
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("You: ")
		userMessage, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read input: %w", err)
		}

		userMessage = strings.TrimSpace(userMessage)
		if userMessage == "quit" {
			break
		}

		memory = append(memory, Message{Role: "user", Content: userMessage})

		response, err := DeepseekOneshotMemory(ctx, "deepseek-v4-pro", memory, temperature, maxTokens, thinking)
		if err != nil {
			return fmt.Errorf("deepseek call: %w", err)
		}
		fmt.Println("Assistant:", response)

		memory = append(memory, Message{Role: "assistant", Content: response})
	}

	return nil
}
