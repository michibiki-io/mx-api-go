package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/michibiki-io/mx-api-go/internal/audit"
	"github.com/michibiki-io/mx-api-go/internal/config"
	"github.com/michibiki-io/mx-api-go/internal/mail"
	"github.com/michibiki-io/mx-api-go/internal/requestvalidator"
	"go.uber.org/zap"
)

type fakeSender struct {
	message mail.Message
	err     error
	count   int
}

func (s *fakeSender) Send(_ context.Context, msg mail.Message) error {
	s.count++
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

func testRouterWithAudit(t *testing.T, cfg *config.Config, sender *fakeSender) (http.Handler, *audit.Store) {
	t.Helper()
	cfg.Mail.TemplatePath = filepath.Join(t.TempDir(), "template.html")
	if err := os.WriteFile(cfg.Mail.TemplatePath, []byte("Hello {{ name }}"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := audit.Open(context.Background(), filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	validatorEngine, err := requestvalidator.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return NewRouter(cfg, validatorEngine, sender, zap.NewNop(), store), store
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
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["status"] != "Ok" {
		t.Fatalf("status payload = %q", body["status"])
	}
	if body["version"] == "" {
		t.Fatalf("version payload missing: %s", rec.Body.String())
	}
}

func TestStatusEndpointIncludesVersion(t *testing.T) {
	router := testRouter(t, &fakeSender{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["status"] != "Ok" {
		t.Fatalf("status payload = %q", body["status"])
	}
	if body["version"] == "" {
		t.Fatalf("version payload missing: %s", rec.Body.String())
	}
}

func TestSchemaIncludesVersion(t *testing.T) {
	router := testRouter(t, &fakeSender{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/schema", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	version, ok := body["version"].(string)
	if !ok || version == "" {
		t.Fatalf("version payload missing: %s", rec.Body.String())
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

func TestSendmailRejectsHeaderInjectionSubject(t *testing.T) {
	sender := &fakeSender{}
	router := testRouter(t, sender)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sendmail", bytes.NewBufferString(`{"name":"Jane","email":"jane@example.com","subject":"Question\nBcc: attacker@example.com","message":"Hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Referer", "http://localhost:5173/")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if sender.count != 0 {
		t.Fatalf("sender count = %d, want 0", sender.count)
	}
	if !strings.Contains(rec.Body.String(), "validation_header_injection") {
		t.Fatalf("header injection validation missing: %s", rec.Body.String())
	}
}

func TestSendmailIdempotencyKeyReplaysSuccessWithoutSendingAgain(t *testing.T) {
	sender := &fakeSender{}
	router := testRouter(t, sender)

	req := validRequest(http.MethodPost, "/api/v1/sendmail")
	req.Header.Set("Idempotency-Key", "contact-submit-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", rec.Code, rec.Body.String())
	}

	replayReq := validRequest(http.MethodPost, "/api/v1/sendmail")
	replayReq.Header.Set("Idempotency-Key", "contact-submit-1")
	replayRec := httptest.NewRecorder()
	router.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("replay status = %d, body = %s", replayRec.Code, replayRec.Body.String())
	}
	if sender.count != 1 {
		t.Fatalf("sender count = %d, want 1", sender.count)
	}
}

func TestSendmailIdempotencyKeyRejectsDifferentPayload(t *testing.T) {
	sender := &fakeSender{}
	router := testRouter(t, sender)

	req := validRequest(http.MethodPost, "/api/v1/sendmail")
	req.Header.Set("Idempotency-Key", "contact-submit-2")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", rec.Code, rec.Body.String())
	}

	conflictReq := httptest.NewRequest(http.MethodPost, "/api/v1/sendmail", bytes.NewBufferString(`{"name":"Jane","email":"jane@example.com","tel":"090-1234-5678","subject":"Different","message":"Hello"}`))
	conflictReq.Header.Set("Content-Type", "application/json")
	conflictReq.Header.Set("Origin", "http://localhost:5173")
	conflictReq.Header.Set("Referer", "http://localhost:5173/")
	conflictReq.Header.Set("Idempotency-Key", "contact-submit-2")
	conflictRec := httptest.NewRecorder()
	router.ServeHTTP(conflictRec, conflictReq)
	if conflictRec.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, body = %s", conflictRec.Code, conflictRec.Body.String())
	}
	if sender.count != 1 {
		t.Fatalf("sender count = %d, want 1", sender.count)
	}
}

func TestPublicAPIRateLimitRejectsExcessRequests(t *testing.T) {
	cfg := config.Default()
	cfg.Security.RateLimit.RequestsPerMinute = 1
	cfg.Security.RateLimit.FailureRequestsPerMinute = 0
	cfg.Mail.TemplatePath = filepath.Join(t.TempDir(), "template.html")
	if err := os.WriteFile(cfg.Mail.TemplatePath, []byte("Hello {{ name }}"), 0o600); err != nil {
		t.Fatal(err)
	}
	router := testRouterWithConfig(t, cfg, &fakeSender{})

	first := validRequest(http.MethodPost, "/api/v1/validate")
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", firstRec.Code, firstRec.Body.String())
	}

	second := validRequest(http.MethodPost, "/api/v1/validate")
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, body = %s", secondRec.Code, secondRec.Body.String())
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

func TestAuditLoggingRecordsPublicAPIWithoutSensitiveData(t *testing.T) {
	cfg := config.Default()
	router, store := testRouterWithAudit(t, cfg, &fakeSender{})
	req := validRequest(http.MethodPost, "/api/v1/sendmail")
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Cookie", "session=secret")
	req.Header.Set("X-Request-ID", "req-123")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	page, err := store.List(context.Background(), audit.Filter{Action: "mail.send", RequestID: "req-123"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("mail send audit total = %d, want 1", page.Total)
	}
	raw, _ := json.Marshal(page.Items[0])
	for _, forbidden := range []string{"secret-token", "session=secret", "Hello", "jane@example.com"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("audit event contains sensitive value %q: %s", forbidden, string(raw))
		}
	}
}

func TestAdminHeaderAuthAllowAndDeny(t *testing.T) {
	cfg := config.Default()
	cfg.Admin.Auth.AllowedGroups = []string{"mx-api-admins"}
	router, store := testRouterWithAudit(t, cfg, &fakeSender{})

	deniedReq := httptest.NewRequest(http.MethodGet, "/_admin/api/v1/me", nil)
	deniedReq.Header.Set("X-Forwarded-User", "bob@example.com")
	deniedReq.Header.Set("X-Forwarded-Groups", "users")
	deniedRec := httptest.NewRecorder()
	router.ServeHTTP(deniedRec, deniedReq)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("denied status = %d, body = %s", deniedRec.Code, deniedRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/_admin/api/v1/me", nil)
	req.Header.Set("X-Forwarded-User", "alice@example.com")
	req.Header.Set("X-Forwarded-Email", "alice@example.com")
	req.Header.Set("X-Forwarded-Groups", "users, mx-api-admins")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["user"] != "alice@example.com" || body["mode"] != "header" {
		t.Fatalf("unexpected me response: %#v", body)
	}

	page, err := store.List(context.Background(), audit.Filter{Action: "admin.access.denied"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("denied audit total = %d, want 1", page.Total)
	}
}

func TestAdminNoneAuthModeAllowsAndReportsWarningState(t *testing.T) {
	cfg := config.Default()
	cfg.Admin.Auth.Mode = "none"
	cfg.Admin.Dashboard.TimestampFormat = "2006/01/02 15:04 MST"
	router, _ := testRouterWithAudit(t, cfg, &fakeSender{})
	req := httptest.NewRequest(http.MethodGet, "/_admin/api/v1/me", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["authDisabled"] != true {
		t.Fatalf("authDisabled = %#v, want true", body["authDisabled"])
	}
	if body["auditTimestampFormat"] != "2006/01/02 15:04 MST" {
		t.Fatalf("auditTimestampFormat = %#v", body["auditTimestampFormat"])
	}
	if body["commit"] == "" || body["shortCommit"] == "" {
		t.Fatalf("commit fields missing: %#v", body)
	}
}

func TestAdminMailServerCheckRequiresHeaderAuthAndDashboardToken(t *testing.T) {
	cfg := config.Default()
	cfg.SMTP.ServerAddr = "127.0.0.1:1"
	cfg.SMTP.TLSMode = "plain"
	cfg.SMTP.AuthenticationEnabled = false
	cfg.SMTP.Timeout = 50 * time.Millisecond
	router, _ := testRouterWithAudit(t, cfg, &fakeSender{})

	req := httptest.NewRequest(http.MethodPost, "/_admin/api/v1/mail-server-check", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	setAdminHeaders(req)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status without dashboard token = %d, body = %s", rec.Code, rec.Body.String())
	}

	token := dashboardTokenFromConfigJS(t, router)
	req = httptest.NewRequest(http.MethodPost, "/_admin/api/v1/mail-server-check", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(dashboardTokenHeader, token)
	setAdminHeaders(req)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status with dashboard token = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Status     string `json:"status"`
		MailServer struct {
			Reachable             bool   `json:"reachable"`
			AuthenticationEnabled bool   `json:"authenticationEnabled"`
			Code                  string `json:"code"`
		} `json:"mailServer"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Status != "Error" || body.MailServer.Code == "" || body.MailServer.AuthenticationEnabled {
		t.Fatalf("unexpected mail server check response: %#v", body)
	}
}

func TestAdminMailServerCheckDisabledWhenAdminAuthNone(t *testing.T) {
	cfg := config.Default()
	cfg.Admin.Auth.Mode = "none"
	router, _ := testRouterWithAudit(t, cfg, &fakeSender{})

	req := httptest.NewRequest(http.MethodPost, "/_admin/api/v1/mail-server-check", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(dashboardTokenHeader, "01234567-89ab-4def-8123-456789abcdef")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "admin header authentication is required") {
		t.Fatalf("unexpected body = %s", rec.Body.String())
	}
}

func TestAdminAuditEventsFiltersAndPaginates(t *testing.T) {
	cfg := config.Default()
	cfg.Admin.Auth.Mode = "none"
	cfg.Admin.Dashboard.TimestampFormat = "2006-01-02 15:04:05 MST"
	cfg.Admin.Dashboard.TimestampTimezone = "Asia/Tokyo"
	router, store := testRouterWithAudit(t, cfg, &fakeSender{})
	base := time.Date(2026, 5, 1, 13, 24, 23, 0, time.UTC)
	for i, event := range []audit.Event{
		{Actor: "public", Action: "validation.request", Method: "POST", Path: "/api/v1/validate", StatusCode: 400, Result: audit.ResultFailure},
		{Actor: "public", Action: "mail.send", Method: "POST", Path: "/api/v1/sendmail", StatusCode: 200, Result: audit.ResultSuccess},
		{Actor: "public", Action: "mail.send", Method: "POST", Path: "/api/v1/sendmail", StatusCode: 400, Result: audit.ResultFailure},
	} {
		event.Timestamp = base.Add(time.Duration(i) * time.Second)
		if err := store.Record(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/_admin/api/v1/audit-events?action=mail.send&limit=1&offset=1", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []struct {
			audit.Event
			TimestampDisplay string `json:"timestampDisplay"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Total != 2 || len(body.Items) != 1 {
		t.Fatalf("body = %#v, want total 2 and one item", body)
	}
	if !strings.HasSuffix(body.Items[0].TimestampDisplay, "JST") {
		t.Fatalf("timestampDisplay = %q, want JST suffix", body.Items[0].TimestampDisplay)
	}
}

func TestAdminAuditResetClearsEventsAndLeavesMarker(t *testing.T) {
	cfg := config.Default()
	cfg.Admin.Auth.Mode = "none"
	router, store := testRouterWithAudit(t, cfg, &fakeSender{})
	if err := store.Record(context.Background(), audit.Event{Actor: "public", Action: "mail.send", StatusCode: 200, Result: audit.ResultSuccess}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/_admin/api/v1/audit-events/reset", strings.NewReader(`{"confirmation":"RESET","reason":"test reset"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	page, err := store.List(context.Background(), audit.Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Action != "audit.reset" {
		t.Fatalf("page after reset = %#v", page)
	}
}

func TestAdminRequestMetricsAggregates(t *testing.T) {
	cfg := config.Default()
	cfg.Admin.Auth.Mode = "none"
	router, store := testRouterWithAudit(t, cfg, &fakeSender{})
	base := time.Now().UTC().Add(-2 * time.Hour)
	for _, event := range []audit.Event{
		{Timestamp: base.Add(10 * time.Minute), Action: "validation.request", StatusCode: 400, Result: audit.ResultFailure},
		{Timestamp: base.Add(20 * time.Minute), Action: "mail.send", StatusCode: 200, Result: audit.ResultSuccess},
	} {
		if err := store.Record(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	url := "/_admin/api/v1/request-metrics?from=" + base.Format(time.RFC3339Nano) + "&to=" + base.Add(time.Hour).Format(time.RFC3339Nano) + "&bucket=1h"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body audit.Metrics
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Summary.Total != 2 || body.Summary.ValidationFailures != 1 {
		t.Fatalf("metrics summary = %#v", body.Summary)
	}
}

func validRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(`{"name":"Jane","email":"jane@example.com","tel":"090-1234-5678","subject":"Question","message":"Hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Referer", "http://localhost:5173/")
	return req
}

func setAdminHeaders(req *http.Request) {
	req.Header.Set("X-Forwarded-User", "alice@example.com")
	req.Header.Set("X-Forwarded-Email", "alice@example.com")
	req.Header.Set("X-Forwarded-Groups", "mx-api-admins")
}

func dashboardTokenFromConfigJS(t *testing.T, router http.Handler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/config.js", nil)
	setAdminHeaders(req)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("config.js status = %d, body = %s", rec.Code, rec.Body.String())
	}
	matches := regexp.MustCompile(`"dashboardToken":"([0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12})"`).FindStringSubmatch(rec.Body.String())
	if len(matches) != 2 {
		t.Fatalf("dashboard token missing in config.js: %s", rec.Body.String())
	}
	return matches[1]
}
