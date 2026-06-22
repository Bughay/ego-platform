package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/Bughay/egolifter/pkg/agent/deepseek"
	"github.com/Bughay/egolifter/pkg/agent/grok"
)

// llm.go is the plain-chat sibling of agent.go. Where agent.go unifies the
// providers' ReAct agents behind one Agent interface, this unifies their plain
// (non-ReAct) completion calls — oneshot, oneshot-with-memory, and JSON mode —
// behind one LLM interface, one provider-neutral LLMParameters, and the NewLLM
// factory. The design is identical: a small interface, a config that converts
// per provider, and adapters that delegate to the concrete provider functions.
//
// It reuses agent.go's Provider and Message types and the toDeepSeekMessages /
// toGrokMessages converters (same package), so a caller picks a backend by value
// and depends only on the LLM interface.

// ErrWebSearchUnsupported is returned by Complete when ModeWebSearch is requested
// from a provider that has no live-web-search API. Only Grok (xAI's Responses
// endpoint) supports it; DeepSeek's API has no equivalent.
var ErrWebSearchUnsupported = errors.New("agent: web search not supported by provider deepseek")

// LLMMode selects the plain-chat call style. The three styles are mutually
// exclusive, so they are one enum field rather than several overlapping bools.
type LLMMode string

const (
	// ModeChat is an ordinary completion (the default). Both providers support it.
	ModeChat LLMMode = ""
	// ModeJSON requests json_object output. Both providers support it (see
	// deepseek.DeepseekOneshotJSON / grok.GrokOneshotJSON); reasoning is off in
	// this mode, so Thinking is ignored.
	ModeJSON LLMMode = "json"
	// ModeWebSearch requests a live web search before answering. Grok only;
	// DeepSeek returns ErrWebSearchUnsupported.
	ModeWebSearch LLMMode = "websearch"
)

// LLM is the single behavior both backends share for plain chat: produce a
// completion for the configured messages. Keeping the interface this small is
// deliberate — it mirrors Agent.Run and is the only thing callers must depend on.
type LLM interface {
	Complete(ctx context.Context) (string, error)
}

// LLMParameters holds everything needed for one plain completion, independent of
// provider. Messages come from either Memory (used as-is) or, when Memory is
// empty, a system+user pair assembled from SystemPrompt/UserPrompt — so one
// Complete covers both the oneshot and oneshot-with-memory call styles.
type LLMParameters struct {
	Model        string
	SystemPrompt string    // optional; used only when Memory is empty (oneshot form)
	UserPrompt   string    // optional; used only when Memory is empty (oneshot form)
	Memory       []Message // prior turns (oneshot-with-memory form); takes precedence
	Temperature  float64
	MaxTokens    int
	// Thinking requests reasoning mode. DeepSeek honors it; Grok always runs its
	// reasoning models (reasoning-high) so the flag is a no-op there; both ignore
	// it in ModeJSON, where reasoning is deliberately off.
	Thinking bool
	// Mode selects the call style (chat / json / websearch). ModeWebSearch on
	// DeepSeek returns ErrWebSearchUnsupported.
	Mode LLMMode
}

// messages returns the conversation to send: the supplied Memory when present,
// otherwise a system+user pair built from the prompts (empty parts skipped). It
// is the single place the two call styles converge.
func (p LLMParameters) messages() []Message {
	if len(p.Memory) > 0 {
		return p.Memory
	}
	var msgs []Message
	if p.SystemPrompt != "" {
		msgs = append(msgs, Message{Role: "system", Content: p.SystemPrompt})
	}
	if p.UserPrompt != "" {
		msgs = append(msgs, Message{Role: "user", Content: p.UserPrompt})
	}
	return msgs
}

// NewLLM builds an LLM for the given provider from a provider-neutral config. It
// is the package's plain-chat entry point, mirroring NewAgent: callers select the
// backend by value and depend only on the LLM interface.
func NewLLM(p Provider, params LLMParameters) (LLM, error) {
	switch p {
	case DeepSeek:
		return &deepSeekLLM{params: params}, nil
	case Grok:
		return &grokLLM{params: params}, nil
	default:
		return nil, fmt.Errorf("agent: unknown provider %q", p)
	}
}

// deepSeekLLM adapts the deepseek plain-chat functions to the LLM interface.
type deepSeekLLM struct {
	params LLMParameters
}

// Compile-time proof that *deepSeekLLM satisfies LLM.
var _ LLM = (*deepSeekLLM)(nil)

// Complete dispatches on Mode: JSON mode uses the json_object call, web search is
// unsupported on DeepSeek, and the default is an ordinary (optionally thinking)
// completion over the assembled messages.
func (l *deepSeekLLM) Complete(ctx context.Context) (string, error) {
	msgs := toDeepSeekMessages(l.params.messages())
	switch l.params.Mode {
	case ModeJSON:
		return deepseek.DeepseekOneshotJSON(ctx, l.params.Model, msgs, l.params.Temperature, l.params.MaxTokens)
	case ModeWebSearch:
		return "", ErrWebSearchUnsupported
	default:
		return deepseek.DeepseekOneshotMemory(ctx, l.params.Model, msgs, l.params.Temperature, l.params.MaxTokens, l.params.Thinking)
	}
}

// grokLLM adapts the grok plain-chat functions to the LLM interface, in parallel
// with deepSeekLLM.
type grokLLM struct {
	params LLMParameters
}

// Compile-time proof that *grokLLM satisfies LLM.
var _ LLM = (*grokLLM)(nil)

// Complete dispatches on Mode: JSON mode uses the json_object call, web search
// hits the Responses API, and the default is an ordinary completion. Grok ignores
// Thinking (it always runs its reasoning models), so it is not threaded through.
func (l *grokLLM) Complete(ctx context.Context) (string, error) {
	msgs := toGrokMessages(l.params.messages())
	switch l.params.Mode {
	case ModeJSON:
		return grok.GrokOneshotJSON(ctx, l.params.Model, msgs, l.params.Temperature, l.params.MaxTokens)
	case ModeWebSearch:
		return grok.GrokWebSearchMemory(ctx, l.params.Model, msgs, l.params.Temperature, l.params.MaxTokens)
	default:
		return grok.GrokOneshotMemory(ctx, l.params.Model, msgs, l.params.Temperature, l.params.MaxTokens)
	}
}
