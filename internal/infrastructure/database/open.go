package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/michibiki-io/mx-api-go/internal/audit"
	"github.com/michibiki-io/mx-api-go/internal/config"
	"github.com/michibiki-io/mx-api-go/internal/infrastructure/database/bunrepo"
	"github.com/michibiki-io/mx-api-go/internal/infrastructure/database/dbdialect"
	"github.com/michibiki-io/mx-api-go/internal/infrastructure/database/migration"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/mysqldialect"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/schema"
	"go.uber.org/zap"
)

type Bundle struct {
	SQL    *sql.DB
	Bun    *bun.DB
	Helper dbdialect.Helper
}

func Open(ctx context.Context, cfg config.DatabaseConfig) (*Bundle, error) {
	helper, err := dbdialect.New(cfg.Driver)
	if err != nil {
		return nil, err
	}
	sqldb, err := openSQLDB(cfg, helper)
	if err != nil {
		return nil, err
	}
	db := bun.NewDB(sqldb, dialectFor(helper))
	if err := configureSession(ctx, sqldb, helper); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Bundle{SQL: sqldb, Bun: db, Helper: helper}, nil
}

func OpenAuditStore(ctx context.Context, cfg *config.Config, logger *zap.Logger) (*audit.Store, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	dbCfg := resolveDatabaseConfig(cfg)
	bundle, err := Open(ctx, dbCfg)
	if err != nil {
		return nil, err
	}
	if err := migration.EnsureAuditSchema(ctx, bundle.Bun, bundle.Helper); err != nil {
		_ = bundle.Bun.Close()
		return nil, err
	}
	repo := bunrepo.NewAuditRepository(bundle.Bun, bundle.Helper)
	var recorder audit.AuditRecorder
	if cfg.Audit.Async.Enabled {
		recorder = audit.NewAsyncAuditRecorder(repo, toAuditAsyncConfig(cfg.Audit.Async), logger)
	} else {
		recorder = audit.NewSyncAuditRecorder(repo)
	}
	store := audit.NewStore(repo, recorder, bundle.Bun.Close, cfg.Audit.RetentionDays)
	if err := store.PruneRetention(ctx, cfg.Audit.RetentionDays); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func OpenSQLiteAuditStore(ctx context.Context, path string, asyncCfg audit.AsyncConfig, logger *zap.Logger) (*audit.Store, error) {
	cfg := config.Default()
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = sqliteDSNFromPath(path)
	cfg.Audit.Async = fromAuditAsyncConfig(asyncCfg)
	return OpenAuditStore(ctx, cfg, logger)
}

func openSQLDB(cfg config.DatabaseConfig, helper dbdialect.Helper) (*sql.DB, error) {
	driver := dbdialect.NormalizeDriver(cfg.Driver)
	dsn := strings.TrimSpace(cfg.DSN)
	switch driver {
	case "sqlite":
		if err := ensureSQLiteDirectory(dsn); err != nil {
			return nil, err
		}
		db, err := sql.Open(sqliteshim.ShimName, dsn)
		if err != nil {
			return nil, fmt.Errorf("open sqlite database: %w", err)
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(0)
		return db, pingDB(db)
	case "postgres":
		db := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
		applyPoolConfig(db, cfg)
		return db, pingDB(db)
	case "mysql", "mariadb":
		if _, err := mysql.ParseDSN(dsn); err != nil {
			return nil, fmt.Errorf("parse mysql dsn: %w", err)
		}
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return nil, fmt.Errorf("open mysql database: %w", err)
		}
		applyPoolConfig(db, cfg)
		return db, pingDB(db)
	default:
		return nil, fmt.Errorf("unsupported db driver %q", cfg.Driver)
	}
}

func dialectFor(helper dbdialect.Helper) schema.Dialect {
	switch helper.Name() {
	case "postgres":
		return pgdialect.New()
	case "mysql", "mariadb":
		return mysqldialect.New()
	default:
		return sqlitedialect.New()
	}
}

func configureSession(ctx context.Context, db *sql.DB, helper dbdialect.Helper) error {
	for _, statement := range helper.SessionInitStatements() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure %s session: %w", helper.Name(), err)
		}
	}
	return nil
}

func pingDB(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

func applyPoolConfig(db *sql.DB, cfg config.DatabaseConfig) {
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	}
}

func resolveDatabaseConfig(cfg *config.Config) config.DatabaseConfig {
	if cfg == nil {
		return config.Default().Database
	}
	resolved := cfg.Database
	if strings.TrimSpace(resolved.Driver) != "" && strings.TrimSpace(resolved.DSN) != "" {
		return resolved
	}
	legacyType := strings.ToLower(strings.TrimSpace(cfg.Audit.Storage.Type))
	if legacyType == "" {
		legacyType = "sqlite"
	}
	if strings.TrimSpace(resolved.Driver) == "" {
		resolved.Driver = legacyType
	}
	if strings.TrimSpace(resolved.DSN) == "" && legacyType == "sqlite" {
		resolved.DSN = sqliteDSNFromPath(cfg.Audit.Storage.Path)
	}
	if dbdialect.NormalizeDriver(resolved.Driver) == "sqlite" && strings.TrimSpace(resolved.DSN) == "" {
		resolved.DSN = sqliteDSNFromPath("/tmp/mx-api-audit.db")
	}
	return resolved
}

func sqliteDSNFromPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/tmp/mx-api-audit.db"
	}
	if strings.HasPrefix(path, "file:") {
		return path
	}
	if path == ":memory:" || path == "file::memory:" || strings.Contains(path, "mode=memory") {
		return "file::memory:?cache=shared"
	}
	return "file:" + path + "?cache=shared&mode=rwc&_journal_mode=WAL&_busy_timeout=5000"
}

func ensureSQLiteDirectory(dsn string) error {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" || dsn == ":memory:" || strings.Contains(dsn, "mode=memory") || strings.HasPrefix(dsn, "file::memory:") {
		return nil
	}
	path, err := sqlitePathFromDSN(dsn)
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	return os.MkdirAll(filepath.Dir(path), 0o700)
}

func sqlitePathFromDSN(dsn string) (string, error) {
	if !strings.HasPrefix(dsn, "file:") {
		return dsn, nil
	}
	trimmed := strings.TrimPrefix(dsn, "file:")
	if strings.HasPrefix(trimmed, "/") && !strings.Contains(trimmed, "?") {
		return trimmed, nil
	}
	parts := strings.SplitN(trimmed, "?", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", nil
	}
	decoded, err := url.PathUnescape(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode sqlite dsn path: %w", err)
	}
	return decoded, nil
}

func toAuditAsyncConfig(cfg config.AuditAsyncConfig) audit.AsyncConfig {
	return audit.AsyncConfig{
		Enabled:              cfg.Enabled,
		ChannelSize:          cfg.ChannelSize,
		BatchSize:            cfg.BatchSize,
		FlushInterval:        cfg.FlushInterval,
		ShutdownFlushTimeout: cfg.ShutdownFlushTimeout,
		DropOnFull:           cfg.DropOnFull,
		RetryMaxAttempts:     cfg.RetryMaxAttempts,
		RetryInitialBackoff:  cfg.RetryInitialBackoff,
		RetryMaxBackoff:      cfg.RetryMaxBackoff,
		EnqueueTimeout:       cfg.EnqueueTimeout,
	}
}

func fromAuditAsyncConfig(cfg audit.AsyncConfig) config.AuditAsyncConfig {
	return config.AuditAsyncConfig{
		Enabled:              cfg.Enabled,
		ChannelSize:          cfg.ChannelSize,
		BatchSize:            cfg.BatchSize,
		FlushInterval:        cfg.FlushInterval,
		ShutdownFlushTimeout: cfg.ShutdownFlushTimeout,
		DropOnFull:           cfg.DropOnFull,
		RetryMaxAttempts:     cfg.RetryMaxAttempts,
		RetryInitialBackoff:  cfg.RetryInitialBackoff,
		RetryMaxBackoff:      cfg.RetryMaxBackoff,
		EnqueueTimeout:       cfg.EnqueueTimeout,
	}
}
