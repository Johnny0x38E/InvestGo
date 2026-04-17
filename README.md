# InvestGo

[English](./README.md) | [简体中文](./README.zh-CN.md) | [License](./LICENSE)

InvestGo is a desktop investment tracker for watchlists, holdings, charts, hot lists, and alerts.

## Screenshots

| Light                      | Dark                     |
| -------------------------- | ------------------------ |
| ![Light](assets/light.png) | ![Dark](assets/dark.png) |

## Tech Stack

- Go 1.25
- Wails v3
- Vue 3 + TypeScript
- PrimeVue 4
- Vite 8
- Chart.js 4
- EastMoney and Yahoo Finance for market data
- Frankfurter for exchange rates
- macOS app packaging with shell scripts, `swift`, `iconutil`, and `hdiutil`

## Build

Prerequisites:

- Node.js 20+
- Go 1.25+
- macOS 13+ on Apple Silicon for the Darwin arm64 build scripts

Install dependencies:

```bash
npm install
```

Build the frontend bundle:

```bash
npm run build
```

Run checks:

```bash
npm run typecheck
env GOCACHE=/tmp/go-build-cache go test ./...
```

Build the desktop binary:

```bash
./scripts/build-darwin-aarch64.sh
VERSION=1.0.0 ./scripts/build-darwin-aarch64.sh
./scripts/build-darwin-aarch64.sh --dev
```

Notes:

- The build script renders `build/appicon.png`, runs `npm run build`, and outputs `build/bin/investgo-darwin-aarch64`.
- `--dev` enables terminal logging and F12 Web Inspector support.

Build script environment variables:

- `VERSION`
- `APP_VERSION`
- `OUTPUT_FILE`
- `MACOS_MIN_VERSION`
- `GOCACHE`
- `MACOSX_DEPLOYMENT_TARGET`
- `CGO_CFLAGS`
- `CGO_LDFLAGS`

## Package

Package the app bundle and DMG:

```bash
./scripts/package-darwin-aarch64.sh
VERSION=1.0.0 ./scripts/package-darwin-aarch64.sh
./scripts/package-darwin-aarch64.sh --dev
```

Packaging script environment variables:

- `APP_NAME`
- `BINARY_NAME`
- `VERSION`
- `APP_ID`
- `MACOS_MIN_VERSION`
- `VOLUME_NAME`
- `ICON_SOURCE`
- `APPLE_SIGN_IDENTITY`
- `NOTARYTOOL_PROFILE`
- `SKIP_APP_BUILD`
- `SKIP_DMG_CREATE`

Outputs:

- `build/macos/InvestGo.app`
- `build/bin/investgo-<version>-darwin-aarch64.dmg`

## License

This project is open-sourced under the [MIT License](./LICENSE).
