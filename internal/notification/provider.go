package notification

import "context"

type NotificationProvider interface {
	Send(ctx context.Context, msg Message) error
	Name() string
}

type Message struct {
	To        string            `json:"to"`
	Subject   string            `json:"subject"`
	Body      string            `json:"body"`
	EventType string            `json:"event_type"`
	Meta      map[string]string `json:"meta,omitempty"`
}
