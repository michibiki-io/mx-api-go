package migration

import (
	"context"
	"fmt"

	"github.com/michibiki-io/mx-api-go/internal/infrastructure/database/dbdialect"
	"github.com/uptrace/bun"
)

func EnsureAuditSchema(ctx context.Context, db *bun.DB, helper dbdialect.Helper) error {
	for _, statement := range statements(helper) {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply audit schema migration: %w", err)
		}
	}
	return nil
}

func statements(helper dbdialect.Helper) []string {
	base := []string{
		createTable(helper),
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_created_id ON audit_logs (created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_route_created_id ON audit_logs (route, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_created_id ON audit_logs (actor, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_status_created_id ON audit_logs (status_code, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_request_id ON audit_logs (request_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_action_created_id ON audit_logs (action, created_at, id)`,
	}
	return base
}

func createTable(helper dbdialect.Helper) string {
	switch helper.Name() {
	case "postgres":
		return `CREATE TABLE IF NOT EXISTS audit_logs (
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ NOT NULL,
			actor VARCHAR(255),
			actor_source VARCHAR(64),
			action VARCHAR(128) NOT NULL,
			method VARCHAR(16),
			route VARCHAR(255),
			path TEXT,
			status_code INTEGER NOT NULL DEFAULT 0,
			result VARCHAR(32) NOT NULL,
			remote_ip VARCHAR(64),
			user_agent TEXT,
			request_id VARCHAR(128),
			duration_ms BIGINT NOT NULL DEFAULT 0,
			error_code VARCHAR(128),
			error_message TEXT,
			metadata_json TEXT
		)`
	case "mysql":
		fallthrough
	case "mariadb":
		return `CREATE TABLE IF NOT EXISTS audit_logs (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			created_at DATETIME(6) NOT NULL,
			actor VARCHAR(255),
			actor_source VARCHAR(64),
			action VARCHAR(128) NOT NULL,
			method VARCHAR(16),
			route VARCHAR(255),
			path TEXT,
			status_code INT NOT NULL DEFAULT 0,
			result VARCHAR(32) NOT NULL,
			remote_ip VARCHAR(64),
			user_agent TEXT,
			request_id VARCHAR(128),
			duration_ms BIGINT NOT NULL DEFAULT 0,
			error_code VARCHAR(128),
			error_message TEXT,
			metadata_json LONGTEXT
		)`
	default:
		return `CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMP NOT NULL,
			actor TEXT,
			actor_source TEXT,
			action TEXT NOT NULL,
			method TEXT,
			route TEXT,
			path TEXT,
			status_code INTEGER NOT NULL DEFAULT 0,
			result TEXT NOT NULL,
			remote_ip TEXT,
			user_agent TEXT,
			request_id TEXT,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			error_code TEXT,
			error_message TEXT,
			metadata_json TEXT
		)`
	}
}
