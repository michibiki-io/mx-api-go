package adminui

import (
	"embed"
	"encoding/json"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed dist/* dist/assets/*
var assets embed.FS

type RuntimeConfig struct {
	APIBasePath       string `json:"apiBasePath"`
	DashboardBasePath string `json:"dashboardBasePath"`
	DashboardToken    string `json:"dashboardToken,omitempty"`
}

type RuntimeConfigFunc func(*gin.Context, string, string) RuntimeConfig

func Register(root *gin.RouterGroup, basePath string, runtimeConfig RuntimeConfigFunc, middleware ...gin.HandlerFunc) {
	basePath = "/" + strings.Trim(strings.TrimSpace(basePath), "/")
	ui := root.Group(basePath, middleware...)
	ui.GET("", redirectToSlash(basePath))
	ui.GET("/*filepath", func(c *gin.Context) {
		name := strings.TrimPrefix(c.Param("filepath"), "/")
		if name == "" {
			name = "index.html"
		}
		if name == "config.js" {
			configJS(root.BasePath(), basePath, runtimeConfig)(c)
			return
		}
		if fileExists(name) {
			serveAsset(name)(c)
			return
		}
		serveAsset("index.html")(c)
	})
}

func configJS(contextPath, dashboardBasePath string, runtimeConfig RuntimeConfigFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := defaultRuntimeConfig(contextPath, dashboardBasePath)
		if runtimeConfig != nil {
			cfg = runtimeConfig(c, contextPath, dashboardBasePath)
		}
		data, err := json.Marshal(cfg)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Header("Content-Type", "application/javascript; charset=utf-8")
		c.String(http.StatusOK, "window.MX_API_ADMIN = %s;", data)
	}
}

func defaultRuntimeConfig(contextPath, dashboardBasePath string) RuntimeConfig {
	prefix := strings.TrimRight(contextPath, "/")
	return RuntimeConfig{
		APIBasePath:       prefix + "/_admin/api/v1",
		DashboardBasePath: prefix + dashboardBasePath,
	}
}

func redirectToSlash(basePath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, strings.TrimRight(c.Request.URL.Path, "/")+"/")
	}
}

func serveAsset(name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := assets.ReadFile("dist/" + path.Clean(name))
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		contentType := mime.TypeByExtension(path.Ext(name))
		if contentType != "" {
			c.Header("Content-Type", contentType)
		}
		c.Data(http.StatusOK, contentType, data)
	}
}

func fileExists(name string) bool {
	info, err := fs.Stat(assets, "dist/"+path.Clean(name))
	return err == nil && !info.IsDir()
}
