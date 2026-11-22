package auth

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/smtp"
	"strings"
	"testing"

	"craftbridge/backend/internal/platform/config"
)

type fakeSMTPSession struct {
	supportsStartTLS bool
	startTLSCalled   bool
	authCalled       bool
	from             string
	to               string
	message          bytes.Buffer
}

func (s *fakeSMTPSession) Extension(name string) (bool, string) {
	return name == "STARTTLS" && s.supportsStartTLS, ""
}

func (s *fakeSMTPSession) StartTLS(*tls.Config) error {
	s.startTLSCalled = true
	return nil
}

func (s *fakeSMTPSession) Auth(smtp.Auth) error {
	s.authCalled = true
	return nil
}

func (s *fakeSMTPSession) Mail(from string) error {
	s.from = from
	return nil
}

func (s *fakeSMTPSession) Rcpt(to string) error {
	s.to = to
	return nil
}

func (s *fakeSMTPSession) Data() (io.WriteCloser, error) {
	return smtpMessageWriter{Writer: &s.message}, nil
}

func (s *fakeSMTPSession) Quit() error  { return nil }
func (s *fakeSMTPSession) Close() error { return nil }

type smtpMessageWriter struct {
	io.Writer
}

func (smtpMessageWriter) Close() error { return nil }

func TestSMTPResetSenderSendsResetEmailOverStartTLS(t *testing.T) {
	session := &fakeSMTPSession{supportsStartTLS: true}
	sender := NewSMTPPasswordResetSender(&config.Config{
		SMTPHost:        "smtp.example.test",
		SMTPPort:        587,
		SMTPUsername:    "mailer",
		SMTPPassword:    "local-test-secret",
		SMTPFrom:        "CraftBridge <reset@example.test>",
		FrontendBaseURL: "https://craftbridge.example/app",
		AppEnv:          "production",
	})
	sender.dial = func(_ context.Context, host string, port int) (smtpSession, error) {
		if host != "smtp.example.test" || port != 587 {
			t.Fatalf("unexpected SMTP endpoint %s:%d", host, port)
		}
		return session, nil
	}

	if err := sender.SendPasswordReset(context.Background(), "adee@example.test", "abc_123"); err != nil {
		t.Fatalf("SendPasswordReset returned error: %v", err)
	}
	if !session.startTLSCalled || !session.authCalled {
		t.Fatal("expected STARTTLS and SMTP authentication")
	}
	if session.from != "reset@example.test" || session.to != "adee@example.test" {
		t.Fatalf("unexpected mail envelope: from=%q to=%q", session.from, session.to)
	}
	message := session.message.String()
	for _, expected := range []string{
		"Subject: Reset your CraftBridge password",
		"reset@example.test",
		"adee@example.test",
		"https://craftbridge.example/app/reset-password?token=abc_123",
		"within 30 minutes",
	} {
		if !strings.Contains(message, expected) {
			t.Errorf("reset email does not contain %q", expected)
		}
	}
}

func TestSMTPResetSenderRejectsServerWithoutStartTLS(t *testing.T) {
	session := &fakeSMTPSession{}
	sender := NewSMTPPasswordResetSender(&config.Config{
		SMTPHost:        "smtp.example.test",
		SMTPPort:        587,
		SMTPFrom:        "reset@example.test",
		FrontendBaseURL: "https://craftbridge.example",
		AppEnv:          "production",
	})
	sender.dial = func(context.Context, string, int) (smtpSession, error) {
		return session, nil
	}

	err := sender.SendPasswordReset(context.Background(), "adee@example.test", "abc_123")
	if err == nil || !strings.Contains(err.Error(), "required STARTTLS") {
		t.Fatalf("expected STARTTLS requirement error, got %v", err)
	}
	if session.from != "" {
		t.Fatal("SMTP sender continued without establishing TLS")
	}
}

func TestBuildPasswordResetURLRequiresHTTPSOutsideLocalDevelopment(t *testing.T) {
	if _, err := buildPasswordResetURL("http://craftbridge.example", "production", "token"); err == nil {
		t.Fatal("expected non-HTTPS production reset URL to be rejected")
	}
	if got, err := buildPasswordResetURL("http://localhost:4200", "development", "token"); err != nil ||
		got != "http://localhost:4200/reset-password?token=token" {
		t.Fatalf("unexpected local reset URL %q, error %v", got, err)
	}
}
