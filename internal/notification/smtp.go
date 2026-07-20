package notification

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/Maitrimodii/apm_master/internal/config"
)

type SMTPProvider struct {
	cfg *config.Config
}

func NewSMTPProvider(cfg *config.Config) *SMTPProvider {
	return &SMTPProvider{cfg: cfg}
}

func (provider *SMTPProvider) Name() string {
	return "email"
}

func (provider *SMTPProvider) Send(ctx context.Context, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if provider.cfg.SMTPHost == "" {
		return nil
	}

	addr := provider.cfg.SMTPHost + ":" + provider.cfg.SMTPPort
	headers := map[string]string{
		"From":         provider.cfg.EmailFrom,
		"To":           msg.To,
		"Subject":      msg.Subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/html; charset=UTF-8",
	}

	htmlBody := Render(msg.EventType, msg.Subject, msg.Body, msg.Meta)

	var body strings.Builder
	for key, value := range headers {
		body.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}
	body.WriteString("\r\n")
	body.WriteString(htmlBody)

	var auth smtp.Auth
	if provider.cfg.SMTPUsername != "" || provider.cfg.SMTPPassword != "" {
		auth = smtp.PlainAuth("", provider.cfg.SMTPUsername, provider.cfg.SMTPPassword, provider.cfg.SMTPHost)
	}

	if err := smtp.SendMail(addr, auth, provider.cfg.EmailFrom, []string{msg.To}, []byte(body.String())); err != nil {
		return err
	}

	return ctx.Err()
}
