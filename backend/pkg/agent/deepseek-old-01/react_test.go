package deepseek

import "testing"

func TestParseAgentResponse(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantErr    bool
		wantReason string
		wantAct    string
	}{
		{
			name:       "plain json",
			raw:        `{"reasoning":"think","act":"finish|done"}`,
			wantReason: "think",
			wantAct:    "finish|done",
		},
		{
			name:       "fenced json with language tag",
			raw:        "```json\n{\"reasoning\":\"r\",\"act\":\"analyze_html|\"}\n```",
			wantReason: "r",
			wantAct:    "analyze_html|",
		},
		{
			name:       "fenced json without language tag",
			raw:        "```\n{\"reasoning\":\"r2\",\"act\":\"update_css|body{}\"}\n```",
			wantReason: "r2",
			wantAct:    "update_css|body{}",
		},
		{
			name:       "surrounding whitespace",
			raw:        "   \n{\"reasoning\":\"r3\",\"act\":\"finish|ok\"}\n  ",
			wantReason: "r3",
			wantAct:    "finish|ok",
		},
		{
			name:    "malformed json (unquoted key)",
			raw:     `{reasoning:"x",act:"y"}`,
			wantErr: true,
		},
		{
			name:    "trailing garbage after object",
			raw:     `{"reasoning":"x","act":"y"} extra text`,
			wantErr: true,
		},
		{
			name:    "empty string",
			raw:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := parseAgentResponse(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (resp=%+v)", resp)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Reasoning != tt.wantReason {
				t.Errorf("reasoning: got %q, want %q", resp.Reasoning, tt.wantReason)
			}
			if resp.Act != tt.wantAct {
				t.Errorf("act: got %q, want %q", resp.Act, tt.wantAct)
			}
		})
	}
}

func TestStripCodeFence(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no fence", `{"a":1}`, `{"a":1}`},
		{"json fence", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"bare fence", "```\n{\"a\":1}\n```", `{"a":1}`},
		{"leading/trailing space", "  {\"a\":1}  ", `{"a":1}`},
		{"fence no closing", "```json\n{\"a\":1}", `{"a":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripCodeFence(tt.in); got != tt.want {
				t.Errorf("stripCodeFence(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// makeToolCall builds a ToolCall with the given function name and raw JSON
// arguments string for the mapping tests below.
func makeToolCall(name, args string) ToolCall {
	var tc ToolCall
	tc.Type = "function"
	tc.Function.Name = name
	tc.Function.Arguments = args
	return tc
}

func TestToolCallToResponse(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		toolCalls  []ToolCall
		wantNil    bool
		wantReason string
		wantAct    string
	}{
		{
			name:       "tool call maps to act, content is reasoning",
			content:    "I need the user's foods",
			toolCalls:  []ToolCall{makeToolCall("list_foods", `{}`)},
			wantReason: "I need the user's foods",
			wantAct:    "list_foods|{}",
		},
		{
			name:       "tool call with pipe inside json args preserved",
			content:    "writing css",
			toolCalls:  []ToolCall{makeToolCall("update_css", `{"content":"a|b"}`)},
			wantReason: "writing css",
			wantAct:    `update_css|{"content":"a|b"}`,
		},
		{
			name:       "first tool call wins when several are returned",
			content:    "",
			toolCalls:  []ToolCall{makeToolCall("list_meals", `{}`), makeToolCall("list_workouts", `{}`)},
			wantReason: "",
			wantAct:    "list_meals|{}",
		},
		{
			name:       "no tool call but json object content is parsed",
			content:    `{"reasoning":"r","act":"list_foods|{}"}`,
			toolCalls:  nil,
			wantReason: "r",
			wantAct:    "list_foods|{}",
		},
		{
			name:       "no tool call, plain content becomes finish",
			content:    "Here is your final answer.",
			toolCalls:  nil,
			wantReason: "Here is your final answer.",
			wantAct:    "finish|Here is your final answer.",
		},
		{
			name:      "empty content and no tool call is unusable",
			content:   "   ",
			toolCalls: nil,
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := toolCallToResponse(tt.content, tt.toolCalls)
			if tt.wantNil {
				if resp != nil {
					t.Fatalf("expected nil, got %+v", resp)
				}
				return
			}
			if resp == nil {
				t.Fatal("expected non-nil response, got nil")
			}
			if resp.Reasoning != tt.wantReason {
				t.Errorf("reasoning: got %q, want %q", resp.Reasoning, tt.wantReason)
			}
			if resp.Act != tt.wantAct {
				t.Errorf("act: got %q, want %q", resp.Act, tt.wantAct)
			}
		})
	}
}
