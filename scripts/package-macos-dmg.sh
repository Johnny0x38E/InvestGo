#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-Invest Monitor}"
BINARY_NAME="${BINARY_NAME:-invest-monitor}"
VERSION="${VERSION:-0.1.0}"
APP_ID="${APP_ID:-com.example.invest-monitor}"
MACOS_MIN_VERSION="${MACOS_MIN_VERSION:-13.0}"
VOLUME_NAME="${VOLUME_NAME:-$APP_NAME}"
SKIP_APP_BUILD="${SKIP_APP_BUILD:-0}"
SKIP_DMG_CREATE="${SKIP_DMG_CREATE:-0}"

BUILD_DIR="$ROOT_DIR/build"
APP_BUILD_DIR="$BUILD_DIR/macos"
APP_DIR="$APP_BUILD_DIR/$APP_NAME.app"
APP_CONTENTS_DIR="$APP_DIR/Contents"
APP_EXECUTABLE="$APP_CONTENTS_DIR/MacOS/$BINARY_NAME"
APP_RESOURCES_DIR="$APP_CONTENTS_DIR/Resources"
ICON_SOURCE="${ICON_SOURCE:-$BUILD_DIR/appicon.png}"
ICON_TIFF="$BUILD_DIR/InvestMonitor.tiff"
ICNS_FILE="$BUILD_DIR/InvestMonitor.icns"
PLIST_TEMPLATE="$BUILD_DIR/Info.plist.template"
STAGING_DIR="$BUILD_DIR/dmg-staging"
DMG_PATH="$BUILD_DIR/bin/invest-monitor-$VERSION-macos-arm64.dmg"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'Missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

escape_sed_replacement() {
  printf '%s' "$1" | sed -e 's/[\\/&|]/\\&/g'
}

render_info_plist() {
  sed \
    -e "s|__APP_NAME__|$(escape_sed_replacement "$APP_NAME")|g" \
    -e "s|__BINARY_NAME__|$(escape_sed_replacement "$BINARY_NAME")|g" \
    -e "s|__APP_ID__|$(escape_sed_replacement "$APP_ID")|g" \
    -e "s|__VERSION__|$(escape_sed_replacement "$VERSION")|g" \
    -e "s|__ICON_FILE__|$(escape_sed_replacement "$(basename "$ICNS_FILE")")|g" \
    -e "s|__MACOS_MIN_VERSION__|$(escape_sed_replacement "$MACOS_MIN_VERSION")|g" \
    "$PLIST_TEMPLATE" >"$APP_CONTENTS_DIR/Info.plist"
}

generate_icns() {
  rm -f "$ICNS_FILE"
  sips -s format tiff "$ICON_SOURCE" --out "$ICON_TIFF" >/dev/null
  tiff2icns "$ICON_TIFF" "$ICNS_FILE"

  if [[ ! -s "$ICNS_FILE" ]]; then
    rm -f "$ICNS_FILE"
    tiff2icns "$ICON_TIFF" "$ICNS_FILE"
  fi

  if [[ ! -s "$ICNS_FILE" ]]; then
    printf 'Generated icns file is empty: %s\n' "$ICNS_FILE" >&2
    exit 1
  fi

  rm -f "$ICON_TIFF"
}

sign_app_if_configured() {
  if [[ -z "${APPLE_SIGN_IDENTITY:-}" ]]; then
    return
  fi

  codesign \
    --force \
    --deep \
    --options runtime \
    --timestamp \
    --sign "$APPLE_SIGN_IDENTITY" \
    "$APP_DIR"
}

sign_dmg_if_configured() {
  if [[ -z "${APPLE_SIGN_IDENTITY:-}" ]]; then
    return
  fi

  codesign \
    --force \
    --timestamp \
    --sign "$APPLE_SIGN_IDENTITY" \
    "$DMG_PATH"
}

notarize_dmg_if_configured() {
  if [[ -z "${NOTARYTOOL_PROFILE:-}" ]]; then
    return
  fi

  xcrun notarytool submit "$DMG_PATH" --keychain-profile "$NOTARYTOOL_PROFILE" --wait
  xcrun stapler staple "$DMG_PATH"
}

build_app_bundle() {
  if [[ ! -f "$ICON_SOURCE" ]]; then
    printf 'Missing icon source image: %s\n' "$ICON_SOURCE" >&2
    exit 1
  fi

  if [[ ! -f "$PLIST_TEMPLATE" ]]; then
    printf 'Missing Info.plist template: %s\n' "$PLIST_TEMPLATE" >&2
    exit 1
  fi

  rm -rf "$APP_DIR"
  rm -f "$ICON_TIFF"

  mkdir -p "$APP_RESOURCES_DIR"

  OUTPUT_FILE="$APP_EXECUTABLE" MACOS_MIN_VERSION="$MACOS_MIN_VERSION" "$ROOT_DIR/scripts/build-macos-arm64.sh"

  if [[ ! -s "$ICNS_FILE" || "$ICON_SOURCE" -nt "$ICNS_FILE" ]]; then
    generate_icns
  fi
  cp "$ICNS_FILE" "$APP_RESOURCES_DIR/"
  printf 'APPL????' >"$APP_CONTENTS_DIR/PkgInfo"
  render_info_plist
  sign_app_if_configured
}

create_dmg() {
  if [[ ! -d "$APP_DIR" ]]; then
    printf 'Missing app bundle: %s\n' "$APP_DIR" >&2
    exit 1
  fi

  rm -rf "$STAGING_DIR"
  rm -f "$DMG_PATH"

  mkdir -p "$STAGING_DIR"
  ditto "$APP_DIR" "$STAGING_DIR/$APP_NAME.app"
  ln -s /Applications "$STAGING_DIR/Applications"

  hdiutil create \
    -volname "$VOLUME_NAME" \
    -srcfolder "$STAGING_DIR" \
    -format UDZO \
    -ov \
    "$DMG_PATH"

  sign_dmg_if_configured
  notarize_dmg_if_configured
}

if [[ "$SKIP_APP_BUILD" != "1" ]]; then
  require_command npm
  require_command go
  require_command sips
  require_command tiff2icns
fi

if [[ "$SKIP_DMG_CREATE" != "1" ]]; then
  require_command hdiutil
  require_command ditto
fi

if [[ -n "${APPLE_SIGN_IDENTITY:-}" ]]; then
  require_command codesign
fi

if [[ -n "${NOTARYTOOL_PROFILE:-}" ]]; then
  if [[ -z "${APPLE_SIGN_IDENTITY:-}" ]]; then
    printf 'NOTARYTOOL_PROFILE requires APPLE_SIGN_IDENTITY to be set.\n' >&2
    exit 1
  fi
  if [[ "$SKIP_DMG_CREATE" == "1" ]]; then
    printf 'NOTARYTOOL_PROFILE requires DMG creation to be enabled.\n' >&2
    exit 1
  fi
  require_command xcrun
fi

if [[ "$SKIP_APP_BUILD" != "1" ]]; then
  build_app_bundle
fi

if [[ "$SKIP_DMG_CREATE" != "1" ]]; then
  create_dmg
fi

printf 'Built app bundle: %s\n' "$APP_DIR"

if [[ "$SKIP_DMG_CREATE" != "1" ]]; then
  printf 'Built dmg: %s\n' "$DMG_PATH"
fi
