#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export DARWIN_GOARCH="${DARWIN_GOARCH:-amd64}"
export DARWIN_PLATFORM_NAME="${DARWIN_PLATFORM_NAME:-x86_64}"

exec "$ROOT_DIR/scripts/build-darwin-aarch64.sh" "$@"
