#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCAL_PORT_WAS_SET="false"
MAILPIT_LOCAL_PORT_WAS_SET="false"
if [ -n "${LOCAL_PORT+x}" ]; then
  LOCAL_PORT_WAS_SET="true"
fi
if [ -n "${MAILPIT_LOCAL_PORT+x}" ]; then
  MAILPIT_LOCAL_PORT_WAS_SET="true"
fi

CLUSTER_NAME="${CLUSTER_NAME:-mx-api-go-e2e}"
NAMESPACE="${NAMESPACE:-mx-api-e2e}"
RELEASE_NAME="${RELEASE_NAME:-mx-api-go}"
IMAGE_NAME="${IMAGE_NAME:-mx-api-go:e2e}"
LOCAL_PORT="${LOCAL_PORT:-18080}"
MAILPIT_LOCAL_PORT="${MAILPIT_LOCAL_PORT:-18025}"
KEEP_CLUSTER="${KEEP_CLUSTER:-false}"
KEEP_ON_FAILURE="${KEEP_ON_FAILURE:-true}"
KUBECONFIG_PATH="${KUBECONFIG_PATH:-${ROOT_DIR}/.tmp/kind-${CLUSTER_NAME}.kubeconfig}"
TMP_DIR="${ROOT_DIR}/.tmp"
PORT_FORWARD_LOG="${TMP_DIR}/mx-api-go-port-forward.log"
MAILPIT_PORT_FORWARD_LOG="${TMP_DIR}/mx-api-go-mailpit-port-forward.log"
PORT_FORWARD_PID=""
MAILPIT_PORT_FORWARD_PID=""
log() {
  echo "[helm-e2e-kind] $*"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "[helm-e2e-kind] required command not found: $1" >&2
    exit 1
  }
}

kctl() {
  kubectl --kubeconfig "${KUBECONFIG_PATH}" "$@"
}

hctl() {
  helm --kubeconfig "${KUBECONFIG_PATH}" "$@"
}

port_in_use() {
  local port="$1"

  if command -v ss >/dev/null 2>&1; then
    ss -ltn "sport = :${port}" | awk 'NR > 1 { found = 1 } END { exit(found ? 0 : 1) }'
    return $?
  fi

  (echo >"/dev/tcp/127.0.0.1/${port}") >/dev/null 2>&1
}

find_available_port() {
  local preferred_port="$1"
  local max_attempts="${2:-100}"
  local candidate

  for candidate in $(seq "${preferred_port}" "$((preferred_port + max_attempts))"); do
    if ! port_in_use "${candidate}"; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done

  return 1
}

prepare_local_port() {
  local current_port="$1"
  local was_set="$2"
  local label="$3"
  local chosen_port

  if ! port_in_use "${current_port}"; then
    printf '%s\n' "${current_port}"
    return 0
  fi

  if [ "${was_set}" = "true" ]; then
    echo "[helm-e2e-kind] ${label} port ${current_port} is already in use; choose another port explicitly." >&2
    return 1
  fi

  chosen_port="$(find_available_port "$((current_port + 1))")" || {
    echo "[helm-e2e-kind] failed to find an available local port for ${label}" >&2
    return 1
  }

  echo "[helm-e2e-kind] ${label} port ${current_port} is already in use. using ${chosen_port} instead." >&2
  printf '%s\n' "${chosen_port}"
}

render_mailpit_manifests() {
  cat <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mailpit
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mailpit
  template:
    metadata:
      labels:
        app: mailpit
    spec:
      containers:
        - name: mailpit
          image: axllent/mailpit:latest
          ports:
            - containerPort: 1025
              name: smtp
            - containerPort: 8025
              name: web
---
apiVersion: v1
kind: Service
metadata:
  name: mailpit-smtp
spec:
  selector:
    app: mailpit
  ports:
    - name: smtp
      port: 1025
      targetPort: smtp
---
apiVersion: v1
kind: Service
metadata:
  name: mailpit-web
spec:
  selector:
    app: mailpit
  ports:
    - name: web
      port: 8025
      targetPort: web
EOF
}

wait_for_port_forward() {
  local url="$1"
  local label="$2"

  for _ in $(seq 1 30); do
    if curl -fsS "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "[helm-e2e-kind] ${label} port-forward did not become ready: ${url}" >&2
  return 1
}

print_debug_info() {
  log "collecting debug information..."

  if [ -f "${KUBECONFIG_PATH}" ]; then
    kctl -n "${NAMESPACE}" get all || true
    kctl -n "${NAMESPACE}" get events --sort-by=.lastTimestamp || true
    kctl -n "${NAMESPACE}" get configmap || true
    kctl -n "${NAMESPACE}" get secret || true
    kctl -n "${NAMESPACE}" describe deploy "${RELEASE_NAME}" || true
    kctl -n "${NAMESPACE}" describe pods || true
    kctl -n "${NAMESPACE}" logs deploy/"${RELEASE_NAME}" || true
    hctl status "${RELEASE_NAME}" -n "${NAMESPACE}" || true
  fi

  if [ -f "${PORT_FORWARD_LOG}" ]; then
    log "mx-api port-forward log:"
    cat "${PORT_FORWARD_LOG}" || true
  fi

  if [ -f "${MAILPIT_PORT_FORWARD_LOG}" ]; then
    log "mailpit port-forward log:"
    cat "${MAILPIT_PORT_FORWARD_LOG}" || true
  fi
}

cleanup() {
  local exit_code=$?
  local keep_resources="false"

  set +e

  if [ -n "${PORT_FORWARD_PID}" ]; then
    kill "${PORT_FORWARD_PID}" >/dev/null 2>&1 || true
    wait "${PORT_FORWARD_PID}" >/dev/null 2>&1 || true
  fi

  if [ -n "${MAILPIT_PORT_FORWARD_PID}" ]; then
    kill "${MAILPIT_PORT_FORWARD_PID}" >/dev/null 2>&1 || true
    wait "${MAILPIT_PORT_FORWARD_PID}" >/dev/null 2>&1 || true
  fi

  if [ "${exit_code}" -ne 0 ]; then
    print_debug_info
    if [ "${KEEP_ON_FAILURE}" = "true" ]; then
      keep_resources="true"
      log "KEEP_ON_FAILURE=true. keeping kind cluster for debugging: ${CLUSTER_NAME}"
    fi
  fi

  if [ "${KEEP_CLUSTER}" = "true" ]; then
    keep_resources="true"
    log "KEEP_CLUSTER=true. keeping kind cluster: ${CLUSTER_NAME}"
  fi

  if [ "${keep_resources}" = "true" ]; then
    log "dedicated kubeconfig: ${KUBECONFIG_PATH}"
    log "inspect with: kubectl --kubeconfig ${KUBECONFIG_PATH} -n ${NAMESPACE} get all"
    exit "${exit_code}"
  fi

  if kind get clusters | grep -qx "${CLUSTER_NAME}"; then
    log "deleting kind cluster: ${CLUSTER_NAME}"
    kind delete cluster --name "${CLUSTER_NAME}" >/dev/null 2>&1 || true
  fi

  if [ -f "${KUBECONFIG_PATH}" ]; then
    log "removing dedicated kubeconfig: ${KUBECONFIG_PATH}"
    rm -f "${KUBECONFIG_PATH}" || true
  fi

  rm -f "${PORT_FORWARD_LOG}" "${MAILPIT_PORT_FORWARD_LOG}" || true

  exit "${exit_code}"
}

trap cleanup EXIT

require_command docker
require_command kind
require_command kubectl
require_command helm
require_command curl

cd "${ROOT_DIR}"
mkdir -p "${TMP_DIR}"
rm -f "${PORT_FORWARD_LOG}" "${MAILPIT_PORT_FORWARD_LOG}"

LOCAL_PORT="$(prepare_local_port "${LOCAL_PORT}" "${LOCAL_PORT_WAS_SET}" "mx-api")"
MAILPIT_LOCAL_PORT="$(prepare_local_port "${MAILPIT_LOCAL_PORT}" "${MAILPIT_LOCAL_PORT_WAS_SET}" "mailpit")"

log "running helm lint..."
helm lint ./deploy/chart

if kind get clusters | grep -qx "${CLUSTER_NAME}"; then
  log "kind cluster already exists: ${CLUSTER_NAME}"
  kind get kubeconfig --name "${CLUSTER_NAME}" > "${KUBECONFIG_PATH}"
else
  log "creating kind cluster: ${CLUSTER_NAME}"
  KUBECONFIG="${KUBECONFIG_PATH}" kind create cluster \
    --name "${CLUSTER_NAME}" \
    --kubeconfig "${KUBECONFIG_PATH}"
  kind get kubeconfig --name "${CLUSTER_NAME}" > "${KUBECONFIG_PATH}"
fi

log "checking kind cluster connection with dedicated kubeconfig..."
kctl get nodes -o wide

log "building Docker image: ${IMAGE_NAME}"
docker build -t "${IMAGE_NAME}" .

log "loading Docker image into kind: ${IMAGE_NAME}"
kind load docker-image "${IMAGE_NAME}" --name "${CLUSTER_NAME}"

log "creating namespace: ${NAMESPACE}"
kctl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kctl apply -f -

log "deploying Mailpit..."
render_mailpit_manifests | kctl -n "${NAMESPACE}" apply -f -
kctl -n "${NAMESPACE}" rollout status deploy/mailpit --timeout=120s

log "installing Helm chart..."
hctl upgrade --install "${RELEASE_NAME}" ./deploy/chart \
  -n "${NAMESPACE}" \
  --create-namespace \
  --set image.repository=mx-api-go \
  --set image.tag=e2e \
  --set image.pullPolicy=IfNotPresent \
  --set persistence.enabled=false \
  --set-string config.SMTP_SERVER_ADDR=mailpit-smtp:1025 \
  --set-string config.SMTP_AUTHENTICATION_ENABLED=false \
  --set-string config.SMTP_SKIP_VERIFY_CERT=true \
  --set-string config.SMTP_TLS_MODE=plain \
  --set-string config.CONTACT_REPLY_EMAIL=noreply@example.test \
  --set-string config.CONTACT_REPLY_BCC_EMAIL=contact@example.test \
  --set-string config.EMAIL_SUBJECT=mx-api-go\ e2e\ test \
  --set-string config.HOMEPAGE_NAME=E2E\ Test\ Site \
  --set-string config.HOMEPAGE_URL=https://example.test \
  --set-string config.MX_API_ADMIN_DASHBOARD_ENABLED=true \
  --set-string config.MX_API_ADMIN_AUTH_MODE=header \
  --set-string config.MX_API_ADMIN_AUTH_USER_HEADER=X-Forwarded-User \
  --set-string config.MX_API_ADMIN_AUTH_EMAIL_HEADER=X-Forwarded-Email \
  --set-string config.MX_API_ADMIN_AUTH_GROUPS_HEADER=X-Forwarded-Groups \
  --set-string config.MX_API_ADMIN_ALLOWED_GROUPS=mx-api-admins \
  --set-string config.MX_API_AUDIT_ENABLED=true \
  --set-string config.MX_API_AUDIT_SQLITE_PATH=/var/lib/mx-api/audit.db \
  --wait \
  --atomic \
  --timeout 5m

log "waiting for rollout..."
kctl -n "${NAMESPACE}" rollout status deploy/"${RELEASE_NAME}" --timeout=120s

log "starting port-forward: svc/${RELEASE_NAME} ${LOCAL_PORT}:80"
kctl -n "${NAMESPACE}" port-forward "svc/${RELEASE_NAME}" "${LOCAL_PORT}:80" >"${PORT_FORWARD_LOG}" 2>&1 &
PORT_FORWARD_PID="$!"
wait_for_port_forward "http://127.0.0.1:${LOCAL_PORT}/contact/helthz" "mx-api"

log "starting Mailpit port-forward: svc/mailpit-web ${MAILPIT_LOCAL_PORT}:8025"
kctl -n "${NAMESPACE}" port-forward svc/mailpit-web "${MAILPIT_LOCAL_PORT}:8025" >"${MAILPIT_PORT_FORWARD_LOG}" 2>&1 &
MAILPIT_PORT_FORWARD_PID="$!"
wait_for_port_forward "http://127.0.0.1:${MAILPIT_LOCAL_PORT}/api/v1/messages" "mailpit"

log "running smoke test..."
MAILPIT_URL="http://127.0.0.1:${MAILPIT_LOCAL_PORT}" \
IDEMPOTENCY_KEY="e2e-kind-001" \
"${ROOT_DIR}/scripts/e2e-smoke.sh" "http://127.0.0.1:${LOCAL_PORT}"

log "checking Mailpit message API..."
MAILPIT_MESSAGES="$(curl -fsS "http://127.0.0.1:${MAILPIT_LOCAL_PORT}/api/v1/messages")"
printf '%s' "${MAILPIT_MESSAGES}" | grep -Eq '"messages"[[:space:]]*:[[:space:]]*\[[[:space:]]*\{' || {
  echo "[helm-e2e-kind] Mailpit did not report any delivered message" >&2
  echo "${MAILPIT_MESSAGES}" >&2
  exit 1
}

log "e2e completed successfully"
