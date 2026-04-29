#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export DARWIN_PLATFORM_NAME="${DARWIN_PLATFORM_NAME:-x86_64}"
export DARWIN_BUILD_SCRIPT="${DARWIN_BUILD_SCRIPT:-$ROOT_DIR/scripts/build-darwin-x86_64.sh}"

exec "$ROOT_DIR/scripts/package-darwin-aarch64.sh" "$@"
