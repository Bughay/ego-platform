package grok

import (
	"fmt"
	"testing"
)

func TestFinishAnswer(t *testing.T) {
	tests := []struct {
		name string
		act  string
		want string
	}{
		{"finish prefix stripped", "finish|all done", "all done"},
		{"finish prefix with pipe in answer", "finish|a|b|c", "a|b|c"},
		{"no finish prefix returned as-is", "list_foods|{}", "list_foods|{}"},
		{"empty finish", "finish|", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &AgentResponse{Act: tt.act}
			if got := r.FinishAnswer(); got != tt.want {
				t.Errorf("FinishAnswer() = %q, want %q", got, tt.want)
			}
		})
	}
}

// toolMsg builds a role:"tool" result message for the trim tests.
func toolMsg(id string) Message {
	return Message{Role: "tool", ToolCallID: id, Content: "result " + id}
}

func TestTrimMemory(t *testing.T) {
	t.Run("under window is left untouched", func(t *testing.T) {
		a := &Agent{}
		for i := 0; i < 10; i++ {
			a.Memory = append(a.Memory, Message{Role: "user", Content: fmt.Sprintf("m%d", i)})
		}
		before := len(a.Memory)
		a.trimMemory()
		if len(a.Memory) != before {
			t.Fatalf("expected memory unchanged at %d, got %d", before, len(a.Memory))
		}
	})

	t.Run("over window keeps head plus recent window", func(t *testing.T) {
		a := &Agent{}
		a.Memory = append(a.Memory,
			Message{Role: "system", Content: "sys"},
			Message{Role: "user", Content: "first"},
		)
		for i := 0; i < 40; i++ {
			a.Memory = append(a.Memory, Message{Role: "user", Content: fmt.Sprintf("m%d", i)})
		}
		a.trimMemory()

		if len(a.Memory) != 2+memoryWindowSize {
			t.Fatalf("expected %d messages, got %d", 2+memoryWindowSize, len(a.Memory))
		}
		if a.Memory[0].Content != "sys" || a.Memory[1].Content != "first" {
			t.Errorf("head not preserved: got %q, %q", a.Memory[0].Content, a.Memory[1].Content)
		}
		// The last message must be the most recent one appended.
		if got := a.Memory[len(a.Memory)-1].Content; got != "m39" {
			t.Errorf("expected tail to end at m39, got %q", got)
		}
	})

	t.Run("window never begins with an orphaned tool message", func(t *testing.T) {
		// Construct memory so the window boundary (len-memoryWindowSize) lands
		// exactly on tool messages whose assistant parent is trimmed away.
		mem := []Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "first"},
			{Role: "user", Content: "pad0"},
			{Role: "user", Content: "pad1"},
			toolMsg("a"), // index 4 == len-memoryWindowSize (24-20): window start
			toolMsg("b"), // index 5
		}
		for i := 0; i < 18; i++ {
			mem = append(mem, Message{Role: "user", Content: fmt.Sprintf("tail%d", i)})
		}
		a := &Agent{Memory: mem} // len 24

		a.trimMemory()

		if a.Memory[0].Content != "sys" || a.Memory[1].Content != "first" {
			t.Errorf("head not preserved")
		}
		// The two leading orphan tool messages must have been dropped, leaving
		// head(2) + 18 tail messages.
		if len(a.Memory) != 2+18 {
			t.Fatalf("expected orphan tool messages dropped (len 20), got %d", len(a.Memory))
		}
		if a.Memory[2].Role == "tool" {
			t.Errorf("window begins with orphaned tool message: %+v", a.Memory[2])
		}
	})
}
