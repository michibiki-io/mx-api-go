package audit_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/michibiki-io/mx-api-go/internal/audit"
	"go.uber.org/zap"
)

type fakeRepo struct {
	mu        sync.Mutex
	batches   [][]audit.AuditLog
	appendErr []error
	blockCh   chan struct{}
}

func (r *fakeRepo) AppendBatch(_ context.Context, logs []audit.AuditLog) error {
	if r.blockCh != nil {
		<-r.blockCh
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.appendErr) > 0 {
		err := r.appendErr[0]
		r.appendErr = r.appendErr[1:]
		if err != nil {
			return err
		}
	}
	copied := make([]audit.AuditLog, len(logs))
	copy(copied, logs)
	r.batches = append(r.batches, copied)
	return nil
}

func (r *fakeRepo) Get(context.Context, int64) (audit.AuditLog, bool, error) {
	return audit.AuditLog{}, false, nil
}

func (r *fakeRepo) List(context.Context, audit.AuditListQuery) (audit.AuditListResult, error) {
	return audit.AuditListResult{}, nil
}

func (r *fakeRepo) Stats(context.Context, audit.AuditStatsQuery) (audit.AuditStatsResult, error) {
	return audit.AuditStatsResult{}, nil
}

func (r *fakeRepo) TimeSeries(context.Context, audit.AuditStatsQuery) ([]audit.AuditTimeBucket, error) {
	return nil, nil
}

func (r *fakeRepo) StatusBreakdown(context.Context, audit.AuditStatsQuery) ([]audit.AuditStatusCount, error) {
	return nil, nil
}

func (r *fakeRepo) TopRoutes(context.Context, audit.AuditStatsQuery) ([]audit.AuditRouteCount, error) {
	return nil, nil
}

func (r *fakeRepo) Reset(context.Context, audit.AuditLog) error { return nil }
func (r *fakeRepo) DeleteBefore(context.Context, time.Time) error {
	return nil
}

func (r *fakeRepo) batchCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.batches)
}

func (r *fakeRepo) totalLogs() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := 0
	for _, batch := range r.batches {
		total += len(batch)
	}
	return total
}

func TestAsyncAuditRecorderFlushesByInterval(t *testing.T) {
	repo := &fakeRepo{}
	recorder := audit.NewAsyncAuditRecorder(repo, audit.AsyncConfig{
		Enabled:       true,
		BatchSize:     10,
		ChannelSize:   16,
		FlushInterval: 20 * time.Millisecond,
	}, zap.NewNop())
	t.Cleanup(func() {
		_ = recorder.Close(context.Background())
	})

	if err := recorder.Record(context.Background(), audit.Event{Action: "mail.send"}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	waitForCondition(t, 500*time.Millisecond, func() bool {
		return repo.totalLogs() == 1
	})
}

func TestAsyncAuditRecorderFlushesByBatchSize(t *testing.T) {
	repo := &fakeRepo{}
	recorder := audit.NewAsyncAuditRecorder(repo, audit.AsyncConfig{
		Enabled:       true,
		BatchSize:     2,
		ChannelSize:   16,
		FlushInterval: time.Hour,
	}, zap.NewNop())
	t.Cleanup(func() {
		_ = recorder.Close(context.Background())
	})

	for i := 0; i < 2; i++ {
		if err := recorder.Record(context.Background(), audit.Event{Action: "mail.send"}); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}
	waitForCondition(t, 500*time.Millisecond, func() bool {
		return repo.batchCount() == 1
	})
}

func TestAsyncAuditRecorderGracefulShutdownFlushes(t *testing.T) {
	repo := &fakeRepo{}
	recorder := audit.NewAsyncAuditRecorder(repo, audit.AsyncConfig{
		Enabled:       true,
		BatchSize:     10,
		ChannelSize:   16,
		FlushInterval: time.Hour,
	}, zap.NewNop())

	if err := recorder.Record(context.Background(), audit.Event{Action: "mail.send"}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if repo.totalLogs() != 1 {
		t.Fatalf("total logs after close = %d, want 1", repo.totalLogs())
	}
}

func TestAsyncAuditRecorderReturnsQueueFullWhenBlocking(t *testing.T) {
	blockCh := make(chan struct{})
	repo := &fakeRepo{blockCh: blockCh}
	recorder := audit.NewAsyncAuditRecorder(repo, audit.AsyncConfig{
		Enabled:        true,
		BatchSize:      1,
		ChannelSize:    1,
		FlushInterval:  time.Hour,
		EnqueueTimeout: 20 * time.Millisecond,
	}, zap.NewNop())
	t.Cleanup(func() {
		close(blockCh)
		_ = recorder.Close(context.Background())
	})

	if err := recorder.Record(context.Background(), audit.Event{Action: "mail.send"}); err != nil {
		t.Fatalf("first Record() error = %v", err)
	}
	if err := recorder.Record(context.Background(), audit.Event{Action: "mail.send"}); err != nil {
		t.Fatalf("second Record() error = %v", err)
	}
	err := recorder.Record(context.Background(), audit.Event{Action: "mail.send"})
	if !errors.Is(err, audit.ErrQueueFull) {
		t.Fatalf("Record() error = %v, want ErrQueueFull", err)
	}
}

func TestAsyncAuditRecorderRetriesBatchInsert(t *testing.T) {
	repo := &fakeRepo{appendErr: []error{errors.New("boom"), nil}}
	recorder := audit.NewAsyncAuditRecorder(repo, audit.AsyncConfig{
		Enabled:             true,
		BatchSize:           1,
		ChannelSize:         4,
		FlushInterval:       time.Hour,
		RetryMaxAttempts:    2,
		RetryInitialBackoff: 10 * time.Millisecond,
		RetryMaxBackoff:     10 * time.Millisecond,
	}, zap.NewNop())
	t.Cleanup(func() {
		_ = recorder.Close(context.Background())
	})

	if err := recorder.Record(context.Background(), audit.Event{Action: "mail.send"}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	waitForCondition(t, 500*time.Millisecond, func() bool {
		return repo.totalLogs() == 1
	})
}

func TestCursorEncodeDecodeValidation(t *testing.T) {
	cursor, err := audit.EncodeCursor(audit.Cursor{
		CreatedAt: time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
		ID:        42,
	})
	if err != nil {
		t.Fatalf("EncodeCursor() error = %v", err)
	}
	decoded, err := audit.DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("DecodeCursor() error = %v", err)
	}
	if decoded.ID != 42 || !decoded.CreatedAt.Equal(time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("decoded cursor = %#v", decoded)
	}
	if _, err := audit.DecodeCursor("not-a-cursor"); !errors.Is(err, audit.ErrInvalidCursor) {
		t.Fatalf("DecodeCursor() invalid error = %v", err)
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !fn() {
		if time.Now().After(deadline) {
			t.Fatal("condition was not met before timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
