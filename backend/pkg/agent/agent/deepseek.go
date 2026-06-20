package agent

import (
	"context"

	"github.com/Bughay/egolifter/pkg/agent/deepseek"
)

// deepSeekAgent adapts a *deepseek.Agent to the package's Agent interface by
// COMPOSITION: it embeds the concrete agent, which promotes that agent's
// exported fields and methods onto the wrapper. We then declare our own Run,
// which SHADOWS the promoted deepseek.Agent.Run — necessary because the promoted
// method returns *deepseek.AgentResponse, which would not satisfy the Agent
// interface's Run(ctx) (string, error).
type deepSeekAgent struct {
	*deepseek.Agent
}

// Compile-time proof that *deepSeekAgent satisfies Agent. If the method set ever
// drifts, the build fails here rather than at some distant call site.
var _ Agent = (*deepSeekAgent)(nil)

// NewDeepSeek builds a DeepSeek-backed Agent from a provider-neutral Config.
func NewDeepSeek(param AgentParameters) Agent {
	return &deepSeekAgent{
		Agent: &deepseek.Agent{
			Model:        param.Model,
			SystemPrompt: param.SystemPrompt,
			UserPrompt:   param.UserPrompt,
			Memory:       toDeepSeekMessages(param.Memory),
			Thinking:     param.Thinking,
			Registry:     param.Registry,
			SchemaData:   param.SchemaData,
			MaxTokens:    param.MaxTokens,
		},
	}
}

// Run drives the embedded agent's ReAct loop and adapts its result to the
// interface: the provider returns a structured *deepseek.AgentResponse, and we
// hand back just the final answer string so callers never touch provider types.
func (a *deepSeekAgent) Run(ctx context.Context) (string, error) {
	resp, err := a.Agent.Run(ctx)
	if err != nil {
		return "", err
	}
	return resp.FinishAnswer(), nil
}

// toDeepSeekMessages converts the neutral Message slice into deepseek.Message.
// A loop is required: agent.Message and deepseek.Message are distinct named
// types (and []T of two different named types is never directly assignable), so
// each element is rebuilt field by field.
func toDeepSeekMessages(in []Message) []deepseek.Message {
	if in == nil {
		return nil
	}
	out := make([]deepseek.Message, len(in))
	for i, m := range in {
		out[i] = deepseek.Message{Role: m.Role, Content: m.Content}
	}
	return out
}
