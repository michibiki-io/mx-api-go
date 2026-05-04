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

func BenchmarkAuditRecorder_Record_Async(b *testing.B) {
	repo := &fakeRepo{}
	recorder := audit.NewAsyncAuditRecorder(repo, audit.AsyncConfig{
		Enabled:       true,
		BatchSize:     500,
		ChannelSize:   10000,
		FlushInterval: time.Hour,
	}, zap.NewNop())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := recorder.Record(context.Background(), audit.Event{Action: "mail.send"}); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if err := recorder.Close(context.Background()); err != nil {
		b.Fatal(err)
	}
}

func BenchmarkAuditRepository_AppendBatch_SQLite(b *testing.B) {
	store := benchmarkStore(b, false)
	events := make([]audit.Event, 500)
	for i := range events {
		events[i] = audit.Event{
			Timestamp:  time.Now().UTC(),
			Action:     "mail.send",
			Method:     "POST",
			Path:       "/api/v1/sendmail",
			Endpoint:   "/api/v1/sendmail",
			StatusCode: 200,
			Result:     audit.ResultSuccess,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, event := range events {
			if err := store.Record(context.Background(), event); err != nil {
				b.Fatal(err)
			}
		}
		if err := store.Flush(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAuditRepository_ListKeyset_SQLite(b *testing.B) {
	store := benchmarkStore(b, false)
	base := time.Now().UTC()
	for i := 0; i < 1000; i++ {
		if err := store.Record(context.Background(), audit.Event{
			Timestamp:  base.Add(time.Duration(i) * time.Second),
			Action:     "mail.send",
			Method:     "POST",
			Path:       "/api/v1/sendmail",
			Endpoint:   "/api/v1/sendmail",
			StatusCode: 200,
			Result:     audit.ResultSuccess,
		}); err != nil {
			b.Fatal(err)
		}
	}
	if err := store.Flush(context.Background()); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		first, err := store.List(context.Background(), audit.Filter{Limit: 100})
		if err != nil {
			b.Fatal(err)
		}
		if first.NextCursor == "" {
			b.Fatal("expected next cursor")
		}
		if _, err := store.List(context.Background(), audit.Filter{Limit: 100, Cursor: first.NextCursor}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAuditRepository_Stats_SQLite(b *testing.B) {
	store := benchmarkStore(b, false)
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 1000; i++ {
		result := audit.ResultSuccess
		status := 200
		if i%5 == 0 {
			result = audit.ResultFailure
			status = 500
		}
		if err := store.Record(context.Background(), audit.Event{
			Timestamp:  base.Add(time.Duration(i) * time.Second),
			Action:     "mail.send",
			Method:     "POST",
			Path:       "/api/v1/sendmail",
			Endpoint:   "/api/v1/sendmail",
			StatusCode: status,
			Result:     result,
			DurationMS: int64(i % 200),
		}); err != nil {
			b.Fatal(err)
		}
	}
	if err := store.Flush(context.Background()); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.Summary(context.Background(), audit.Filter{
			From:  base,
			To:    base.Add(2 * time.Hour),
			Limit: 100,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkStore(tb testing.TB, async bool) *audit.Store {
	tb.Helper()
	cfg := config.Default()
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = "file:" + tb.TempDir() + "/bench.db?cache=shared&mode=rwc&_journal_mode=WAL&_busy_timeout=5000"
	cfg.Audit.Async.Enabled = async
	store, err := database.OpenAuditStore(context.Background(), cfg, zap.NewNop())
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		_ = store.Close()
	})
	return store
}
