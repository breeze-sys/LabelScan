#!/usr/bin/env sh
set -eu

DEFAULT_URL="${1:-https://labelscan.site}"
OUT="${2:-dist/LabelScan-Go.exe}"
OUT_DIR="$(dirname "$OUT")"

mkdir -p "$OUT_DIR"

if command -v go >/dev/null 2>&1; then
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
    -ldflags="-H windowsgui -X main.defaultURL=${DEFAULT_URL}" \
    -o "$OUT" \
    ./cmd/labelscan-launcher
elif command -v docker >/dev/null 2>&1; then
  docker run --rm \
    -v "$(pwd):/src" \
    -w /src \
    -e CGO_ENABLED=0 \
    -e GOOS=windows \
    -e GOARCH=amd64 \
    golang:1.24-bookworm \
    go build \
      -ldflags="-H windowsgui -X main.defaultURL=${DEFAULT_URL}" \
      -o "$OUT" \
      ./cmd/labelscan-launcher
else
  printf '%s\n' "Go or Docker is required to build the Windows launcher." >&2
  exit 1
fi

printf '%s\n' "$DEFAULT_URL" > "$OUT_DIR/labelscan.url"
printf 'built %s\n' "$OUT"
