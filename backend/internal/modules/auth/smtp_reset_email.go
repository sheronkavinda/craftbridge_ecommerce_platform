package auth

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"

	"craftbridge/backend/internal/platform/config"
)

type PasswordResetEmailSender interface {
	SendPasswordReset(context.Context, string, string) error
}

type smtpSession interface {
	Extension(string) (bool, string)
	StartTLS(*tls.Config) error
	Auth(smtp.Auth) error
	Mail(string) error
	Rcpt(string) error
	Data() (io.WriteCloser, error)
	Quit() error
	Close() error
}

type smtpDialFunc func(context.Context, string, int) (smtpSession, error)

type SMTPPasswordResetSender struct {
	cfg  *config.Config
	dial smtpDialFunc
}

func NewSMTPPasswordResetSender(cfg *config.Config) *SMTPPasswordResetSender {
	return &SMTPPasswordResetSender{cfg: cfg, dial: dialSMTP}
}

func (s *SMTPPasswordResetSender) SendPasswordReset(ctx context.Context, recipient, token string) error {
	if s == nil || s.cfg == nil {
		return errors.New("SMTP settings are not configured")
	}
	host := strings.TrimSpace(s.cfg.SMTPHost)
	if host == "" || s.cfg.SMTPPort < 1 || s.cfg.SMTPPort > 65535 {
		return errors.New("SMTP host and a valid SMTP port are required")
	}
	if (s.cfg.SMTPUsername == "") != (s.cfg.SMTPPassword == "") {
		return errors.New("SMTP username and password must both be configured")
	}
	from, err := mail.ParseAddress(strings.TrimSpace(s.cfg.SMTPFrom))
	if err != nil {
		return fmt.Errorf("invalid SMTP sender address: %w", err)
	}
	to, err := mail.ParseAddress(strings.TrimSpace(recipient))
	if err != nil {
		return fmt.Errorf("invalid password-reset recipient: %w", err)
	}
	resetURL, err := buildPasswordResetURL(s.cfg.FrontendBaseURL, s.cfg.AppEnv, token)
	if err != nil {
		return err
	}
	if s.dial == nil {
		return errors.New("SMTP connection is not configured")
	}
	client, err := s.dial(ctx, host, s.cfg.SMTPPort)
	if err != nil {
		return fmt.Errorf("connect to SMTP server: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("SMTP server does not support required STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}); err != nil {
		return fmt.Errorf("start SMTP TLS: %w", err)
	}
	if s.cfg.SMTPUsername != "" {
		if err := client.Auth(smtp.PlainAuth("", s.cfg.SMTPUsername, s.cfg.SMTPPassword, host)); err != nil {
			return fmt.Errorf("authenticate with SMTP server: %w", err)
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	if err := client.Rcpt(to.Address); err != nil {
		return fmt.Errorf("set SMTP recipient: %w", err)
	}
	body, err := client.Data()
	if err != nil {
		return fmt.Errorf("start SMTP message: %w", err)
	}
	message := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: Reset your CraftBridge password\r\n"+
			"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n"+
			"A password reset was requested for your CraftBridge account.\r\n"+
			"Use the link below within 30 minutes. If you did not request this, ignore this email.\r\n\r\n%s\r\n",
		from.String(),
		to.String(),
		resetURL,
	)
	if _, err := io.WriteString(body, message); err != nil {
		_ = body.Close()
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := body.Close(); err != nil {
		return fmt.Errorf("finish SMTP message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("finish SMTP session: %w", err)
	}
	return nil
}

func buildPasswordResetURL(frontendBaseURL, appEnv, token string) (string, error) {
	if strings.TrimSpace(token) == "" {
		return "", errors.New("password-reset token is required")
	}
	resetURL, err := url.Parse(strings.TrimSpace(frontendBaseURL))
	if err != nil || resetURL.Host == "" || resetURL.User != nil || resetURL.Fragment != "" {
		return "", errors.New("FRONTEND_BASE_URL must be an absolute URL without credentials or a fragment")
	}
	isLocalHTTP := (appEnv == "development" || appEnv == "local") &&
		resetURL.Scheme == "http" &&
		(resetURL.Hostname() == "localhost" || net.ParseIP(resetURL.Hostname()).IsLoopback())
	if resetURL.Scheme != "https" && !isLocalHTTP {
		return "", errors.New("password-reset links must use HTTPS outside local development")
	}
	resetURL.Path = strings.TrimRight(resetURL.Path, "/") + "/reset-password"
	query := resetURL.Query()
	query.Set("token", token)
	resetURL.RawQuery = query.Encode()
	return resetURL.String(), nil
}

func dialSMTP(ctx context.Context, host string, port int) (smtpSession, error) {
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(
		ctx,
		"tcp",
		net.JoinHostPort(host, strconv.Itoa(port)),
	)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(10 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return nil, err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}
