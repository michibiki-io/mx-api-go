<p align="center">
  <img src="docs/images/mx-api-go-logo-str.svg" alt="mx-api-go" width="420">
</p>

`mx-api` is a small Go service for validating contact-form payloads and sending HTML mail. The binary name is `mx-api`; the Go module is `github.com/michibiki-io/mx-api-go`.

## Table of Contents

- [Security Notice](#security-notice)
- [English](#english)
  - [API Reference](#english-api-reference)
  - [Configuration](#english-configuration)
  - [Admin Dashboard and Audit Logs](#english-admin-dashboard-and-audit-logs)
  - [Mail Templates](#english-mail-templates)
  - [Development](#english-development)
  - [Build](#english-build)
- [日本語](#日本語)
  - [API リファレンス](#api-リファレンス)
  - [設定](#設定)
  - [管理ダッシュボードと監査ログ](#管理ダッシュボードと監査ログ)
  - [Validation ルール](#validation-ルール)
  - [メールテンプレート](#メールテンプレート)
  - [開発起動](#開発起動)
  - [ビルド](#ビルド)

## Security Notice

> [!WARNING]
> `mx-api` does not perform authentication or authorization for API use.
> If this API is exposed without protection, attackers can abuse the sendmail endpoint and turn the service into a spam relay.
> Do not expose it directly to the public internet without a protective layer such as [turnstile-appcheck-gateway](https://github.com/michibiki-io/turnstile-appcheck-gateway), Basic Auth, an ingress controller, or reverse-proxy access control.
>
> `mx-api` は API 利用に対する認証・認可を行いません。
> 保護なしで公開すると、sendmail endpoint を悪用され、迷惑メール送信の踏み台になる可能性があります。
> 公開環境では直接公開せず、[turnstile-appcheck-gateway](https://github.com/michibiki-io/turnstile-appcheck-gateway), Basic Auth, ingress controller, reverse proxy の access control などで前段保護してください。

## English

### English API Reference

The service exposes the following endpoints:

- `GET /` -> `{"status":"Ok","version":"<version>"}`
- `GET /helthz` -> `{"status":"Ok","version":"<version>"}`
- `GET /api/v1/validate` -> `{"status":"Ok","version":"<version>"}`
- `POST /api/v1/validate` -> `{"status":"Ok","version":"<version>"}` when validation passes
- `GET /api/v1/sendmail` -> `{"status":"Ok","version":"<version>"}`
- `POST /api/v1/sendmail` -> `{"status":"Ok","version":"<version>"}` when mail is sent
- `GET /api/v1/schema` -> includes `version`

When the admin dashboard is enabled, the following administrator endpoints are also exposed under the service context path:

- `GET /admin/` -> embedded admin dashboard UI by default
- `GET /_admin/api/v1/me`
- `GET /_admin/api/v1/request-metrics`
- `GET /_admin/api/v1/audit-options`
- `GET /_admin/api/v1/audit-events`
- `GET /_admin/api/v1/audit-events/:id`
- `POST /_admin/api/v1/audit-events/reset`

Validation errors use this response shape:

```json
{
  "status": "BadRequest",
  "errors": {
    "email": {
      "code": "validation_is_email",
      "message": "must be a valid email address"
    }
  }
}
```

### English Configuration

`configs/config.yaml` is a local runtime file and is intentionally ignored by git because it may contain environment-specific values. Start from [configs/config.example.yaml](/home/staratlas@ad.michibiki.io/workspace/mx-api-go/configs/config.example.yaml), create your local config, and edit it for your environment:

```sh
cp configs/config.example.yaml configs/config.yaml
```

By default, `mx-api` tries to load `./configs/config.yaml` when it exists. Set `MX_API_CONFIG` or `CONFIG_PATH` to load another YAML file.

After loading the main config, `mx-api` also loads `config.override.yaml` from the same directory when it exists. Set `MX_API_CONFIG_OVERRIDE` or `CONFIG_OVERRIDE_PATH` to load an override file from another path. Override files are applied after `config.yaml` and before environment variables.

Use `config.override.yaml` when you only need to patch or delete a small part of the default config. Normal scalar values are overwritten, `mail.extra` and `validation` entries are merged by key, and entries set to `null` are deleted. `form.fields` is patched by `name`; `_delete: true` removes a matching field and is ignored when the field does not exist.

```yaml
form:
  fields:
    - name: "organization"
      _delete: true

    - name: "subject"
      required: false
      rules: []

validation:
  message_length: null
```

The following environment variables are supported:

- `MX_API_CONFIG`
- `CONFIG_PATH`
- `MX_API_CONFIG_OVERRIDE`
- `CONFIG_OVERRIDE_PATH`
- `CONTEXT_PATH`
- `BIND_PORT`
- `MODE`
- `ALLOWED_ORIGINS`
- `MX_API_RATE_LIMIT_ENABLED`
- `MX_API_RATE_LIMIT_REQUESTS_PER_MINUTE`
- `MX_API_RATE_LIMIT_FAILURE_REQUESTS_PER_MINUTE`
- `MX_API_IDEMPOTENCY_ENABLED`
- `MX_API_IDEMPOTENCY_TTL_SECONDS`
- `SMTP_SERVER_ADDR`
- `SMTP_AUTHENTICATION_ENABLED`
- `SMTP_SKIP_VERIFY_CERT`
- `SMTP_CLIENT_USERNAME`
- `SMTP_CLIENT_PASSWORD`
- `CONTACT_REPLY_EMAIL`
- `CONTACT_REPLY_BCC_EMAIL`
- `CONTACT_FORM_REQUIRED_FIELD`
- `EMAIL_SUBJECT`
- `HOMEPAGE_NAME`
- `HOMEPAGE_URL`
- `MAIL_TEMPLATE_PATH`
- `MAIL_TIMEZONE`
- `MAIL_SUBMITTED_AT_FORMAT`
- `MX_API_ADMIN_DASHBOARD_ENABLED`
- `MX_API_ADMIN_BASE_PATH`
- `MX_API_ADMIN_AUDIT_TIMESTAMP_FORMAT`
- `MX_API_ADMIN_AUDIT_TIMESTAMP_TIMEZONE`
- `MX_API_ADMIN_AUTH_MODE`
- `MX_API_ADMIN_AUTH_USER_HEADER`
- `MX_API_ADMIN_AUTH_EMAIL_HEADER`
- `MX_API_ADMIN_AUTH_GROUPS_HEADER`
- `MX_API_ADMIN_ALLOWED_USERS`
- `MX_API_ADMIN_ALLOWED_GROUPS`
- `MX_API_AUDIT_ENABLED`
- `MX_API_AUDIT_STORAGE_TYPE`
- `MX_API_AUDIT_SQLITE_PATH`
- `MX_API_AUDIT_RETENTION_DAYS`

Add form fields under `form.fields` and attach validation rules by name. Rules are defined under `validation` and use `go-playground/validator` tags, plus the built-in `jp_phone` rule. Every field has a `max_length`; when omitted, `mx-api` fills a conservative default by field type and enforces it server-side.

Public `POST` requests are protected by a lightweight in-memory rate limiter. It is intended as a last line of defense inside the service, not as a replacement for upstream protection. In multi-instance deployments, each instance keeps its own counters.

`POST /api/v1/sendmail` supports `Idempotency-Key`. When the same key and the same payload are retried within `security.idempotency.ttl_seconds`, the service replays the successful response without sending another email. Static sites must generate and send a stable key for each form submission attempt to benefit from this behavior.

SMTP credentials are intentionally read only from `SMTP_CLIENT_USERNAME` and `SMTP_CLIENT_PASSWORD`; YAML values for those fields are ignored.

### English Admin Dashboard and Audit Logs

`mx-api` can serve a lightweight administrator dashboard at `admin.dashboard.base_path` (default: `/admin`). The dashboard contains API/validation/mail/status doughnut charts, an API request trend chart, server-side audit log filters, pagination, a row detail modal, and a guarded audit-log reset dialog.

The dashboard is designed for operational visibility: administrators can scan request health from the top charts, inspect traffic trends, and then drill into individual audit events with filters and pagination.

![mx-api admin dashboard](docs/images/admin-dashboard.png)

Audit events are stored in SQLite when `audit.enabled` is true. The default embedded config uses `/var/lib/mx-api/audit.db`; persist that directory in containers or Kubernetes when audit history must survive restarts. `audit.retention_days` deletes older events at startup when the value is greater than zero.

```yaml
admin:
  dashboard:
    enabled: true
    base_path: "/admin"
    timestamp_format: "2006-01-02 15:04:05 MST"
    timestamp_timezone: "Asia/Tokyo"
  auth:
    mode: "header"
    user_header: "X-Forwarded-User"
    email_header: "X-Forwarded-Email"
    groups_header: "X-Forwarded-Groups"
    allowed_users: []
    allowed_groups: ["mx-api-admins"]

audit:
  enabled: true
  storage:
    type: "sqlite"
    path: "/var/lib/mx-api/audit.db"
  retention_days: 90
```

Authentication modes:

- `header`: `mx-api` trusts identity headers injected by an upstream auth layer. Access is allowed only when `allowed_users` or `allowed_groups` matches. Missing identity returns `401`; non-admin identity returns `403`.
- `none`: `mx-api` performs no dashboard authentication. The UI shows a visible warning banner. This mode must be protected by upstream access control such as ingress auth, reverse-proxy auth, oauth2-proxy, Authelia, VPN-only exposure, or Basic Auth. Do not expose it directly to the public internet.

Audit logging intentionally avoids sensitive data. It records operational metadata such as timestamp, actor, action, method, path, endpoint, result, status code, request ID, duration, remote address, user agent summary, and high-level error code/message. It does not store Authorization headers, cookies, SMTP credentials, raw request bodies, submitted form contents, full mail body, full message text, or secret tokens.

Audit log timestamps in the dashboard use Go time layouts. Set `MX_API_ADMIN_AUDIT_TIMESTAMP_FORMAT` and optionally `MX_API_ADMIN_AUDIT_TIMESTAMP_TIMEZONE` to change the display, for example `2006-01-02 15:04:05 MST` with `Asia/Tokyo`.

The dashboard reset button opens a confirmation dialog. `POST /_admin/api/v1/audit-events/reset` requires `{"confirmation":"RESET"}`; existing audit events are deleted and a new `audit.reset` marker remains visible.

### English Mail Templates

Templates are Jinja-style files rendered with Pongo2. The default template is embedded in the binary. Configure another file path when you need to replace it:

```yaml
mail:
  template_path: "default_mail_template.html"
  timezone: "Asia/Tokyo"
  submitted_at_format: "2006-01-02 15:04:05 MST"
```

Template variables include all field names, `fields`, `contact_name`, `homepage_url`, `submitted_at`, and `mail`.
`fields` contains the submitted fields in configured `form.fields` order.
`submitted_at` is formatted with `mail.timezone` and `mail.submitted_at_format`. The format string uses Go's time layout syntax.

### English Development

Run the API and the lightweight development console:

```sh
cp configs/config.example.yaml configs/config.yaml
cp dev/.env.example dev/.env
docker compose up --build
```

Edit `dev/.env` with your SMTP credentials before starting the stack.

Open `http://localhost:5173`. The frontend reads `GET /api/v1/schema` and builds the input form from backend config.

The compose stack also exposes the embedded admin dashboard at:

```text
http://localhost:8080/contact/admin/
```

For local development, `docker-compose.yaml` sets `MX_API_ADMIN_AUTH_MODE=none`, so the dashboard is reachable without trusted auth headers and displays the disabled-auth warning banner.

To rebuild the embedded admin dashboard assets locally:

```sh
cd frontend
npm install
npm run check
npm run build
```

### English Build

```sh
docker build -t mx-api .
```

The Docker image builds and tests the Go binary during the build stage.
It also builds the embedded Svelte admin dashboard before the Go binary is compiled.
For container deployments that use SQLite audit storage, mount persistent storage at `/var/lib/mx-api`.
Release automation computes the next version, creates the release tag from the merge commit, and injects the resolved version into the image build with the generic build args `BUILD_VERSION` and `BUILD_COMMIT`. This keeps the workflow reusable across repositories and avoids conflicts with branch protection rules. The admin dashboard displays both values and links the commit hash to the matching GitHub commit when `BUILD_COMMIT` is available.

You can optionally pass the version and commit hash into a local image build:

```sh
docker build \
  --build-arg BUILD_VERSION=0.2.0 \
  --build-arg BUILD_COMMIT="$(git rev-parse HEAD)" \
  -t mx-api .
```

## 日本語

`mx-api` は、問い合わせフォームの入力値検証と HTML メール送信を行う小さな Go サービスです。起動 binary 名は `mx-api`、Go module は `github.com/michibiki-io/mx-api-go` です。

### API リファレンス

このサービスは以下の endpoint を提供します。

- `GET /` -> `{"status":"Ok","version":"<version>"}`
- `GET /helthz` -> `{"status":"Ok","version":"<version>"}`
- `GET /api/v1/validate` -> `{"status":"Ok","version":"<version>"}`
- `POST /api/v1/validate` -> validation 成功時 `{"status":"Ok","version":"<version>"}`
- `GET /api/v1/sendmail` -> `{"status":"Ok","version":"<version>"}`
- `POST /api/v1/sendmail` -> メール送信成功時 `{"status":"Ok","version":"<version>"}`
- `GET /api/v1/schema` -> `version` を含む

管理ダッシュボードが有効な場合は、service context path 配下に以下の管理者向け endpoint も公開されます。

- `GET /admin/` -> デフォルトの埋め込み管理ダッシュボード UI
- `GET /_admin/api/v1/me`
- `GET /_admin/api/v1/request-metrics`
- `GET /_admin/api/v1/audit-options`
- `GET /_admin/api/v1/audit-events`
- `GET /_admin/api/v1/audit-events/:id`
- `POST /_admin/api/v1/audit-events/reset`

validation error は以下のレスポンス形式です。

```json
{
  "status": "BadRequest",
  "errors": {
    "email": {
      "code": "validation_is_email",
      "message": "must be a valid email address"
    }
  }
}
```

### 設定

`configs/config.yaml` はローカル実行用の runtime file です。動作確認用の値や環境依存の値が入る可能性があるため、gitignore 対象です。[configs/config.example.yaml](/home/staratlas@ad.michibiki.io/workspace/mx-api-go/configs/config.example.yaml) からローカル用 config を作成して編集してください。

```sh
cp configs/config.example.yaml configs/config.yaml
```

`mx-api` はデフォルトで `./configs/config.yaml` が存在する場合に読み込みます。別の YAML を使う場合は `MX_API_CONFIG` または `CONFIG_PATH` を指定します。

main config の読み込み後、同じディレクトリに `config.override.yaml` があれば追加で読み込みます。別パスの override file を使う場合は `MX_API_CONFIG_OVERRIDE` または `CONFIG_OVERRIDE_PATH` を指定します。override file は `config.yaml` の後、環境変数の前に適用されます。

デフォルト config の一部だけを変更・削除したい場合は `config.override.yaml` を使います。通常の scalar 値は上書きされ、`mail.extra` と `validation` は key 単位で merge されます。key に `null` を指定すると削除されます。`form.fields` は `name` をキーに patch され、`_delete: true` は一致する field を削除します。一致する field がない場合は無視されます。

```yaml
form:
  fields:
    - name: "organization"
      _delete: true

    - name: "subject"
      required: false
      rules: []

validation:
  message_length: null
```

以下の環境変数を利用できます。

- `MX_API_CONFIG`
- `CONFIG_PATH`
- `MX_API_CONFIG_OVERRIDE`
- `CONFIG_OVERRIDE_PATH`
- `CONTEXT_PATH`
- `BIND_PORT`
- `MODE`
- `ALLOWED_ORIGINS`
- `MX_API_RATE_LIMIT_ENABLED`
- `MX_API_RATE_LIMIT_REQUESTS_PER_MINUTE`
- `MX_API_RATE_LIMIT_FAILURE_REQUESTS_PER_MINUTE`
- `MX_API_IDEMPOTENCY_ENABLED`
- `MX_API_IDEMPOTENCY_TTL_SECONDS`
- `SMTP_SERVER_ADDR`
- `SMTP_AUTHENTICATION_ENABLED`
- `SMTP_SKIP_VERIFY_CERT`
- `SMTP_CLIENT_USERNAME`
- `SMTP_CLIENT_PASSWORD`
- `CONTACT_REPLY_EMAIL`
- `CONTACT_REPLY_BCC_EMAIL`
- `CONTACT_FORM_REQUIRED_FIELD`
- `EMAIL_SUBJECT`
- `HOMEPAGE_NAME`
- `HOMEPAGE_URL`
- `MAIL_TEMPLATE_PATH`
- `MAIL_TIMEZONE`
- `MAIL_SUBMITTED_AT_FORMAT`
- `MX_API_ADMIN_DASHBOARD_ENABLED`
- `MX_API_ADMIN_BASE_PATH`
- `MX_API_ADMIN_AUDIT_TIMESTAMP_FORMAT`
- `MX_API_ADMIN_AUDIT_TIMESTAMP_TIMEZONE`
- `MX_API_ADMIN_AUTH_MODE`
- `MX_API_ADMIN_AUTH_USER_HEADER`
- `MX_API_ADMIN_AUTH_EMAIL_HEADER`
- `MX_API_ADMIN_AUTH_GROUPS_HEADER`
- `MX_API_ADMIN_ALLOWED_USERS`
- `MX_API_ADMIN_ALLOWED_GROUPS`
- `MX_API_AUDIT_ENABLED`
- `MX_API_AUDIT_STORAGE_TYPE`
- `MX_API_AUDIT_SQLITE_PATH`
- `MX_API_AUDIT_RETENTION_DAYS`

各 field には `max_length` があります。省略時は field type に応じた保守的な既定値を `mx-api` が補完し、server-side で必ず検証します。

public `POST` request には軽量な in-memory rate limit が適用されます。これは service 内の最後の防御線であり、前段保護の代替ではありません。multi-instance 構成では instance ごとに counter を持ちます。

`POST /api/v1/sendmail` は `Idempotency-Key` に対応しています。同じ key と同じ payload が `security.idempotency.ttl_seconds` 内に再送された場合、メールを再送せず成功レスポンスを再利用します。この効果を得るには、静的サイト側が form submission ごとに安定した key を生成して送信する必要があります。

SMTP 認証情報は `SMTP_CLIENT_USERNAME` と `SMTP_CLIENT_PASSWORD` の環境変数からのみ取得します。YAML に `smtp.client_username` や `smtp.client_password` を書いても無視されます。

API を subpath 配下に置く場合は `server.context_path` または `CONTEXT_PATH` を設定します。

```yaml
server:
  context_path: "/contact"
```

この場合、API は `/contact/api/v1/validate` のように公開されます。

### 管理ダッシュボードと監査ログ

`mx-api` は `admin.dashboard.base_path`（デフォルト `/admin`）で軽量な管理者向け dashboard を配信できます。dashboard には API / validation / mail / status の doughnut chart、API request trend graph、server-side filter 付き audit log table、pagination、row detail modal、確認付き audit-log reset dialog が含まれます。

dashboard は運用状況を素早く確認するための画面です。上段の chart で request health を把握し、request trend を確認したうえで、filter と pagination を使って個別の audit event を調査できます。

![mx-api admin dashboard](docs/images/admin-dashboard.png)

`audit.enabled` が true の場合、監査ログは SQLite に保存されます。埋め込み config の保存先は `/var/lib/mx-api/audit.db` です。container / Kubernetes で監査履歴を restart 後も残す場合は、この directory を永続 volume として mount してください。`audit.retention_days` が 1 以上なら、起動時に指定日数より古い event を削除します。

```yaml
admin:
  dashboard:
    enabled: true
    base_path: "/admin"
    timestamp_format: "2006-01-02 15:04:05 MST"
    timestamp_timezone: "Asia/Tokyo"
  auth:
    mode: "header"
    user_header: "X-Forwarded-User"
    email_header: "X-Forwarded-Email"
    groups_header: "X-Forwarded-Groups"
    allowed_users: []
    allowed_groups: ["mx-api-admins"]

audit:
  enabled: true
  storage:
    type: "sqlite"
    path: "/var/lib/mx-api/audit.db"
  retention_days: 90
```

認証モード:

- `header`: upstream auth layer が付与した header を信頼します。`allowed_users` または `allowed_groups` に一致した場合だけ許可します。identity header がなければ `401`、admin 条件に合わなければ `403` を返します。
- `none`: `mx-api` は dashboard の認証を行いません。UI には警告 banner が表示されます。この mode は ingress auth、reverse proxy auth、oauth2-proxy、Authelia、VPN-only exposure、Basic Auth など、必ず upstream の access control で保護してください。public internet に直接公開してはいけません。

監査ログは機密情報を保存しない設計です。記録するのは timestamp、actor、action、method、path、endpoint、result、status code、request ID、duration、remote address、user agent summary、高レベルな error code/message などの運用 metadata です。Authorization header、cookie、SMTP credential、raw request body、送信 form 全体、mail body、message text、secret token は保存しません。

dashboard 上の audit log timestamp は Go の time layout で表示します。`MX_API_ADMIN_AUDIT_TIMESTAMP_FORMAT` と、必要に応じて `MX_API_ADMIN_AUDIT_TIMESTAMP_TIMEZONE` を設定してください。例: `2006-01-02 15:04:05 MST` と `Asia/Tokyo`。

dashboard の reset button は確認 dialog を開きます。`POST /_admin/api/v1/audit-events/reset` は `{"confirmation":"RESET"}` を要求し、既存の audit event を削除したうえで `audit.reset` marker を 1 件残します。

### Validation ルール

入力項目は `form.fields` に定義し、各 field の `rules` で適用する validation rule 名を指定します。

```yaml
form:
  fields:
    - name: "email"
      label: "Email"
      type: "email"
      required: true
      max_length: 320
      rules: ["required", "email"]
```

rule の実体は `validation` に定義します。

```yaml
validation:
  required:
    tag: "required"
    code: "validation_required"
    message: "cannot be blank"

  email:
    tag: "email"
    code: "validation_is_email"
    message: "must be a valid email address"
```

`tag` は `go-playground/validator` の組み込み tag、または Go code 側で登録した独自 tag です。`jp_phone` はこのプロジェクトで実装している独自 tag です。

正規表現で独自 rule を作る場合は `pattern` を使います。

```yaml
validation:
  jp_postal_code:
    code: "validation_postal_code"
    message: "must be a valid Japanese postal code"
    pattern: '^\\d{3}-?\\d{4}$'
```

`code` と `message` は validation error 時の API レスポンスに使われます。公開済みの frontend や利用者がいる場合は、影響を確認してから変更してください。

### メールテンプレート

メールテンプレートは Jinja-style template として Pongo2 で render します。デフォルトテンプレートはバイナリに埋め込まれます。差し替える場合は `mail.template_path` に別のファイルパスを指定します。

```yaml
mail:
  template_path: "default_mail_template.html"
  timezone: "Asia/Tokyo"
  submitted_at_format: "2006-01-02 15:04:05 MST"
```

template では、各 field 名、`fields`、`contact_name`、`homepage_url`、`submitted_at`、`mail` を利用できます。
`fields` は `form.fields` の設定順で並ぶ入力項目です。
`submitted_at` は `mail.timezone` と `mail.submitted_at_format` で整形されます。format は Go の time layout 形式です。

### 開発起動

API と開発用 frontend を同時に起動します。

```sh
cp configs/config.example.yaml configs/config.yaml
cp dev/.env.example dev/.env
docker compose up --build
```

起動前に `dev/.env` の SMTP 認証情報を編集してください。

Frontend は `http://localhost:5173` です。Frontend は `GET /api/v1/schema` を読み取り、backend config に基づいて入力欄を生成します。

compose stack では、埋め込み管理ダッシュボードも以下で開けます。

```text
http://localhost:8080/contact/admin/
```

ローカル開発用に `docker-compose.yaml` では `MX_API_ADMIN_AUTH_MODE=none` を設定しているため、trusted auth header なしで dashboard を開けます。この場合、UI には認証無効の警告 banner が表示されます。

埋め込み管理ダッシュボード assets をローカルで再生成する場合は以下を実行します。

```sh
cd frontend
npm install
npm run check
npm run build
```

### ビルド

```sh
docker build -t mx-api .
```

Docker build stage では Go test と binary build を実行します。
GitHub Actions では `BUILD_VERSION` と `BUILD_COMMIT` を Docker build に渡します。admin dashboard には version と commit hash が表示され、commit hash は GitHub commit への link になります。
