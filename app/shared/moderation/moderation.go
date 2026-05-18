package moderation

import "context"

// Decision represents a content moderation decision.
type Decision struct {
	Allowed bool
	Reason  string
}

// Checker is the interface for synchronous content moderation.
// Implementations check message content and return an allow/deny decision.
type Checker interface {
	// Check evaluates whether the given content is allowed.
	// content is the raw message text to moderate.
	Check(ctx context.Context, content string) (Decision, error)
}

// NoopChecker is a no-op implementation that allows all content.
type NoopChecker struct{}

func (NoopChecker) Check(_ context.Context, _ string) (Decision, error) {
	return Decision{Allowed: true}, nil
}

// Ensure NoopChecker implements Checker.
var _ Checker = NoopChecker{}