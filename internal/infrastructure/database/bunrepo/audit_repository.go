package bunrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/michibiki-io/mx-api-go/internal/audit"
	"github.com/michibiki-io/mx-api-go/internal/infrastructure/database/bunmodel"
	"github.com/michibiki-io/mx-api-go/internal/infrastructure/database/dbdialect"
	"github.com/uptrace/bun"
)

type AuditRepository struct {
	db     *bun.DB
	helper dbdialect.Helper
}

func NewAuditRepository(db *bun.DB, helper dbdialect.Helper) *AuditRepository {
	return &AuditRepository{db: db, helper: helper}
}

func (r *AuditRepository) AppendBatch(ctx context.Context, logs []audit.AuditLog) error {
	if len(logs) == 0 {
		return nil
	}
	rows := make([]bunmodel.AuditLogRow, 0, len(logs))
	for _, item := range logs {
		rows = append(rows, toRow(item))
	}
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewInsert().Model(&rows).Exec(ctx)
		return err
	})
}

func (r *AuditRepository) Get(ctx context.Context, id int64) (audit.AuditLog, bool, error) {
	var row bunmodel.AuditLogRow
	err := r.db.NewSelect().
		Model(&row).
		Where("id = ?", id).
		Scan(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return audit.AuditLog{}, false, nil
		}
		return audit.AuditLog{}, false, err
	}
	item, ok, err := fromRow(row)
	if err != nil {
		return audit.AuditLog{}, false, err
	}
	return item, ok, nil
}

func (r *AuditRepository) List(ctx context.Context, q audit.AuditListQuery) (audit.AuditListResult, error) {
	q = normalizeListQuery(q)

	if q.Cursor != "" {
		if _, err := audit.DecodeCursor(q.Cursor); err != nil {
			return audit.AuditListResult{}, audit.ErrInvalidCursor
		}
	}

	total := 0
	if !q.SkipTotal {
		countQuery := r.db.NewSelect().TableExpr("audit_logs")
		applyListFilters(countQuery, q, false)
		var err error
		total, err = countQuery.Count(ctx)
		if err != nil {
			return audit.AuditListResult{}, err
		}
	}

	rows := make([]bunmodel.AuditLogRow, 0, q.Limit+1)
	selectQuery := r.db.NewSelect().
		Model(&rows)
	applyListFilters(selectQuery, q, true)
	selectQuery.OrderExpr("created_at DESC, id DESC").Limit(q.Limit + 1)
	if q.Cursor == "" && q.Offset > 0 {
		selectQuery.Offset(q.Offset)
	}
	if err := selectQuery.Scan(ctx); err != nil {
		return audit.AuditListResult{}, err
	}

	result := audit.AuditListResult{
		Items:   make([]audit.Event, 0, min(len(rows), q.Limit)),
		Total:   total,
		HasNext: len(rows) > q.Limit,
	}
	trimmed := rows
	if result.HasNext {
		trimmed = rows[:q.Limit]
	}
	for _, row := range trimmed {
		item, _, err := fromRow(row)
		if err != nil {
			return audit.AuditListResult{}, err
		}
		result.Items = append(result.Items, item)
	}
	if result.HasNext && len(result.Items) > 0 {
		cursor, err := audit.EncodeCursor(audit.Cursor{
			CreatedAt: result.Items[len(result.Items)-1].Timestamp,
			ID:        result.Items[len(result.Items)-1].ID,
		})
		if err != nil {
			return audit.AuditListResult{}, err
		}
		result.NextCursor = cursor
	}
	return result, nil
}

func (r *AuditRepository) Stats(ctx context.Context, q audit.AuditStatsQuery) (audit.AuditStatsResult, error) {
	q = normalizeStatsQuery(q)

	var row struct {
		Total               int     `bun:"total"`
		Successful          int     `bun:"successful"`
		Failed              int     `bun:"failed"`
		ValidationSuccesses int     `bun:"validation_successes"`
		ValidationFailures  int     `bun:"validation_failures"`
		MailSendSuccesses   int     `bun:"mail_send_successes"`
		MailSendFailures    int     `bun:"mail_send_failures"`
		Count4xx            int     `bun:"count_4xx"`
		Count5xx            int     `bun:"count_5xx"`
		AverageDurationMS   float64 `bun:"average_duration_ms"`
		MaxDurationMS       int64   `bun:"max_duration_ms"`
	}

	query := r.db.NewSelect().
		TableExpr("audit_logs").
		ColumnExpr("COUNT(*) AS total").
		ColumnExpr("COALESCE(SUM(CASE WHEN result = 'success' THEN 1 ELSE 0 END), 0) AS successful").
		ColumnExpr("COALESCE(SUM(CASE WHEN result <> 'success' THEN 1 ELSE 0 END), 0) AS failed").
		ColumnExpr("COALESCE(SUM(CASE WHEN action = 'validation.request' AND result = 'success' THEN 1 ELSE 0 END), 0) AS validation_successes").
		ColumnExpr("COALESCE(SUM(CASE WHEN action = 'validation.request' AND result <> 'success' THEN 1 ELSE 0 END), 0) AS validation_failures").
		ColumnExpr("COALESCE(SUM(CASE WHEN action = 'mail.send' AND result = 'success' THEN 1 ELSE 0 END), 0) AS mail_send_successes").
		ColumnExpr("COALESCE(SUM(CASE WHEN action = 'mail.send' AND result <> 'success' THEN 1 ELSE 0 END), 0) AS mail_send_failures").
		ColumnExpr("COALESCE(SUM(CASE WHEN status_code >= 400 AND status_code < 500 THEN 1 ELSE 0 END), 0) AS count_4xx").
		ColumnExpr("COALESCE(SUM(CASE WHEN status_code >= 500 THEN 1 ELSE 0 END), 0) AS count_5xx").
		ColumnExpr("COALESCE(AVG(duration_ms), 0.0) AS average_duration_ms").
		ColumnExpr("COALESCE(MAX(duration_ms), 0) AS max_duration_ms")
	applyStatsFilters(query, q)
	if err := query.Scan(ctx, &row); err != nil {
		return audit.AuditStatsResult{}, err
	}
	return audit.AuditStatsResult{
		Total:               row.Total,
		Successful:          row.Successful,
		Failed:              row.Failed,
		ValidationSuccesses: row.ValidationSuccesses,
		ValidationFailures:  row.ValidationFailures,
		MailSendSuccesses:   row.MailSendSuccesses,
		MailSendFailures:    row.MailSendFailures,
		Count4xx:            row.Count4xx,
		Count5xx:            row.Count5xx,
		AverageDurationMS:   row.AverageDurationMS,
		MaxDurationMS:       row.MaxDurationMS,
	}, nil
}

func (r *AuditRepository) TimeSeries(ctx context.Context, q audit.AuditStatsQuery) ([]audit.AuditTimeBucket, error) {
	q = normalizeStatsQuery(q)
	type bucketRow struct {
		Bucket string `bun:"bucket"`
		Count  int    `bun:"count"`
	}
	rows := make([]bucketRow, 0)
	query := r.db.NewSelect().
		TableExpr("audit_logs").
		ColumnExpr(r.helper.TimeBucketExpr("created_at", q.Bucket) + " AS bucket").
		ColumnExpr("COUNT(*) AS count")
	applyStatsFilters(query, q)
	query.GroupExpr("bucket").OrderExpr("bucket ASC")
	if err := query.Scan(ctx, &rows); err != nil {
		return nil, err
	}
	out := make([]audit.AuditTimeBucket, 0, len(rows))
	for _, row := range rows {
		ts, err := dbdialect.ParseBucketTimestamp(row.Bucket)
		if err != nil {
			return nil, err
		}
		out = append(out, audit.AuditTimeBucket{Timestamp: ts, Count: row.Count})
	}
	return out, nil
}

func (r *AuditRepository) StatusBreakdown(ctx context.Context, q audit.AuditStatsQuery) ([]audit.AuditStatusCount, error) {
	q = normalizeStatsQuery(q)
	rows := make([]audit.AuditStatusCount, 0)
	query := r.db.NewSelect().
		TableExpr("audit_logs").
		ColumnExpr("status_code AS status_code").
		ColumnExpr("COUNT(*) AS count")
	applyStatsFilters(query, q)
	query.GroupExpr("status_code").OrderExpr("status_code ASC")
	if err := query.Scan(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *AuditRepository) TopRoutes(ctx context.Context, q audit.AuditStatsQuery) ([]audit.AuditRouteCount, error) {
	q = normalizeStatsQuery(q)
	rows := make([]struct {
		Endpoint string `bun:"endpoint"`
		Count    int    `bun:"count"`
	}, 0)
	query := r.db.NewSelect().
		TableExpr("audit_logs").
		ColumnExpr("route AS endpoint").
		ColumnExpr("COUNT(*) AS count")
	applyStatsFilters(query, q)
	query.Where("route <> ''").GroupExpr("route").OrderExpr("count DESC, route ASC").Limit(q.Limit)
	if err := query.Scan(ctx, &rows); err != nil {
		return nil, err
	}
	out := make([]audit.AuditRouteCount, 0, len(rows))
	for _, row := range rows {
		out = append(out, audit.AuditRouteCount{Endpoint: row.Endpoint, Count: row.Count})
	}
	return out, nil
}

func (r *AuditRepository) Reset(ctx context.Context, marker audit.AuditLog) error {
	row := toRow(marker)
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().Table("audit_logs").Where("1 = 1").Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewInsert().Model(&row).Exec(ctx)
		return err
	})
}

func (r *AuditRepository) DeleteBefore(ctx context.Context, cutoff time.Time) error {
	_, err := r.db.NewDelete().
		Table("audit_logs").
		Where("created_at < ?", cutoff.UTC()).
		Exec(ctx)
	return err
}

func applyListFilters(query *bun.SelectQuery, q audit.AuditListQuery, includeCursor bool) {
	if !q.From.IsZero() {
		query.Where("created_at >= ?", q.From.UTC())
	}
	if !q.To.IsZero() {
		query.Where("created_at <= ?", q.To.UTC())
	}
	applyCommonFilters(query, audit.AuditStatsQuery{
		Actor:       q.Actor,
		Action:      q.Action,
		Endpoint:    q.Endpoint,
		Method:      q.Method,
		Result:      q.Result,
		StatusCode:  q.StatusCode,
		StatusClass: q.StatusClass,
		RequestID:   q.RequestID,
	})
	if q.Path != "" {
		query.Where("path LIKE ?", "%"+q.Path+"%")
	}
	if includeCursor && q.Cursor != "" {
		cursor, err := audit.DecodeCursor(q.Cursor)
		if err == nil {
			query.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.Where("created_at < ?", cursor.CreatedAt.UTC()).
					WhereOr("created_at = ? AND id < ?", cursor.CreatedAt.UTC(), cursor.ID)
			})
		}
	}
}

func applyStatsFilters(query *bun.SelectQuery, q audit.AuditStatsQuery) {
	if !q.From.IsZero() {
		query.Where("created_at >= ?", q.From.UTC())
	}
	if !q.To.IsZero() {
		query.Where("created_at <= ?", q.To.UTC())
	}
	applyCommonFilters(query, q)
}

func applyCommonFilters(query *bun.SelectQuery, q audit.AuditStatsQuery) {
	if q.Actor != "" {
		query.Where("actor = ?", q.Actor)
	}
	if q.Action == "__public_api__" {
		query.Where("action IN (?)", bun.In([]string{"public.api.access", "validation.request", "mail.send"}))
	} else if q.Action == "__business_api__" {
		query.Where("action IN (?)", bun.In([]string{"validation.request", "mail.send"}))
	} else if q.Action != "" {
		query.Where("action = ?", q.Action)
	}
	if q.Endpoint != "" {
		query.Where("route = ?", q.Endpoint)
	}
	if q.Method != "" {
		query.Where("method = ?", q.Method)
	}
	if q.Result != "" {
		query.Where("result = ?", q.Result)
	}
	if q.RequestID != "" {
		query.Where("request_id = ?", q.RequestID)
	}
	if q.StatusCode > 0 {
		query.Where("status_code = ?", q.StatusCode)
	}
	if q.StatusClass != "" {
		if prefix, ok := strings.CutSuffix(q.StatusClass, "xx"); ok {
			if n, err := strconv.Atoi(prefix); err == nil && n >= 1 && n <= 5 {
				query.Where("status_code >= ? AND status_code < ?", n*100, (n+1)*100)
			}
		}
	}
}

func normalizeListQuery(q audit.AuditListQuery) audit.AuditListQuery {
	if q.Limit <= 0 {
		q.Limit = 100
	}
	if q.Limit > 500 {
		q.Limit = 500
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	q.Method = strings.ToUpper(strings.TrimSpace(q.Method))
	q.Result = strings.ToLower(strings.TrimSpace(q.Result))
	q.StatusClass = strings.ToLower(strings.TrimSpace(q.StatusClass))
	q.Actor = strings.TrimSpace(q.Actor)
	q.Action = strings.TrimSpace(q.Action)
	q.Endpoint = strings.TrimSpace(q.Endpoint)
	q.Path = strings.TrimSpace(q.Path)
	q.RequestID = strings.TrimSpace(q.RequestID)
	return q
}

func normalizeStatsQuery(q audit.AuditStatsQuery) audit.AuditStatsQuery {
	q.Method = strings.ToUpper(strings.TrimSpace(q.Method))
	q.Result = strings.ToLower(strings.TrimSpace(q.Result))
	q.StatusClass = strings.ToLower(strings.TrimSpace(q.StatusClass))
	q.Actor = strings.TrimSpace(q.Actor)
	q.Action = strings.TrimSpace(q.Action)
	q.Endpoint = strings.TrimSpace(q.Endpoint)
	q.RequestID = strings.TrimSpace(q.RequestID)
	if q.Limit <= 0 {
		q.Limit = 10
	}
	return q
}

func toRow(item audit.AuditLog) bunmodel.AuditLogRow {
	metadataJSON := ""
	if len(item.Metadata) > 0 {
		if raw, err := json.Marshal(item.Metadata); err == nil {
			metadataJSON = string(raw)
		}
	}
	return bunmodel.AuditLogRow{
		ID:           item.ID,
		CreatedAt:    item.Timestamp.UTC(),
		Actor:        truncate(item.Actor, 255),
		ActorSource:  truncate(item.ActorSource, 64),
		Action:       truncate(item.Action, 128),
		Method:       truncate(item.Method, 16),
		Route:        truncate(item.Endpoint, 255),
		Path:         truncate(item.Path, 2048),
		StatusCode:   item.StatusCode,
		Result:       truncate(item.Result, 32),
		RemoteIP:     truncate(item.RemoteAddr, 64),
		UserAgent:    truncate(item.UserAgent, 512),
		RequestID:    truncate(item.RequestID, 128),
		DurationMS:   item.DurationMS,
		ErrorCode:    truncate(item.ErrorCode, 128),
		ErrorMessage: truncate(item.Message, 1024),
		MetadataJSON: metadataJSON,
	}
}

func fromRow(row bunmodel.AuditLogRow) (audit.AuditLog, bool, error) {
	item := audit.AuditLog{
		ID:          row.ID,
		Timestamp:   row.CreatedAt.UTC(),
		Actor:       row.Actor,
		ActorSource: row.ActorSource,
		Action:      row.Action,
		Method:      row.Method,
		Path:        row.Path,
		Endpoint:    row.Route,
		StatusCode:  row.StatusCode,
		Result:      row.Result,
		RemoteAddr:  row.RemoteIP,
		UserAgent:   row.UserAgent,
		RequestID:   row.RequestID,
		DurationMS:  row.DurationMS,
		ErrorCode:   row.ErrorCode,
		Message:     row.ErrorMessage,
	}
	if row.MetadataJSON != "" {
		if err := json.Unmarshal([]byte(row.MetadataJSON), &item.Metadata); err != nil {
			return audit.AuditLog{}, false, fmt.Errorf("unmarshal audit metadata: %w", err)
		}
	}
	return item, true, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncate(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}
