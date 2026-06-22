package agent

import (
	"context"
	"errors"
	"testing"
)

func TestNewLLMRoutesToProvider(t *testing.T) {
	params := LLMParameters{Model: "m"}

	ds, err := NewLLM(DeepSeek, params)
	if err != nil {
		t.Fatalf("NewLLM(DeepSeek): %v", err)
	}
	if _, ok := ds.(*deepSeekLLM); !ok {
		t.Errorf("NewLLM(DeepSeek) = %T, want *deepSeekLLM", ds)
	}

	gk, err := NewLLM(Grok, params)
	if err != nil {
		t.Fatalf("NewLLM(Grok): %v", err)
	}
	if _, ok := gk.(*grokLLM); !ok {
		t.Errorf("NewLLM(Grok) = %T, want *grokLLM", gk)
	}

	if _, err := NewLLM(Provider("nope"), params); err == nil {
		t.Error("NewLLM(unknown) should return an error")
	}
}

func TestLLMParametersMessages(t *testing.T) {
	// Memory takes precedence and is used verbatim.
	mem := []Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "yo"}}
	if got := (LLMParameters{Memory: mem, SystemPrompt: "ignored", UserPrompt: "ignored"}).messages(); len(got) != 2 || got[0].Content != "hi" {
		t.Errorf("messages() with Memory = %+v, want the memory slice", got)
	}

	// Without memory, a system+user pair is assembled.
	got := (LLMParameters{SystemPrompt: "sys", UserPrompt: "u"}).messages()
	if len(got) != 2 || got[0].Role != "system" || got[0].Content != "sys" || got[1].Role != "user" || got[1].Content != "u" {
		t.Errorf("messages() = %+v, want [system,user]", got)
	}

	// Empty parts are skipped.
	if got := (LLMParameters{UserPrompt: "only-user"}).messages(); len(got) != 1 || got[0].Role != "user" {
		t.Errorf("messages() with only UserPrompt = %+v, want a single user turn", got)
	}
	if got := (LLMParameters{}).messages(); got != nil {
		t.Errorf("messages() with nothing set = %+v, want nil", got)
	}
}

func TestDeepSeekLLMWebSearchUnsupported(t *testing.T) {
	llm, err := NewLLM(DeepSeek, LLMParameters{Model: "m", UserPrompt: "hi", Mode: ModeWebSearch})
	if err != nil {
		t.Fatalf("NewLLM: %v", err)
	}
	_, err = llm.Complete(context.Background())
	if !errors.Is(err, ErrWebSearchUnsupported) {
		t.Errorf("Complete with ModeWebSearch on DeepSeek = %v, want ErrWebSearchUnsupported", err)
	}
}
