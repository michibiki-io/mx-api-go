package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (h *Handler) allowedOrigin(c *gin.Context) {
	origin := c.Request.Header.Get("Origin")
	referer := c.Request.Header.Get("Referer")
	if origin == "" || (c.Request.Method != http.MethodOptions && referer == "") {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "Unauthorized"})
		c.Abort()
		return
	}

	if !h.isAllowedOrigin(origin) {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "Unauthorized"})
		c.Abort()
		return
	}

	c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
	c.Writer.Header().Set("Access-Control-Allow-Headers", "X-Firebase-AppCheck, X-Turnstile-AppCheck, Content-Type")
	c.Next()
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
