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

	router := httpapi.NewRouter(cfg, validatorEngine, mail.NewSMTPSender(cfg), logger)
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
