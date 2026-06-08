#!/usr/bin/env sh
set -eu

CONDA_BIN="${CONDA_BIN:-${HOME}/miniconda3/bin/conda}"
ENV_NAME="${ENV_NAME:-labelscan}"
ORACLE_HOST="${ORACLE_HOST:-127.0.0.1}"
TARGET_PORT="${TARGET_PORT:-18000}"
SHADOW_PORT="${SHADOW_PORT:-18001}"
WEB_ADDR="${LABELSCAN_ADDR:-127.0.0.1:18080}"
TARGET_API="${TARGET_API:-http://127.0.0.1:${TARGET_PORT}}"
SHADOW_API="${SHADOW_API:-http://127.0.0.1:${SHADOW_PORT}}"
TARGET_CUDA_VISIBLE_DEVICES="${TARGET_CUDA_VISIBLE_DEVICES:-0}"
SHADOW_CUDA_VISIBLE_DEVICES="${SHADOW_CUDA_VISIBLE_DEVICES:-0}"

mkdir -p output

start_service() {
  name="$1"
  pattern="$2"
  log_path="$3"
  shift 3

  if pgrep -u "$(id -u)" -f "$pattern" >/dev/null 2>&1; then
    printf '%s already running\n' "$name"
    return
  fi

  setsid -f "$@" > "$log_path" 2>&1
  printf '%s started, log: %s\n' "$name" "$log_path"
}

start_service \
  target-oracle \
  "conda run -n ${ENV_NAME} env .*PORT=${TARGET_PORT} .*SERVICE_NAME=target-oracle" \
  output/target-oracle.log \
  "$CONDA_BIN" run -n "$ENV_NAME" env \
    PORT="$TARGET_PORT" \
    ORACLE_HOST="$ORACLE_HOST" \
    SERVICE_NAME=target-oracle \
    CUDA_VISIBLE_DEVICES="$TARGET_CUDA_VISIBLE_DEVICES" \
    MODEL_PATH=python_server/CIFAR10/target/3000/best_checkpoint_ep.pth \
    python python_server/server.py

start_service \
  shadow-oracle \
  "conda run -n ${ENV_NAME} env .*PORT=${SHADOW_PORT} .*SERVICE_NAME=shadow-oracle" \
  output/shadow-oracle.log \
  "$CONDA_BIN" run -n "$ENV_NAME" env \
    PORT="$SHADOW_PORT" \
    ORACLE_HOST="$ORACLE_HOST" \
    SERVICE_NAME=shadow-oracle \
    CUDA_VISIBLE_DEVICES="$SHADOW_CUDA_VISIBLE_DEVICES" \
    MODEL_PATH=python_server/CIFAR10/shadow_json_aligned/best_checkpoint_ep.pth \
    python python_server/server.py

start_service \
  labelscan-web \
  "conda run -n ${ENV_NAME} env CGO_ENABLED=0 go run . --serve --addr ${WEB_ADDR}" \
  output/labelscan-web.log \
  "$CONDA_BIN" run -n "$ENV_NAME" env \
    CGO_ENABLED=0 \
    go run . \
      --serve \
      --addr "$WEB_ADDR" \
      --preset smoke \
      --audit-mode full \
      --target-api "$TARGET_API" \
      --shadow-api "$SHADOW_API" \
      --shadow-config shadow_config.json \
      --member-index target_members.json \
      --member-root data/cifar-10-batches-bin \
      --calibration-data data/cifar-10-batches-bin/test_batch.bin \
      --non-member-data data/cifar-10-batches-bin/test_batch.bin

printf 'Target Oracle: http://%s:%s, CUDA_VISIBLE_DEVICES=%s\n' "$ORACLE_HOST" "$TARGET_PORT" "$TARGET_CUDA_VISIBLE_DEVICES"
printf 'Shadow Oracle: http://%s:%s, CUDA_VISIBLE_DEVICES=%s\n' "$ORACLE_HOST" "$SHADOW_PORT" "$SHADOW_CUDA_VISIBLE_DEVICES"
printf 'Web console: http://%s\n' "$WEB_ADDR"
