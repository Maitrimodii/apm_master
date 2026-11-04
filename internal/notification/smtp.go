package notification

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// ErrSMTPNotConfigured is returned when an email is sent without an SMTP host.
var ErrSMTPNotConfigured = errors.New("smtp host is not configured")

// defaultSMTPTimeout bounds a whole send when ctx has no earlier deadline.
const defaultSMTPTimeout = 30 * time.Second

// SMTPConfig is the mail relay the email provider sends through.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// SMTPProvider delivers email over SMTP, upgrading to TLS when offered.
type SMTPProvider struct {
	cfg SMTPConfig
}

func NewSMTPProvider(cfg SMTPConfig) *SMTPProvider {
	return &SMTPProvider{cfg: cfg}
}

func (p *SMTPProvider) Channel() Channel { return ChannelEmail }

func (p *SMTPProvider) Send(ctx context.Context, msg Message) error {
	if p.cfg.Host == "" {
		return ErrSMTPNotConfigured
	}
	to, err := mail.ParseAddress(msg.To)
	if err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}
	from, err := mail.ParseAddress(p.cfg.From)
	if err != nil {
		return fmt.Errorf("invalid sender: %w", err)
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultSMTPTimeout)
		defer cancel()
	}
	deadline, _ := ctx.Deadline()

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(p.cfg.Host, p.cfg.Port))
	if err != nil {
		return fmt.Errorf("dial smtp: %w", err)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return fmt.Errorf("set smtp deadline: %w", err)
	}

	client, err := smtp.NewClient(conn, p.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer func() { _ = client.Close() }()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: p.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if p.cfg.Username != "" || p.cfg.Password != "" {
		if err := client.Auth(smtp.PlainAuth("", p.cfg.Username, p.cfg.Password, p.cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to.Address); err != nil {
		return fmt.Errorf("smtp RCPT TO: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := w.Write(buildMessage(from, to, msg)); err != nil {
		_ = w.Close()
		return fmt.Errorf("write message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("finish message: %w", err)
	}
	return client.Quit()
}

// buildMessage renders headers and the HTML body. The subject is Q-encoded
// and stripped of line breaks, so caller text can't inject headers.
func buildMessage(from, to *mail.Address, msg Message) []byte {
	subject := strings.NewReplacer("\r", " ", "\n", " ").Replace(msg.Subject)
	var b strings.Builder
	b.WriteString("From: " + from.String() + "\r\n")
	b.WriteString("To: " + to.String() + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(Render(msg.EventType, msg.Subject, msg.Body, msg.Meta))
	return []byte(b.String())
}
