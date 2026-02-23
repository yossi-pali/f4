package event

import "context"

// Publisher publishes events to the event bus.
type Publisher interface {
	Publish(ctx context.Context, topic string, payload any) error
	Close() error
}
