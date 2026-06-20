package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stubAgent is a no-network Agent used to test composition (NewFallback) without
// hitting any provider. calls is incremented on every Run so tests can assert
// which agents actually ran.
type stubAgent struct {
	answer string
	err    error
	calls  *int
}

func (s stubAgent) Run(context.Context) (string, error) {
	if s.calls != nil {
		*s.calls++
	}
	return s.answer, s.err
}

var _ Agent = stubAgent{}

func TestNewRoutesToProvider(t *testing.T) {
	param := AgentParameters{Model: "m"}

	ds, err := NewAgent(DeepSeek, param)
	if err != nil {
		t.Fatalf("New(DeepSeek): %v", err)
	}
	if _, ok := ds.(*deepSeekAgent); !ok {
		t.Errorf("New(DeepSeek) = %T, want *deepSeekAgent", ds)
	}

	gk, err := NewAgent(Grok, param)
	if err != nil {
		t.Fatalf("New(Grok): %v", err)
	}
	if _, ok := gk.(*grokAgent); !ok {
		t.Errorf("New(Grok) = %T, want *grokAgent", gk)
	}

	if _, err := NewAgent(Provider("nope"), param); err == nil {
		t.Error("New(unknown) should return an error")
	}
}

func TestNewDeepSeekMapsConfig(t *testing.T) {
	reg := map[string]func(string) (string, error){
		"x": func(string) (string, error) { return "", nil },
	}
	param := AgentParameters{
		Model:        "deepseek-v4-pro",
		SystemPrompt: "sys",
		UserPrompt:   "u",
		Memory:       []Message{{Role: "user", Content: "hi"}},
		Thinking:     true,
		Registry:     reg,
		SchemaData:   []byte("[]"),
		MaxTokens:    1234,
	}

	a, ok := NewDeepSeek(param).(*deepSeekAgent)
	if !ok {
		t.Fatal("NewDeepSeek did not return *deepSeekAgent")
	}

	// Fields are read through the embedded *deepseek.Agent (promotion).
	if a.Model != "deepseek-v4-pro" || a.SystemPrompt != "sys" || a.UserPrompt != "u" {
		t.Errorf("string fields not mapped: %+v", a.Agent)
	}
	if !a.Thinking || a.MaxTokens != 1234 {
		t.Errorf("Thinking/MaxTokens not mapped: thinking=%v max=%d", a.Thinking, a.MaxTokens)
	}
	if string(a.SchemaData) != "[]" {
		t.Errorf("SchemaData not passed through: %q", string(a.SchemaData))
	}
	if len(a.Registry) != 1 {
		t.Errorf("Registry not passed through: %v", a.Registry)
	}
	if len(a.Memory) != 1 || a.Memory[0].Role != "user" || a.Memory[0].Content != "hi" {
		t.Errorf("Memory not converted: %+v", a.Memory)
	}
}

func TestToProviderMessages(t *testing.T) {
	if toDeepSeekMessages(nil) != nil {
		t.Error("toDeepSeekMessages(nil) should be nil")
	}
	if toGrokMessages(nil) != nil {
		t.Error("toGrokMessages(nil) should be nil")
	}

	in := []Message{{Role: "system", Content: "s"}, {Role: "user", Content: "u"}}
	if got := toGrokMessages(in); len(got) != 2 || got[0].Role != "system" || got[1].Content != "u" {
		t.Errorf("toGrokMessages = %+v", got)
	}
	if got := toDeepSeekMessages(in); len(got) != 2 || got[0].Content != "s" || got[1].Role != "user" {
		t.Errorf("toDeepSeekMessages = %+v", got)
	}
}

func TestFallbackUsesPrimaryOnSuccess(t *testing.T) {
	var pc, sc int
	a := NewFallback(
		stubAgent{answer: "primary", calls: &pc},
		stubAgent{answer: "secondary", calls: &sc},
	)
	got, err := a.Run(context.Background())
	if err != nil || got != "primary" {
		t.Fatalf("got %q, err %v; want primary", got, err)
	}
	if pc != 1 || sc != 0 {
		t.Errorf("calls primary=%d secondary=%d, want 1,0", pc, sc)
	}
}

func TestFallbackFallsBackOnPrimaryError(t *testing.T) {
	var pc, sc int
	a := NewFallback(
		stubAgent{err: errors.New("boom"), calls: &pc},
		stubAgent{answer: "secondary", calls: &sc},
	)
	got, err := a.Run(context.Background())
	if err != nil || got != "secondary" {
		t.Fatalf("got %q, err %v; want secondary", got, err)
	}
	if pc != 1 || sc != 1 {
		t.Errorf("calls primary=%d secondary=%d, want 1,1", pc, sc)
	}
}

func TestFallbackBothFail(t *testing.T) {
	a := NewFallback(
		stubAgent{err: errors.New("primary-err")},
		stubAgent{err: errors.New("secondary-err")},
	)
	_, err := a.Run(context.Background())
	if err == nil {
		t.Fatal("expected error when both fail")
	}
	if !strings.Contains(err.Error(), "primary-err") || !strings.Contains(err.Error(), "secondary-err") {
		t.Errorf("error should mention both failures: %v", err)
	}
}

func TestFallbackCancelledContextRunsNothing(t *testing.T) {
	var pc, sc int
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a := NewFallback(
		stubAgent{answer: "primary", calls: &pc},
		stubAgent{answer: "secondary", calls: &sc},
	)
	if _, err := a.Run(ctx); err == nil {
		t.Fatal("expected context error")
	}
	if pc != 0 || sc != 0 {
		t.Errorf("no agent should run on a cancelled context, got primary=%d secondary=%d", pc, sc)
	}
}
