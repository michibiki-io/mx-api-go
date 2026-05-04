package audit

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type Store struct {
	repo          Repository
	recorder      AuditRecorder
	closeDB       func() error
	retentionDays int
}

func NewStore(repo Repository, recorder AuditRecorder, closeDB func() error, retentionDays int) *Store {
	if recorder == nil {
		recorder = NoopRecorder{}
	}
	return &Store{
		repo:          repo,
		recorder:      recorder,
		closeDB:       closeDB,
		retentionDays: retentionDays,
	}
}

func (s *Store) Record(ctx context.Context, event Event) error {
	if s == nil || s.recorder == nil {
		return nil
	}
	return s.recorder.Record(ctx, event)
}

func (s *Store) Flush(ctx context.Context) error {
	if s == nil || s.recorder == nil {
		return nil
	}
	return s.recorder.Flush(ctx)
}

func (s *Store) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	var err error
	if s.recorder != nil {
		err = s.recorder.Close(ctx)
	}
	if s.closeDB != nil {
		err = errors.Join(err, s.closeDB())
	}
	return err
}

func (s *Store) Close() error {
	timeout := 5 * time.Second
	if s != nil && s.recorder != nil {
		if async, ok := s.recorder.(*AsyncAuditRecorder); ok && async.cfg.ShutdownFlushTimeout > 0 {
			timeout = async.cfg.ShutdownFlushTimeout
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.Shutdown(ctx)
}

func (s *Store) Get(ctx context.Context, id int64) (Event, bool, error) {
	if s == nil || s.repo == nil {
		return Event{}, false, nil
	}
	return s.repo.Get(ctx, id)
}

func (s *Store) List(ctx context.Context, filter Filter) (Page, error) {
	if s == nil || s.repo == nil {
		return Page{}, nil
	}
	return s.repo.List(ctx, normalizeFilter(filter))
}

func (s *Store) Summary(ctx context.Context, filter Filter) (Summary, error) {
	if s == nil || s.repo == nil {
		return Summary{}, nil
	}
	return s.repo.Stats(ctx, statsQueryFromFilter(filter, ""))
}

func (s *Store) Metrics(ctx context.Context, filter MetricsFilter) (Metrics, error) {
	if s == nil || s.repo == nil {
		return Metrics{}, nil
	}
	filter = normalizeMetricsFilter(filter)
	query := AuditStatsQuery{
		From:        filter.From,
		To:          filter.To,
		Action:      businessAPIActionFilter,
		Endpoint:    filter.Endpoint,
		Method:      filter.Method,
		Result:      filter.Result,
		StatusCode:  filter.StatusCode,
		StatusClass: filter.StatusClass,
		RequestID:   filter.RequestID,
		Bucket:      bucketSizeFromDuration(filter.Bucket),
	}
	points, err := s.repo.TimeSeries(ctx, normalizeStatsQuery(query))
	if err != nil {
		return Metrics{}, err
	}
	summary, err := s.repo.Stats(ctx, query)
	if err != nil {
		return Metrics{}, err
	}
	filled := fillMetricPoints(filter.From, filter.To, query.Bucket, points)
	return Metrics{
		From:    filter.From,
		To:      filter.To,
		Bucket:  filter.Bucket.String(),
		Points:  filled,
		Summary: summary,
	}, nil
}

func (s *Store) Reset(ctx context.Context, marker Event) error {
	if s == nil || s.repo == nil {
		return nil
	}
	return s.repo.Reset(ctx, cloneAndPrepareEvent(marker))
}

func (s *Store) PruneRetention(ctx context.Context, days int) error {
	if s == nil || s.repo == nil {
		return nil
	}
	if days <= 0 {
		days = s.retentionDays
	}
	if days <= 0 {
		return nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	return s.repo.DeleteBefore(ctx, cutoff)
}

func statsQueryFromFilter(filter Filter, bucket AuditTimeBucketSize) AuditStatsQuery {
	filter = normalizeFilter(filter)
	return normalizeStatsQuery(AuditStatsQuery{
		From:        filter.From,
		To:          filter.To,
		Actor:       filter.Actor,
		Action:      filter.Action,
		Endpoint:    filter.Endpoint,
		Method:      filter.Method,
		Result:      filter.Result,
		StatusCode:  filter.StatusCode,
		StatusClass: filter.StatusClass,
		RequestID:   filter.RequestID,
		Bucket:      bucket,
	})
}

func fillMetricPoints(from, to time.Time, bucket AuditTimeBucketSize, sparse []AuditTimeBucket) []MetricPoint {
	lookup := make(map[int64]int, len(sparse))
	for _, point := range sparse {
		lookup[point.Timestamp.Unix()] = point.Count
	}
	points := make([]MetricPoint, 0)
	for ts := bucketStart(from, bucket); !ts.After(to); ts = ts.Add(bucketDuration(bucket)) {
		points = append(points, MetricPoint{
			Timestamp: ts,
			Count:     lookup[ts.Unix()],
		})
	}
	return points
}

func cloneAndPrepareEvent(event Event) Event {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	} else {
		event.Timestamp = event.Timestamp.UTC()
	}
	if event.Actor == "" {
		event.Actor = "anonymous"
	}
	if event.Result == "" {
		event.Result = ResultSuccess
	}
	event.Metadata = cloneMetadata(SafeMetadata(event.Metadata))
	return event
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil
	}
	return cloned
}

func truncate(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}
