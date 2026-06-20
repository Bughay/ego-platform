package grok

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

// errEmptyContent is returned by GrokOneshotJSON when the model sends an empty
// body. Callers that have a fallback strategy (e.g. a future ReAct agent) can
// detect this with errors.Is and retry via text mode. Mirrors the DeepSeek
// client's sentinel so the two backends behave the same.
var errEmptyContent = errors.New("LLM returned empty JSON content")

// defaultModel is grok-4.3 because it is the only model that supports the
// reasoning_effort parameter, which the reasoning call styles below rely on.
// grokResponsesURL is xAI's Responses API. The web_search tool is only available
// here, not on the chat completions endpoint (grokURL).
const (
	grokResponsesURL = "https://api.x.ai/v1/responses"
	grokURL          = "https://api.x.ai/v1/chat/completions"

	apiTimeout   = 300 * time.Second
	defaultModel = "grok-4.3"
)

// httpClient is shared across all requests so the underlying TCP connections
// are reused (keep-alive) across many calls instead of dialing a fresh
// connection every time.
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
	Temperature *float64 `json:"temperature,omitempty"`
	// MaxCompletionTokens is xAI's current field; the OpenAI-style max_tokens is
	// deprecated on this API.
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	ResponseFormat      *ResponseFormat `json:"response_format,omitempty"`
	// ReasoningEffort is only honored by grok-4.3. Values: none/low/medium/high.
	// Note: Grok has no extra_body/thinking concept like DeepSeek — reasoning is
	// controlled solely by this field.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type ResponseFormat struct {
	Type string `json:"type"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// responsesTemplate is the request body for the Responses API. It differs from
// ChatTemplate (chat completions): the conversation is `input` (not `messages`),
// the cap is `max_output_tokens` (not `max_completion_tokens`), and reasoning is
// a nested object (not a flat `reasoning_effort`).
type responsesTemplate struct {
	Model string          `json:"model"`
	Input []Message       `json:"input"`
	Tools []responsesTool `json:"tools,omitempty"`
	// Temperature is a pointer so an explicit 0 is still sent (see ChatTemplate).
	Temperature *float64 `json:"temperature,omitempty"`
	// MaxOutputTokens caps the response. Note: on the Responses API this budget
	// INCLUDES reasoning tokens, so callers keep reasoning effort low to leave
	// room for the visible answer.
	MaxOutputTokens int           `json:"max_output_tokens,omitempty"`
	Reasoning       *reasoningOpt `json:"reasoning,omitempty"`
	// Store false keeps the call stateless: we resend the full conversation each
	// turn (memory lives in our DB), so xAI need not persist it server-side.
	Store bool `json:"store"`
}

type responsesTool struct {
	Type string `json:"type"`
}

type reasoningOpt struct {
	Effort string `json:"effort"`
}

// responsesResponse decodes only what we need from the Responses API result: the
// assistant text lives in output[] items of type "message", whose content[] holds
// "output_text" parts. Other output items (reasoning, server-side tool calls) are
// interleaved and ignored.
type responsesResponse struct {
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Status string `json:"status"`
}

// EnsureAPIKey loads the .env file (once) and verifies GROKAPIKEY is present.
func EnsureAPIKey() error { return helper.EnsureAPIKey("GROKAPIKEY") }

// GrokWebSearchMemory sends the full conversation to the Responses API with the
// web_search tool enabled so Grok searches the live web before answering. The
// caller owns the memory slice; the returned text may include the model's inline
// citations / markdown image embeds.
func GrokWebSearchMemory(ctx context.Context, model string, memory []Message, temperature float64, maxTokens int) (string, error) {
	return doResponsesRequest(ctx, buildWebSearchResponses(model, memory, temperature, maxTokens))
}

// GrokOneshot sends a single system+user exchange as a reasoning call and
// returns the assistant's content.
func GrokOneshot(ctx context.Context, model string, systemMessage string, userMessage string, temperature float64, maxTokens int) (string, error) {
	chat := buildReasoningChat(model, []Message{
		{Role: "system", Content: systemMessage},
		{Role: "user", Content: userMessage},
	}, temperature, maxTokens)

	resp, err := doRequest(ctx, chat)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

// GrokOneshotMemory sends the full conversation history as a reasoning call and
// returns the assistant's content. The caller owns the memory slice (appending
// the new assistant reply, trimming, etc.).
func GrokOneshotMemory(ctx context.Context, model string, memory []Message, temperature float64, maxTokens int) (string, error) {
	chat := buildReasoningChat(model, memory, temperature, maxTokens)

	resp, err := doRequest(ctx, chat)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

// GrokOneshotJSON sends messages in json_object mode (reasoning disabled) and
// returns the raw JSON content. It returns errEmptyContent on a blank body so a
// caller with a fallback strategy can detect it via errors.Is.
func GrokOneshotJSON(ctx context.Context, model string, messages []Message, temperature float64, maxTokens int) (string, error) {
	chat := buildJSONChat(model, messages, temperature, maxTokens)

	resp, err := doRequest(ctx, chat)
	if err != nil {
		return "", err
	}

	if resp.Choices[0].FinishReason == "length" {
		return "", fmt.Errorf("output truncated due to max_completion_tokens limit (current: %d)", maxTokens)
	}

	content := resp.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		return "", errEmptyContent
	}
	return content, nil
}

// GrokMemoryLoop runs an interactive stdin REPL that keeps the full conversation
// in memory and sends it to grok-4.3 on each turn. Type "quit" to exit.
func GrokMemoryLoop(ctx context.Context, systemMessage string, temperature float64, maxTokens int) error {
	memory := []Message{
		{Role: "system", Content: systemMessage},
	}

	fmt.Println("I am your Grok assistant with memory, how may I help you?")
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

		response, err := GrokOneshotMemory(ctx, defaultModel, memory, temperature, maxTokens)
		if err != nil {
			return fmt.Errorf("grok call: %w", err)
		}
		fmt.Println("Assistant:", response)

		memory = append(memory, Message{Role: "assistant", Content: response})
	}

	return nil
}

// buildReasoningChat builds the request for the reasoning call styles (oneshot /
// memory). reasoning_effort is "high" (the analog of DeepSeek's "max"). Live
// search is not requested: xAI removed search_parameters from the chat
// completions API (sending it now returns 410), so Grok behaves as a pure LLM by
// default.
func buildReasoningChat(model string, messages []Message, temperature float64, maxTokens int) *ChatTemplate {
	return &ChatTemplate{
		Model:               model,
		Messages:            messages,
		Stream:              false,
		Temperature:         &temperature,
		MaxCompletionTokens: maxTokens,
		ReasoningEffort:     "high",
	}
}

// buildJSONChat builds the request for json_object mode. reasoning_effort is
// "none": disabling reasoning keeps the model's output in `content` (not
// `reasoning_content`) and avoids stressing constrained JSON decoding, mirroring
// the DeepSeek client's deliberate omission of reasoning for JSON mode.
func buildJSONChat(model string, messages []Message, temperature float64, maxTokens int) *ChatTemplate {
	return &ChatTemplate{
		Model:               model,
		Messages:            messages,
		Stream:              false,
		Temperature:         &temperature,
		MaxCompletionTokens: maxTokens,
		ResponseFormat:      &ResponseFormat{Type: "json_object"},
		ReasoningEffort:     "none",
	}
}

// doRequest performs a single Grok chat-completion HTTP call and returns the
// decoded response. It centralizes the boilerplate so each call style is just
// "build a ChatTemplate, hand it here, post-process".
//
// The call is bound to ctx: cancelling ctx (timeout or manual cancel) aborts the
// in-flight HTTP request.
func doRequest(ctx context.Context, chat *ChatTemplate) (*ChatResponse, error) {
	apiKey, err := helper.LoadAPIKey("GROKAPIKEY")
	if err != nil {
		return nil, err
	}

	jsonData, err := json.Marshal(chat)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, grokURL, bytes.NewReader(jsonData))
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

// buildWebSearchResponses builds a Responses API request with the web_search tool
// enabled. reasoning effort is "low" (the API default) deliberately: since
// max_output_tokens also covers reasoning tokens, low effort avoids starving the
// visible answer.
func buildWebSearchResponses(model string, input []Message, temperature float64, maxTokens int) *responsesTemplate {
	return &responsesTemplate{
		Model:           model,
		Input:           input,
		Tools:           []responsesTool{{Type: "web_search"}},
		Temperature:     &temperature,
		MaxOutputTokens: maxTokens,
		Reasoning:       &reasoningOpt{Effort: "low"},
		Store:           false,
	}
}

// extractOutputText concatenates the assistant's visible text from the Responses
// API output, skipping non-message items (reasoning, tool calls).
func extractOutputText(r *responsesResponse) string {
	var sb strings.Builder
	for _, item := range r.Output {
		if item.Type != "message" {
			continue
		}
		for _, c := range item.Content {
			if c.Type == "output_text" {
				sb.WriteString(c.Text)
			}
		}
	}
	return sb.String()
}

// doResponsesRequest performs a single Responses API call and returns the
// assistant's visible text. It mirrors doRequest's boilerplate but targets
// grokResponsesURL and decodes the Responses-shaped body. The call is bound to
// ctx (cancelling ctx aborts the in-flight request).
func doResponsesRequest(ctx context.Context, body *responsesTemplate) (string, error) {
	apiKey, err := helper.LoadAPIKey("GROKAPIKEY")
	if err != nil {
		return "", err
	}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, grokResponsesURL, bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, helper.ReadErrorBody(resp.Body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	if strings.TrimSpace(string(bodyBytes)) == "" {
		return "", fmt.Errorf("empty response body")
	}

	var response responsesResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}

	text := extractOutputText(&response)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("no output_text in response (status %q)", response.Status)
	}
	return text, nil
}
