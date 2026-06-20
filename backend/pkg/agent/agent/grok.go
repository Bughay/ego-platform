package agent

import (
	"context"

	"github.com/Bughay/egolifter/pkg/agent/grok"
)

// grokAgent adapts a *grok.Agent to the Agent interface exactly as
// deepSeekAgent does for DeepSeek: it embeds the concrete agent (composition)
// and shadows Run to return the final answer string. The parallel structure is
// the point — adding a backend means writing one more small adapter, nothing
// else changes.
type grokAgent struct {
	*grok.Agent
}

// Compile-time proof that *grokAgent satisfies Agent.
var _ Agent = (*grokAgent)(nil)

// NewGrok builds a Grok-backed Agent from a provider-neutral Config.
func NewGrok(param AgentParameters) Agent {
	return &grokAgent{
		Agent: &grok.Agent{
			Model:        param.Model,
			SystemPrompt: param.SystemPrompt,
			UserPrompt:   param.UserPrompt,
			Memory:       toGrokMessages(param.Memory),
			Thinking:     param.Thinking,
			Registry:     param.Registry,
			SchemaData:   param.SchemaData,
			MaxTokens:    param.MaxTokens,
		},
	}
}

// Run drives the embedded agent's ReAct loop and returns just the final answer.
func (a *grokAgent) Run(ctx context.Context) (string, error) {
	resp, err := a.Agent.Run(ctx)
	if err != nil {
		return "", err
	}
	return resp.FinishAnswer(), nil
}

// toGrokMessages converts the neutral Message slice into grok.Message, element
// by element (see toDeepSeekMessages for why a direct slice assignment cannot
// work across two distinct named types).
func toGrokMessages(in []Message) []grok.Message {
	if in == nil {
		return nil
	}
	out := make([]grok.Message, len(in))
	for i, m := range in {
		out[i] = grok.Message{Role: m.Role, Content: m.Content}
	}
	return out
}
