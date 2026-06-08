#!/usr/bin/env sh
set -eu

ENV_NAME="${ENV_NAME:-labelscan}"

kill_tree() {
  pid="$1"
  children="$(pgrep -P "$pid" 2>/dev/null || true)"
  for child in $children; do
    kill_tree "$child"
  done
  kill "$pid" 2>/dev/null || true
}

stop_pattern() {
  name="$1"
  pattern="$2"
  pids="$(pgrep -u "$(id -u)" -f "$pattern" 2>/dev/null || true)"
  if [ -z "$pids" ]; then
    printf '%s not running\n' "$name"
    return
  fi
  for pid in $pids; do
    case "$pid" in
      $$) continue ;;
    esac
    kill_tree "$pid"
  done
  printf '%s stopped\n' "$name"
}

stop_pattern target-oracle "conda run -n ${ENV_NAME} env .*SERVICE_NAME=target-oracle"
stop_pattern shadow-oracle "conda run -n ${ENV_NAME} env .*SERVICE_NAME=shadow-oracle"
stop_pattern labelscan-web "conda run -n ${ENV_NAME} env CGO_ENABLED=0 go run . --serve"
