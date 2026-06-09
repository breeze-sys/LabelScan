#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
LOG_DIR="${LOG_DIR:-${ROOT_DIR}/output}"
WATCH_INTERVAL="${WATCH_INTERVAL:-60}"

CLOUDFLARED_BIN="${CLOUDFLARED_BIN:-${HOME}/.local/bin/cloudflared}"
TUNNEL_CONFIG="${TUNNEL_CONFIG:-${HOME}/.cloudflared/labelscan-go.yml}"
TUNNEL_NAME="${TUNNEL_NAME:-labelscan-go}"
LOCAL_HEALTH_URL="${LOCAL_HEALTH_URL:-http://127.0.0.1:18080/api/status?audit_mode=full&target_api=http://127.0.0.1:18000&shadow_api=http://127.0.0.1:18001}"
PUBLIC_HEALTH_URL="${PUBLIC_HEALTH_URL:-https://labelscan.site/api/status}"
CURL_BIN="${CURL_BIN:-/usr/bin/curl}"
if [ ! -x "$CURL_BIN" ]; then
  CURL_BIN="curl"
fi

mkdir -p "$LOG_DIR"

run_from_root() {
  cd "$ROOT_DIR"
  "$@"
}

current_user_pgrep() {
  pgrep -u "$(id -u)" -f "$1" 2>/dev/null || true
}

current_user_pgrep_full() {
  pgrep -afu "$(id -u)" "$1" 2>/dev/null || true
}

tunnel_pattern() {
  printf '%s' "cloudflared tunnel --config ${TUNNEL_CONFIG} run ${TUNNEL_NAME}"
}

watchdog_pattern() {
  printf '%s' "${ROOT_DIR}/scripts/labelscan_service.sh watch"
}

is_healthy() {
  "$CURL_BIN" -fsS --max-time 20 "$1" >/dev/null 2>&1
}

start_backend() {
  run_from_root sh scripts/start_local_no_docker.sh
}

stop_backend() {
  run_from_root sh scripts/stop_local_no_docker.sh
}

start_tunnel() {
  if [ -n "$(current_user_pgrep "$(tunnel_pattern)")" ]; then
    printf '%s\n' "cloudflared tunnel already running"
    return
  fi
  setsid -f "$CLOUDFLARED_BIN" tunnel --config "$TUNNEL_CONFIG" run "$TUNNEL_NAME" > "$LOG_DIR/cloudflared-labelscan-go.log" 2>&1
  printf '%s\n' "cloudflared tunnel started, log: $LOG_DIR/cloudflared-labelscan-go.log"
}

stop_tunnel() {
  pids="$(current_user_pgrep "$(tunnel_pattern)")"
  if [ -z "$pids" ]; then
    printf '%s\n' "cloudflared tunnel not running"
    return
  fi
  for pid in $pids; do
    kill "$pid" 2>/dev/null || true
  done
  printf '%s\n' "cloudflared tunnel stopped"
}

start_stack() {
  start_backend
  start_tunnel
}

stop_watchdog() {
  pids="$(current_user_pgrep "$(watchdog_pattern)")"
  if [ -z "$pids" ]; then
    printf '%s\n' "watchdog not running"
    return
  fi
  for pid in $pids; do
    case "$pid" in
      $$) continue ;;
    esac
    kill "$pid" 2>/dev/null || true
  done
  printf '%s\n' "watchdog stopped"
}

stop_stack() {
  stop_watchdog
  stop_tunnel
  stop_backend
}

status_stack() {
  printf '%s\n' "Processes:"
  current_user_pgrep_full 'python_server/server.py|cloudflared|go run . --serve|Label-Only-MIA-Go --serve|labelscan_service.sh watch' || true
  printf '\nLocal health: '
  if is_healthy "$LOCAL_HEALTH_URL"; then printf '%s\n' "ok"; else printf '%s\n' "failed"; fi
  printf 'Public health: '
  if is_healthy "$PUBLIC_HEALTH_URL"; then printf '%s\n' "ok"; else printf '%s\n' "failed"; fi
}

watch_loop() {
  printf '[%s] watchdog started\n' "$(date '+%F %T')"
  while :; do
    if ! is_healthy "$LOCAL_HEALTH_URL"; then
      printf '[%s] local health failed; starting backend\n' "$(date '+%F %T')"
      start_backend || true
      sleep 10
    fi
    if [ -z "$(current_user_pgrep "$(tunnel_pattern)")" ]; then
      printf '[%s] tunnel missing; starting tunnel\n' "$(date '+%F %T')"
      start_tunnel || true
      sleep 5
    fi
    if ! is_healthy "$PUBLIC_HEALTH_URL"; then
      printf '[%s] public health failed; tunnel process will be checked again next round\n' "$(date '+%F %T')"
    fi
    sleep "$WATCH_INTERVAL"
  done
}

start_watchdog() {
  if [ -n "$(current_user_pgrep "$(watchdog_pattern)")" ]; then
    printf '%s\n' "watchdog already running"
    return
  fi
  start_stack
  setsid -f "$ROOT_DIR/scripts/labelscan_service.sh" watch > "$LOG_DIR/labelscan-watchdog.log" 2>&1
  printf '%s\n' "watchdog started, log: $LOG_DIR/labelscan-watchdog.log"
}

case "${1:-status}" in
  start) start_stack ;;
  stop) stop_stack ;;
  restart) stop_stack; start_stack ;;
  status) status_stack ;;
  watch) watch_loop ;;
  start-watchdog) start_watchdog ;;
  stop-watchdog) stop_watchdog ;;
  *)
    printf '%s\n' "Usage: $0 {start|stop|restart|status|watch|start-watchdog|stop-watchdog}" >&2
    exit 2
    ;;
esac
