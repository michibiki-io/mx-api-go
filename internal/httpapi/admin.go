package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/michibiki-io/mx-api-go/internal/adminui"
	"github.com/michibiki-io/mx-api-go/internal/audit"
	"github.com/michibiki-io/mx-api-go/internal/version"
)

func (h *Handler) adminMe(c *gin.Context) {
	identity := currentAdminIdentity(c)
	c.JSON(http.StatusOK, gin.H{
		"mode":                 h.cfg.Admin.Auth.Mode,
		"authDisabled":         identity.AuthDisabled,
		"user":                 identity.User,
		"email":                identity.Email,
		"groups":               identity.Groups,
		"version":              version.Value(),
		"commit":               version.Commit(),
		"shortCommit":          version.ShortCommit(),
		"commitURL":            commitURL(version.Commit()),
		"auditTimestampFormat": h.cfg.Admin.Dashboard.TimestampFormat,
	})
}

func (h *Handler) adminRequestMetrics(c *gin.Context) {
	if h.auditRead == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "audit storage is not available"})
		return
	}
	filter := audit.MetricsFilter{
		From:        queryTime(c, "from"),
		To:          queryTime(c, "to"),
		Bucket:      queryDuration(c, "bucket"),
		Endpoint:    c.Query("endpoint"),
		Method:      c.Query("method"),
		Result:      c.Query("result"),
		StatusClass: c.Query("status_class"),
	}
	metrics, err := h.auditRead.Metrics(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load request metrics"})
		return
	}
	h.recordAdminAPI(c, "admin.metrics.view", "Admin viewed request metrics")
	c.JSON(http.StatusOK, metrics)
}

func (h *Handler) adminAuditOptions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"actions": audit.KnownActions(),
		"endpoints": []string{
			"/api/v1/schema",
			"/api/v1/form-schema",
			"/api/v1/validate",
			"/api/v1/sendmail",
			"/_admin/api/v1/me",
			"/_admin/api/v1/request-metrics",
			"/_admin/api/v1/audit-events",
			"/_admin/api/v1/mail-server-check",
		},
		"results": []string{audit.ResultSuccess, audit.ResultFailure, audit.ResultDenied},
	})
}

func (h *Handler) adminAuditEvents(c *gin.Context) {
	if h.auditRead == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "audit storage is not available"})
		return
	}
	filter := audit.Filter{
		From:        queryTime(c, "from"),
		To:          queryTime(c, "to"),
		Actor:       c.Query("actor"),
		Action:      c.Query("action"),
		Endpoint:    c.Query("endpoint"),
		Path:        c.Query("path"),
		Method:      c.Query("method"),
		Result:      c.Query("result"),
		StatusCode:  queryInt(c, "status_code", 0),
		StatusClass: c.Query("status_class"),
		RequestID:   c.Query("request_id"),
		Limit:       queryInt(c, "limit", 100),
		Cursor:      c.Query("cursor"),
		Offset:      queryInt(c, "offset", 0),
		SkipTotal:   !queryBool(c, "include_total", true),
	}
	page, err := h.auditRead.List(c.Request.Context(), filter)
	if err != nil {
		if err == audit.ErrInvalidCursor {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid audit cursor"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load audit events"})
		return
	}
	var nextCursor any
	if filter.Cursor == "" && c.Query("cursor") == "" {
		if page.HasNext {
			nextCursor = filter.Offset + len(page.Items)
		}
	} else if page.NextCursor != "" {
		nextCursor = page.NextCursor
	}
	h.recordAdminAPI(c, "audit.view", "Admin viewed audit logs")
	responseTotal := any(nil)
	if !filter.SkipTotal {
		responseTotal = page.Total
	}
	c.JSON(http.StatusOK, gin.H{
		"items":      h.auditEventResponses(page.Items),
		"total":      responseTotal,
		"hasNext":    page.HasNext,
		"nextCursor": nextCursor,
	})
}

func (h *Handler) adminAuditEvent(c *gin.Context) {
	if h.auditRead == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "audit storage is not available"})
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid audit event id"})
		return
	}
	event, ok, err := h.auditRead.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load audit event"})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "audit event not found"})
		return
	}
	h.recordAdminAPI(c, "audit.detail.view", "Admin viewed audit log detail")
	c.JSON(http.StatusOK, h.auditEventResponse(event))
}

type auditEventResponse struct {
	audit.Event
	TimestampDisplay string `json:"timestampDisplay"`
}

func (h *Handler) auditEventResponses(events []audit.Event) []auditEventResponse {
	out := make([]auditEventResponse, 0, len(events))
	for _, event := range events {
		out = append(out, h.auditEventResponse(event))
	}
	return out
}

func (h *Handler) auditEventResponse(event audit.Event) auditEventResponse {
	return auditEventResponse{
		Event:            event,
		TimestampDisplay: h.formatAuditTimestamp(event.Timestamp),
	}
}

func (h *Handler) formatAuditTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	location := time.UTC
	if configured := strings.TrimSpace(h.cfg.Admin.Dashboard.TimestampTimezone); configured != "" {
		if loaded, err := time.LoadLocation(configured); err == nil {
			location = loaded
		}
	}
	layout := strings.TrimSpace(h.cfg.Admin.Dashboard.TimestampFormat)
	if layout == "" {
		layout = time.RFC3339
	}
	return value.In(location).Format(layout)
}

func commitURL(commit string) string {
	commit = strings.TrimSpace(commit)
	if commit == "" || commit == "unknown" {
		return ""
	}
	return "https://github.com/michibiki-io/mx-api-go/commit/" + commit
}

func (h *Handler) adminAuditReset(c *gin.Context) {
	if h.auditRead == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "audit storage is not available"})
		return
	}
	var body struct {
		Confirmation string `json:"confirmation"`
		Reason       string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid reset request"})
		return
	}
	if body.Confirmation != "RESET" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirmation must be RESET"})
		return
	}
	identity := currentAdminIdentity(c)
	marker := audit.Event{
		Actor:       identity.User,
		ActorSource: "admin:" + identity.AuthMode,
		Action:      "audit.reset",
		Method:      c.Request.Method,
		Path:        c.Request.URL.Path,
		Endpoint:    c.FullPath(),
		StatusCode:  http.StatusOK,
		Result:      audit.ResultSuccess,
		RemoteAddr:  clientAddress(c),
		UserAgent:   c.Request.UserAgent(),
		RequestID:   requestID(c),
		Message:     "Admin reset audit log",
		Metadata: map[string]any{
			"reason": strings.TrimSpace(body.Reason),
		},
	}
	if err := h.auditRead.Reset(c.Request.Context(), marker); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset audit events"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) adminMailServerCheck(c *gin.Context) {
	timeout := h.cfg.SMTP.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	result := h.mailChecker.Check(ctx)
	status := "Ok"
	auditResult := audit.ResultSuccess
	message := "Admin checked backend mail server"
	if result.Code != "" {
		status = "Error"
		auditResult = audit.ResultFailure
		message = "Backend mail server check failed"
	}
	h.recordAdminMailServerCheck(c, auditResult, message, result.Code)
	c.JSON(http.StatusOK, gin.H{
		"status":     status,
		"mailServer": result,
	})
}

func (h *Handler) adminDashboardRuntimeConfig(_ *gin.Context, contextPath, dashboardBasePath string) adminui.RuntimeConfig {
	prefix := strings.TrimRight(contextPath, "/")
	cfg := adminui.RuntimeConfig{
		APIBasePath:       prefix + "/_admin/api/v1",
		DashboardBasePath: prefix + dashboardBasePath,
	}
	if strings.EqualFold(h.cfg.Admin.Auth.Mode, "header") && h.dashboardTokens != nil {
		cfg.DashboardToken = h.dashboardTokens.Issue()
	}
	return cfg
}

func (h *Handler) recordAdminAPI(c *gin.Context, action, message string) {
	identity := currentAdminIdentity(c)
	h.recordAudit(c.Request.Context(), audit.Event{
		Actor:       identity.User,
		ActorSource: "admin:" + identity.AuthMode,
		Action:      action,
		Method:      c.Request.Method,
		Path:        c.Request.URL.Path,
		Endpoint:    c.FullPath(),
		StatusCode:  c.Writer.Status(),
		Result:      audit.ResultSuccess,
		RemoteAddr:  clientAddress(c),
		UserAgent:   c.Request.UserAgent(),
		RequestID:   requestID(c),
		Message:     message,
	})
}

func (h *Handler) recordAdminMailServerCheck(c *gin.Context, result, message, code string) {
	identity := currentAdminIdentity(c)
	event := audit.Event{
		Actor:       identity.User,
		ActorSource: "admin:" + identity.AuthMode,
		Action:      "mail.server.check",
		Method:      c.Request.Method,
		Path:        c.Request.URL.Path,
		Endpoint:    c.FullPath(),
		StatusCode:  http.StatusOK,
		Result:      result,
		RemoteAddr:  clientAddress(c),
		UserAgent:   c.Request.UserAgent(),
		RequestID:   requestID(c),
		Message:     message,
	}
	if code != "" {
		event.ErrorCode = code
	}
	h.recordAudit(c.Request.Context(), event)
}

func queryInt(c *gin.Context, key string, fallback int) int {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func queryTime(c *gin.Context, key string) time.Time {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed
	}
	return time.Time{}
}

func queryDuration(c *gin.Context, key string) time.Duration {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return 0
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		return parsed
	}
	return 0
}

func queryBool(c *gin.Context, key string, fallback bool) bool {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
