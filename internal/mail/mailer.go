package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/flosch/pongo2/v6"
	"github.com/michibiki-io/mx-api-go/internal/config"
)

type Message struct {
	From       string
	Recipients []string
	Subject    string
	Body       string
}

type Sender interface {
	Send(context.Context, Message) error
}

type SMTPSender struct {
	cfg *config.Config
}

func NewSMTPSender(cfg *config.Config) *SMTPSender {
	return &SMTPSender{cfg: cfg}
}

func RenderTemplate(path string, data map[string]any) (string, error) {
	tpl, err := pongo2.FromFile(path)
	if err != nil {
		return "", err
	}
	rendered, err := tpl.Execute(pongo2.Context(data))
	if err != nil {
		return "", err
	}
	return rendered, nil
}

func (s *SMTPSender) Send(ctx context.Context, msg Message) error {
	if s.cfg.SMTP.ServerAddr == "" {
		return fmt.Errorf("smtp server address is empty")
	}
	host, _, err := net.SplitHostPort(s.cfg.SMTP.ServerAddr)
	if err != nil {
		return fmt.Errorf("invalid smtp server address %q: %w", s.cfg.SMTP.ServerAddr, err)
	}

	conn, err := s.dial(ctx, host)
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	defer client.Quit()

	if strings.EqualFold(s.cfg.SMTP.TLSMode, "starttls") {
		tlsCfg := s.tlsConfig(host)
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("start tls: %w", err)
			}
		}
	}

	if s.cfg.SMTP.AuthenticationEnabled {
		auth := smtp.PlainAuth("", s.cfg.SMTP.ClientUsername, s.cfg.SMTP.ClientPassword, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authenticate smtp client: %w", err)
		}
	}

	if err := client.Mail(msg.From); err != nil {
		return fmt.Errorf("smtp MAIL: %w", err)
	}
	for _, recipient := range msg.Recipients {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("smtp RCPT %q: %w", recipient, err)
		}
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	defer writer.Close()

	if _, err := writer.Write(buildMIMEMessage(msg)); err != nil {
		return fmt.Errorf("write smtp body: %w", err)
	}
	return nil
}

func (s *SMTPSender) dial(ctx context.Context, host string) (net.Conn, error) {
	timeout := s.cfg.SMTP.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout}

	if strings.EqualFold(s.cfg.SMTP.TLSMode, "implicit") {
		tlsDialer := &tls.Dialer{NetDialer: dialer, Config: s.tlsConfig(host)}
		conn, err := tlsDialer.DialContext(ctx, "tcp", s.cfg.SMTP.ServerAddr)
		if err != nil {
			return nil, fmt.Errorf("dial implicit tls smtp %s: %w", s.cfg.SMTP.ServerAddr, err)
		}
		return conn, nil
	}

	conn, err := dialer.DialContext(ctx, "tcp", s.cfg.SMTP.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("dial smtp %s: %w", s.cfg.SMTP.ServerAddr, err)
	}
	return conn, nil
}

func (s *SMTPSender) tlsConfig(host string) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         host,
		InsecureSkipVerify: s.cfg.SMTP.SkipVerifyCert,
	}
}

func buildMIMEMessage(msg Message) []byte {
	var buf bytes.Buffer
	from := msg.From
	if parsed, err := mail.ParseAddress(msg.From); err == nil {
		from = parsed.String()
	}
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + strings.Join(msg.Recipients, ", ") + "\r\n")
	buf.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", msg.Subject) + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(msg.Body)
	return buf.Bytes()
}
