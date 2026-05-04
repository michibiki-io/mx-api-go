package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/michibiki-io/mx-api-go/internal/audit"
	"github.com/michibiki-io/mx-api-go/internal/config"
	"github.com/michibiki-io/mx-api-go/internal/infrastructure/database"
	"go.uber.org/zap"
)

func TestStoreRecordsFiltersAndPaginatesAuditEvents(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC()
	events := []audit.Event{
		{Timestamp: now.Add(-2 * time.Minute), Actor: "public", Action: "validation.request", Method: "POST", Path: "/api/v1/validate", StatusCode: 400, Result: audit.ResultFailure, ErrorCode: "validation_failed"},
		{Timestamp: now.Add(-1 * time.Minute), Actor: "public", Action: "mail.send", Method: "POST", Path: "/api/v1/sendmail", StatusCode: 200, Result: audit.ResultSuccess},
		{Timestamp: now, Actor: "admin@example.com", Action: "audit.view", Method: "GET", Path: "/_admin/api/v1/audit-events", StatusCode: 200, Result: audit.ResultSuccess},
	}
	for _, event := range events {
		if err := store.Record(context.Background(), event); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}

	firstPage, err := store.List(context.Background(), audit.Filter{Result: audit.ResultSuccess, Limit: 1})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	page, err := store.List(context.Background(), audit.Filter{Result: audit.ResultSuccess, Limit: 1, Cursor: firstPage.NextCursor})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if page.Total != 2 || len(page.Items) != 1 {
		t.Fatalf("page = %#v, want total 2 and one item", page)
	}
	if page.Items[0].Action != "mail.send" {
		t.Fatalf("second success action = %q", page.Items[0].Action)
	}
}

func TestMetricsAggregationAndSummary(t *testing.T) {
	store := newTestStore(t)
	base := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	for _, event := range []audit.Event{
		{Timestamp: base.Add(5 * time.Minute), Action: "validation.request", StatusCode: 200, Result: audit.ResultSuccess},
		{Timestamp: base.Add(10 * time.Minute), Action: "validation.request", StatusCode: 400, Result: audit.ResultFailure},
		{Timestamp: base.Add(20 * time.Minute), Action: "mail.send", StatusCode: 200, Result: audit.ResultSuccess},
		{Timestamp: base.Add(65 * time.Minute), Action: "mail.send", StatusCode: 500, Result: audit.ResultFailure},
	} {
		if err := store.Record(context.Background(), event); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}

	metrics, err := store.Metrics(context.Background(), audit.MetricsFilter{
		From:   base,
		To:     base.Add(2 * time.Hour),
		Bucket: time.Hour,
	})
	if err != nil {
		t.Fatalf("Metrics() error = %v", err)
	}
	if len(metrics.Points) != 3 {
		t.Fatalf("points len = %d, want 3", len(metrics.Points))
	}
	if metrics.Points[0].Count != 3 || metrics.Points[1].Count != 1 {
		t.Fatalf("points = %#v", metrics.Points)
	}
	if metrics.Summary.ValidationSuccesses != 1 || metrics.Summary.ValidationFailures != 1 || metrics.Summary.MailSendSuccesses != 1 || metrics.Summary.Count5xx != 1 {
		t.Fatalf("summary = %#v", metrics.Summary)
	}
}

func TestMetricsWithNoBusinessEvents(t *testing.T) {
	store := newTestStore(t)
	base := time.Date(2026, 5, 4, 6, 0, 0, 0, time.UTC)
	for _, event := range []audit.Event{
		{Timestamp: base.Add(10 * time.Minute), Action: "admin.dashboard.view", StatusCode: 200, Result: audit.ResultSuccess},
		{Timestamp: base.Add(20 * time.Minute), Action: "audit.view", StatusCode: 200, Result: audit.ResultSuccess},
	} {
		if err := store.Record(context.Background(), event); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}

	metrics, err := store.Metrics(context.Background(), audit.MetricsFilter{
		From:   base,
		To:     base.Add(2 * time.Hour),
		Bucket: time.Hour,
	})
	if err != nil {
		t.Fatalf("Metrics() error = %v", err)
	}
	if len(metrics.Points) != 3 {
		t.Fatalf("points len = %d, want 3", len(metrics.Points))
	}
	for _, point := range metrics.Points {
		if point.Count != 0 {
			t.Fatalf("point count = %d, want 0: %#v", point.Count, metrics.Points)
		}
	}
	if metrics.Summary.Total != 0 || metrics.Summary.Successful != 0 || metrics.Summary.Failed != 0 {
		t.Fatalf("summary = %#v", metrics.Summary)
	}
}

func TestStoreResetLeavesMarker(t *testing.T) {
	store := newTestStore(t)
	if err := store.Record(context.Background(), audit.Event{Action: "mail.send", Result: audit.ResultSuccess}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if err := store.Reset(context.Background(), audit.Event{
		Actor:   "admin@example.com",
		Action:  "audit.reset",
		Result:  audit.ResultSuccess,
		Message: "Admin reset audit log",
	}); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	page, err := store.List(context.Background(), audit.Filter{Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Action != "audit.reset" {
		t.Fatalf("page after reset = %#v", page)
	}
}

func TestSafeMetadataDropsSensitiveValues(t *testing.T) {
	metadata := audit.SafeMetadata(map[string]any{
		"requestSize":   123,
		"password":      "secret",
		"Authorization": "Bearer token",
		"cookie":        "session=value",
		"message":       "raw mail body",
	})
	if metadata["requestSize"] != 123 {
		t.Fatalf("safe metadata missing allowed value: %#v", metadata)
	}
	for _, key := range []string{"password", "Authorization", "cookie", "message"} {
		if _, ok := metadata[key]; ok {
			t.Fatalf("sensitive key %q was retained: %#v", key, metadata)
		}
	}
}

func newTestStore(t *testing.T) *audit.Store {
	t.Helper()
	cfg := config.Default()
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = "file:" + t.TempDir() + "/audit.db?cache=shared&mode=rwc&_journal_mode=WAL&_busy_timeout=5000"
	cfg.Audit.Async.Enabled = false
	store, err := database.OpenAuditStore(context.Background(), cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("OpenAuditStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
