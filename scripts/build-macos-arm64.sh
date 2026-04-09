#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_FILE="${OUTPUT_FILE:-$ROOT_DIR/build/bin/invest-monitor-macos-arm64}"
MACOS_MIN_VERSION="${MACOS_MIN_VERSION:-13.0}"
DEV_BUILD=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    -dev|--dev)
      DEV_BUILD=1
      shift
      ;;
    *)
      printf 'Unknown option: %s\n' "$1" >&2
      exit 1
      ;;
  esac
done

mkdir -p "$(dirname "$OUTPUT_FILE")"
cd "$ROOT_DIR"

"$ROOT_DIR/scripts/render-app-icon.sh"

npm run build

export CGO_ENABLED=1
export GOOS=darwin
export GOARCH=arm64
export GOCACHE="${GOCACHE:-/tmp/go-build-cache}"
export MACOSX_DEPLOYMENT_TARGET="${MACOSX_DEPLOYMENT_TARGET:-$MACOS_MIN_VERSION}"
export CGO_CFLAGS="${CGO_CFLAGS:--mmacosx-version-min=$MACOS_MIN_VERSION}"
export CGO_LDFLAGS="${CGO_LDFLAGS:--mmacosx-version-min=$MACOS_MIN_VERSION}"

LDFLAGS="-s -w"
BUILD_TAGS="production"
if [[ "$DEV_BUILD" == "1" ]]; then
  LDFLAGS="$LDFLAGS -X main.defaultTerminalLogging=1 -X main.defaultDevToolsBuild=1"
  BUILD_TAGS="production devtools"
fi

go build -tags "$BUILD_TAGS" -trimpath -ldflags="$LDFLAGS" -o "$OUTPUT_FILE" .

printf 'Built %s\n' "$OUTPUT_FILE"
