package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flosch/pongo2/v6"
	"github.com/michibiki-io/mx-api-go/internal/config"
)

const (
	defaultTemplateName       = "default_mail_template.html"
	defaultSimpleTemplateName = "default_mail_template_simple.html"
)

//go:embed templates/*.html
var defaultTemplates embed.FS

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
	tpl, err := loadTemplate(path)
	if err != nil {
		return "", err
	}
	rendered, err := tpl.Execute(pongo2.Context(data))
	if err != nil {
		return "", err
	}
	return rendered, nil
}

func loadTemplate(path string) (*pongo2.Template, error) {
	if strings.TrimSpace(path) != "" {
		if _, err := os.Stat(path); err == nil {
			return pongo2.FromFile(path)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}

	if name := embeddedTemplateName(path); name != "" {
		content, err := defaultTemplates.ReadFile("templates/" + name)
		if err != nil {
			return nil, err
		}
		return pongo2.FromString(string(content))
	}

	return pongo2.FromFile(path)
}

func embeddedTemplateName(path string) string {
	cleanPath := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	switch cleanPath {
	case ".", "", defaultTemplateName, "templates/" + defaultTemplateName, "/app/templates/" + defaultTemplateName, "/etc/mx-api/templates/" + defaultTemplateName:
		return defaultTemplateName
	case defaultSimpleTemplateName, "templates/" + defaultSimpleTemplateName, "/app/templates/" + defaultSimpleTemplateName, "/etc/mx-api/templates/" + defaultSimpleTemplateName:
		return defaultSimpleTemplateName
	default:
		return ""
	}
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

	if _, err := writer.Write(buildMIMEMessage(msg)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write smtp body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish smtp DATA: %w", err)
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
