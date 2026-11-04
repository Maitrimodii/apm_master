package notification

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"testing"
)

func TestBuildMessageEncodesSubjectAndBlocksHeaderInjection(t *testing.T) {
	from := &mail.Address{Address: "no-reply@example.com"}
	to := &mail.Address{Address: "ops@example.com"}
	raw := string(buildMessage(from, to, Message{Subject: "Alert ünïcode\r\nBcc: attacker@evil.example", Body: "b"}))

	headers, _, _ := strings.Cut(raw, "\r\n\r\n")
	if strings.Contains(headers, "\r\nBcc:") {
		t.Fatalf("subject injected a header:\n%s", headers)
	}
	if !strings.Contains(headers, "Subject: =?utf-8?q?") {
		t.Fatalf("subject not Q-encoded:\n%s", headers)
	}
}

func TestSMTPSendRequiresHost(t *testing.T) {
	err := NewSMTPProvider(SMTPConfig{}).Send(context.Background(), Message{To: "a@example.com"})
	if !errors.Is(err, ErrSMTPNotConfigured) {
		t.Fatalf("Send() error = %v, want ErrSMTPNotConfigured", err)
	}
}

func TestSMTPSendRejectsInvalidRecipient(t *testing.T) {
	p := NewSMTPProvider(SMTPConfig{Host: "localhost", Port: "25", From: "no-reply@example.com"})
	if err := p.Send(context.Background(), Message{To: "not an address\r\nBcc: x@y"}); err == nil {
		t.Fatal("Send() error = nil for invalid recipient")
	}
}
