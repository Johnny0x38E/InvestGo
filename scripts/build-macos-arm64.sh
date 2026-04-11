#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   ./scripts/build-macos-arm64.sh
#   VERSION=0.1.0 ./scripts/build-macos-arm64.sh
#   VERSION=0.1.0 ./scripts/build-macos-arm64.sh --dev
#
# Notes:
# - Version is injected at build time. If VERSION/APP_VERSION is omitted, the app shows "dev".
# - Use --dev when you want the binary to support F12 Web Inspector.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_FILE="${OUTPUT_FILE:-$ROOT_DIR/build/bin/investgo-macos-arm64}"
MACOS_MIN_VERSION="${MACOS_MIN_VERSION:-13.0}"
APP_VERSION="${APP_VERSION:-${VERSION:-dev}}"
DEV_BUILD=0

print_usage() {
  printf '%s\n' \
    'Usage:' \
    '  ./scripts/build-macos-arm64.sh' \
    '  VERSION=0.1.0 ./scripts/build-macos-arm64.sh' \
    '  VERSION=0.1.0 ./scripts/build-macos-arm64.sh --dev' \
    '' \
    'Notes:' \
    '  - Version is injected at build time. Without VERSION/APP_VERSION, the app shows "dev".' \
    '  - Use --dev to enable F12 Web Inspector support in the built app.'
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -dev|--dev)
      DEV_BUILD=1
      shift
      ;;
    -h|--help)
      print_usage
      exit 0
      ;;
    *)
      printf 'Unknown option: %s\n' "$1" >&2
      printf '\n' >&2
      print_usage >&2
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

LDFLAGS="-s -w -X main.appVersion=$APP_VERSION"
BUILD_TAGS="production"
if [[ "$DEV_BUILD" == "1" ]]; then
  LDFLAGS="$LDFLAGS -X main.defaultTerminalLogging=1 -X main.defaultDevToolsBuild=1"
  BUILD_TAGS="production devtools"
fi

go build -tags "$BUILD_TAGS" -trimpath -ldflags="$LDFLAGS" -o "$OUTPUT_FILE" .

printf 'Built %s\n' "$OUTPUT_FILE"
