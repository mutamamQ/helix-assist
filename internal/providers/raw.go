package providers

import (
	"context"
	"fmt"
)

// RawChatter is implemented by providers that can run an arbitrary prompt
// against a chosen model (currently the bryant provider).
type RawChatter interface {
	Raw(ctx context.Context, model, system, user string, maxTokens int) (string, error)
}

// Raw runs a free-form prompt on the current provider if it supports it.
func (r *Registry) Raw(ctx context.Context, model, system, user string, maxTokens int) (string, error) {
	p, err := r.Get()
	if err != nil {
		return "", err
	}
	rc, ok := p.(RawChatter)
	if !ok {
		return "", fmt.Errorf("current provider does not support raw prompts")
	}
	return rc.Raw(ctx, model, system, user, maxTokens)
}

// SupportsRaw reports whether the current provider implements RawChatter.
func (r *Registry) SupportsRaw() bool {
	p, err := r.Get()
	if err != nil {
		return false
	}
	_, ok := p.(RawChatter)
	return ok
}
