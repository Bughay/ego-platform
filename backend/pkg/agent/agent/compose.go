package agent

import (
	"context"
	"fmt"
)

// fallbackAgent is an Agent built FROM other Agents: it tries primary, and if
// that errors, falls back to secondary. It is the clearest demonstration that
// once behavior is expressed as an interface, you compose it freely — a
// fallbackAgent is itself an Agent, so it can wrap concrete adapters, other
// fallbacks, or anything else implementing Agent.
type fallbackAgent struct {
	primary   Agent
	secondary Agent
}

// Compile-time proof that *fallbackAgent is itself an Agent.
var _ Agent = (*fallbackAgent)(nil)

// NewFallback composes two Agents into one that prefers primary and only uses
// secondary when primary fails — e.g. NewFallback(NewGrok(cfg), NewDeepSeek(cfg))
// to ride out one provider's outage. Both must be non-nil.
func NewFallback(primary, secondary Agent) Agent {
	return &fallbackAgent{primary: primary, secondary: secondary}
}

// Run tries the primary agent first; on any error it falls back to the secondary
// and, if that also fails, reports both failures.
func (a *fallbackAgent) Run(ctx context.Context) (string, error) {
	// A cancelled context is the caller's intent, not a provider failure — don't
	// burn the fallback on it.
	if err := ctx.Err(); err != nil {
		return "", err
	}

	answer, err := a.primary.Run(ctx)
	if err == nil {
		return answer, nil
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}

	answer, secErr := a.secondary.Run(ctx)
	if secErr == nil {
		return answer, nil
	}
	return "", fmt.Errorf("fallback: primary failed (%v); secondary failed (%w)", err, secErr)
}
