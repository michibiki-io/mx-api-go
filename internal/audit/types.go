package audit

import (
	"context"
	"errors"
	"time"
)

const (
	ResultSuccess = "success"
	ResultFailure = "failure"
	ResultDenied  = "denied"

	publicAPIActionFilter   = "__public_api__"
	businessAPIActionFilter = "__business_api__"
)

var (
	ErrInvalidCursor  = errors.New("invalid audit cursor")
	ErrRecorderClosed = errors.New("audit recorder is closed")
	ErrQueueFull      = errors.New("audit queue is full")
)

var knownActions = []string{
	"public.api.access",
	"validation.request",
	"mail.send",
	"admin.dashboard.view",
	"admin.access.denied",
	"admin.metrics.view",
	"mail.server.check",
	"audit.view",
	"audit.detail.view",
	"audit.reset",
	"system.startup",
}

func KnownActions() []string {
	return append([]string{}, knownActions...)
}

type Recorder interface {
	Record(context.Context, Event) error
}

type AuditRecorder interface {
	Recorder
	Flush(context.Context) error
	Close(context.Context) error
}

type NoopRecorder struct{}

func (NoopRecorder) Record(context.Context, Event) error { return nil }
func (NoopRecorder) Flush(context.Context) error         { return nil }
func (NoopRecorder) Close(context.Context) error         { return nil }

type Event struct {
	ID          int64          `json:"id"`
	Timestamp   time.Time      `json:"timestamp"`
	Actor       string         `json:"actor"`
	ActorSource string         `json:"actorSource"`
	Action      string         `json:"action"`
	Method      string         `json:"method"`
	Path        string         `json:"path"`
	Endpoint    string         `json:"endpoint"`
	StatusCode  int            `json:"statusCode"`
	Result      string         `json:"result"`
	RemoteAddr  string         `json:"remoteAddr"`
	UserAgent   string         `json:"userAgent"`
	RequestID   string         `json:"requestId"`
	DurationMS  int64          `json:"durationMs"`
	ErrorCode   string         `json:"errorCode"`
	Message     string         `json:"message"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type AuditLog = Event

type Filter struct {
	From        time.Time
	To          time.Time
	Actor       string
	Action      string
	Endpoint    string
	Path        string
	Method      string
	Result      string
	StatusCode  int
	StatusClass string
	RequestID   string
	Limit       int
	Cursor      string
	Offset      int
	SkipTotal   bool
}

type AuditListQuery = Filter

type Page struct {
	Items      []Event `json:"items"`
	Total      int     `json:"total"`
	NextCursor string  `json:"nextCursor,omitempty"`
	HasNext    bool    `json:"hasNext"`
}

type AuditListResult = Page

type Summary struct {
	Total               int     `json:"total"`
	Successful          int     `json:"successful"`
	Failed              int     `json:"failed"`
	ValidationSuccesses int     `json:"validationSuccesses"`
	ValidationFailures  int     `json:"validationFailures"`
	MailSendSuccesses   int     `json:"mailSendSuccesses"`
	MailSendFailures    int     `json:"mailSendFailures"`
	Count4xx            int     `json:"count4xx"`
	Count5xx            int     `json:"count5xx"`
	AverageDurationMS   float64 `json:"averageDurationMs"`
	MaxDurationMS       int64   `json:"maxDurationMs"`
}

type AuditStatsResult = Summary

type AuditTimeBucketSize string

const (
	AuditTimeBucketMinute AuditTimeBucketSize = "minute"
	AuditTimeBucketHour   AuditTimeBucketSize = "hour"
	AuditTimeBucketDay    AuditTimeBucketSize = "day"
)

type AuditStatsQuery struct {
	From        time.Time
	To          time.Time
	Actor       string
	Action      string
	Endpoint    string
	Method      string
	Result      string
	StatusCode  int
	StatusClass string
	RequestID   string
	Bucket      AuditTimeBucketSize
	Limit       int
}

type MetricsFilter struct {
	From        time.Time
	To          time.Time
	Bucket      time.Duration
	Endpoint    string
	Method      string
	Result      string
	StatusCode  int
	StatusClass string
	RequestID   string
}

type MetricPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Count     int       `json:"count"`
}

type Metrics struct {
	From    time.Time     `json:"from"`
	To      time.Time     `json:"to"`
	Bucket  string        `json:"bucket"`
	Points  []MetricPoint `json:"points"`
	Summary Summary       `json:"summary"`
}

type AuditTimeBucket struct {
	Timestamp time.Time `json:"timestamp"`
	Count     int       `json:"count"`
}

type AuditStatusCount struct {
	StatusCode int `json:"statusCode"`
	Count      int `json:"count"`
}

type AuditRouteCount struct {
	Endpoint string `json:"endpoint"`
	Count    int    `json:"count"`
}

type Cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int64     `json:"id"`
}

type Repository interface {
	AppendBatch(ctx context.Context, logs []AuditLog) error
	Get(ctx context.Context, id int64) (AuditLog, bool, error)
	List(ctx context.Context, q AuditListQuery) (AuditListResult, error)
	Stats(ctx context.Context, q AuditStatsQuery) (AuditStatsResult, error)
	TimeSeries(ctx context.Context, q AuditStatsQuery) ([]AuditTimeBucket, error)
	StatusBreakdown(ctx context.Context, q AuditStatsQuery) ([]AuditStatusCount, error)
	TopRoutes(ctx context.Context, q AuditStatsQuery) ([]AuditRouteCount, error)
	Reset(ctx context.Context, marker AuditLog) error
	DeleteBefore(ctx context.Context, cutoff time.Time) error
}
