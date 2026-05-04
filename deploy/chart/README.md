# mx-api-go Helm Chart

This chart installs the `mx-api-go` microservice for contact-form validation and sendmail delivery, including optional admin dashboard and Bun-backed audit logging for SQLite, PostgreSQL, and MariaDB/MySQL.

## Prerequisites

- Helm 3.8 or later
- Kubernetes 1.26 or later
- Access to `ghcr.io/michibiki-io/charts/mx-api-go` for OCI installs
- SMTP connectivity for actual mail delivery

The published chart version and `appVersion` are overridden during release packaging so they match the application release version. If `image.tag` is empty, the chart uses `.Chart.AppVersion`.

## Install from Local Path

```bash
helm upgrade --install mx-api ./deploy/chart \
  --namespace mx-api \
  --create-namespace
```

## Install from OCI Registry

```bash
helm install mx-api \
  oci://ghcr.io/michibiki-io/charts/mx-api-go \
  --version 0.1.0 \
  --namespace mx-api \
  --create-namespace
```

## Upgrade

```bash
helm upgrade mx-api \
  oci://ghcr.io/michibiki-io/charts/mx-api-go \
  --version 0.1.0 \
  --namespace mx-api
```

## Uninstall

```bash
helm uninstall mx-api --namespace mx-api
```

## Common Examples

### Ingress

```bash
helm upgrade --install mx-api ./deploy/chart \
  --namespace mx-api \
  --create-namespace \
  --set config.CONTEXT_PATH=/contact \
  --set ingress.enabled=true \
  --set ingress.className=traefik \
  --set ingress.hosts[0].host=contact.example.com \
  --set ingress.hosts[0].paths[0].path=/contact \
  --set ingress.hosts[0].paths[0].pathType=Prefix
```

### External Secret

```bash
kubectl create secret generic mx-api-secret \
  --namespace mx-api \
  --from-literal=SMTP_CLIENT_USERNAME="$SMTP_CLIENT_USERNAME" \
  --from-literal=SMTP_CLIENT_PASSWORD="$SMTP_CLIENT_PASSWORD"

helm upgrade --install mx-api ./deploy/chart \
  --namespace mx-api \
  --create-namespace \
  --set existingSecret=mx-api-secret
```

### SQLite Audit Log with Persistent Storage

```bash
helm upgrade --install mx-api ./deploy/chart \
  --namespace mx-api \
  --create-namespace \
  --set persistence.enabled=true \
  --set persistence.size=5Gi \
  --set-string config.DB_DRIVER=sqlite \
  --set config.MX_API_AUDIT_SQLITE_PATH=/var/lib/mx-api/audit.db
```

If `DB_DRIVER=sqlite`, keep the default single-replica deployment unless you redesign storage for shared access. Persist `/var/lib/mx-api` when audit history must survive pod restarts. `DB_DSN` defaults to an SQLite DSN derived from `MX_API_AUDIT_SQLITE_PATH`, so existing SQLite installs can continue setting only the legacy path value.

When `DB_DRIVER` is not `sqlite`, the chart does not create or mount the audit data volume.

### PostgreSQL Audit Log Backend

```bash
helm upgrade --install mx-api ./deploy/chart \
  --namespace mx-api \
  --create-namespace \
  --set persistence.enabled=false \
  --set-string config.DB_DRIVER=postgres \
  --set-string secrets.DB_DSN='postgres://user:password@postgresql:5432/mx_api?sslmode=disable' \
  --set-string config.DB_MAX_OPEN_CONNS=20 \
  --set-string config.DB_MAX_IDLE_CONNS=10 \
  --set-string config.DB_CONN_MAX_LIFETIME=30m \
  --set-string config.DB_CONN_MAX_IDLE_TIME=5m
```

Use `--set-string secrets.DB_DSN='...'` or an `existingSecret` containing `DB_DSN` for production database credentials. Values under `config` are rendered into a ConfigMap.

### MariaDB / MySQL Audit Log Backend

```bash
helm upgrade --install mx-api ./deploy/chart \
  --namespace mx-api \
  --create-namespace \
  --set persistence.enabled=false \
  --set-string config.DB_DRIVER=mariadb \
  --set-string secrets.DB_DSN='user:password@tcp(mariadb:3306)/mx_api?parseTime=true&charset=utf8mb4&loc=UTC'
```

### Async Audit Recorder

```bash
helm upgrade --install mx-api ./deploy/chart \
  --namespace mx-api \
  --create-namespace \
  --set-string config.AUDIT_ASYNC_ENABLED=true \
  --set-string config.AUDIT_CHANNEL_SIZE=10000 \
  --set-string config.AUDIT_BATCH_SIZE=500 \
  --set-string config.AUDIT_FLUSH_INTERVAL=100ms \
  --set-string config.AUDIT_SHUTDOWN_FLUSH_TIMEOUT=5s \
  --set-string config.AUDIT_DROP_ON_FULL=false \
  --set-string config.AUDIT_RETRY_MAX_ATTEMPTS=3 \
  --set-string config.AUDIT_RETRY_INITIAL_BACKOFF=100ms \
  --set-string config.AUDIT_RETRY_MAX_BACKOFF=2s \
  --set-string config.AUDIT_ENQUEUE_TIMEOUT=1s
```

The Kubernetes default `terminationGracePeriodSeconds` is normally enough for the default `AUDIT_SHUTDOWN_FLUSH_TIMEOUT=5s`. Increase the pod grace period outside this chart if you set a larger shutdown flush timeout.

### Admin Dashboard with Header Auth

```bash
helm upgrade --install mx-api ./deploy/chart \
  --namespace mx-api \
  --create-namespace \
  --set config.MX_API_ADMIN_DASHBOARD_ENABLED=true \
  --set config.MX_API_ADMIN_AUTH_MODE=header \
  --set config.MX_API_ADMIN_AUTH_USER_HEADER=X-Forwarded-User \
  --set config.MX_API_ADMIN_AUTH_EMAIL_HEADER=X-Forwarded-Email \
  --set config.MX_API_ADMIN_AUTH_GROUPS_HEADER=X-Forwarded-Groups \
  --set config.MX_API_ADMIN_ALLOWED_GROUPS=mx-api-admins
```

### Admin Dashboard with Disabled Auth

```bash
helm upgrade --install mx-api ./deploy/chart \
  --namespace mx-api \
  --create-namespace \
  --set config.MX_API_ADMIN_DASHBOARD_ENABLED=true \
  --set config.MX_API_ADMIN_AUTH_MODE=none
```

When `MX_API_ADMIN_AUTH_MODE=none`, protect the dashboard with an upstream control such as private ingress, VPN, trusted reverse proxy authentication, or Basic Auth. Do not expose it directly to the public internet.

## Config File vs Environment Variables

- `secrets` and `existingSecret` are used for sensitive environment values such as `SMTP_CLIENT_USERNAME`, `SMTP_CLIENT_PASSWORD`, and credential-bearing `DB_DSN` values.
- `config` is rendered to a `ConfigMap` and injected with `envFrom`.
- `configFile.enabled=true` also mounts `config.yaml` at `/etc/mx-api/config.yaml`.
- `configFile.data={}` renders a generated baseline YAML derived from `configs/config.example.yaml`.
- `configFile.enabled=false` keeps the deployment env-driven and does not mount the YAML file.

## Listing Published Versions

```bash
oras repo tags ghcr.io/michibiki-io/charts/mx-api-go
```

## Important Values

- `image.repository`, `image.tag`, `image.pullPolicy`: container image selection
- `service.type`, `service.port`, `service.externalName`: Service behavior, including `ExternalName`
- `ingress.*`: host, path, class, TLS, and custom annotations
- `persistence.*`: PVC creation and mount path for audit SQLite data
- `config.CONTEXT_PATH`: public base path such as `/contact`
- `config.SMTP_*`: SMTP server address, auth toggle, TLS mode, and certificate behavior
- `config.MX_API_ADMIN_*`: admin dashboard and auth behavior
- `config.MX_API_AUDIT_*`: audit log enablement, legacy SQLite path, and retention
- `config.DB_*`: audit database driver, DSN, and connection pool tuning
- `config.AUDIT_*`: async audit recorder queue, batch, retry, and shutdown flush tuning
- `secrets.*` or `existingSecret`: SMTP credentials and optional secret-backed `DB_DSN`
- `configFile.*`: mounted YAML config generation or override
