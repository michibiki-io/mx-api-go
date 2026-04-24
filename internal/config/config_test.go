package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppliesLegacyRequiredFieldsFromEnv(t *testing.T) {
	t.Setenv("CONTACT_FORM_REQUIRED_FIELD", "tel, organization")

	cfg, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	required := map[string]bool{}
	for _, field := range cfg.Form.Fields {
		required[field.Name] = field.Required
	}
	if !required["tel"] {
		t.Fatal("tel should be required from CONTACT_FORM_REQUIRED_FIELD")
	}
	if !required["organization"] {
		t.Fatal("organization should be required from CONTACT_FORM_REQUIRED_FIELD")
	}
}

func TestLoadParsesYAMLConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
server:
  context_path: contact
  port: 9090
mail:
  template_path: /templates/contact.html
  timezone: Asia/Tokyo
  submitted_at_format: 2006/01/02 15:04:05 MST
form:
  fields:
    - name: company
      label: Company
      required: true
      rules: [required]
validation:
  required:
    tag: required
    code: validation_required
    message: cannot be blank
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.ContextPath != "/contact" {
		t.Fatalf("ContextPath = %q, want /contact", cfg.Server.ContextPath)
	}
	if cfg.Server.Port != 9090 {
		t.Fatalf("Port = %d, want 9090", cfg.Server.Port)
	}
	if len(cfg.Form.Fields) != 1 || cfg.Form.Fields[0].Name != "company" {
		t.Fatalf("fields were not loaded: %#v", cfg.Form.Fields)
	}
	if cfg.Mail.Timezone != "Asia/Tokyo" {
		t.Fatalf("Mail.Timezone = %q, want Asia/Tokyo", cfg.Mail.Timezone)
	}
	if cfg.Mail.SubmittedAtFormat != "2006/01/02 15:04:05 MST" {
		t.Fatalf("Mail.SubmittedAtFormat = %q, want configured format", cfg.Mail.SubmittedAtFormat)
	}
}

func TestLoadIgnoresSMTPCredentialsFromYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
smtp:
  client_username: smtp-user
  client_password: change-me
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SMTP.ClientUsername != "" {
		t.Fatalf("ClientUsername = %q, want empty when env is unset", cfg.SMTP.ClientUsername)
	}
	if cfg.SMTP.ClientPassword != "" {
		t.Fatalf("ClientPassword = %q, want empty when env is unset", cfg.SMTP.ClientPassword)
	}

	t.Setenv("SMTP_CLIENT_USERNAME", "env-user")
	t.Setenv("SMTP_CLIENT_PASSWORD", "env-pass")
	cfg, err = Load(path)
	if err != nil {
		t.Fatalf("Load() with env error = %v", err)
	}
	if cfg.SMTP.ClientUsername != "env-user" {
		t.Fatalf("ClientUsername = %q, want env-user", cfg.SMTP.ClientUsername)
	}
	if cfg.SMTP.ClientPassword != "env-pass" {
		t.Fatalf("ClientPassword = %q, want env-pass", cfg.SMTP.ClientPassword)
	}
}
