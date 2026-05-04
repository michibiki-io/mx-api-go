package dbdialect

import (
	"fmt"
	"strings"
	"time"

	"github.com/michibiki-io/mx-api-go/internal/audit"
)

type Helper interface {
	Name() string
	TimeBucketExpr(column string, bucket audit.AuditTimeBucketSize) string
	NowExpr() string
	Placeholder(n int) string
	SupportsReturning() bool
	SessionInitStatements() []string
}

type sqliteHelper struct{}
type postgresHelper struct{}
type mysqlHelper struct{}

func New(driver string) (Helper, error) {
	switch NormalizeDriver(driver) {
	case "sqlite":
		return sqliteHelper{}, nil
	case "postgres":
		return postgresHelper{}, nil
	case "mysql", "mariadb":
		return mysqlHelper{}, nil
	default:
		return nil, fmt.Errorf("unsupported db driver %q", driver)
	}
}

func NormalizeDriver(driver string) string {
	driver = strings.ToLower(strings.TrimSpace(driver))
	switch driver {
	case "postgresql":
		return "postgres"
	default:
		return driver
	}
}

func (sqliteHelper) Name() string                    { return "sqlite" }
func (sqliteHelper) NowExpr() string                 { return "CURRENT_TIMESTAMP" }
func (sqliteHelper) Placeholder(_ int) string        { return "?" }
func (sqliteHelper) SupportsReturning() bool         { return false }
func (sqliteHelper) SessionInitStatements() []string { return nil }
func (sqliteHelper) TimeBucketExpr(column string, bucket audit.AuditTimeBucketSize) string {
	switch bucket {
	case audit.AuditTimeBucketDay:
		return fmt.Sprintf("strftime('%%Y-%%m-%%dT00:00:00Z', %s)", column)
	case audit.AuditTimeBucketHour:
		return fmt.Sprintf("strftime('%%Y-%%m-%%dT%%H:00:00Z', %s)", column)
	default:
		return fmt.Sprintf("strftime('%%Y-%%m-%%dT%%H:%%M:00Z', %s)", column)
	}
}

func (postgresHelper) Name() string                    { return "postgres" }
func (postgresHelper) NowExpr() string                 { return "CURRENT_TIMESTAMP" }
func (postgresHelper) Placeholder(n int) string        { return fmt.Sprintf("$%d", n) }
func (postgresHelper) SupportsReturning() bool         { return true }
func (postgresHelper) SessionInitStatements() []string { return []string{"SET TIME ZONE 'UTC'"} }
func (postgresHelper) TimeBucketExpr(column string, bucket audit.AuditTimeBucketSize) string {
	granularity := "minute"
	switch bucket {
	case audit.AuditTimeBucketDay:
		granularity = "day"
	case audit.AuditTimeBucketHour:
		granularity = "hour"
	}
	return fmt.Sprintf("to_char(date_trunc('%s', %s AT TIME ZONE 'UTC'), 'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"')", granularity, column)
}

func (mysqlHelper) Name() string                    { return "mysql" }
func (mysqlHelper) NowExpr() string                 { return "UTC_TIMESTAMP(6)" }
func (mysqlHelper) Placeholder(_ int) string        { return "?" }
func (mysqlHelper) SupportsReturning() bool         { return false }
func (mysqlHelper) SessionInitStatements() []string { return []string{"SET time_zone = '+00:00'"} }
func (mysqlHelper) TimeBucketExpr(column string, bucket audit.AuditTimeBucketSize) string {
	switch bucket {
	case audit.AuditTimeBucketDay:
		return fmt.Sprintf("DATE_FORMAT(%s, '%%Y-%%m-%%dT00:00:00Z')", column)
	case audit.AuditTimeBucketHour:
		return fmt.Sprintf("DATE_FORMAT(%s, '%%Y-%%m-%%dT%%H:00:00Z')", column)
	default:
		return fmt.Sprintf("DATE_FORMAT(%s, '%%Y-%%m-%%dT%%H:%%i:00Z')", column)
	}
}

func ParseBucketTimestamp(value string) (time.Time, error) {
	return time.Parse(time.RFC3339, value)
}
