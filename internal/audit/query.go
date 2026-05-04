package audit

import (
	"strings"
	"time"
)

func normalizeFilter(filter Filter) Filter {
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	if filter.Limit > 500 {
		filter.Limit = 500
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	filter.Method = strings.ToUpper(strings.TrimSpace(filter.Method))
	filter.Result = strings.ToLower(strings.TrimSpace(filter.Result))
	filter.StatusClass = strings.ToLower(strings.TrimSpace(filter.StatusClass))
	filter.Actor = strings.TrimSpace(filter.Actor)
	filter.Action = strings.TrimSpace(filter.Action)
	filter.Endpoint = strings.TrimSpace(filter.Endpoint)
	filter.Path = strings.TrimSpace(filter.Path)
	filter.RequestID = strings.TrimSpace(filter.RequestID)
	filter.Cursor = strings.TrimSpace(filter.Cursor)
	return filter
}

func normalizeMetricsFilter(filter MetricsFilter) MetricsFilter {
	now := time.Now().UTC()
	if filter.To.IsZero() {
		filter.To = now
	}
	if filter.From.IsZero() {
		filter.From = filter.To.Add(-24 * time.Hour)
	}
	filter.From = filter.From.UTC()
	filter.To = filter.To.UTC()
	filter.Method = strings.ToUpper(strings.TrimSpace(filter.Method))
	filter.Result = strings.ToLower(strings.TrimSpace(filter.Result))
	filter.StatusClass = strings.ToLower(strings.TrimSpace(filter.StatusClass))
	filter.Endpoint = strings.TrimSpace(filter.Endpoint)
	filter.RequestID = strings.TrimSpace(filter.RequestID)
	if filter.Bucket <= 0 {
		filter.Bucket = chooseBucket(filter.To.Sub(filter.From))
	}
	return filter
}

func normalizeStatsQuery(query AuditStatsQuery) AuditStatsQuery {
	query.From = query.From.UTC()
	query.To = query.To.UTC()
	query.Actor = strings.TrimSpace(query.Actor)
	query.Action = strings.TrimSpace(query.Action)
	query.Endpoint = strings.TrimSpace(query.Endpoint)
	query.Method = strings.ToUpper(strings.TrimSpace(query.Method))
	query.Result = strings.ToLower(strings.TrimSpace(query.Result))
	query.StatusClass = strings.ToLower(strings.TrimSpace(query.StatusClass))
	query.RequestID = strings.TrimSpace(query.RequestID)
	if query.Bucket == "" {
		query.Bucket = bucketSizeFromDuration(chooseBucket(query.To.Sub(query.From)))
	}
	if query.Limit <= 0 {
		query.Limit = 10
	}
	return query
}

func chooseBucket(window time.Duration) time.Duration {
	switch {
	case window <= 6*time.Hour:
		return time.Minute
	case window <= 7*24*time.Hour:
		return time.Hour
	default:
		return 24 * time.Hour
	}
}

func bucketSizeFromDuration(value time.Duration) AuditTimeBucketSize {
	switch {
	case value >= 24*time.Hour:
		return AuditTimeBucketDay
	case value >= time.Hour:
		return AuditTimeBucketHour
	default:
		return AuditTimeBucketMinute
	}
}

func bucketDuration(size AuditTimeBucketSize) time.Duration {
	switch size {
	case AuditTimeBucketDay:
		return 24 * time.Hour
	case AuditTimeBucketHour:
		return time.Hour
	default:
		return time.Minute
	}
}

func bucketStart(t time.Time, bucket AuditTimeBucketSize) time.Time {
	t = t.UTC()
	switch bucket {
	case AuditTimeBucketDay:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	case AuditTimeBucketHour:
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC)
	default:
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC)
	}
}
