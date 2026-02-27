package notifications

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

// EmailNotifier sends notifications via SMTP email
type EmailNotifier struct {
	smtpHost     string
	smtpPort     int
	username     string
	password     string
	from         string
	to           []string
	useTLS       bool
	insecureSkip bool
}

// EmailConfig contains SMTP email configuration
type EmailConfig struct {
	SMTPHost string
	SMTPPort int
	Username string
	// Password is the SMTP authentication password.
	// SECURITY: This field is intentionally exported for configuration purposes.
	// Users should NEVER hardcode passwords - instead use:
	//   - Environment variables: ${SMTP_PASSWORD}
	//   - Kubernetes secrets mounted as env vars
	//   - Secret management systems (Vault, AWS Secrets Manager, etc.)
	// The password is used at runtime and never logged or persisted.
	// #nosec G117 - Exported by design, secure usage documented
	Password     string
	From         string
	To           []string
	UseTLS       bool
	InsecureSkip bool
}

// NewEmailNotifier creates a new email notifier
func NewEmailNotifier(config EmailConfig) *EmailNotifier {
	return &EmailNotifier{
		smtpHost:     config.SMTPHost,
		smtpPort:     config.SMTPPort,
		username:     config.Username,
		password:     config.Password,
		from:         config.From,
		to:           config.To,
		useTLS:       config.UseTLS,
		insecureSkip: config.InsecureSkip,
	}
}

// Name returns the name of the notifier
func (e *EmailNotifier) Name() string {
	return "email"
}

// Send sends a notification via email
func (e *EmailNotifier) Send(ctx context.Context, event *Event) error {
	if e.smtpHost == "" {
		return fmt.Errorf("SMTP host is not configured")
	}

	if len(e.to) == 0 {
		return fmt.Errorf("no email recipients configured")
	}

	// Build email message
	subject := fmt.Sprintf("[%s] %s - %s", strings.ToUpper(string(event.Severity)), event.Type, event.Namespace)
	body := e.buildEmailBody(event)
	message := e.formatEmailMessage(subject, body)

	// Send email
	return e.sendEmail(ctx, message)
}

// buildEmailBody builds the email body from an event
func (e *EmailNotifier) buildEmailBody(event *Event) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Event Type: %s\n", event.Type))
	sb.WriteString(fmt.Sprintf("Severity: %s\n", event.Severity))
	sb.WriteString(fmt.Sprintf("Timestamp: %s\n", event.Timestamp.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Namespace: %s\n", event.Namespace))

	if event.Workload != "" {
		sb.WriteString(fmt.Sprintf("Workload: %s\n", event.Workload))
	}

	if event.Pod != "" {
		sb.WriteString(fmt.Sprintf("Pod: %s\n", event.Pod))
	}

	if event.Container != "" {
		sb.WriteString(fmt.Sprintf("Container: %s\n", event.Container))
	}

	sb.WriteString(fmt.Sprintf("\nMessage:\n%s\n", event.Message))

	if len(event.Metadata) > 0 {
		sb.WriteString("\nAdditional Information:\n")
		for key, value := range event.Metadata {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", key, value))
		}
	}

	sb.WriteString("\n---\n")
	sb.WriteString("This is an automated notification from Intelligent Cluster Optimizer.\n")

	return sb.String()
}

// formatEmailMessage formats the email message with headers
func (e *EmailNotifier) formatEmailMessage(subject, body string) []byte {
	var msg strings.Builder

	msg.WriteString(fmt.Sprintf("From: %s\r\n", e.from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(e.to, ", ")))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	return []byte(msg.String())
}

// sendEmail sends the email via SMTP
func (e *EmailNotifier) sendEmail(ctx context.Context, message []byte) error {
	addr := fmt.Sprintf("%s:%d", e.smtpHost, e.smtpPort)

	// Create channel for sending result
	done := make(chan error, 1)

	go func() {
		var err error
		if e.useTLS {
			err = e.sendWithTLS(addr, message)
		} else {
			err = e.sendPlain(addr, message)
		}
		done <- err
	}()

	// Wait for send to complete or context to be cancelled
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sendWithTLS sends email using TLS
func (e *EmailNotifier) sendWithTLS(addr string, message []byte) error {
	// Create TLS config
	tlsConfig := &tls.Config{
		ServerName:         e.smtpHost,
		InsecureSkipVerify: e.insecureSkip, // #nosec G402 - configurable for internal SMTP servers
	}

	// Connect to SMTP server with TLS
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	// Create SMTP client
	client, err := smtp.NewClient(conn, e.smtpHost)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer func() { _ = client.Quit() }() // Best effort cleanup

	// Authenticate if credentials provided
	if e.username != "" && e.password != "" {
		auth := smtp.PlainAuth("", e.username, e.password, e.smtpHost)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}

	// Send email
	if err := client.Mail(e.from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	for _, recipient := range e.to {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", recipient, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to send DATA command: %w", err)
	}

	if _, err := w.Write(message); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close message writer: %w", err)
	}

	return nil
}

// sendPlain sends email without TLS (plain text)
func (e *EmailNotifier) sendPlain(addr string, message []byte) error {
	var auth smtp.Auth
	if e.username != "" && e.password != "" {
		auth = smtp.PlainAuth("", e.username, e.password, e.smtpHost)
	}

	err := smtp.SendMail(addr, auth, e.from, e.to, message)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}
