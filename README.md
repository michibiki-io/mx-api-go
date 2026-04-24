# mx-api

`mx-api` is a small Go service for validating contact-form payloads and sending HTML mail. The binary name is `mx-api`; the Go module is `github.com/michibiki-io/mx-api-go`.

## Table of Contents

- [Security Notice](#security-notice)
- [English](#english)
  - [API Reference](#english-api-reference)
  - [Configuration](#english-configuration)
  - [Mail Templates](#english-mail-templates)
  - [Development](#english-development)
  - [Build](#english-build)
- [日本語](#日本語)
  - [API リファレンス](#api-リファレンス)
  - [設定](#設定)
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

- `GET /` -> `{"status":"Ok"}`
- `GET /helthz` -> `{"status":"Ok"}`
- `GET /api/v1/validate` -> `{"status":"Ok"}`
- `POST /api/v1/validate` -> `{"status":"Ok"}` when validation passes
- `GET /api/v1/sendmail` -> `{"status":"Ok"}`
- `POST /api/v1/sendmail` -> `{"status":"Ok"}` when mail is sent

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

The following environment variables are supported:

- `CONTEXT_PATH`
- `BIND_PORT`
- `MODE`
- `ALLOWED_ORIGINS`
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

Add form fields under `form.fields` and attach validation rules by name. Rules are defined under `validation` and use `go-playground/validator` tags, plus the built-in `jp_phone` rule.

SMTP credentials are intentionally read only from `SMTP_CLIENT_USERNAME` and `SMTP_CLIENT_PASSWORD`; YAML values for those fields are ignored.

### English Mail Templates

Templates are Jinja-style files rendered with Pongo2. Configure the active template with:

```yaml
mail:
  template_path: "/etc/mx-api/templates/default_mail_template.html"
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

### English Build

```sh
docker build -t mx-api .
```

The Docker image builds and tests the Go binary during the build stage.

## 日本語

`mx-api` は、問い合わせフォームの入力値検証と HTML メール送信を行う小さな Go サービスです。起動 binary 名は `mx-api`、Go module は `github.com/michibiki-io/mx-api-go` です。

### API リファレンス

このサービスは以下の endpoint を提供します。

- `GET /` -> `{"status":"Ok"}`
- `GET /helthz` -> `{"status":"Ok"}`
- `GET /api/v1/validate` -> `{"status":"Ok"}`
- `POST /api/v1/validate` -> validation 成功時 `{"status":"Ok"}`
- `GET /api/v1/sendmail` -> `{"status":"Ok"}`
- `POST /api/v1/sendmail` -> メール送信成功時 `{"status":"Ok"}`

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

以下の環境変数を利用できます。

- `CONTEXT_PATH`
- `BIND_PORT`
- `MODE`
- `ALLOWED_ORIGINS`
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

SMTP 認証情報は `SMTP_CLIENT_USERNAME` と `SMTP_CLIENT_PASSWORD` の環境変数からのみ取得します。YAML に `smtp.client_username` や `smtp.client_password` を書いても無視されます。

API を subpath 配下に置く場合は `server.context_path` または `CONTEXT_PATH` を設定します。

```yaml
server:
  context_path: "/contact"
```

この場合、API は `/contact/api/v1/validate` のように公開されます。

### Validation ルール

入力項目は `form.fields` に定義し、各 field の `rules` で適用する validation rule 名を指定します。

```yaml
form:
  fields:
    - name: "email"
      label: "Email"
      type: "email"
      required: true
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

メールテンプレートは Jinja-style template として Pongo2 で render します。差し替える場合は `mail.template_path` を変更します。

```yaml
mail:
  template_path: "/etc/mx-api/templates/default_mail_template.html"
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

### ビルド

```sh
docker build -t mx-api .
```

Docker build stage では Go test と binary build を実行します。
