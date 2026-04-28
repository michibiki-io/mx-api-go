package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/michibiki-io/mx-api-go/internal/config"
	"github.com/michibiki-io/mx-api-go/internal/mail"
	"github.com/michibiki-io/mx-api-go/internal/requestvalidator"
	"github.com/michibiki-io/mx-api-go/internal/version"
	"go.uber.org/zap"
)

type Handler struct {
	cfg       *config.Config
	validator *requestvalidator.Engine
	mailer    mail.Sender
	logger    *zap.Logger
}

func NewRouter(cfg *config.Config, validator *requestvalidator.Engine, sender mail.Sender, logger *zap.Logger) http.Handler {
	if strings.EqualFold(cfg.Server.Mode, "debug") {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	handler := &Handler{cfg: cfg, validator: validator, mailer: sender, logger: logger}
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
