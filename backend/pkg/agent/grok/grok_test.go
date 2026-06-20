package grok

import (
	"encoding/json"
	"testing"
)

// marshalChat builds the wire JSON for a ChatTemplate and decodes it back into a
// generic map so tests can assert exactly which keys are present/absent and what
// their values are — without touching the network.
func marshalChat(t *testing.T, chat *ChatTemplate) map[string]any {
	t.Helper()
	raw, err := json.Marshal(chat)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func TestBuildReasoningChat(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
	}
	got := marshalChat(t, buildReasoningChat("grok-4.3", messages, 0, 1000))

	tests := []struct {
		name string
		key  string
		want any
	}{
		{"model", "model", "grok-4.3"},
		{"reasoning is high", "reasoning_effort", "high"},
		{"uses max_completion_tokens", "max_completion_tokens", float64(1000)},
		// temperature 0 must be on the wire (pointer behavior), not dropped.
		{"explicit zero temperature", "temperature", float64(0)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got[tc.key] != tc.want {
				t.Errorf("key %q = %v, want %v", tc.key, got[tc.key], tc.want)
			}
		})
	}

	// DeepSeek-only / deprecated fields must NOT appear. search_parameters is
	// included here because xAI removed live search from the chat completions API
	// (sending it returns 410), so we must not emit it at all.
	for _, key := range []string{"max_tokens", "extra_body", "thinking", "response_format", "search_parameters"} {
		if _, present := got[key]; present {
			t.Errorf("unexpected key %q present in reasoning request", key)
		}
	}
}

func TestBuildJSONChat(t *testing.T) {
	messages := []Message{{Role: "user", Content: "give me json"}}
	got := marshalChat(t, buildJSONChat("grok-4.3", messages, 0.2, 500))

	// reasoning is explicitly disabled for constrained JSON decoding.
	if got["reasoning_effort"] != "none" {
		t.Errorf("reasoning_effort = %v, want none", got["reasoning_effort"])
	}

	format, ok := got["response_format"].(map[string]any)
	if !ok {
		t.Fatalf("response_format missing or wrong type: %v", got["response_format"])
	}
	if format["type"] != "json_object" {
		t.Errorf("response_format.type = %v, want json_object", format["type"])
	}

	if got["max_completion_tokens"] != float64(500) {
		t.Errorf("max_completion_tokens = %v, want 500", got["max_completion_tokens"])
	}

	// Deprecated fields must NOT appear: max_tokens (replaced by
	// max_completion_tokens) and search_parameters (xAI removed live search from
	// the chat completions API; sending it returns 410).
	for _, key := range []string{"max_tokens", "search_parameters"} {
		if _, present := got[key]; present {
			t.Errorf("deprecated key %q must not be present", key)
		}
	}
}

// marshalResponses builds the wire JSON for a responsesTemplate and decodes it
// back into a generic map, like marshalChat but for the Responses API body.
func marshalResponses(t *testing.T, body *responsesTemplate) map[string]any {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func TestBuildWebSearchResponses(t *testing.T) {
	input := []Message{{Role: "user", Content: "what is xAI?"}}
	got := marshalResponses(t, buildWebSearchResponses("grok-4.3", input, 0, 1000))

	if got["model"] != "grok-4.3" {
		t.Errorf("model = %v, want grok-4.3", got["model"])
	}
	if got["store"] != false {
		t.Errorf("store = %v, want false", got["store"])
	}
	if got["max_output_tokens"] != float64(1000) {
		t.Errorf("max_output_tokens = %v, want 1000", got["max_output_tokens"])
	}
	// temperature 0 must be on the wire (pointer behavior), not dropped.
	if got["temperature"] != float64(0) {
		t.Errorf("temperature = %v, want 0", got["temperature"])
	}

	// the web_search tool must be enabled.
	tools, ok := got["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %v, want one entry", got["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok || tool["type"] != "web_search" {
		t.Errorf("tools[0] = %v, want type web_search", tools[0])
	}

	// reasoning effort is low so the answer is not starved by reasoning tokens.
	reasoning, ok := got["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("reasoning missing or wrong type: %v", got["reasoning"])
	}
	if reasoning["effort"] != "low" {
		t.Errorf("reasoning.effort = %v, want low", reasoning["effort"])
	}

	if _, present := got["input"]; !present {
		t.Error("input must be present")
	}

	// chat-completions-only fields must NOT appear on a Responses API body.
	for _, key := range []string{"messages", "max_completion_tokens", "search_parameters", "reasoning_effort"} {
		if _, present := got[key]; present {
			t.Errorf("unexpected chat-completions key %q in responses body", key)
		}
	}
}

func TestBuildToolChat(t *testing.T) {
	tools := []Tool{{
		Type: "function",
		Function: Function{
			Name:        "list_foods",
			Description: "list foods",
			Parameters:  Parameters{Type: "object"},
		},
	}}
	messages := []Message{{Role: "user", Content: "hi"}}
	got := marshalChat(t, buildToolChat("grok-4.3", messages, 0, 2000, tools, true))

	// thinking=true -> reasoning_effort high (Grok's analog of DeepSeek "max").
	if got["reasoning_effort"] != "high" {
		t.Errorf("reasoning_effort = %v, want high", got["reasoning_effort"])
	}
	if got["tool_choice"] != "auto" {
		t.Errorf("tool_choice = %v, want auto", got["tool_choice"])
	}
	if got["max_completion_tokens"] != float64(2000) {
		t.Errorf("max_completion_tokens = %v, want 2000", got["max_completion_tokens"])
	}
	// temperature 0 must be on the wire (pointer behavior), not dropped.
	if got["temperature"] != float64(0) {
		t.Errorf("temperature = %v, want 0", got["temperature"])
	}

	gotTools, ok := got["tools"].([]any)
	if !ok || len(gotTools) != 1 {
		t.Fatalf("tools = %v, want one entry", got["tools"])
	}
	tool, ok := gotTools[0].(map[string]any)
	if !ok || tool["type"] != "function" {
		t.Errorf("tools[0] = %v, want type function", gotTools[0])
	}

	// Plain-chat / deprecated fields must NOT appear on a tool call.
	for _, key := range []string{"max_tokens", "response_format", "extra_body", "search_parameters"} {
		if _, present := got[key]; present {
			t.Errorf("unexpected key %q present in tool request", key)
		}
	}
}

func TestBuildToolChat_NoToolsNoThinking(t *testing.T) {
	got := marshalChat(t, buildToolChat("grok-4.3", []Message{{Role: "user", Content: "hi"}}, 0.1, 1000, nil, false))

	// With no tools, the tool fields degrade off the wire entirely.
	for _, key := range []string{"tools", "tool_choice"} {
		if _, present := got[key]; present {
			t.Errorf("unexpected key %q present without tools", key)
		}
	}
	// thinking=false leaves reasoning_effort unset (omitempty).
	if _, present := got["reasoning_effort"]; present {
		t.Errorf("reasoning_effort must be absent when thinking is false")
	}
}

func TestExtractOutputText(t *testing.T) {
	// A realistic Responses API body: a reasoning item (must be skipped) followed
	// by the assistant message whose two output_text parts must be concatenated.
	const raw = `{
		"status": "completed",
		"output": [
			{"type": "reasoning", "content": [{"type": "summary_text", "text": "thinking..."}]},
			{"type": "message", "content": [
				{"type": "output_text", "text": "Hello "},
				{"type": "output_text", "text": "world"}
			]}
		]
	}`

	var r responsesResponse
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got := extractOutputText(&r); got != "Hello world" {
		t.Errorf("extractOutputText = %q, want %q", got, "Hello world")
	}
}
