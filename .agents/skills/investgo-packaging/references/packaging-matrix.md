# Packaging Matrix

## Current macOS flow

- `./scripts/package-darwin-aarch64.sh`
- `./scripts/package-darwin-aarch64.sh --dev`
- `./scripts/package-darwin-x86_64.sh`
- `./scripts/package-darwin-x86_64.sh --dev`

The packaging script builds a macOS `.app` bundle, optionally signs it, creates a `.dmg`, optionally signs that DMG, and optionally notarizes and staples it.

## Required tools

- `npm`
- `go`
- `swift`
- `sips`
- `iconutil`
- `hdiutil`
- `ditto`
- `codesign` when `APPLE_SIGN_IDENTITY` is set
- `xcrun` when `NOTARYTOOL_PROFILE` is set

## Important environment variables

- `VERSION`
- `APP_NAME`
- `BINARY_NAME`
- `APP_ID`
- `MACOS_MIN_VERSION`
- `APPLE_SIGN_IDENTITY`
- `NOTARYTOOL_PROFILE`
- `SKIP_APP_BUILD`
- `SKIP_DMG_CREATE`
- `DARWIN_PLATFORM_NAME`
- `DARWIN_BUILD_SCRIPT`

## Current outputs

- `build/macos/InvestGo.app`
- `build/bin/investgo-<version>-darwin-aarch64.dmg`
- `build/bin/investgo-<version>-darwin-x86_64.dmg`
- `build/bin/investgo-windows-amd64.exe` is a runnable Windows build artifact, not a packaged installer.

## Coupling to build

- Apple Silicon packaging depends on `scripts/build-darwin-aarch64.sh`.
- Intel packaging depends on `scripts/build-darwin-x86_64.sh`, which delegates to the shared Darwin build script with `GOARCH=amd64`.
- PNG icon generation depends on `scripts/render-svg-icon.swift`, which is macOS-only.
- ICNS generation prefers `scripts/render-icns.swift` and falls back to the generated `.iconset` plus `iconutil` path.
- The script assumes macOS bundle metadata such as `Info.plist`, `.icns`, and an `/Applications` link in the DMG staging directory.

## Windows x64 release gap

- Windows x64 has a build script for a runnable `.exe`, but it still needs a Windows packaging path.
- Windows x64 needs a different icon/resource and packaging story, such as `.ico` plus `.zip`, Inno Setup `.exe`, `.msi`, or `.msix`.
- `Info.plist`, `.icns`, `hdiutil`, and notarization do not transfer to Windows.
- Signing, installer metadata, WebView2 bootstrap/detection, Start Menu shortcuts, uninstall metadata, and optional auto-launch/post-install launch need a Windows-specific implementation rather than conditional branches on the current DMG script.
