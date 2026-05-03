SHELL := bash

.PHONY: e2e-kind e2e-kind-keep e2e-kind-clean compose-e2e compose-e2e-down

e2e-kind:
	chmod +x scripts/helm-e2e-kind.sh scripts/e2e-smoke.sh
	./scripts/helm-e2e-kind.sh

e2e-kind-keep:
	chmod +x scripts/helm-e2e-kind.sh scripts/e2e-smoke.sh
	KEEP_CLUSTER=true ./scripts/helm-e2e-kind.sh

e2e-kind-clean:
	kind delete cluster --name mx-api-go-e2e || true
	rm -f .tmp/kind-mx-api-go-e2e.kubeconfig || true

compose-e2e:
	test -f configs/config.yaml || cp configs/config.example.yaml configs/config.yaml
	test -f dev/.env || cp dev/.env.example dev/.env
	chmod +x scripts/e2e-smoke.sh
	@set -euo pipefail; \
	find_port() { \
		local port="$$1"; \
		local used_csv="$$2"; \
		local end_port="$$((port + 200))"; \
		while [ "$$port" -lt "$$end_port" ]; do \
			case ",$$used_csv," in \
				*,"$$port",*) port="$$((port + 1))"; continue ;; \
			esac; \
			if ! ss -ltn "sport = :$$port" | awk 'NR > 1 { found = 1 } END { exit(found ? 0 : 1) }'; then \
				echo "$$port"; \
				return 0; \
			fi; \
			port="$$((port + 1))"; \
		done; \
		echo "no available port found" >&2; \
		return 1; \
	}; \
	API_PORT="$${COMPOSE_E2E_API_PORT:-}"; \
	FRONTEND_PORT="$${COMPOSE_E2E_FRONTEND_PORT:-}"; \
	MAILPIT_PORT="$${COMPOSE_E2E_MAILPIT_PORT:-}"; \
	USED_PORTS=""; \
	if [ -z "$$API_PORT" ]; then API_PORT="$$(find_port 18080 "$$USED_PORTS")"; fi; \
	USED_PORTS="$$API_PORT"; \
	if [ -z "$$FRONTEND_PORT" ]; then FRONTEND_PORT="$$(find_port 15173 "$$USED_PORTS")"; fi; \
	USED_PORTS="$$USED_PORTS,$$FRONTEND_PORT"; \
	if [ -z "$$MAILPIT_PORT" ]; then MAILPIT_PORT="$$(find_port 18025 "$$USED_PORTS")"; fi; \
	echo "[compose-e2e] api=http://127.0.0.1:$$API_PORT"; \
	echo "[compose-e2e] frontend=http://127.0.0.1:$$FRONTEND_PORT"; \
	echo "[compose-e2e] mailpit=http://127.0.0.1:$$MAILPIT_PORT"; \
	COMPOSE_E2E_API_PORT="$$API_PORT" \
	COMPOSE_E2E_FRONTEND_PORT="$$FRONTEND_PORT" \
	COMPOSE_E2E_MAILPIT_PORT="$$MAILPIT_PORT" \
	docker compose -f docker-compose.yaml -f docker-compose.e2e.yaml up -d --build; \
	MAILPIT_URL="http://127.0.0.1:$$MAILPIT_PORT" IDEMPOTENCY_KEY=compose-e2e-001 ./scripts/e2e-smoke.sh "http://127.0.0.1:$$API_PORT"

compose-e2e-down:
	docker compose -f docker-compose.yaml -f docker-compose.e2e.yaml down -v
