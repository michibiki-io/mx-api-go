package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultConfigPath         = "./configs/config.yaml"
	defaultConfigOverrideName = "config.override.yaml"
)

type Config struct {
	Server     ServerConfig              `yaml:"server" json:"server"`
	Security   SecurityConfig            `yaml:"security" json:"security"`
	Admin      AdminConfig               `yaml:"admin" json:"admin"`
	Audit      AuditConfig               `yaml:"audit" json:"audit"`
	SMTP       SMTPConfig                `yaml:"smtp" json:"-"`
	Mail       MailConfig                `yaml:"mail" json:"mail"`
	Form       FormConfig                `yaml:"form" json:"form"`
	Validation map[string]ValidationRule `yaml:"validation" json:"validation"`
}

type ServerConfig struct {
	Bind              string        `yaml:"bind" json:"bind"`
	Port              int           `yaml:"port" json:"port"`
	ContextPath       string        `yaml:"context_path" json:"contextPath"`
	Mode              string        `yaml:"mode" json:"mode"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout" json:"-"`
	ShutdownTimeout   time.Duration `yaml:"shutdown_timeout" json:"-"`
}

func (c ServerConfig) Address() string {
	return net.JoinHostPort(c.Bind, strconv.Itoa(c.Port))
}

type SecurityConfig struct {
	AllowedOrigins []string          `yaml:"allowed_origins" json:"allowedOrigins"`
	RateLimit      RateLimitConfig   `yaml:"rate_limit" json:"rateLimit"`
	Idempotency    IdempotencyConfig `yaml:"idempotency" json:"idempotency"`
}

type RateLimitConfig struct {
	Enabled                  bool `yaml:"enabled" json:"enabled"`
	RequestsPerMinute        int  `yaml:"requests_per_minute" json:"requestsPerMinute"`
	FailureRequestsPerMinute int  `yaml:"failure_requests_per_minute" json:"failureRequestsPerMinute"`
}

type IdempotencyConfig struct {
	Enabled    bool `yaml:"enabled" json:"enabled"`
	TTLSeconds int  `yaml:"ttl_seconds" json:"ttlSeconds"`
}

type AdminConfig struct {
	Dashboard AdminDashboardConfig `yaml:"dashboard" json:"dashboard"`
	Auth      AdminAuthConfig      `yaml:"auth" json:"auth"`
}

type AdminDashboardConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	BasePath string `yaml:"base_path" json:"basePath"`
}

type AdminAuthConfig struct {
	Mode          string   `yaml:"mode" json:"mode"`
	UserHeader    string   `yaml:"user_header" json:"userHeader"`
	EmailHeader   string   `yaml:"email_header" json:"emailHeader"`
	GroupsHeader  string   `yaml:"groups_header" json:"groupsHeader"`
	AllowedUsers  []string `yaml:"allowed_users" json:"allowedUsers"`
	AllowedGroups []string `yaml:"allowed_groups" json:"allowedGroups"`
}

type AuditConfig struct {
	Enabled       bool               `yaml:"enabled" json:"enabled"`
	Storage       AuditStorageConfig `yaml:"storage" json:"storage"`
	RetentionDays int                `yaml:"retention_days" json:"retentionDays"`
}

type AuditStorageConfig struct {
	Type string `yaml:"type" json:"type"`
	Path string `yaml:"path" json:"path"`
}

type SMTPConfig struct {
	ServerAddr            string        `yaml:"server_addr"`
	AuthenticationEnabled bool          `yaml:"authentication_enabled"`
	SkipVerifyCert        bool          `yaml:"skip_verify_cert"`
	ClientUsername        string        `yaml:"client_username"`
	ClientPassword        string        `yaml:"client_password"`
	TLSMode               string        `yaml:"tls_mode"`
	Timeout               time.Duration `yaml:"timeout"`
}

type MailConfig struct {
	From                   string            `yaml:"from" json:"from"`
	Recipients             []string          `yaml:"recipients" json:"recipients"`
	Subject                string            `yaml:"subject" json:"subject"`
	TemplatePath           string            `yaml:"template_path" json:"templatePath"`
	ContactName            string            `yaml:"contact_name" json:"contactName"`
	HomepageURL            string            `yaml:"homepage_url" json:"homepageUrl"`
	Timezone               string            `yaml:"timezone" json:"timezone"`
	SubmittedAtFormat      string            `yaml:"submitted_at_format" json:"submittedAtFormat"`
	InternalErrorRecipient string            `yaml:"internal_error_recipient" json:"-"`
	Extra                  map[string]string `yaml:"extra" json:"extra"`
}

type FormConfig struct {
	Fields []FieldConfig `yaml:"fields" json:"fields"`
}

type FieldConfig struct {
	Name        string   `yaml:"name" json:"name"`
	Label       string   `yaml:"label" json:"label"`
	Type        string   `yaml:"type" json:"type"`
	Required    bool     `yaml:"required" json:"required"`
	MaxLength   int      `yaml:"max_length" json:"maxLength"`
	Default     string   `yaml:"default" json:"default"`
	Placeholder string   `yaml:"placeholder" json:"placeholder"`
	Help        string   `yaml:"help" json:"help"`
	Options     []string `yaml:"options" json:"options"`
	Rules       []string `yaml:"rules" json:"rules"`
}

type ValidationRule struct {
	Tag     string `yaml:"tag" json:"tag"`
	Code    string `yaml:"code" json:"code"`
	Message string `yaml:"message" json:"message"`
	Pattern string `yaml:"pattern" json:"pattern"`
}

func Load(path string) (*Config, error) {
	cfg := Default()
	configPath := firstNonEmpty(path, os.Getenv("MX_API_CONFIG"), os.Getenv("CONFIG_PATH"))
	if configPath == "" {
		configPath = defaultConfigPath
	}

	if fileExists(configPath) {
		raw, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("read config: %w", err)
		}
		if err := yaml.Unmarshal(raw, cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}

	overridePath := firstNonEmpty(os.Getenv("MX_API_CONFIG_OVERRIDE"), os.Getenv("CONFIG_OVERRIDE_PATH"), defaultOverridePath(configPath))
	if fileExists(overridePath) {
		raw, err := os.ReadFile(overridePath)
		if err != nil {
			return nil, fmt.Errorf("read config override: %w", err)
		}
		if err := applyOverride(cfg, raw); err != nil {
			return nil, fmt.Errorf("parse config override: %w", err)
		}
	}

	applyEnv(cfg)
	normalize(cfg)
	return cfg, nil
}

func defaultOverridePath(configPath string) string {
	if configPath == "" {
		return filepath.Join("./configs", defaultConfigOverrideName)
	}
	return filepath.Join(filepath.Dir(configPath), defaultConfigOverrideName)
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Bind:              "0.0.0.0",
			Port:              8080,
			Mode:              "release",
			ReadHeaderTimeout: 5 * time.Second,
			ShutdownTimeout:   10 * time.Second,
		},
		Security: SecurityConfig{
			AllowedOrigins: []string{"127.0.0.1", "localhost", "localhost:5173"},
			RateLimit: RateLimitConfig{
				Enabled:                  true,
				RequestsPerMinute:        60,
				FailureRequestsPerMinute: 20,
			},
			Idempotency: IdempotencyConfig{
				Enabled:    true,
				TTLSeconds: 600,
			},
		},
		Admin: AdminConfig{
			Dashboard: AdminDashboardConfig{
				Enabled:  true,
				BasePath: "/admin",
			},
			Auth: AdminAuthConfig{
				Mode:          "header",
				UserHeader:    "X-Forwarded-User",
				EmailHeader:   "X-Forwarded-Email",
				GroupsHeader:  "X-Forwarded-Groups",
				AllowedGroups: []string{"mx-api-admins"},
			},
		},
		Audit: AuditConfig{
			Enabled: true,
			Storage: AuditStorageConfig{
				Type: "sqlite",
				Path: "/tmp/mx-api-audit.db",
			},
			RetentionDays: 90,
		},
		SMTP: SMTPConfig{
			AuthenticationEnabled: true,
			TLSMode:               "implicit",
			Timeout:               10 * time.Second,
		},
		Mail: MailConfig{
			From:                   "noreply@example.com",
			Recipients:             []string{"bcc@example.com"},
			Subject:                "We received an inquiry from a customer",
			TemplatePath:           "default_mail_template.html",
			ContactName:            "Homepage",
			HomepageURL:            "https://example.com",
			Timezone:               "UTC",
			SubmittedAtFormat:      "2006-01-02 15:04:05 MST",
			InternalErrorRecipient: "bcc@example.com",
			Extra:                  map[string]string{},
		},
		Form: FormConfig{Fields: []FieldConfig{
			{Name: "name", Label: "Name", Type: "text", Required: true, MaxLength: 255, Rules: []string{"required"}},
			{Name: "email", Label: "Email", Type: "email", Required: true, MaxLength: 320, Rules: []string{"required", "email"}},
			{Name: "tel", Label: "Tel", Type: "tel", MaxLength: 32, Rules: []string{"jp_phone"}},
			{Name: "organization", Label: "Organization", Type: "text", MaxLength: 255},
			{Name: "subject", Label: "Subject", Type: "text", MaxLength: 200},
			{Name: "message", Label: "Message", Type: "textarea", Required: true, MaxLength: 4000, Rules: []string{"required"}},
		}},
		Validation: map[string]ValidationRule{
			"required": {Tag: "required", Code: "validation_required", Message: "cannot be blank"},
			"email":    {Tag: "email", Code: "validation_is_email", Message: "must be a valid email address"},
			"jp_phone": {Tag: "jp_phone", Code: "validation_is_phone_number", Message: "must be a valid phone number in Japan"},
		},
	}
}

func applyOverride(cfg *Config, raw []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return err
	}
	if len(root.Content) == 0 || isNullNode(root.Content[0]) {
		return nil
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return fmt.Errorf("root must be a mapping")
	}

	fieldsNode := extractFormFieldsNode(doc)
	mailExtraNode := extractMailExtraNode(doc)
	validationNode := mappingValue(doc, "validation")
	removeMappingKey(doc, "validation")

	if len(doc.Content) > 0 {
		if err := doc.Decode(cfg); err != nil {
			return err
		}
	}
	if err := applyValidationOverride(cfg, validationNode); err != nil {
		return err
	}
	if err := applyMailExtraOverride(cfg, mailExtraNode); err != nil {
		return err
	}
	if err := applyFieldOverrides(cfg, fieldsNode); err != nil {
		return err
	}
	return nil
}

func extractFormFieldsNode(doc *yaml.Node) *yaml.Node {
	formNode := mappingValue(doc, "form")
	if formNode == nil || formNode.Kind != yaml.MappingNode {
		return nil
	}

	fieldsNode := mappingValue(formNode, "fields")
	removeMappingKey(formNode, "fields")
	if len(formNode.Content) == 0 {
		removeMappingKey(doc, "form")
	}
	return fieldsNode
}

func extractMailExtraNode(doc *yaml.Node) *yaml.Node {
	mailNode := mappingValue(doc, "mail")
	if mailNode == nil || mailNode.Kind != yaml.MappingNode {
		return nil
	}

	extraNode := mappingValue(mailNode, "extra")
	removeMappingKey(mailNode, "extra")
	if len(mailNode.Content) == 0 {
		removeMappingKey(doc, "mail")
	}
	return extraNode
}

func applyValidationOverride(cfg *Config, node *yaml.Node) error {
	if node == nil {
		return nil
	}
	if isNullNode(node) {
		cfg.Validation = map[string]ValidationRule{}
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("validation must be a mapping")
	}
	if cfg.Validation == nil {
		cfg.Validation = map[string]ValidationRule{}
	}

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		key := keyNode.Value
		if isNullNode(valueNode) {
			delete(cfg.Validation, key)
			continue
		}

		rule := cfg.Validation[key]
		if err := valueNode.Decode(&rule); err != nil {
			return fmt.Errorf("validation.%s: %w", key, err)
		}
		cfg.Validation[key] = rule
	}
	return nil
}

func applyMailExtraOverride(cfg *Config, node *yaml.Node) error {
	if node == nil {
		return nil
	}
	if isNullNode(node) {
		cfg.Mail.Extra = map[string]string{}
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("mail.extra must be a mapping")
	}
	if cfg.Mail.Extra == nil {
		cfg.Mail.Extra = map[string]string{}
	}

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		key := keyNode.Value
		if isNullNode(valueNode) {
			delete(cfg.Mail.Extra, key)
			continue
		}

		var value string
		if err := valueNode.Decode(&value); err != nil {
			return fmt.Errorf("mail.extra.%s: %w", key, err)
		}
		cfg.Mail.Extra[key] = value
	}
	return nil
}

func applyFieldOverrides(cfg *Config, node *yaml.Node) error {
	if node == nil {
		return nil
	}
	if isNullNode(node) {
		cfg.Form.Fields = nil
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("form.fields must be a sequence")
	}

	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return fmt.Errorf("form.fields item at line %d must be a mapping", item.Line)
		}

		var meta struct {
			Name   string `yaml:"name"`
			Delete bool   `yaml:"_delete"`
		}
		if err := item.Decode(&meta); err != nil {
			return fmt.Errorf("form.fields item at line %d: %w", item.Line, err)
		}
		meta.Name = strings.TrimSpace(meta.Name)
		if meta.Name == "" {
			return fmt.Errorf("form.fields item at line %d must include name", item.Line)
		}

		fieldIndex := findFieldIndex(cfg.Form.Fields, meta.Name)
		if meta.Delete {
			if fieldIndex >= 0 {
				cfg.Form.Fields = append(cfg.Form.Fields[:fieldIndex], cfg.Form.Fields[fieldIndex+1:]...)
			}
			continue
		}

		if fieldIndex >= 0 {
			field := cfg.Form.Fields[fieldIndex]
			if err := item.Decode(&field); err != nil {
				return fmt.Errorf("form.fields.%s: %w", meta.Name, err)
			}
			cfg.Form.Fields[fieldIndex] = field
			continue
		}

		var field FieldConfig
		if err := item.Decode(&field); err != nil {
			return fmt.Errorf("form.fields.%s: %w", meta.Name, err)
		}
		cfg.Form.Fields = append(cfg.Form.Fields, field)
	}
	return nil
}

func findFieldIndex(fields []FieldConfig, name string) int {
	for i := range fields {
		if strings.EqualFold(fields[i].Name, name) {
			return i
		}
	}
	return -1
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func removeMappingKey(node *yaml.Node, key string) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			return true
		}
	}
	return false
}

func isNullNode(node *yaml.Node) bool {
	return node == nil || (node.Kind == yaml.ScalarNode && node.Tag == "!!null")
}

func applyEnv(cfg *Config) {
	setString(&cfg.Server.ContextPath, "CONTEXT_PATH")
	setString(&cfg.Server.Mode, "MODE")
	setInt(&cfg.Server.Port, "BIND_PORT")

	if v := os.Getenv("ALLOWED_ORIGINS"); v != "" {
		cfg.Security.AllowedOrigins = splitCSV(v)
	}
	setBool(&cfg.Security.RateLimit.Enabled, "MX_API_RATE_LIMIT_ENABLED")
	setInt(&cfg.Security.RateLimit.RequestsPerMinute, "MX_API_RATE_LIMIT_REQUESTS_PER_MINUTE")
	setInt(&cfg.Security.RateLimit.FailureRequestsPerMinute, "MX_API_RATE_LIMIT_FAILURE_REQUESTS_PER_MINUTE")
	setBool(&cfg.Security.Idempotency.Enabled, "MX_API_IDEMPOTENCY_ENABLED")
	setInt(&cfg.Security.Idempotency.TTLSeconds, "MX_API_IDEMPOTENCY_TTL_SECONDS")

	setBool(&cfg.Admin.Dashboard.Enabled, "MX_API_ADMIN_DASHBOARD_ENABLED")
	setString(&cfg.Admin.Dashboard.BasePath, "MX_API_ADMIN_BASE_PATH")
	setString(&cfg.Admin.Auth.Mode, "MX_API_ADMIN_AUTH_MODE")
	setString(&cfg.Admin.Auth.UserHeader, "MX_API_ADMIN_AUTH_USER_HEADER")
	setString(&cfg.Admin.Auth.EmailHeader, "MX_API_ADMIN_AUTH_EMAIL_HEADER")
	setString(&cfg.Admin.Auth.GroupsHeader, "MX_API_ADMIN_AUTH_GROUPS_HEADER")
	if v := os.Getenv("MX_API_ADMIN_ALLOWED_USERS"); v != "" {
		cfg.Admin.Auth.AllowedUsers = splitCSV(v)
	}
	if v := os.Getenv("MX_API_ADMIN_ALLOWED_GROUPS"); v != "" {
		cfg.Admin.Auth.AllowedGroups = splitCSV(v)
	}

	setBool(&cfg.Audit.Enabled, "MX_API_AUDIT_ENABLED")
	setString(&cfg.Audit.Storage.Type, "MX_API_AUDIT_STORAGE_TYPE")
	setString(&cfg.Audit.Storage.Path, "MX_API_AUDIT_SQLITE_PATH")
	setInt(&cfg.Audit.RetentionDays, "MX_API_AUDIT_RETENTION_DAYS")

	setString(&cfg.SMTP.ServerAddr, "SMTP_SERVER_ADDR")
	setBool(&cfg.SMTP.AuthenticationEnabled, "SMTP_AUTHENTICATION_ENABLED")
	setBool(&cfg.SMTP.SkipVerifyCert, "SMTP_SKIP_VERIFY_CERT")
	cfg.SMTP.ClientUsername = os.Getenv("SMTP_CLIENT_USERNAME")
	cfg.SMTP.ClientPassword = os.Getenv("SMTP_CLIENT_PASSWORD")

	setString(&cfg.Mail.From, "CONTACT_REPLY_EMAIL")
	if v := os.Getenv("CONTACT_REPLY_BCC_EMAIL"); v != "" {
		recipients := splitCSV(v)
		if len(recipients) > 0 {
			cfg.Mail.Recipients = recipients
			cfg.Mail.InternalErrorRecipient = recipients[0]
		}
	}
	setString(&cfg.Mail.Subject, "EMAIL_SUBJECT")
	setString(&cfg.Mail.ContactName, "HOMEPAGE_NAME")
	setString(&cfg.Mail.HomepageURL, "HOMEPAGE_URL")
	setString(&cfg.Mail.TemplatePath, "MAIL_TEMPLATE_PATH")
	setString(&cfg.Mail.Timezone, "MAIL_TIMEZONE")
	setString(&cfg.Mail.SubmittedAtFormat, "MAIL_SUBMITTED_AT_FORMAT")

	if v := os.Getenv("CONTACT_FORM_REQUIRED_FIELD"); v != "" {
		applyLegacyRequiredFields(cfg, splitCSV(v))
	}
}

func normalize(cfg *Config) {
	cfg.Server.ContextPath = strings.TrimSpace(cfg.Server.ContextPath)
	if cfg.Server.ContextPath != "" {
		cfg.Server.ContextPath = "/" + strings.Trim(strings.TrimSpace(cfg.Server.ContextPath), "/")
	}
	if cfg.Server.Bind == "" {
		cfg.Server.Bind = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	cfg.Admin.Dashboard.BasePath = normalizePath(cfg.Admin.Dashboard.BasePath, "/admin")
	cfg.Admin.Auth.Mode = strings.ToLower(strings.TrimSpace(cfg.Admin.Auth.Mode))
	if cfg.Admin.Auth.Mode == "" {
		cfg.Admin.Auth.Mode = "header"
	}
	if cfg.Admin.Auth.UserHeader == "" {
		cfg.Admin.Auth.UserHeader = "X-Forwarded-User"
	}
	if cfg.Admin.Auth.EmailHeader == "" {
		cfg.Admin.Auth.EmailHeader = "X-Forwarded-Email"
	}
	if cfg.Admin.Auth.GroupsHeader == "" {
		cfg.Admin.Auth.GroupsHeader = "X-Forwarded-Groups"
	}
	cfg.Admin.Auth.AllowedUsers = cleanList(cfg.Admin.Auth.AllowedUsers)
	cfg.Admin.Auth.AllowedGroups = cleanList(cfg.Admin.Auth.AllowedGroups)
	cfg.Audit.Storage.Type = strings.ToLower(strings.TrimSpace(cfg.Audit.Storage.Type))
	if cfg.Audit.Storage.Type == "" {
		cfg.Audit.Storage.Type = "sqlite"
	}
	if cfg.Audit.Storage.Path == "" {
		cfg.Audit.Storage.Path = "/tmp/mx-api-audit.db"
	}
	if cfg.Audit.RetentionDays < 0 {
		cfg.Audit.RetentionDays = 0
	}
	if cfg.SMTP.TLSMode == "" {
		cfg.SMTP.TLSMode = "implicit"
	}
	if cfg.SMTP.Timeout == 0 {
		cfg.SMTP.Timeout = 10 * time.Second
	}
	if cfg.Security.RateLimit.RequestsPerMinute < 0 {
		cfg.Security.RateLimit.RequestsPerMinute = 0
	}
	if cfg.Security.RateLimit.FailureRequestsPerMinute < 0 {
		cfg.Security.RateLimit.FailureRequestsPerMinute = 0
	}
	if cfg.Security.Idempotency.TTLSeconds <= 0 {
		cfg.Security.Idempotency.TTLSeconds = 600
	}
	if cfg.Mail.TemplatePath != "" && !filepath.IsAbs(cfg.Mail.TemplatePath) {
		cfg.Mail.TemplatePath = filepath.Clean(cfg.Mail.TemplatePath)
	}
	if cfg.Mail.InternalErrorRecipient == "" && len(cfg.Mail.Recipients) > 0 {
		cfg.Mail.InternalErrorRecipient = cfg.Mail.Recipients[0]
	}
	if cfg.Mail.Timezone == "" {
		cfg.Mail.Timezone = "UTC"
	}
	if cfg.Mail.SubmittedAtFormat == "" {
		cfg.Mail.SubmittedAtFormat = "2006-01-02 15:04:05 MST"
	}
	for i := range cfg.Form.Fields {
		field := &cfg.Form.Fields[i]
		field.Name = strings.TrimSpace(field.Name)
		if field.Label == "" {
			field.Label = field.Name
		}
		if field.Type == "" {
			field.Type = "text"
		}
		if field.MaxLength <= 0 {
			field.MaxLength = defaultFieldMaxLength(*field)
		}
		if field.Required && !slices.Contains(field.Rules, "required") {
			field.Rules = append([]string{"required"}, field.Rules...)
		}
	}
}

func defaultFieldMaxLength(field FieldConfig) int {
	switch {
	case strings.EqualFold(field.Name, "subject"):
		return 200
	case strings.EqualFold(field.Type, "textarea"):
		return 4000
	case strings.EqualFold(field.Type, "email"):
		return 320
	case strings.EqualFold(field.Type, "tel"):
		return 32
	default:
		return 255
	}
}

func applyLegacyRequiredFields(cfg *Config, fields []string) {
	for _, requiredField := range fields {
		requiredField = strings.ToLower(strings.TrimSpace(requiredField))
		for i := range cfg.Form.Fields {
			if strings.EqualFold(cfg.Form.Fields[i].Name, requiredField) {
				cfg.Form.Fields[i].Required = true
				if !slices.Contains(cfg.Form.Fields[i].Rules, "required") {
					cfg.Form.Fields[i].Rules = append([]string{"required"}, cfg.Form.Fields[i].Rules...)
				}
			}
		}
	}
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func cleanList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func normalizePath(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if value == "/" {
		return "/"
	}
	return "/" + strings.Trim(value, "/")
}

func setString(target *string, key string) {
	if v := os.Getenv(key); v != "" {
		*target = v
	}
}

func setInt(target *int, key string) {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			*target = parsed
		}
	}
}

func setBool(target *bool, key string) {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			*target = parsed
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
