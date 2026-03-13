package notify

import (
	"context"
	"errors"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/gosuda/steerlane/internal/config"
)

type EmailPayload struct {
	To      string
	Subject string
	Body    string
}

type EmailSender interface {
	Send(ctx context.Context, payload EmailPayload) error
}

type smtpEmailSender struct {
	host        string
	addr        string
	username    string
	password    string
	fromAddress string
}

func NewEmailSender(cfg config.EmailConfig) EmailSender {
	if !cfg.IsEnabled() {
		return nil
	}

	return &smtpEmailSender{
		host:        cfg.SMTPHost,
		addr:        fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort),
		username:    cfg.SMTPUsername,
		password:    cfg.SMTPPassword,
		fromAddress: cfg.FromAddress,
	}
}

func (s *smtpEmailSender) Send(ctx context.Context, payload EmailPayload) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if payload.To == "" {
		return errors.New("send fallback email: recipient is required")
	}

	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	if err := smtp.SendMail(s.addr, auth, s.fromAddress, []string{payload.To}, buildEmailMessage(s.fromAddress, payload)); err != nil {
		return fmt.Errorf("send fallback email: %w", err)
	}

	return nil
}

func buildEmailMessage(from string, payload EmailPayload) []byte {
	headers := []string{
		"From: " + from,
		"To: " + payload.To,
		"Subject: " + payload.Subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
	}

	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + payload.Body)
}
