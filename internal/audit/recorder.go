package audit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

type AsyncConfig struct {
	Enabled              bool
	ChannelSize          int
	BatchSize            int
	FlushInterval        time.Duration
	ShutdownFlushTimeout time.Duration
	DropOnFull           bool
	RetryMaxAttempts     int
	RetryInitialBackoff  time.Duration
	RetryMaxBackoff      time.Duration
	EnqueueTimeout       time.Duration
}

type SyncAuditRecorder struct {
	repo Repository
}

func NewSyncAuditRecorder(repo Repository) *SyncAuditRecorder {
	return &SyncAuditRecorder{repo: repo}
}

func (r *SyncAuditRecorder) Record(ctx context.Context, event Event) error {
	if r == nil || r.repo == nil {
		return nil
	}
	prepared := cloneAndPrepareEvent(event)
	return r.repo.AppendBatch(ctx, []AuditLog{prepared})
}

func (r *SyncAuditRecorder) Flush(context.Context) error { return nil }
func (r *SyncAuditRecorder) Close(context.Context) error { return nil }

type AsyncAuditRecorder struct {
	repo   Repository
	cfg    AsyncConfig
	logger *zap.Logger

	mu           sync.RWMutex
	closed       bool
	queue        chan Event
	flushReqs    chan chan error
	done         chan struct{}
	doneErr      error
	droppedTotal atomic.Uint64
}

func NewAsyncAuditRecorder(repo Repository, cfg AsyncConfig, logger *zap.Logger) *AsyncAuditRecorder {
	cfg = normalizeAsyncConfig(cfg)
	if logger == nil {
		logger = zap.NewNop()
	}
	r := &AsyncAuditRecorder{
		repo:      repo,
		cfg:       cfg,
		logger:    logger,
		queue:     make(chan Event, cfg.ChannelSize),
		flushReqs: make(chan chan error),
		done:      make(chan struct{}),
	}
	go r.run()
	return r
}

func (r *AsyncAuditRecorder) Record(ctx context.Context, event Event) error {
	if r == nil || r.repo == nil {
		return nil
	}
	prepared := cloneAndPrepareEvent(event)

	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return ErrRecorderClosed
	}

	if r.cfg.DropOnFull {
		select {
		case r.queue <- prepared:
			return nil
		default:
			dropped := r.droppedTotal.Add(1)
			r.logger.Warn("dropped audit logs because queue is full",
				zap.Uint64("audit_dropped_total", dropped),
				zap.Int("audit_channel_depth", len(r.queue)))
			return nil
		}
	}

	timeout := r.cfg.EnqueueTimeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case r.queue <- prepared:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrQueueFull
	}
}

func (r *AsyncAuditRecorder) Flush(ctx context.Context) error {
	if r == nil || r.repo == nil {
		return nil
	}
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		select {
		case <-r.done:
			return r.doneErr
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ack := make(chan error, 1)
	r.mu.RUnlock()

	select {
	case r.flushReqs <- ack:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-ack:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *AsyncAuditRecorder) Close(ctx context.Context) error {
	if r == nil || r.repo == nil {
		return nil
	}
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.queue)
	}
	r.mu.Unlock()

	select {
	case <-r.done:
		return r.doneErr
	case <-ctx.Done():
		r.logger.Error("audit shutdown flush timeout", zap.Int("audit_channel_depth", len(r.queue)))
		return ctx.Err()
	}
}

func (r *AsyncAuditRecorder) run() {
	defer close(r.done)

	ticker := time.NewTicker(r.cfg.FlushInterval)
	defer ticker.Stop()

	buffer := make([]AuditLog, 0, r.cfg.BatchSize)
	flush := func(reason string) error {
		if len(buffer) == 0 {
			return nil
		}
		start := time.Now()
		if err := r.flushWithRetry(context.Background(), buffer); err != nil {
			r.logger.Error("audit flush failed",
				zap.String("reason", reason),
				zap.Int("audit_flush_batch_size", len(buffer)),
				zap.Duration("audit_flush_duration_ms", time.Since(start)),
				zap.Error(err))
			return err
		}
		r.logger.Debug("audit flush succeeded",
			zap.String("reason", reason),
			zap.Int("audit_flush_total", 1),
			zap.Int("audit_flush_batch_size", len(buffer)),
			zap.Duration("audit_flush_duration_ms", time.Since(start)),
			zap.Int("audit_channel_depth", len(r.queue)))
		buffer = buffer[:0]
		return nil
	}

	for {
		select {
		case event, ok := <-r.queue:
			if !ok {
				for pending := range r.queue {
					buffer = append(buffer, pending)
				}
				r.doneErr = flush("shutdown")
				return
			}
			buffer = append(buffer, event)
			if len(buffer) >= r.cfg.BatchSize {
				if err := flush("batch"); err != nil {
					r.doneErr = err
				}
			}
		case ack := <-r.flushReqs:
			err := flush("manual")
			if err != nil {
				r.doneErr = err
			}
			ack <- err
		case <-ticker.C:
			if err := flush("interval"); err != nil {
				r.doneErr = err
			}
		}
	}
}

func (r *AsyncAuditRecorder) flushWithRetry(ctx context.Context, batch []AuditLog) error {
	if len(batch) == 0 {
		return nil
	}
	payload := make([]AuditLog, len(batch))
	copy(payload, batch)

	backoff := r.cfg.RetryInitialBackoff
	var err error
	for attempt := 1; attempt <= r.cfg.RetryMaxAttempts; attempt++ {
		err = r.repo.AppendBatch(ctx, payload)
		if err == nil {
			return nil
		}
		if attempt == r.cfg.RetryMaxAttempts {
			break
		}
		r.logger.Warn("audit flush retry",
			zap.Int("audit_retry_total", attempt),
			zap.Int("audit_flush_batch_size", len(payload)),
			zap.Duration("backoff", backoff),
			zap.Error(err))
		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return errors.Join(err, ctx.Err())
		}
		if backoff < r.cfg.RetryMaxBackoff {
			backoff *= 2
			if backoff > r.cfg.RetryMaxBackoff {
				backoff = r.cfg.RetryMaxBackoff
			}
		}
	}
	r.logger.Error("audit retry exhausted",
		zap.Int("audit_retry_total", r.cfg.RetryMaxAttempts),
		zap.Int("audit_flush_error_total", 1),
		zap.Int("audit_flush_batch_size", len(payload)),
		zap.Error(err))
	return err
}

func normalizeAsyncConfig(cfg AsyncConfig) AsyncConfig {
	if cfg.ChannelSize <= 0 {
		cfg.ChannelSize = 10000
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 500
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 100 * time.Millisecond
	}
	if cfg.ShutdownFlushTimeout <= 0 {
		cfg.ShutdownFlushTimeout = 5 * time.Second
	}
	if cfg.RetryMaxAttempts <= 0 {
		cfg.RetryMaxAttempts = 3
	}
	if cfg.RetryInitialBackoff <= 0 {
		cfg.RetryInitialBackoff = 100 * time.Millisecond
	}
	if cfg.RetryMaxBackoff <= 0 {
		cfg.RetryMaxBackoff = 2 * time.Second
	}
	if cfg.EnqueueTimeout <= 0 {
		cfg.EnqueueTimeout = time.Second
	}
	return cfg
}
