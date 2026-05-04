package bunmodel

import (
	"time"

	"github.com/uptrace/bun"
)

type AuditLogRow struct {
	bun.BaseModel `bun:"table:audit_logs"`
	ID            int64     `bun:"id,pk,autoincrement"`
	CreatedAt     time.Time `bun:"created_at,notnull"`
	Actor         string    `bun:"actor"`
	ActorSource   string    `bun:"actor_source"`
	Action        string    `bun:"action,notnull"`
	Method        string    `bun:"method"`
	Route         string    `bun:"route"`
	Path          string    `bun:"path"`
	StatusCode    int       `bun:"status_code"`
	Result        string    `bun:"result,notnull"`
	RemoteIP      string    `bun:"remote_ip"`
	UserAgent     string    `bun:"user_agent"`
	RequestID     string    `bun:"request_id"`
	DurationMS    int64     `bun:"duration_ms"`
	ErrorCode     string    `bun:"error_code"`
	ErrorMessage  string    `bun:"error_message"`
	MetadataJSON  string    `bun:"metadata_json"`
}
