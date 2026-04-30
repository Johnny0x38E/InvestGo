# Build Matrix

Use `AGENTS.md` for the standard build and test commands. This reference only captures build behavior that is not obvious from the global project guide.

## Current outputs

- `frontend/dist`: generated frontend assets embedded by `main.go`.
- `build/appicon.png`: generated app icon embedded by `main.go`.
- `build/bin/investgo-darwin-aarch64`: Darwin arm64 binary output.
- `build/bin/investgo-darwin-x86_64`: Darwin Intel binary output.
- `build/bin/investgo-windows-amd64.exe`: Windows x64 runnable binary output.

## Build chain behavior

- `main.go` embeds `frontend/dist`, so the frontend must be built before the final desktop binary.
- `scripts/build-darwin-aarch64.sh` renders the app icon, runs `npm run build`, then builds Go with `GOOS=darwin` and `GOARCH=arm64`.
- `scripts/build-darwin-x86_64.sh` wraps the same Darwin build flow with `GOOS=darwin` and `GOARCH=amd64`.
- `scripts/build-windows-amd64.ps1` copies `frontend/src/assets/appicon.png` to `build/appicon.png` when missing, runs `npm run build`, then builds Go with `GOOS=windows`, `GOARCH=amd64`, and `CGO_ENABLED=0`.
- The macOS `--dev` flag and Windows `-Dev` switch inject `main.defaultTerminalLogging=1` and `main.defaultDevToolsBuild=1`.

## External tool assumptions

- `scripts/render-app-icon.sh` requires `swift`.
- `scripts/render-svg-icon.swift` uses AppKit and is therefore macOS-only.
- `scripts/build-darwin-aarch64.sh` sets `MACOSX_DEPLOYMENT_TARGET` and macOS-specific CGO flags, with `DARWIN_GOARCH` and `DARWIN_PLATFORM_NAME` available for architecture wrappers.
- `scripts/build-darwin-x86_64.sh` depends on the same macOS toolchain and delegates to `scripts/build-darwin-aarch64.sh` with Intel defaults.
- `scripts/build-windows-amd64.ps1` requires PowerShell, `go`, and `npm`. `magick` is only needed when `ICON_SOURCE` is overridden to an SVG file.
- Windows 11 runtime requires Microsoft Edge WebView2 Runtime to be available.

## Windows x64 status

- A runnable Windows x64 `.exe` can be produced with `scripts/build-windows-amd64.ps1`.
- The Windows build currently uses the embedded PNG for Wails runtime window icon data, but it does not yet embed a Windows `.ico`, version resource, or application manifest into the executable.
- Keep Windows compilation separate from future Windows installer packaging and signing.
