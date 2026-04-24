package mail

import (
	"path/filepath"
	"testing"
)

func TestRenderDefaultTemplate(t *testing.T) {
	body, err := RenderTemplate(filepath.Join("..", "..", "templates", "default_mail_template.html"), map[string]any{
		"name": "Jane",
		"fields": []map[string]string{
			{"Name": "name", "Label": "Name", "Value": "Jane", "HTMLValue": "Jane"},
			{"Name": "message", "Label": "Message", "Value": "Hello", "HTMLValue": "Hello"},
		},
		"contact_name": "Homepage",
		"homepage_url": "https://example.com",
		"submitted_at": "2026-04-24 00:00:00 UTC",
	})
	if err != nil {
		t.Fatalf("RenderTemplate() error = %v", err)
	}
	if body == "" {
		t.Fatal("rendered body is empty")
	}
}
