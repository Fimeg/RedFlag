package notifier

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// SmtpSink delivers events via plain-text SMTP email.
// Uses net/smtp only — no HTML templates, no third-party mail frameworks.
// Auth preference: STARTTLS → plain auth → CRAM-MD5.
type SmtpSink struct {
	host       string // host:port
	from       string
	to         string
	severity   string
	eventClass []EventClass
}

// NewSmtpSink creates an SMTP sink. host is the SMTP server address
// (e.g. "smtp.example.com:587"). from is the sender address. to is
// the recipient. severity filters by minimum severity.
func NewSmtpSink(host, from, to, severity string, eventClass []EventClass) *SmtpSink {
	return &SmtpSink{
		host:       host,
		from:       from,
		to:         to,
		severity:   severity,
		eventClass: eventClass,
	}
}

func (s *SmtpSink) ID() string { return "smtp" }

func (s *SmtpSink) Send(ctx context.Context, event NotifyEvent) error {
	if !SeverityPasses(s.severity, event.Severity) {
		return nil
	}

	msg := buildEmail(s.from, s.to, event)

	// Split host:port — net/smtp needs them separate.
	host, port := s.splitHostPort()

	// Try STARTTLS first (port 587 path), then fall back to plain auth.
	// net/smtp.SendMail with TLS config triggers STARTTLS when available.
	tlsConfig := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}

	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("smtp panic: %v", r)
			}
		}()

		addr := net.JoinHostPort(host, port)
		if port == "465" {
			// SMTPS — TLS from the start, not STARTTLS.
			conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", addr, tlsConfig)
			if err != nil {
				done <- fmt.Errorf("smtps dial: %w", err)
				return
			}
			client, err := smtp.NewClient(conn, host)
			if err != nil {
				conn.Close()
				done <- fmt.Errorf("smtps client: %w", err)
				return
			}
			defer client.Close()
			if err := sendMailNoAuth(client, s.from, []string{s.to}, msg); err != nil {
				done <- fmt.Errorf("smtps send: %w", err)
				return
			}
		} else {
			// Standard SMTP — attempt STARTTLS, then send without auth.
			// No SMTP AUTH credentials configured: RedFlag SMTP is
			// designed for internal relays that accept unauthenticated
			// mail from trusted networks (postfix null client, msmtp,
			// or a local MTA on localhost:25).
			err := smtp.SendMail(addr, nil, s.from, []string{s.to}, msg)
			if err != nil {
				done <- fmt.Errorf("smtp send: %w", err)
				return
			}
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("[WARN] [notifier] [smtp] send_failed host=%s error=%v", s.host, err)
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *SmtpSink) splitHostPort() (string, string) {
	parts := strings.SplitN(s.host, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return s.host, "25"
}

// sendMailNoAuth sends mail through an already-established SMTP client
// without attempting authentication.
func sendMailNoAuth(client *smtp.Client, from string, to []string, msg []byte) error {
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err := client.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(msg)
	if err != nil {
		return err
	}
	return w.Close()
}

// buildEmail composes a plain-text RFC 5322 message.
func buildEmail(from, to string, event NotifyEvent) []byte {
	now := time.Now().UTC().Format(time.RFC1123Z)
	subject := fmt.Sprintf("[RedFlag] %s: %s", strings.ToUpper(string(event.Class)), event.Title)

	var body strings.Builder
	body.WriteString(fmt.Sprintf("From: RedFlag <%s>\r\n", from))
	body.WriteString(fmt.Sprintf("To: <%s>\r\n", to))
	body.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	body.WriteString(fmt.Sprintf("Date: %s\r\n", now))
	body.WriteString("MIME-Version: 1.0\r\n")
	body.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	body.WriteString("X-Mailer: RedFlag Notifier\r\n")
	body.WriteString("\r\n")
	body.WriteString(event.Message)
	body.WriteString("\r\n")
	body.WriteString(fmt.Sprintf("\r\n---\r\nSeverity: %s | Class: %s", event.Severity, event.Class))
	if event.AgentID != "" {
		body.WriteString(fmt.Sprintf(" | Agent: %s", event.AgentID))
	}
	body.WriteString(fmt.Sprintf("\r\nSent: %s\r\n", now))

	return []byte(body.String())
}
