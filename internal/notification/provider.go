package notification

import "context"

type Provider interface {
	Send(ctx context.Context, msg Message) error
	// Channel is the channel this provider delivers; jobs are routed by it.
	Channel() Channel
}

type Message struct {
	To        string            `json:"to"`
	Subject   string            `json:"subject"`
	Body      string            `json:"body"`
	EventType EventType         `json:"event_type"`
	Meta      map[string]string `json:"meta,omitempty"`
}
