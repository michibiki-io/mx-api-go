package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/michibiki-io/mx-api-go/internal/adminui"
	"github.com/michibiki-io/mx-api-go/internal/audit"
	"github.com/michibiki-io/mx-api-go/internal/config"
	"github.com/michibiki-io/mx-api-go/internal/mail"
	"github.com/michibiki-io/mx-api-go/internal/requestvalidator"
	"github.com/michibiki-io/mx-api-go/internal/version"
	"go.uber.org/zap"
)

type Handler struct {
	cfg                *config.Config
	validator          *requestvalidator.Engine
	mailer             mail.Sender
	mailChecker        *mail.SMTPSender
	logger             *zap.Logger
	audit              audit.Recorder
	auditRead          auditReader
	rateLimiter        *fixedWindowLimiter
	failureRateLimiter *fixedWindowLimiter
	idempotency        *idempotencyStore
	dashboardTokens    *dashboardTokenStore
}

type auditReader interface {
	audit.Recorder
	List(context.Context, audit.Filter) (audit.Page, error)
	Get(context.Context, string) (audit.Event, bool, error)
	Summary(context.Context, audit.Filter) (audit.Summary, error)
	Metrics(context.Context, audit.MetricsFilter) (audit.Metrics, error)
	Reset(context.Context, audit.Event) error
}

func NewRouter(cfg *config.Config, validator *requestvalidator.Engine, sender mail.Sender, logger *zap.Logger, recorders ...audit.Recorder) http.Handler {
	if strings.EqualFold(cfg.Server.Mode, "debug") {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	recorder := audit.Recorder(audit.NoopRecorder{})
	if len(recorders) > 0 && recorders[0] != nil {
		recorder = recorders[0]
	}
	handler := &Handler{
		cfg:                cfg,
		validator:          validator,
		mailer:             sender,
		mailChecker:        mail.NewSMTPSender(cfg),
		logger:             logger,
		audit:              recorder,
		rateLimiter:        newFixedWindowLimiter(rateLimitWindow),
		failureRateLimiter: newFixedWindowLimiter(rateLimitWindow),
		idempotency:        newIdempotencyStore(time.Duration(cfg.Security.Idempotency.TTLSeconds) * time.Second),
		dashboardTokens:    newDashboardTokenStore(30 * time.Minute),
	}
	if reader, ok := recorder.(auditReader); ok {
		handler.auditRead = reader
	}
	engine := gin.New()
	engine.Use(gin.Recovery())
	_ = engine.SetTrustedProxies(nil)

	root := engine.Group(cfg.Server.ContextPath)
	{
		root.GET("/", status("Ok"))
		root.GET("/helthz", status("Ok"))
		root.GET("/healthz", status("Ok"))
	}

	api := root.Group("/api/v1")
	api.Use(handler.auditPublicAPI())
	api.Use(handler.rateLimitPublicAPI())
	{
		api.GET("/schema", handler.schema)
		api.GET("/form-schema", handler.schema)

		validate := api.Group("/validate").Use(handler.allowedOrigin)
		validate.OPTIONS("", optionStatus)
		validate.GET("", status("Ok"))
		validate.POST("", handler.validatePost, status("Ok"))

		sendmail := api.Group("/sendmail").Use(handler.allowedOrigin)
		sendmail.OPTIONS("", optionStatus)
		sendmail.GET("", status("Ok"))
		sendmail.POST("", handler.validatePost, handler.sendmailPost)
	}

	if cfg.Admin.Dashboard.Enabled {
		adminAPI := root.Group("/_admin/api/v1")
		adminAPI.Use(handler.adminRequired())
		{
			adminAPI.GET("/me", handler.adminMe)
			adminAPI.GET("/request-metrics", handler.adminRequestMetrics)
			adminAPI.GET("/audit-options", handler.adminAuditOptions)
			adminAPI.GET("/audit-events", handler.adminAuditEvents)
			adminAPI.GET("/audit-events/:id", handler.adminAuditEvent)
			adminAPI.POST("/audit-events/reset", handler.adminAuditReset)
			adminAPI.POST("/mail-server-check", handler.adminHeaderAuthRequired(), handler.adminDashboardTokenRequired(), handler.adminMailServerCheck)
		}
		adminui.Register(root, cfg.Admin.Dashboard.BasePath, handler.adminDashboardRuntimeConfig, handler.adminRequired(), handler.adminDashboardAccess())
	}

	return engine
}

func status(value string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": value, "version": version.Value()})
	}
}

func optionStatus(c *gin.Context) {
	c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST")
	c.JSON(http.StatusOK, gin.H{"status": "Ok", "version": version.Value()})
}
