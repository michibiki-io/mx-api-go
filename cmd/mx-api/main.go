package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/michibiki-io/mx-api-go/internal/audit"
	"github.com/michibiki-io/mx-api-go/internal/config"
	"github.com/michibiki-io/mx-api-go/internal/httpapi"
	"github.com/michibiki-io/mx-api-go/internal/logging"
	"github.com/michibiki-io/mx-api-go/internal/mail"
	"github.com/michibiki-io/mx-api-go/internal/requestvalidator"
	"github.com/michibiki-io/mx-api-go/internal/version"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		panic(err)
	}

	logger, err := logging.New(cfg.Server.Mode)
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	validatorEngine, err := requestvalidator.New(cfg)
	if err != nil {
		logger.Fatal("failed to initialize validator", zap.Error(err))
	}

	var auditRecorder audit.Recorder = audit.NoopRecorder{}
	if cfg.Audit.Enabled {
		if cfg.Audit.Storage.Type != "sqlite" {
			logger.Fatal("unsupported audit storage type", zap.String("type", cfg.Audit.Storage.Type))
		}
		store, err := audit.Open(context.Background(), cfg.Audit.Storage.Path)
		if err != nil {
			logger.Fatal("failed to initialize audit store", zap.Error(err))
		}
		defer store.Close()
		auditRecorder = store
		if err := store.PruneRetention(context.Background(), cfg.Audit.RetentionDays); err != nil {
			logger.Warn("failed to prune audit logs", zap.Error(err))
		}
		_ = store.Record(context.Background(), audit.Event{
			Actor:       "system",
			ActorSource: "system",
			Action:      "system.startup",
			Result:      audit.ResultSuccess,
			Message:     "mx-api started",
		})
	}

	router := httpapi.NewRouter(cfg, validatorEngine, mail.NewSMTPSender(cfg), logger, auditRecorder)
	server := &http.Server{
		Addr:              cfg.Server.Address(),
		Handler:           router,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
	}

	go func() {
		logger.Info("mx-api started",
			zap.String("address", server.Addr),
			zap.String("context_path", cfg.Server.ContextPath),
			zap.String("version", version.Value()),
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("server stopped unexpectedly", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
		return
	}

	time.Sleep(50 * time.Millisecond)
	logger.Info("mx-api stopped")
}
