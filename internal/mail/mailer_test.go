package mail

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/michibiki-io/mx-api-go/internal/config"
)

func TestRenderDefaultTemplateAliases(t *testing.T) {
	tests := []string{
		"",
		"default_mail_template.html",
		"templates/default_mail_template.html",
		"/app/templates/default_mail_template.html",
		"/etc/mx-api/templates/default_mail_template.html",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			body := renderTestTemplate(t, path)
			if body == "" {
				t.Fatal("rendered body is empty")
			}
		})
	}
}

func renderTestTemplate(t *testing.T, path string) string {
	t.Helper()
	body, err := RenderTemplate(path, map[string]any{
		"name": "Jane",
		"fields": []map[string]string{
			{"Name": "name", "Label": "Name", "Value": "Jane", "HTMLValue": "Jane"},
			{"Name": "message", "Label": "Message", "Value": "Hello", "HTMLValue": "Hello"},
		},
		"contact_name": "Homepage",
		"homepage_url": "https://example.com",
		"submitted_at": "2026-04-24 00:00:00 UTC",
	})
	if err != nil {
		t.Fatalf("RenderTemplate() error = %v", err)
	}
	return body
}

func TestSMTPSenderReportsDataCloseError(t *testing.T) {
	addr, stop := startSMTPTestServer(t, "554 message rejected")
	defer stop()

	sender := NewSMTPSender(&config.Config{
		SMTP: config.SMTPConfig{
			ServerAddr: addr,
			Timeout:    time.Second,
		},
	})

	err := sender.Send(context.Background(), Message{
		From:       "from@example.com",
		Recipients: []string{"to@example.com"},
		Subject:    "Subject",
		Body:       "Body",
	})
	if err == nil {
		t.Fatal("Send() error = nil, want DATA close error")
	}
	if !strings.Contains(err.Error(), "finish smtp DATA") {
		t.Fatalf("Send() error = %q, want DATA close context", err)
	}
}

func TestSMTPSenderCheckPlainServer(t *testing.T) {
	addr, stop := startSMTPTestServer(t, "250 OK")
	defer stop()

	sender := NewSMTPSender(&config.Config{
		SMTP: config.SMTPConfig{
			ServerAddr: addr,
			TLSMode:    "plain",
			Timeout:    time.Second,
		},
	})

	result := sender.Check(context.Background())
	if result.Code != "" {
		t.Fatalf("Check() code = %q, message = %q", result.Code, result.Message)
	}
	if !result.Reachable {
		t.Fatal("Check() reachable = false, want true")
	}
	if result.AuthenticationEnabled || result.Authenticated {
		t.Fatalf("Check() auth flags = enabled:%t authenticated:%t, want false false", result.AuthenticationEnabled, result.Authenticated)
	}
	if result.TLSActive {
		t.Fatal("Check() TLSActive = true, want false")
	}
	if result.CheckedAt.IsZero() {
		t.Fatal("Check() checkedAt is zero")
	}
}

func TestSMTPSenderCheckInvalidConfig(t *testing.T) {
	sender := NewSMTPSender(&config.Config{})

	result := sender.Check(context.Background())
	if result.Code != "invalid_config" {
		t.Fatalf("Check() code = %q, want invalid_config", result.Code)
	}
	if result.Reachable {
		t.Fatal("Check() reachable = true, want false")
	}
}

func TestPrepareMessageRejectsUnsafeHeaders(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
	}{
		{
			name: "subject",
			msg: Message{
				From:       "from@example.com",
				Recipients: []string{"to@example.com"},
				Subject:    "Subject\r\nBcc: attacker@example.com",
			},
		},
		{
			name: "from",
			msg: Message{
				From:       "from@example.com\r\nBcc: attacker@example.com",
				Recipients: []string{"to@example.com"},
				Subject:    "Subject",
			},
		},
		{
			name: "recipient",
			msg: Message{
				From:       "from@example.com",
				Recipients: []string{"to@example.com\nBcc: attacker@example.com"},
				Subject:    "Subject",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := prepareMessage(tt.msg); err == nil {
				t.Fatal("prepareMessage() error = nil, want unsafe header error")
			}
		})
	}
}

func TestPrepareMessageAcceptsDisplayAddresses(t *testing.T) {
	prepared, err := prepareMessage(Message{
		From:       "Example <from@example.com>",
		Recipients: []string{"Contact <to@example.com>"},
		Subject:    "Subject",
	})
	if err != nil {
		t.Fatalf("prepareMessage() error = %v", err)
	}
	if prepared.From.Address != "from@example.com" || prepared.Recipients[0].Address != "to@example.com" {
		t.Fatalf("prepared addresses = %#v %#v", prepared.From, prepared.Recipients[0])
	}
}

func startSMTPTestServer(t *testing.T, dataResponse string) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		writeLine := func(line string) bool {
			_, err := conn.Write([]byte(line + "\r\n"))
			return err == nil
		}
		if !writeLine("220 localhost ESMTP") {
			return
		}

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				if _, err := conn.Write([]byte("250-localhost\r\n250 OK\r\n")); err != nil {
					return
				}
			case strings.HasPrefix(cmd, "MAIL FROM:"):
				if !writeLine("250 OK") {
					return
				}
			case strings.HasPrefix(cmd, "RCPT TO:"):
				if !writeLine("250 OK") {
					return
				}
			case cmd == "DATA":
				if !writeLine("354 End data with <CR><LF>.<CR><LF>") {
					return
				}
				for {
					dataLine, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					if strings.TrimRight(dataLine, "\r\n") == "." {
						break
					}
				}
				if !writeLine(dataResponse) {
					return
				}
			case cmd == "QUIT":
				_ = writeLine("221 Bye")
				return
			default:
				if !writeLine("250 OK") {
					return
				}
			}
		}
	}()

	stop := func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}
	return listener.Addr().String(), stop
}
