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
	if cfg.Form.Fields[0].MaxLength != 255 {
		t.Fatalf("field MaxLength = %d, want default 255", cfg.Form.Fields[0].MaxLength)
	}
	if cfg.Mail.Timezone != "Asia/Tokyo" {
		t.Fatalf("Mail.Timezone = %q, want Asia/Tokyo", cfg.Mail.Timezone)
	}
	if cfg.Mail.SubmittedAtFormat != "2006/01/02 15:04:05 MST" {
		t.Fatalf("Mail.SubmittedAtFormat = %q, want configured format", cfg.Mail.SubmittedAtFormat)
	}
}

func TestLoadAppliesConfigOverrideFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	overridePath := filepath.Join(dir, "config.override.yaml")
	if err := os.WriteFile(path, []byte(`
mail:
  subject: Base subject
  extra:
    keep: keep-base
    remove: remove-base
validation:
  email:
    tag: email
    code: validation_is_email
    message: default email
  custom:
    tag: required
    code: custom_code
    message: custom
form:
  fields:
    - name: email
      label: Email
      type: email
      required: true
      rules: [required, email]
    - name: organization
      label: Organization
      type: text
    - name: subject
      label: Subject
      type: text
      required: true
      rules: [required]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overridePath, []byte(`
mail:
  subject: Override subject
  extra:
    remove: null
    add: added
validation:
  email:
    message: custom email
  custom: null
form:
  fields:
    - name: organization
      _delete: true
    - name: subject
      required: false
      rules: []
    - name: plan
      label: Plan
      type: select
      options: [basic, business]
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Mail.Subject != "Override subject" {
		t.Fatalf("Mail.Subject = %q, want Override subject", cfg.Mail.Subject)
	}
	if cfg.Mail.Extra["keep"] != "keep-base" {
		t.Fatalf("Mail.Extra[keep] = %q, want keep-base", cfg.Mail.Extra["keep"])
	}
	if _, ok := cfg.Mail.Extra["remove"]; ok {
		t.Fatal("Mail.Extra[remove] should be deleted")
	}
	if cfg.Mail.Extra["add"] != "added" {
		t.Fatalf("Mail.Extra[add] = %q, want added", cfg.Mail.Extra["add"])
	}

	emailRule := cfg.Validation["email"]
	if emailRule.Tag != "email" || emailRule.Code != "validation_is_email" || emailRule.Message != "custom email" {
		t.Fatalf("email validation override was not merged: %#v", emailRule)
	}
	if _, ok := cfg.Validation["custom"]; ok {
		t.Fatal("custom validation should be deleted")
	}

	if _, ok := fieldByName(cfg.Form.Fields, "organization"); ok {
		t.Fatal("organization field should be deleted")
	}
	subject, ok := fieldByName(cfg.Form.Fields, "subject")
	if !ok {
		t.Fatal("subject field should exist")
	}
	if subject.Required {
		t.Fatal("subject required should be overridden to false")
	}
	if len(subject.Rules) != 0 {
		t.Fatalf("subject rules = %#v, want empty", subject.Rules)
	}
	plan, ok := fieldByName(cfg.Form.Fields, "plan")
	if !ok {
		t.Fatal("plan field should be appended")
	}
	if plan.Type != "select" || len(plan.Options) != 2 {
		t.Fatalf("plan field was not appended correctly: %#v", plan)
	}
}

func TestLoadIgnoresMissingFieldDeleteInOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	overridePath := filepath.Join(dir, "config.override.yaml")
	if err := os.WriteFile(path, []byte(`
form:
  fields: []
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overridePath, []byte(`
form:
  fields:
    - name: organization
      _delete: true
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Form.Fields) != 0 {
		t.Fatalf("fields = %#v, want empty", cfg.Form.Fields)
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

func TestLoadAppliesAdminAndAuditEnvironment(t *testing.T) {
	t.Setenv("MX_API_ADMIN_DASHBOARD_ENABLED", "false")
	t.Setenv("MX_API_ADMIN_BASE_PATH", "ops")
	t.Setenv("MX_API_ADMIN_AUTH_MODE", "none")
	t.Setenv("MX_API_ADMIN_AUTH_USER_HEADER", "Remote-User")
	t.Setenv("MX_API_ADMIN_AUTH_EMAIL_HEADER", "Remote-Email")
	t.Setenv("MX_API_ADMIN_AUTH_GROUPS_HEADER", "Remote-Groups")
	t.Setenv("MX_API_ADMIN_ALLOWED_USERS", "alice@example.com,bob@example.com")
	t.Setenv("MX_API_ADMIN_ALLOWED_GROUPS", "admins, operators")
	t.Setenv("MX_API_AUDIT_ENABLED", "false")
	t.Setenv("MX_API_AUDIT_STORAGE_TYPE", "sqlite")
	t.Setenv("MX_API_AUDIT_SQLITE_PATH", "/tmp/test-audit.db")
	t.Setenv("MX_API_AUDIT_RETENTION_DAYS", "7")
	t.Setenv("MX_API_RATE_LIMIT_ENABLED", "false")
	t.Setenv("MX_API_RATE_LIMIT_REQUESTS_PER_MINUTE", "11")
	t.Setenv("MX_API_RATE_LIMIT_FAILURE_REQUESTS_PER_MINUTE", "3")
	t.Setenv("MX_API_IDEMPOTENCY_ENABLED", "false")
	t.Setenv("MX_API_IDEMPOTENCY_TTL_SECONDS", "120")

	cfg, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Admin.Dashboard.Enabled {
		t.Fatal("admin dashboard should be disabled from env")
	}
	if cfg.Admin.Dashboard.BasePath != "/ops" {
		t.Fatalf("BasePath = %q, want /ops", cfg.Admin.Dashboard.BasePath)
	}
	if cfg.Admin.Auth.Mode != "none" || cfg.Admin.Auth.UserHeader != "Remote-User" || cfg.Admin.Auth.EmailHeader != "Remote-Email" || cfg.Admin.Auth.GroupsHeader != "Remote-Groups" {
		t.Fatalf("admin auth env not applied: %#v", cfg.Admin.Auth)
	}
	if len(cfg.Admin.Auth.AllowedUsers) != 2 || cfg.Admin.Auth.AllowedUsers[0] != "alice@example.com" {
		t.Fatalf("AllowedUsers = %#v", cfg.Admin.Auth.AllowedUsers)
	}
	if len(cfg.Admin.Auth.AllowedGroups) != 2 || cfg.Admin.Auth.AllowedGroups[1] != "operators" {
		t.Fatalf("AllowedGroups = %#v", cfg.Admin.Auth.AllowedGroups)
	}
	if cfg.Audit.Enabled || cfg.Audit.Storage.Path != "/tmp/test-audit.db" || cfg.Audit.RetentionDays != 7 {
		t.Fatalf("audit env not applied: %#v", cfg.Audit)
	}
	if cfg.Security.RateLimit.Enabled || cfg.Security.RateLimit.RequestsPerMinute != 11 || cfg.Security.RateLimit.FailureRequestsPerMinute != 3 {
		t.Fatalf("rate limit env not applied: %#v", cfg.Security.RateLimit)
	}
	if cfg.Security.Idempotency.Enabled || cfg.Security.Idempotency.TTLSeconds != 120 {
		t.Fatalf("idempotency env not applied: %#v", cfg.Security.Idempotency)
	}
}

func fieldByName(fields []FieldConfig, name string) (FieldConfig, bool) {
	for _, field := range fields {
		if field.Name == name {
			return field, true
		}
	}
	return FieldConfig{}, false
}
