package httpapi

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/michibiki-io/mx-api-go/internal/mail"
	"go.uber.org/zap"
)

const contactRequestKey = "contactRequest"

type templateFieldItem struct {
	Name      string
	Label     string
	Value     string
	HTMLValue string
}

func (h *Handler) validatePost(c *gin.Context) {
	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "BadRequest"})
		c.Abort()
		return
	}

	values := h.valuesFromPayload(payload)
	if errors := h.validator.Validate(values); len(errors) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "BadRequest", "errors": errors})
		c.Abort()
		return
	}

	c.Set(contactRequestKey, values)
	c.Next()
}

func (h *Handler) sendmailPost(c *gin.Context) {
	tmp, ok := c.Get(contactRequestKey)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"status": "BadRequest"})
		c.Abort()
		return
	}
	values, ok := tmp.(map[string]string)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"status": "BadRequest"})
		c.Abort()
		return
	}

	body, err := mail.RenderTemplate(h.cfg.Mail.TemplatePath, h.templateData(values))
	if err != nil {
		h.logger.Warn("failed to render mail template", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "BadRequest",
			"errors": "Invalid request, please review your input and try again.",
		})
		c.Abort()
		return
	}

	subject := strings.TrimSpace(values["subject"])
	if subject == "" {
		subject = h.cfg.Mail.Subject
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.cfg.SMTP.Timeout)
	defer cancel()
	if err := h.mailer.Send(ctx, mail.Message{
		From:       h.cfg.Mail.From,
		Recipients: h.cfg.Mail.Recipients,
		Subject:    subject,
		Body:       body,
	}); err != nil {
		h.logger.Error("failed to send mail", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "BadRequest",
			"errors": fmt.Sprintf("An internal error occurred, please try later or send us an email at %s.", h.cfg.Mail.InternalErrorRecipient),
		})
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "Ok"})
}

func (h *Handler) schema(c *gin.Context) {
	h.setSchemaCORS(c)
	c.JSON(http.StatusOK, gin.H{
		"status":      "Ok",
		"contextPath": h.cfg.Server.ContextPath,
		"apiBasePath": h.cfg.Server.ContextPath + "/api/v1",
		"fields":      h.cfg.Form.Fields,
		"validation":  h.cfg.Validation,
	})
}

func (h *Handler) valuesFromPayload(payload map[string]any) map[string]string {
	values := map[string]string{}
	for key, value := range payload {
		values[key] = stringify(value)
	}
	for _, field := range h.cfg.Form.Fields {
		if _, ok := values[field.Name]; !ok {
			values[field.Name] = field.Default
		}
	}
	return values
}

func (h *Handler) templateData(values map[string]string) map[string]any {
	location, err := time.LoadLocation(h.cfg.Mail.Timezone)
	if err != nil {
		h.logger.Warn("failed to load configured mail timezone; falling back to UTC", zap.String("timezone", h.cfg.Mail.Timezone), zap.Error(err))
		location = time.UTC
	}

	fields := h.templateFields(values)
	data := map[string]any{
		"fields":       fields,
		"Fields":       fields,
		"contact_name": h.cfg.Mail.ContactName,
		"homepage_url": h.cfg.Mail.HomepageURL,
		"submitted_at": time.Now().In(location).Format(h.cfg.Mail.SubmittedAtFormat),
		"mail":         h.cfg.Mail,
	}
	for key, value := range h.cfg.Mail.Extra {
		data[key] = value
	}
	for _, item := range fields {
		data[item.Name] = item.Value
		data[legacyTemplateName(item.Name)] = item.Value
	}
	data["ContactName"] = h.cfg.Mail.ContactName
	data["URL"] = h.cfg.Mail.HomepageURL
	return data
}

func (h *Handler) templateFields(values map[string]string) []templateFieldItem {
	items := make([]templateFieldItem, 0, len(h.cfg.Form.Fields))
	seen := map[string]struct{}{}
	for _, field := range h.cfg.Form.Fields {
		value := values[field.Name]
		items = append(items, templateFieldItem{
			Name:      field.Name,
			Label:     field.Label,
			Value:     value,
			HTMLValue: htmlLineBreaks(value),
		})
		seen[field.Name] = struct{}{}
	}
	for key, value := range values {
		if _, ok := seen[key]; ok {
			continue
		}
		items = append(items, templateFieldItem{Name: key, Label: key, Value: value, HTMLValue: htmlLineBreaks(value)})
	}
	return items
}

func htmlLineBreaks(value string) string {
	escaped := html.EscapeString(value)
	escaped = strings.ReplaceAll(escaped, "\r\n", "\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\n")
	return strings.ReplaceAll(escaped, "\n", "<br>")
}

func stringify(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func legacyTemplateName(name string) string {
	switch strings.ToLower(name) {
	case "name":
		return "Name"
	case "email":
		return "Email"
	case "tel":
		return "Tel"
	case "organization":
		return "Organization"
	case "subject":
		return "Subject"
	case "message":
		return "Message"
	default:
		return name
	}
}
