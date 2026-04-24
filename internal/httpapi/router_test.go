package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/michibiki-io/mx-api-go/internal/config"
	"github.com/michibiki-io/mx-api-go/internal/mail"
	"github.com/michibiki-io/mx-api-go/internal/requestvalidator"
	"go.uber.org/zap"
)

type fakeSender struct {
	message mail.Message
	err     error
}

func (s *fakeSender) Send(_ context.Context, msg mail.Message) error {
	s.message = msg
	return s.err
}

func testRouter(t *testing.T, sender *fakeSender) http.Handler {
	t.Helper()
	cfg := config.Default()
	cfg.Mail.TemplatePath = filepath.Join(t.TempDir(), "template.html")
	if err := os.WriteFile(cfg.Mail.TemplatePath, []byte("Hello {{ name }}"), 0o600); err != nil {
		t.Fatal(err)
	}
	validatorEngine, err := requestvalidator.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return NewRouter(cfg, validatorEngine, sender, zap.NewNop())
}

func testRouterWithConfig(t *testing.T, cfg *config.Config, sender *fakeSender) http.Handler {
	t.Helper()
	validatorEngine, err := requestvalidator.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return NewRouter(cfg, validatorEngine, sender, zap.NewNop())
}

func TestValidatePostKeepsLegacyErrorShape(t *testing.T) {
	router := testRouter(t, &fakeSender{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/validate", bytes.NewBufferString(`{"name":"","email":"bad","message":""}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Referer", "http://localhost:5173/")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"status":"BadRequest"`)) {
		t.Fatalf("legacy status missing: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"code":"validation_required"`)) {
		t.Fatalf("legacy validation code missing: %s", rec.Body.String())
	}
}

func TestValidatePostSuccessUsesOk(t *testing.T) {
	router := testRouter(t, &fakeSender{})
	req := validRequest(http.MethodPost, "/api/v1/validate")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != `{"status":"Ok"}` {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestSendmailPostRendersTemplateAndSends(t *testing.T) {
	sender := &fakeSender{}
	router := testRouter(t, sender)
	req := validRequest(http.MethodPost, "/api/v1/sendmail")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if sender.message.Body != "Hello Jane" {
		t.Fatalf("rendered body = %q", sender.message.Body)
	}
	if sender.message.Subject != "Question" {
		t.Fatalf("subject = %q", sender.message.Subject)
	}
}

func TestSendmailPostFormatsSubmittedAtWithConfiguredTimezone(t *testing.T) {
	cfg := config.Default()
	cfg.Mail.Timezone = "Asia/Tokyo"
	cfg.Mail.SubmittedAtFormat = "2006/01/02 15:04:05 MST"
	cfg.Mail.TemplatePath = filepath.Join(t.TempDir(), "template.html")
	if err := os.WriteFile(cfg.Mail.TemplatePath, []byte("{{ submitted_at }}"), 0o600); err != nil {
		t.Fatal(err)
	}

	sender := &fakeSender{}
	router := testRouterWithConfig(t, cfg, sender)
	req := validRequest(http.MethodPost, "/api/v1/sendmail")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} JST$`).MatchString(sender.message.Body) {
		t.Fatalf("submitted_at = %q, want configured Asia/Tokyo format", sender.message.Body)
	}
}

func TestSendmailPostRendersHTMLLineBreaksSafely(t *testing.T) {
	cfg := config.Default()
	cfg.Mail.TemplatePath = filepath.Join(t.TempDir(), "template.html")
	if err := os.WriteFile(cfg.Mail.TemplatePath, []byte(`{% for field in fields %}{% if field.Name == "message" %}{{ field.HTMLValue|safe }}{% endif %}{% endfor %}`), 0o600); err != nil {
		t.Fatal(err)
	}

	sender := &fakeSender{}
	router := testRouterWithConfig(t, cfg, sender)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sendmail", bytes.NewBufferString(`{"name":"Jane","email":"jane@example.com","message":"Line 1\n<script>"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Referer", "http://localhost:5173/")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	want := "Line 1<br>&lt;script&gt;"
	if sender.message.Body != want {
		t.Fatalf("rendered body = %q, want %q", sender.message.Body, want)
	}
}

func TestSendmailPostRendersFieldsInConfiguredOrder(t *testing.T) {
	cfg := config.Default()
	cfg.Mail.TemplatePath = filepath.Join(t.TempDir(), "template.html")
	if err := os.WriteFile(cfg.Mail.TemplatePath, []byte(`{% for field in fields %}{{ field.Name }}={{ field.Value }};{% endfor %}`), 0o600); err != nil {
		t.Fatal(err)
	}

	sender := &fakeSender{}
	router := testRouterWithConfig(t, cfg, sender)
	req := validRequest(http.MethodPost, "/api/v1/sendmail")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	want := "name=Jane;email=jane@example.com;tel=090-1234-5678;organization=;subject=Question;message=Hello;"
	if sender.message.Body != want {
		t.Fatalf("rendered body = %q, want %q", sender.message.Body, want)
	}
}

func TestMissingOriginIsUnauthorized(t *testing.T) {
	router := testRouter(t, &fakeSender{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/validate", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func validRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(`{"name":"Jane","email":"jane@example.com","tel":"090-1234-5678","subject":"Question","message":"Hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Referer", "http://localhost:5173/")
	return req
}
