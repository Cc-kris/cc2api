#!/usr/bin/env bash
# Isolated V03 UI acceptance only. Never reuses the host's development services.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
RUNTIME="$ROOT/.e2e-runtime"
FRONTEND="$ROOT/frontend"
BACKEND="$ROOT/backend"
export GOTOOLCHAIN=auto GOEXPERIMENT=jsonv2

port_free() { ! lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; }
require_clean_environment() {
  local port
  for port in 8280 3280 55435 56382 18921; do
    port_free "$port" || { printf 'Port %s is occupied; refusing to reuse an unknown service.\n' "$port" >&2; return 1; }
  done
  for name in v042-e2e-pg v042-e2e-redis; do
    if docker ps -a --format '{{.Names}}' | grep -Fxq "$name"; then
      printf 'Container %s exists; run stop before a fresh start.\n' "$name" >&2
      return 1
    fi
  done
  if [[ -d "$RUNTIME" ]] && find "$RUNTIME" -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then
    printf '%s contains leftover runtime data; run stop before a fresh start.\n' "$RUNTIME" >&2
    return 1
  fi
}
start_environment() {
  if [[ -f "$RUNTIME/server.pid" ]] && kill -0 "$(<"$RUNTIME/server.pid")" 2>/dev/null; then
    printf 'Isolated acceptance environment is already running; use restart to apply a new retry setting.\n' >&2
    return 1
  fi
  require_clean_environment
  mkdir -p "$RUNTIME" "$BACKEND/bin"
  chmod 700 "$RUNTIME"
  # A production frontend build is embedded and served by the real backend.
  pnpm --dir "$FRONTEND" install --frozen-lockfile
  pnpm --dir "$FRONTEND" build
  test -s "$BACKEND/internal/web/dist/index.html"
  (cd "$BACKEND" && go build -tags='embed e2e_payment' -o bin/sub2api-e2e-payment ./cmd/server)
  # The normal production binary must not include the local payment transport.
  (cd "$BACKEND" && go build -tags=embed -o bin/sub2api-production-check ./cmd/server)
  docker run -d --name v042-e2e-pg -e POSTGRES_PASSWORD=postgres \
    -p 127.0.0.1:55435:5432 postgres:18-alpine >/dev/null
  docker run -d --name v042-e2e-redis -p 127.0.0.1:56382:6379 redis:7-alpine >/dev/null
  local attempts=0
  while ! docker exec v042-e2e-pg pg_isready -U postgres -d postgres >/dev/null 2>&1 || \
        ! docker exec v042-e2e-redis redis-cli ping | grep -Fxq PONG; do
    attempts=$((attempts + 1))
    if ((attempts >= 60)); then
      printf 'Isolated databases did not become ready.\n' >&2
      return 1
    fi
    sleep 1
  done
  cp "$ROOT/data/model_pricing.json" "$RUNTIME/model_pricing.json"
  cp "$ROOT/data/model_pricing.sha256" "$RUNTIME/model_pricing.sha256"
  PRICING_HASH_FILE="$RUNTIME/model_pricing.sha256" node "$FRONTEND/e2e/v042-ui/mock/upstream.mjs" >>"$RUNTIME/mock.log" 2>&1 &
  echo $! >"$RUNTIME/mock.pid"
  disown
  while ! curl -fsS -o /dev/null http://127.0.0.1:18921/__requests; do
    kill -0 "$(<"$RUNTIME/mock.pid")" 2>/dev/null || { printf 'Isolated mock exited during startup.\n' >&2; return 1; }
    sleep 0.1
  done
  local setup_env=()
  if [[ ! -f "$RUNTIME/config.yaml" ]]; then
    setup_env=(AUTO_SETUP=true
      DATABASE_HOST=127.0.0.1 DATABASE_PORT=55435 DATABASE_USER=postgres
      DATABASE_PASSWORD=postgres DATABASE_DBNAME=sub2api DATABASE_SSLMODE=disable
      REDIS_HOST=127.0.0.1 REDIS_PORT=56382 REDIS_DB=0
      ADMIN_EMAIL=admin@v042e2e.local ADMIN_PASSWORD=V042-e2e-Admin-Passw0rd!)
  fi
  # AUTO_SETUP continues into the main server. Keep that same managed process;
  # do not run initialization in the foreground and then start a second server.
  DATA_DIR="$RUNTIME" env "${setup_env[@]}" SERVER_HOST=127.0.0.1 SERVER_PORT=8280 SERVER_MODE=release RUN_MODE=production \
    GATEWAY_FAILOVER_SAME_ACCOUNT_RETRIES="${GATEWAY_FAILOVER_SAME_ACCOUNT_RETRIES:-0}" \
    SUB2API_E2E_AIRWALLEX_API_BASE=http://127.0.0.1:18921 \
    PRICING_REMOTE_URL=http://127.0.0.1:18921/unused-pricing \
    PRICING_HASH_URL=http://127.0.0.1:18921/unused-pricing.sha256 \
    PRICING_DATA_DIR="$RUNTIME" PRICING_FALLBACK_FILE="$ROOT/data/model_pricing.json" \
    "$BACKEND/bin/sub2api-e2e-payment" >>"$RUNTIME/server.log" 2>&1 &
  echo $! >"$RUNTIME/server.pid"
  disown
  attempts=0
  while ! curl -fsS -o /dev/null http://127.0.0.1:8280/login; do
    kill -0 "$(<"$RUNTIME/server.pid")" 2>/dev/null || { printf 'Isolated backend exited during startup; inspect .e2e-runtime/server.log.\n' >&2; return 1; }
    attempts=$((attempts + 1))
    if ((attempts >= 60)); then
      printf 'Isolated backend did not become ready; inspect .e2e-runtime/server.log.\n' >&2
      return 1
    fi
    sleep 1
  done
  printf 'Isolated UI is ready at http://127.0.0.1:8280. Only loopback mock services are used.\n'
}
stop_environment() {
  local pidfile pid running=0
  for pidfile in "$RUNTIME/server.pid" "$RUNTIME/mock.pid"; do
    if [[ -f "$pidfile" ]] && kill -0 "$(<"$pidfile")" 2>/dev/null; then
      running=1
      break
    fi
  done
  if ((running == 0)); then
    # A manually launched server/mock does not own this command's PID files.
    # Never remove its runtime database or leave an unmanaged process behind.
    local port
    for port in 8280 18921; do
      port_free "$port" || { printf 'Port %s is occupied by an unmanaged process; refusing cleanup.\n' "$port" >&2; return 1; }
    done
  fi
  for pidfile in "$RUNTIME/server.pid" "$RUNTIME/mock.pid"; do
    if [[ -f "$pidfile" ]]; then
      pid="$(<"$pidfile")"
      if [[ "$pid" =~ ^[0-9]+$ ]]; then
        kill "$pid" 2>/dev/null || true
        for _ in {1..50}; do
          kill -0 "$pid" 2>/dev/null || break
          sleep 0.1
        done
      fi
      rm -f "$pidfile"
    fi
  done
  docker rm -f v042-e2e-pg v042-e2e-redis >/dev/null 2>&1 || true
  # The isolated bootstrap config contains database credentials and a JWT
  # secret. Never reuse it after the dedicated containers are destroyed.
  rm -f "$RUNTIME/.installed" /tmp/v042-e2e-mock-state.json
  rm -rf -- "$RUNTIME" "$BACKEND/internal/web/dist"
  rm -f "$BACKEND/bin/sub2api-e2e-payment" "$BACKEND/bin/sub2api-production-check"
}
case "${1:-}" in
  start) start_environment ;;
  restart)
    stop_environment
    start_environment
    ;;
  stop) stop_environment ;;
  test) pnpm --dir "$FRONTEND" exec playwright test --config=playwright.v042.config.ts "${@:2}" ;;
  acceptance)
    # AC05 and stream-continuation cases require different server retry settings.
    # Run both phases serially against fresh isolated databases, never together.
    trap stop_environment EXIT
    stop_environment
    GATEWAY_FAILOVER_SAME_ACCOUNT_RETRIES=2 start_environment
    pnpm --dir "$FRONTEND" exec playwright test --config=playwright.v042.config.ts \
      --grep AC05 --output=e2e/v042-ui/test-results/retry
    stop_environment
    GATEWAY_FAILOVER_SAME_ACCOUNT_RETRIES=0 start_environment
    pnpm --dir "$FRONTEND" exec playwright test --config=playwright.v042.config.ts \
      --grep-invert AC05 --output=e2e/v042-ui/test-results/default
    ;;
  *) printf 'Usage: %s start | restart | test [Playwright args] | acceptance | stop\nacceptance runs all cases in two serial configurations and cleans up.\nAC05 requires GATEWAY_FAILOVER_SAME_ACCOUNT_RETRIES=2, other specs require 0.\n' "$0" >&2; exit 2 ;;
esac
