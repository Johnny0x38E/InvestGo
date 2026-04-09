# Invest Monitor

一个从零开始构建的 Go + Wails v3 桌面投资观察台，目标是长期日用，而不是一次性的迁移实验。

## 当前能力

- 实时行情：
  - A 股 / 港股优先走腾讯公开行情，失败后回退新浪
  - 美股 / 美股 ETF 走新浪公开行情
- 多周期走势：
  - 为观察列表里的标的提供日线 / 周线 / 月线价格走势
  - 历史图表统一走 Yahoo Finance Chart API
- 观察列表与提醒：
  - 标的代码规范化、市场自动识别、仓位 / 成本 / 标签 / 备注
  - 价格阈值提醒，支持启停与最近触发时间
- 桌面体验：
  - macOS 使用隐藏原生标题栏 + 自定义拖拽区
  - 界面按桌面工作台组织，而不是单页网页样式
- 本地持久化：
  - 状态存储在用户配置目录下的 `state.json`

## 项目结构

```text
invest-monitor-v3/
├── frontend/
│   ├── app.js            # 页面状态与交互编排
│   ├── chart.js          # 走势图渲染
│   ├── format.js         # 前端格式化工具
│   ├── app.css
│   └── index.html
├── internal/monitor/
│   ├── model.go          # 领域模型
│   ├── quotes.go         # 实时行情 provider
│   ├── history.go        # 历史走势 provider
│   ├── store.go          # 本地状态与业务协调
│   ├── http.go           # 内部 HTTP API
│   └── *_test.go
├── scripts/
│   └── build-macos-arm64.sh
├── main.go
└── go.mod
```

## 运行

```bash
cd invest-monitor-v3
env GOCACHE=/tmp/go-build-cache go build .
./invest-monitor-v3
```

## macOS arm64 构建

```bash
cd invest-monitor-v3
./scripts/build-macos-arm64.sh
```

产物输出到：

```text
build/bin/invest-monitor-macos-arm64
```

## macOS arm64 DMG 打包

```bash
cd invest-monitor-v3
VERSION=0.1.0 \
APP_ID=com.example.invest-monitor \
./scripts/package-macos-dmg.sh
```

默认产物：

```text
build/macos/Invest Monitor.app
build/bin/invest-monitor-0.1.0-macos-arm64.dmg
```

可选签名与公证：

```bash
APPLE_SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" \
NOTARYTOOL_PROFILE="invest-monitor-notary" \
VERSION=0.1.0 \
APP_ID=com.yourdomain.invest-monitor \
./scripts/package-macos-dmg.sh
```

说明：

- `APP_ID` 在本地自用时可以先用占位值；对外分发时应改成你自己的稳定 bundle identifier。
- 图标单一源文件是 `frontend/src/assets/app-mark.svg`；构建脚本会先把它渲染成 `build/appicon.png`，再生成 macOS bundle 所需的 `.icns` 图标资源。
- `SKIP_DMG_CREATE=1` 可以只生成 `.app`；`SKIP_APP_BUILD=1` 可以复用已有 `.app` 重新封装 `.dmg`。
- 设置了 `APPLE_SIGN_IDENTITY` 时，脚本会对 `.app` 和 `.dmg` 做 `codesign`。
- 额外设置 `NOTARYTOOL_PROFILE` 时，脚本会提交 DMG 到 Apple notarization，然后执行 `stapler staple`。
- 打出来的 DMG 自带 `/Applications` 快捷方式，适合拖拽安装到 Apple Silicon 的 macOS。

## 数据源说明

- `qt.gtimg.cn`
  - A 股 / 港股实时行情
- `hq.sinajs.cn`
  - 实时行情回退源，美股实时行情主源
- `query1.finance.yahoo.com`
  - 日线 / 周线 / 月线历史走势

这些源适合个人观察工具，不适合做严格交易执行或合规记账。公开接口字段随时可能变化，所以代码里做了 provider 分层和基础测试。

## 当前边界

- 北交所实时价支持，但历史走势还没有补到统一历史图表 provider
- 提醒规则目前只做本地命中判断，还没有接桌面通知中心
- 持久化仍然是 JSON，尚未升级到 SQLite

## 下一步建议

1. 给提醒规则接系统通知，命中阈值时直接弹出桌面通知。
2. 给标的录入接搜索 / 自动补全，而不是手工输入代码。
3. 把历史走势和实时快照存档到 SQLite，补齐观察日志。
4. 给图表增加成交量与切换基准指数。
