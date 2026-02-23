package event

import "context"

// NoopPublisher is a no-op publisher for when no event bus is configured.
type NoopPublisher struct{}

func (p *NoopPublisher) Publish(_ context.Context, _ string, _ any) error { return nil }
func (p *NoopPublisher) Close() error                                     { return nil }
