// Package agent is a thin provider-neutral layer over the DeepSeek and Grok
// ReAct agents. Both providers expose structurally-identical agents, but a
// caller would otherwise have to import a specific provider and depend on its
// concrete types. This package hides that behind one small interface (Agent),
// one provider-neutral Config, and a factory (New), so callers can pick a
// backend at runtime and swap it without touching their own code.
//
// The implementation is pure Go composition — no inheritance (Go has none).
// Each provider is wrapped by an adapter that embeds the concrete agent and
// satisfies the Agent interface (see deepseek.go / grok.go). teaching.md walks
// through the design end to end.
package agent

import (
	"context"
	"fmt"
)

// Agent is the single behavior both backends share: run a ReAct loop to
// completion and return the final answer. Keeping the interface this small is
// deliberate — it is the only thing every provider must do, so it is the only
// thing callers must depend on.
type Agent interface {
	Run(ctx context.Context) (string, error)
}

// Message is a provider-neutral chat turn used to seed prior conversation. It
// intentionally carries only Role and Content: the tool-calling fields on the
// providers' own Message types are filled in by their Run loop during a tool
// call, never by the caller seeding history. Each adapter converts this into its
// provider's Message type (see toDeepSeekMessages / toGrokMessages) because those
// are distinct named struct types and cannot cross the package boundary without
// conversion.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Config holds everything needed to build a run, independent of provider. The
// fields that are already provider-agnostic (Registry is an unnamed map type;
// SchemaData is raw bytes both providers parse into their own Tool type inside
// Run) pass straight through; only Memory is converted per provider.
type AgentParameters struct {
	Model        string
	SystemPrompt string
	UserPrompt   string
	Memory       []Message
	Thinking     bool
	Registry     map[string]func(string) (string, error)
	SchemaData   []byte
	MaxTokens    int
}

// Provider identifies which backend New should build.
type Provider string

const (
	DeepSeek Provider = "deepseek"
	Grok     Provider = "grok"
)

// New builds an Agent for the given provider from a provider-neutral Config.
// This is the package's main entry point: callers select the backend by value
// (e.g. from config or a request field) and depend only on the Agent interface.
func New(p Provider, param AgentParameters) (Agent, error) {
	switch p {
	case DeepSeek:
		return NewDeepSeek(param), nil
	case Grok:
		return NewGrok(param), nil
	default:
		return nil, fmt.Errorf("agent: unknown provider %q", p)
	}
}
