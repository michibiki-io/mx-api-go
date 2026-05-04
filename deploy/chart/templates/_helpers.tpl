{{/*
Expand the name of the chart.
*/}}
{{- define "mx-api-go.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "mx-api-go.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "mx-api-go.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels.
*/}}
{{- define "mx-api-go.labels" -}}
helm.sh/chart: {{ include "mx-api-go.chart" . }}
app.kubernetes.io/name: {{ include "mx-api-go.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels.
*/}}
{{- define "mx-api-go.selectorLabels" -}}
app.kubernetes.io/name: {{ include "mx-api-go.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Create the name of the service account to use.
*/}}
{{- define "mx-api-go.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "mx-api-go.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Create the ConfigMap name.
*/}}
{{- define "mx-api-go.configMapName" -}}
{{- printf "%s-config" (include "mx-api-go.fullname" .) -}}
{{- end -}}

{{/*
Create the Secret name.
*/}}
{{- define "mx-api-go.secretName" -}}
{{- default (printf "%s-secret" (include "mx-api-go.fullname" .)) .Values.existingSecret -}}
{{- end -}}

{{/*
Normalize the context path for probes and notes.
*/}}
{{- define "mx-api-go.contextPath" -}}
{{- $raw := default "/" (index .Values.config "CONTEXT_PATH") -}}
{{- $trimmed := trim $raw -}}
{{- if or (eq $trimmed "") (eq $trimmed "/") -}}
/
{{- else if hasPrefix "/" $trimmed -}}
{{- trimSuffix "/" $trimmed -}}
{{- else -}}
/{{ trimSuffix "/" $trimmed }}
{{- end -}}
{{- end -}}

{{/*
Return the health path. The application exposes both /helthz and /healthz.
Use the documented /helthz path here.
*/}}
{{- define "mx-api-go.healthPath" -}}
{{- $base := include "mx-api-go.contextPath" . -}}
{{- if eq $base "/" -}}
/helthz
{{- else -}}
{{- printf "%s/helthz" $base -}}
{{- end -}}
{{- end -}}

{{/*
Return whether a writable data volume is needed.
*/}}
{{- define "mx-api-go.dataVolumeEnabled" -}}
{{- $storageType := lower (default "sqlite" (index .Values.config "MX_API_AUDIT_STORAGE_TYPE")) -}}
{{- $driver := lower (default $storageType (index .Values.config "DB_DRIVER")) -}}
{{- if eq $driver "postgresql" -}}
{{- $driver = "postgres" -}}
{{- end -}}
{{- if eq $driver "sqlite" -}}
true
{{- else -}}
false
{{- end -}}
{{- end -}}

{{/*
Return the writable data mount path.
*/}}
{{- define "mx-api-go.dataMountPath" -}}
{{- default "/var/lib/mx-api" .Values.persistence.mountPath -}}
{{- end -}}

{{/*
Return a SQLite DSN from the legacy audit storage path.
*/}}
{{- define "mx-api-go.sqliteDSN" -}}
{{- $path := trim (default "/var/lib/mx-api/audit.db" .) -}}
{{- if eq $path "" -}}
file:/var/lib/mx-api/audit.db?cache=shared&mode=rwc&_journal_mode=WAL&_busy_timeout=5000
{{- else if hasPrefix "file:" $path -}}
{{- $path -}}
{{- else if eq $path ":memory:" -}}
file::memory:?cache=shared
{{- else -}}
{{- printf "file:%s?cache=shared&mode=rwc&_journal_mode=WAL&_busy_timeout=5000" $path -}}
{{- end -}}
{{- end -}}

{{/*
Render a generated baseline config.yaml when configFile.data is empty.
Environment variables from .Values.config still override these values at runtime.
*/}}
{{- define "mx-api-go.renderConfigFile" -}}
{{- if not (empty .Values.configFile.data) -}}
{{- toYaml .Values.configFile.data -}}
{{- else -}}
{{- $contextPath := include "mx-api-go.contextPath" . -}}
{{- $adminBasePath := default "/admin" (index .Values.config "MX_API_ADMIN_BASE_PATH") -}}
{{- $auditPath := default "/var/lib/mx-api/audit.db" (index .Values.config "MX_API_AUDIT_SQLITE_PATH") -}}
{{- $sqliteDSN := include "mx-api-go.sqliteDSN" $auditPath -}}
{{- $dbDriver := default (default "sqlite" (index .Values.config "MX_API_AUDIT_STORAGE_TYPE")) (index .Values.config "DB_DRIVER") -}}
{{- $dbDSNDefault := "" -}}
{{- if eq (lower $dbDriver) "sqlite" -}}
{{- $dbDSNDefault = $sqliteDSN -}}
{{- end -}}
{{- $dbDSN := default $dbDSNDefault (index .Values.config "DB_DSN") -}}
server:
  context_path: {{ $contextPath | quote }}
  port: 8080
  mode: {{ default "release" (index .Values.config "MODE") | quote }}
security:
  allowed_origins:
    - "www.example.com"
    - "localhost:5173"
    - "127.0.0.1:5173"
  rate_limit:
    enabled: true
    requests_per_minute: 60
    failure_requests_per_minute: 20
  idempotency:
    enabled: true
    ttl_seconds: 600
admin:
  dashboard:
    enabled: true
    base_path: {{ $adminBasePath | quote }}
    timestamp_format: {{ default "2006-01-02 15:04:05 MST" (index .Values.config "MX_API_ADMIN_AUDIT_TIMESTAMP_FORMAT") | quote }}
    timestamp_timezone: {{ default "Asia/Tokyo" (index .Values.config "MX_API_ADMIN_AUDIT_TIMESTAMP_TIMEZONE") | quote }}
  auth:
    mode: {{ default "header" (index .Values.config "MX_API_ADMIN_AUTH_MODE") | quote }}
    user_header: {{ default "X-Forwarded-User" (index .Values.config "MX_API_ADMIN_AUTH_USER_HEADER") | quote }}
    email_header: {{ default "X-Forwarded-Email" (index .Values.config "MX_API_ADMIN_AUTH_EMAIL_HEADER") | quote }}
    groups_header: {{ default "X-Forwarded-Groups" (index .Values.config "MX_API_ADMIN_AUTH_GROUPS_HEADER") | quote }}
    allowed_users: []
    allowed_groups:
      - "mx-api-admins"
database:
  driver: {{ $dbDriver | quote }}
  dsn: {{ $dbDSN | quote }}
  max_open_conns: {{ default "20" (index .Values.config "DB_MAX_OPEN_CONNS") }}
  max_idle_conns: {{ default "10" (index .Values.config "DB_MAX_IDLE_CONNS") }}
  conn_max_lifetime: {{ default "30m" (index .Values.config "DB_CONN_MAX_LIFETIME") }}
  conn_max_idle_time: {{ default "5m" (index .Values.config "DB_CONN_MAX_IDLE_TIME") }}
audit:
  enabled: true
  async:
    enabled: {{ eq (lower (default "true" (index .Values.config "AUDIT_ASYNC_ENABLED"))) "true" }}
    channel_size: {{ default "10000" (index .Values.config "AUDIT_CHANNEL_SIZE") }}
    batch_size: {{ default "500" (index .Values.config "AUDIT_BATCH_SIZE") }}
    flush_interval: {{ default "100ms" (index .Values.config "AUDIT_FLUSH_INTERVAL") }}
    shutdown_flush_timeout: {{ default "5s" (index .Values.config "AUDIT_SHUTDOWN_FLUSH_TIMEOUT") }}
    drop_on_full: {{ eq (lower (default "false" (index .Values.config "AUDIT_DROP_ON_FULL"))) "true" }}
    retry_max_attempts: {{ default "3" (index .Values.config "AUDIT_RETRY_MAX_ATTEMPTS") }}
    retry_initial_backoff: {{ default "100ms" (index .Values.config "AUDIT_RETRY_INITIAL_BACKOFF") }}
    retry_max_backoff: {{ default "2s" (index .Values.config "AUDIT_RETRY_MAX_BACKOFF") }}
    enqueue_timeout: {{ default "1s" (index .Values.config "AUDIT_ENQUEUE_TIMEOUT") }}
  storage:
    type: {{ default "sqlite" (index .Values.config "MX_API_AUDIT_STORAGE_TYPE") | quote }}
    path: {{ $auditPath | quote }}
  retention_days: 90
smtp:
  server_addr: {{ default "smtp.example.com:465" (index .Values.config "SMTP_SERVER_ADDR") | quote }}
  authentication_enabled: {{ eq (lower (default "true" (index .Values.config "SMTP_AUTHENTICATION_ENABLED"))) "true" }}
  skip_verify_cert: {{ eq (lower (default "false" (index .Values.config "SMTP_SKIP_VERIFY_CERT"))) "true" }}
  tls_mode: {{ default "implicit" (index .Values.config "SMTP_TLS_MODE") | quote }}
mail:
  from: {{ default "noreply@example.com" (index .Values.config "CONTACT_REPLY_EMAIL") | quote }}
  recipients:
    - {{ default "contact@example.com" (index .Values.config "CONTACT_REPLY_BCC_EMAIL") | quote }}
  subject: {{ default "We received an inquiry from a customer" (index .Values.config "EMAIL_SUBJECT") | quote }}
  template_path: {{ default "default_mail_template.html" (index .Values.config "MAIL_TEMPLATE_PATH") | quote }}
  contact_name: {{ default "Example Site" (index .Values.config "HOMEPAGE_NAME") | quote }}
  homepage_url: {{ default "https://www.example.com" (index .Values.config "HOMEPAGE_URL") | quote }}
  timezone: {{ default "Asia/Tokyo" (index .Values.config "MAIL_TIMEZONE") | quote }}
  submitted_at_format: {{ default "2006-01-02 15:04:05 MST" (index .Values.config "MAIL_SUBMITTED_AT_FORMAT") | quote }}
form:
  fields:
    - name: "name"
      label: "Name"
      type: "text"
      required: true
      max_length: 255
      rules: ["required"]
    - name: "email"
      label: "Email"
      type: "email"
      required: true
      max_length: 320
      rules: ["required", "email"]
    - name: "tel"
      label: "Tel"
      type: "tel"
      max_length: 32
      rules: ["jp_phone"]
    - name: "organization"
      label: "Organization"
      type: "text"
      max_length: 255
    - name: "subject"
      label: "Subject"
      type: "text"
      required: true
      max_length: 200
      rules: ["required"]
    - name: "message"
      label: "Message"
      type: "textarea"
      required: true
      max_length: 4000
      rules: ["required", "message_length"]
validation:
  required:
    tag: "required"
    code: "validation_required"
    message: "cannot be blank"
  email:
    tag: "email"
    code: "validation_is_email"
    message: "must be a valid email address"
  jp_phone:
    tag: "jp_phone"
    code: "validation_is_phone_number"
    message: "must be a valid phone number in Japan"
  message_length:
    tag: "max=4000"
    code: "validation_max"
    message: "must be 4000 characters or less"
{{- end -}}
{{- end -}}
