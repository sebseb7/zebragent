#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
OUTPUT="${OUTPUT:-zebragent}"

echo "Building ${OUTPUT} (GOOS=${GOOS} GOARCH=${GOARCH}) using Docker..."

docker run --rm \
  --user "$(id -u):$(id -g)" \
  -v "${SCRIPT_DIR}:/app" \
  -w /app \
  -e GOOS="${GOOS}" \
  -e GOARCH="${GOARCH}" \
  -e CGO_ENABLED=0 \
  -e GOCACHE=/tmp/gocache \
  -e GOPATH=/tmp/gopath \
  golang:1.24-alpine \
  go build -trimpath -ldflags "-s -w" -o "${OUTPUT}" .

echo "[OK] ${OUTPUT} built successfully in ${SCRIPT_DIR}."
ls -lh "${SCRIPT_DIR}/${OUTPUT}"
