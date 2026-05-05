package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (h *Handler) publicCORS(c *gin.Context) {
	h.setPublicCORS(c)

	origin := strings.TrimSpace(c.Request.Header.Get("Origin"))
	if origin == "" || !h.isAllowedOrigin(origin) {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "Unauthorized"})
		c.Abort()
		return
	}

	c.Next()
}

func (h *Handler) setPublicCORS(c *gin.Context) {
	addVary(c, "Origin", "Access-Control-Request-Headers", "Access-Control-Request-Method")

	origin := strings.TrimSpace(c.Request.Header.Get("Origin"))
	if origin == "" || !h.isAllowedOrigin(origin) {
		return
	}
	c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
	c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	if requestedHeaders := strings.TrimSpace(c.Request.Header.Get("Access-Control-Request-Headers")); requestedHeaders != "" {
		c.Writer.Header().Set("Access-Control-Allow-Headers", requestedHeaders)
	}
}

func (h *Handler) setSchemaCORS(c *gin.Context) {
	origin := c.Request.Header.Get("Origin")
	if origin != "" && h.isAllowedOrigin(origin) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	}
}

func (h *Handler) isAllowedOrigin(origin string) bool {
	allowed := map[string]struct{}{}
	for _, item := range h.cfg.Security.AllowedOrigins {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.HasPrefix(item, "http://") || strings.HasPrefix(item, "https://") {
			allowed[item] = struct{}{}
			continue
		}
		allowed[fmt.Sprintf("http://%s", item)] = struct{}{}
		allowed[fmt.Sprintf("https://%s", item)] = struct{}{}
		allowed[fmt.Sprintf("http://www.%s", item)] = struct{}{}
		allowed[fmt.Sprintf("https://www.%s", item)] = struct{}{}
	}
	_, ok := allowed[origin]
	return ok
}

func addVary(c *gin.Context, values ...string) {
	header := c.Writer.Header()
	current := header.Get("Vary")
	parts := make([]string, 0, len(values)+1)
	seen := map[string]struct{}{}
	for _, part := range strings.Split(current, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parts = append(parts, part)
		seen[strings.ToLower(part)] = struct{}{}
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		parts = append(parts, value)
		seen[key] = struct{}{}
	}
	if len(parts) > 0 {
		header.Set("Vary", strings.Join(parts, ", "))
	}
}
