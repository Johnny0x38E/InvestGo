# InvestGo

[English](./README.md) | [简体中文](./README.zh-CN.md) | [许可证](./LICENSE)

InvestGo 是一个基于 Wails 的桌面投资跟踪工具，用于管理自选、持仓、组合分析、热门榜单、历史走势图和价格提醒。

InvestGo 主要把 Wails 用作 Go 后端和 Vue 前端的轻量桌面容器，打包后的桌面应用不需要像 Electron 一样携带 Chromium 和 Node.js 运行时。对于本项目这种形态，Wails 通常可以带来更小的应用体积、更低的空闲内存占用和更快的启动速度，同时仍然提供原生窗口、嵌入资源、生命周期钩子、DevTools 支持和平台集成能力。

> - Electron 成就了很多优秀的跨平台桌面应用，但它也让重复打包浏览器运行时成为了许多日常设备上的常见成本。我们需要更多轻量的跨平台桌面方案，尽可能复用系统 WebView，让原生后端保持简洁。
> - 本项目当前使用 Wails v3 alpha.74。Wails v3 仍处于 alpha 阶段，后续官方版本中的 API、运行时行为和构建细节都可能发生变化。
> - InvestGo 主要是个人使用和学习项目。项目开源供参考，但不保证长期维护、兼容性或稳定的功能路线图。

## 截图

| 亮色                       | 暗色                     |
| -------------------------- | ------------------------ |
| ![Light](assets/light.png) | ![Dark](assets/dark.png) |

## 技术栈

- 后端：Go 1.25、Wails v3 alpha.74、标准 HTTP handler。
- 前端：Vue 3、TypeScript、PrimeVue 4、Vite 8、Chart.js 4。
- 行情数据：东方财富、Yahoo Finance、新浪财经、雪球、腾讯财经、Alpha Vantage、Twelve Data、Finnhub、Polygon。
- 汇率数据：Frankfurter。
- macOS 打包：Shell 脚本以及 `swift`、`sips`、`iconutil`、`hdiutil`、`ditto`。

## 架构

本仓库非 monorepo。Go module 根目录就是仓库根目录，前端位于 `frontend/`。

- `main.go` 嵌入 `frontend/dist` 和 `build/appicon.png`，创建 Wails v3 应用，接入平台设置，并提供一个 HTTP mux。
- `/api/*` 路由由 `internal/api` 处理。前端通过 `frontend/src/api.ts` 中的标准 `fetch()` 调用后端；应用数据不依赖 Wails JS bindings。
- `internal/core/store` 负责持久化状态、运行时状态、行情刷新、历史缓存、组合概览、提醒计算和 JSON 存储。
- `internal/core/marketdata` 注册行情和历史数据 provider，并创建历史路由器。
- `internal/core/provider` 保存具体 provider 实现。
- `internal/core/hot` 负责热门榜单池、缓存、补全和排序。
- `internal/platform` 隔离代理检测、窗口选项等桌面平台差异。
- `internal/logger` 保存后端和前端开发日志。

默认持久化状态路径：

- macOS：`~/Library/Application Support/investgo/state.json`

默认开发日志路径：

- macOS：`~/Library/Application Support/investgo/logs/app.log`

## 开发

前置要求：

- Node.js 20+
- Go 1.25+
- 当前桌面构建和打包脚本需要 Apple Silicon macOS 13+

安装依赖：

```bash
npm install
```

运行前端开发服务器：

```bash
npm run dev
```

开发服务器运行在 5173 端口。此模式没有 Wails runtime，因此 `frontend/src/wails-runtime.ts` 必须保持可空安全。

执行检查：

```bash
npm run typecheck
env GOCACHE=/tmp/go-build-cache go test ./...
```

构建前端产物：

```bash
npm run build
```

构建桌面二进制：

```bash
./scripts/build-darwin-aarch64.sh
VERSION=1.0.0 ./scripts/build-darwin-aarch64.sh
./scripts/build-darwin-aarch64.sh --dev
```

构建脚本会生成 `build/appicon.png`，执行 `npm run build`，并输出 `build/bin/investgo-darwin-aarch64`。

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

输出产物：

- `build/macos/InvestGo.app`
- `build/bin/investgo-<version>-darwin-aarch64.dmg`

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

## 运行时说明

- `--dev` 会在构建时启用终端日志和 F12 Web Inspector 支持。F12 仍需要在应用内启用 `developerMode` 设置。
- 版本号通过 `-X main.appVersion=$APP_VERSION` 注入。如果没有设置 `VERSION` 或 `APP_VERSION`，应用显示 `dev`。
- 代理模式支持 `none`、`system` 和 `custom`。系统代理检测目前只在 macOS 上调用 `scutil --proxy`。
- 前端可见文案是双语的。修改用户可见文案时，应同时更新 `frontend/src/i18n.ts` 中的 `zh-CN` 和 `en-US`。
- 当前没有前端测试。前端验证使用 `npm run typecheck`，后端验证使用 `internal/**` 下的 Go tests。

## 免责声明

1. 因使用本软件而产生的任何投资损失或收益。
2. 本软件所提供数据的准确性、及时性或完整性。
3. 因网络故障、数据源变更或其他技术问题导致的数据中断或错误。
4. 任何基于本软件信息做出的投资决策及其结果。

投资有风险，入市需谨慎。用户在使用本软件前应充分了解投资风险，并自行承担所有投资决策的后果。

## 许可证

本项目基于 [MIT License](./LICENSE) 开源。
