package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/michibiki-io/mx-api-go/internal/audit"
	"go.uber.org/zap"
)

const adminIdentityKey = "adminIdentity"

type adminIdentity struct {
	User         string   `json:"user"`
	Email        string   `json:"email"`
	Groups       []string `json:"groups"`
	AuthMode     string   `json:"authMode"`
	AuthDisabled bool     `json:"authDisabled"`
}

func (h *Handler) auditPublicAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		if !h.cfg.Audit.Enabled {
			return
		}
		event := h.eventFromRequest(c, start)
		event.Action = publicAction(c.Request.Method, c.FullPath())
		event.Actor = "public"
		event.ActorSource = "public"
		event.Message = "Public API access"
		if event.Action == "validation.request" && event.Result != audit.ResultSuccess {
			event.ErrorCode = "validation_failed"
			event.Message = "Validation request failed"
		}
		if event.Action == "mail.send" {
			if event.Result == audit.ResultSuccess {
				event.Message = "Mail send request succeeded"
			} else {
				event.ErrorCode = "mail_send_failed"
				event.Message = "Mail send request failed"
			}
		}
		h.recordAudit(c.Request.Context(), event)
	}
}

func (h *Handler) adminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, status, allowed := h.adminIdentityFromRequest(c)
		if allowed {
			c.Set(adminIdentityKey, identity)
			c.Next()
			return
		}
		result := audit.ResultDenied
		actor := identity.User
		if actor == "" {
			actor = "anonymous"
		}
		h.recordAudit(c.Request.Context(), audit.Event{
			Actor:       actor,
			ActorSource: "admin:" + h.cfg.Admin.Auth.Mode,
			Action:      "admin.access.denied",
			Method:      c.Request.Method,
			Path:        c.Request.URL.Path,
			Endpoint:    c.FullPath(),
			StatusCode:  status,
			Result:      result,
			RemoteAddr:  clientAddress(c),
			UserAgent:   c.Request.UserAgent(),
			RequestID:   requestID(c),
			ErrorCode:   "admin_access_denied",
			Message:     "Denied admin access",
		})
		if status == http.StatusUnauthorized {
			c.AbortWithStatusJSON(status, gin.H{"error": "admin authentication is required"})
			return
		}
		c.AbortWithStatusJSON(status, gin.H{"error": "admin authorization is required"})
	}
}

func (h *Handler) adminDashboardAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		if !h.cfg.Audit.Enabled {
			return
		}
		path := c.Request.URL.Path
		if strings.Contains(path[strings.LastIndex(path, "/")+1:], ".") || strings.HasSuffix(path, "config.js") {
			return
		}
		identity := currentAdminIdentity(c)
		event := h.eventFromRequest(c, start)
		event.Actor = identity.User
		event.ActorSource = "admin:" + identity.AuthMode
		event.Action = "admin.dashboard.view"
		event.Message = "Admin dashboard accessed"
		h.recordAudit(c.Request.Context(), event)
	}
}

func (h *Handler) adminHeaderAuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.EqualFold(strings.TrimSpace(h.cfg.Admin.Auth.Mode), "header") {
			c.Next()
			return
		}
		h.recordAdminDenied(c, http.StatusForbidden, "admin_header_auth_required", "Admin header authentication is required")
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin header authentication is required"})
	}
}

func (h *Handler) adminDashboardTokenRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.dashboardTokens != nil && h.dashboardTokens.Valid(strings.TrimSpace(c.GetHeader(dashboardTokenHeader))) {
			c.Next()
			return
		}
		h.recordAdminDenied(c, http.StatusForbidden, "dashboard_token_invalid", "Admin dashboard token is invalid")
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin dashboard token is invalid"})
	}
}

func (h *Handler) eventFromRequest(c *gin.Context, start time.Time) audit.Event {
	statusCode := c.Writer.Status()
	result := audit.ResultSuccess
	if statusCode >= 400 {
		result = audit.ResultFailure
	}
	return audit.Event{
		Method:     c.Request.Method,
		Path:       c.Request.URL.Path,
		Endpoint:   c.FullPath(),
		StatusCode: statusCode,
		Result:     result,
		RemoteAddr: clientAddress(c),
		UserAgent:  c.Request.UserAgent(),
		RequestID:  requestID(c),
		DurationMS: time.Since(start).Milliseconds(),
		Metadata: map[string]any{
			"queryPresent": c.Request.URL.RawQuery != "",
		},
	}
}

func (h *Handler) recordAdminDenied(c *gin.Context, status int, code, message string) {
	identity := currentAdminIdentity(c)
	actor := identity.User
	if actor == "" {
		actor = "anonymous"
	}
	h.recordAudit(c.Request.Context(), audit.Event{
		Actor:       actor,
		ActorSource: "admin:" + identity.AuthMode,
		Action:      "admin.access.denied",
		Method:      c.Request.Method,
		Path:        c.Request.URL.Path,
		Endpoint:    c.FullPath(),
		StatusCode:  status,
		Result:      audit.ResultDenied,
		RemoteAddr:  clientAddress(c),
		UserAgent:   c.Request.UserAgent(),
		RequestID:   requestID(c),
		ErrorCode:   code,
		Message:     message,
	})
}

func (h *Handler) adminIdentityFromRequest(c *gin.Context) (adminIdentity, int, bool) {
	mode := strings.ToLower(strings.TrimSpace(h.cfg.Admin.Auth.Mode))
	if mode == "none" {
		return adminIdentity{User: "anonymous", AuthMode: "none", AuthDisabled: true}, http.StatusOK, true
	}
	if mode != "header" {
		return adminIdentity{AuthMode: mode}, http.StatusForbidden, false
	}
	auth := h.cfg.Admin.Auth
	user := strings.TrimSpace(c.GetHeader(auth.UserHeader))
	email := strings.TrimSpace(c.GetHeader(auth.EmailHeader))
	groups := splitHeaderGroups(c.GetHeader(auth.GroupsHeader))
	if user == "" {
		user = email
	}
	identity := adminIdentity{User: user, Email: email, Groups: groups, AuthMode: "header"}
	if user == "" {
		return identity, http.StatusUnauthorized, false
	}
	if adminAllowed(identity, auth.AllowedUsers, auth.AllowedGroups) {
		return identity, http.StatusOK, true
	}
	return identity, http.StatusForbidden, false
}

func adminAllowed(identity adminIdentity, users, groups []string) bool {
	userSet := make(map[string]struct{}, len(users)*2)
	for _, user := range users {
		user = strings.ToLower(strings.TrimSpace(user))
		if user == "" {
			continue
		}
		userSet[user] = struct{}{}
	}
	if _, ok := userSet[strings.ToLower(identity.User)]; ok {
		return true
	}
	if identity.Email != "" {
		if _, ok := userSet[strings.ToLower(identity.Email)]; ok {
			return true
		}
	}
	groupSet := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		group = strings.ToLower(strings.TrimSpace(group))
		if group != "" {
			groupSet[group] = struct{}{}
		}
	}
	for _, group := range identity.Groups {
		if _, ok := groupSet[strings.ToLower(strings.TrimSpace(group))]; ok {
			return true
		}
	}
	return false
}

func currentAdminIdentity(c *gin.Context) adminIdentity {
	value, ok := c.Get(adminIdentityKey)
	if !ok {
		return adminIdentity{User: "anonymous", AuthMode: "none", AuthDisabled: true}
	}
	identity, ok := value.(adminIdentity)
	if !ok {
		return adminIdentity{User: "anonymous", AuthMode: "none", AuthDisabled: true}
	}
	if identity.User == "" {
		identity.User = "anonymous"
	}
	return identity
}

func (h *Handler) recordAudit(ctx context.Context, event audit.Event) {
	if h.audit == nil {
		return
	}
	if err := h.audit.Record(ctx, event); err != nil {
		h.logger.Warn("failed to record audit event", zap.Error(err))
	}
}

func publicAction(method, endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	switch {
	case method == http.MethodPost && strings.HasSuffix(endpoint, "/validate"):
		return "validation.request"
	case method == http.MethodPost && strings.HasSuffix(endpoint, "/sendmail"):
		return "mail.send"
	default:
		return "public.api.access"
	}
}

func splitHeaderGroups(value string) []string {
	parts := strings.Split(value, ",")
	groups := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			groups = append(groups, part)
		}
	}
	return groups
}

func requestID(c *gin.Context) string {
	for _, header := range []string{"X-Request-ID", "X-Correlation-ID", "X-Amzn-Trace-Id"} {
		if value := strings.TrimSpace(c.GetHeader(header)); value != "" {
			return value
		}
	}
	return ""
}

func clientAddress(c *gin.Context) string {
	if value := strings.TrimSpace(c.GetHeader("X-Forwarded-For")); value != "" {
		if first, _, ok := strings.Cut(value, ","); ok {
			return strings.TrimSpace(first)
		}
		return value
	}
	if value := strings.TrimSpace(c.GetHeader("X-Real-IP")); value != "" {
		return value
	}
	return c.ClientIP()
}
