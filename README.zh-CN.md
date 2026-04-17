# InvestGo

[English](./README.md) | [简体中文](./README.zh-CN.md) | [许可证](./LICENSE)

InvestGo 是一个桌面投资跟踪工具，用于管理自选、持仓、走势图、热门榜单和价格提醒。

## 截图

| 亮色                       | 暗色                     |
| -------------------------- | ------------------------ |
| ![Light](assets/light.png) | ![Dark](assets/dark.png) |

## 技术栈

- Go 1.25
- Wails v3
- Vue 3 + TypeScript
- PrimeVue 4
- Vite 8
- Chart.js 4
- 东方财富与 Yahoo Finance（行情数据）
- Frankfurter（汇率数据）
- 基于 Shell 脚本、`swift`、`iconutil`、`hdiutil` 的 macOS 打包流程

## 构建

前置要求：

- Node.js 20+
- Go 1.25+
- 用于 Darwin arm64 构建脚本的 Apple Silicon macOS 13+

安装依赖：

```bash
npm install
```

构建前端产物：

```bash
npm run build
```

执行检查：

```bash
npm run typecheck
env GOCACHE=/tmp/go-build-cache go test ./...
```

构建桌面二进制：

```bash
./scripts/build-darwin-aarch64.sh
VERSION=1.0.0 ./scripts/build-darwin-aarch64.sh
./scripts/build-darwin-aarch64.sh --dev
```

说明：

- 构建脚本会生成 `build/appicon.png`，执行 `npm run build`，并输出 `build/bin/investgo-darwin-aarch64`。
- `--dev` 会启用终端日志和 F12 Web Inspector。

构建脚本支持的环境变量：

- `VERSION`
- `APP_VERSION`
- `OUTPUT_FILE`
- `MACOS_MIN_VERSION`
- `GOCACHE`
- `MACOSX_DEPLOYMENT_TARGET`
- `CGO_CFLAGS`
- `CGO_LDFLAGS`

## 打包

打包 `.app` 和 `.dmg`：

```bash
./scripts/package-darwin-aarch64.sh
VERSION=1.0.0 ./scripts/package-darwin-aarch64.sh
./scripts/package-darwin-aarch64.sh --dev
```

打包脚本支持的环境变量：

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

输出产物：

- `build/macos/InvestGo.app`
- `build/bin/investgo-<version>-darwin-aarch64.dmg`

## 许可证

本项目基于 [MIT License](./LICENSE) 开源。
