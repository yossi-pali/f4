package event

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

// NatsPublisher implements Publisher using NATS.
type NatsPublisher struct {
	conn *nats.Conn
}

// NewNatsPublisher creates a NATS publisher. If url is empty, returns a no-op publisher.
func NewNatsPublisher(url string) (Publisher, error) {
	if url == "" {
		return &NoopPublisher{}, nil
	}
	conn, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}
	return &NatsPublisher{conn: conn}, nil
}

func (p *NatsPublisher) Publish(_ context.Context, topic string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return p.conn.Publish(topic, data)
}

func (p *NatsPublisher) Close() error {
	p.conn.Close()
	return nil
}
