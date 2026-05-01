package audit

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRecordsFiltersAndPaginatesAuditEvents(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC()
	events := []Event{
		{Timestamp: now.Add(-2 * time.Minute), Actor: "public", Action: "validation.request", Method: "POST", Path: "/api/v1/validate", StatusCode: 400, Result: ResultFailure, ErrorCode: "validation_failed"},
		{Timestamp: now.Add(-1 * time.Minute), Actor: "public", Action: "mail.send", Method: "POST", Path: "/api/v1/sendmail", StatusCode: 200, Result: ResultSuccess},
		{Timestamp: now, Actor: "admin@example.com", Action: "audit.view", Method: "GET", Path: "/_admin/api/v1/audit-events", StatusCode: 200, Result: ResultSuccess},
	}
	for _, event := range events {
		if err := store.Record(context.Background(), event); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}

	page, err := store.List(context.Background(), Filter{Result: ResultSuccess, Limit: 1, Offset: 1})
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
	for _, event := range []Event{
		{Timestamp: base.Add(5 * time.Minute), Action: "validation.request", StatusCode: 200, Result: ResultSuccess},
		{Timestamp: base.Add(10 * time.Minute), Action: "validation.request", StatusCode: 400, Result: ResultFailure},
		{Timestamp: base.Add(20 * time.Minute), Action: "mail.send", StatusCode: 200, Result: ResultSuccess},
		{Timestamp: base.Add(65 * time.Minute), Action: "mail.send", StatusCode: 500, Result: ResultFailure},
	} {
		if err := store.Record(context.Background(), event); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}

	metrics, err := store.Metrics(context.Background(), MetricsFilter{
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

func TestStoreResetLeavesMarker(t *testing.T) {
	store := newTestStore(t)
	if err := store.Record(context.Background(), Event{Action: "mail.send", Result: ResultSuccess}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if err := store.Reset(context.Background(), Event{
		Actor:   "admin@example.com",
		Action:  "audit.reset",
		Result:  ResultSuccess,
		Message: "Admin reset audit log",
	}); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	page, err := store.List(context.Background(), Filter{Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Action != "audit.reset" {
		t.Fatalf("page after reset = %#v", page)
	}
}

func TestSafeMetadataDropsSensitiveValues(t *testing.T) {
	metadata := SafeMetadata(map[string]any{
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

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
