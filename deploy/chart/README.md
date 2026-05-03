# mx-api-go Helm Chart

This chart installs the `mx-api-go` microservice for contact-form validation and sendmail delivery, including optional admin dashboard and SQLite-backed audit logging.

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
  --set config.MX_API_AUDIT_SQLITE_PATH=/var/lib/mx-api/audit.db
```

If `MX_API_AUDIT_STORAGE_TYPE=sqlite`, keep the default single-replica deployment unless you redesign storage for shared access. Persist `/var/lib/mx-api` when audit history must survive pod restarts.

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

- `secrets` and `existingSecret` are used only for sensitive values such as `SMTP_CLIENT_USERNAME` and `SMTP_CLIENT_PASSWORD`.
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
- `config.MX_API_AUDIT_*`: audit log enablement, backend type, and retention
- `secrets.*` or `existingSecret`: SMTP credentials
- `configFile.*`: mounted YAML config generation or override
