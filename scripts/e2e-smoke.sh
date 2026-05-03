#!/usr/bin/env bash
set -Eeuo pipefail

BASE_URL="${1:-http://127.0.0.1:18080}"
MAILPIT_URL="${MAILPIT_URL:-}"
IDEMPOTENCY_KEY="${IDEMPOTENCY_KEY:-e2e-smoke-001}"
PUBLIC_ORIGIN="${PUBLIC_ORIGIN:-http://localhost:5173}"
PUBLIC_REFERER="${PUBLIC_REFERER:-${PUBLIC_ORIGIN}/}"

log() {
  echo "[e2e-smoke] $*"
}

fail_with_response() {
  local label="$1"
  local url="$2"

  echo "[e2e-smoke] ${label} failed" >&2
  curl -i "${url}" || true
  exit 1
}

curl_json() {
  local method="$1"
  local url="$2"
  local data="${3:-}"
  shift 3 || true
  local extra_args=("$@")

  if [ -n "${data}" ]; then
    curl -fsS -X "${method}" "${url}" \
      -H "Content-Type: application/json" \
      -H "Origin: ${PUBLIC_ORIGIN}" \
      -H "Referer: ${PUBLIC_REFERER}" \
      "${extra_args[@]}" \
      -d "${data}"
    return
  fi

  curl -fsS -X "${method}" "${url}" \
    -H "Origin: ${PUBLIC_ORIGIN}" \
    -H "Referer: ${PUBLIC_REFERER}" \
    "${extra_args[@]}"
}

PAYLOAD='{
  "name": "E2E Tester",
  "email": "tester@example.test",
  "tel": "090-1234-5678",
  "organization": "E2E Inc.",
  "subject": "E2E Sendmail Test",
  "message": "This is an e2e smoke test."
}'

log "BASE_URL=${BASE_URL}"

log "waiting for health endpoint..."
for i in $(seq 1 60); do
  if curl -fsS "${BASE_URL}/contact/helthz" >/dev/null; then
    log "health check passed"
    break
  fi

  if [ "${i}" -eq 60 ]; then
    fail_with_response "health check" "${BASE_URL}/contact/helthz"
  fi

  sleep 2
done

log "schema check..."
SCHEMA_RESPONSE="$(curl -fsS "${BASE_URL}/contact/api/v1/schema")"
printf '%s' "${SCHEMA_RESPONSE}" | grep -q '"status":"Ok"' || {
  echo "[e2e-smoke] schema response did not include status Ok" >&2
  echo "${SCHEMA_RESPONSE}" >&2
  exit 1
}

log "validate API check..."
VALIDATE_RESPONSE="$(curl_json POST "${BASE_URL}/contact/api/v1/validate" "${PAYLOAD}")"
printf '%s' "${VALIDATE_RESPONSE}" | grep -q '"status":"Ok"' || {
  echo "[e2e-smoke] validate response did not include status Ok" >&2
  echo "${VALIDATE_RESPONSE}" >&2
  exit 1
}

log "sendmail API check..."
SENDMAIL_RESPONSE="$(curl_json POST "${BASE_URL}/contact/api/v1/sendmail" "${PAYLOAD}" -H "Idempotency-Key: ${IDEMPOTENCY_KEY}")"
printf '%s' "${SENDMAIL_RESPONSE}" | grep -q '"status":"Ok"' || {
  echo "[e2e-smoke] sendmail response did not include status Ok" >&2
  echo "${SENDMAIL_RESPONSE}" >&2
  exit 1
}

log "admin dashboard check..."
ADMIN_RESPONSE="$(curl -fsS "${BASE_URL}/contact/admin/" \
  -H "X-Forwarded-User: admin" \
  -H "X-Forwarded-Email: admin@example.test" \
  -H "X-Forwarded-Groups: mx-api-admins")"
printf '%s' "${ADMIN_RESPONSE}" | grep -qi '<html' || {
  echo "[e2e-smoke] admin dashboard response did not look like HTML" >&2
  echo "${ADMIN_RESPONSE}" >&2
  exit 1
}

if [ -n "${MAILPIT_URL}" ]; then
  log "checking Mailpit messages..."
  MAILPIT_RESPONSE="$(curl -fsS "${MAILPIT_URL}/api/v1/messages")"
  printf '%s' "${MAILPIT_RESPONSE}" | grep -Eq '"messages"[[:space:]]*:[[:space:]]*\[[[:space:]]*\{' || {
    echo "[e2e-smoke] Mailpit response did not contain any delivered message" >&2
    echo "${MAILPIT_RESPONSE}" >&2
    exit 1
  }
fi

log "smoke test completed"
