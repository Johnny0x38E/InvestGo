# InvestGo

个人桌面投资观察工具，基于 Go 1.25 + Wails v3 构建，前端使用 Vue 3 + TypeScript + PrimeVue。主要运行于 macOS arm64，其他平台支持有限。

> 本工具仅供个人行情观察，不提供任何投资建议。

---

## 功能概览

### 自选列表

- 新增、编辑、删除标的，保存后立即触发一次行情补刷
- 字段涵盖：Symbol、Name、Market、Currency、持仓数量、成本价、当前价、涨跌幅、投资逻辑（Thesis）、Tags
- 支持定投（DCA）记录维护；有定投记录时，持仓数量和成本价由定投记录自动推算，不可手动覆盖
- 按 `UpdatedAt` 倒序展示

### 定投（DCA）

每个自选标的可附挂多笔定投记录，支持以下字段：

| 字段        | 说明                                       | 必填 |
| ----------- | ------------------------------------------ | ---- |
| 日期        | 本次定投日期                               | 是   |
| 投入金额    | 本次实际投入的资金金额                     | 是   |
| 买入股/份   | 本次实际获得的份额 / 股数                  | 是   |
| 买入价      | 手动录入的实际成交价（含手续费的到手价格） | 否   |
| 手续费/佣金 | 本次定投产生的手续费或佣金                 | 否   |
| 备注        | 自由文本                                   | 否   |

**有效成本推算规则**（与后端 `sanitiseItem` 保持一致）：

1. 若填写了**买入价** (`Price > 0`)：`effectiveCost = Price × Shares`（买入价视为已含手续费的到手成交价）
2. 否则：`effectiveCost = max(Amount − Fee, 0)`（从投入金额中扣除手续费）

**加权均价** = Σ effectiveCost / Σ Shares；结果自动写入标的的 `CostPrice` 和 `Quantity`，所有持仓盈亏计算均继承该结果，无须在多处维护。

在自选列表点击定投徽标（`N 笔`）可打开**定投明细弹窗**，查看每笔记录及整体汇总（总投入、总手续费/佣金、累计份额、加权均价、当前资产、浮动盈亏）；点击"编辑记录"可直接跳转到标的编辑弹窗的定投标签页。

### 实时行情

- **主源**：东方财富（覆盖沪深港美全市场）；**备源**：Yahoo Finance
- A股 / 港股 / 美股可分别独立选择 provider
- 美股并发尝试 NASDAQ(105) / NYSE(106) / NYSE Arca(107) 三个交易所
- 北交所（CN-BJ）暂不支持东方财富实时行情，仅可使用 Yahoo Finance
- 每次刷新后重新评估所有提醒规则并持久化

### 历史走势

- 区间：`1h` / `1d` / `1w` / `1mo` / `1y` / `3y` / `all`
- 主源：东方财富 K 线 API；备源：Yahoo Chart API
- Store 根据市场和用户偏好自动 fallback，图表数据不做本地缓存

### 价格提醒

- 规则字段：名称、关联标的、条件（above / below）、阈值、启停、触发状态、最近触发时间
- 每次行情刷新后全量 evaluate，命中结果保存到磁盘
- 目前仅本地命中判断，未接 macOS 系统通知中心

### 热门榜单

- 分类：`cn-a` / `cn-etf` / `hk` / `hk-etf` / `us-sp500` / `us-nasdaq` / `us-dow` / `us-etf`
- 排序：volume / gainers / losers / market-cap / price
- 主源：东方财富 clist API；美股 ETF 种子补充：Yahoo Search
- 支持关键词搜索和无限滚动分页
- 已在自选列表中的标的显示"已添加"标签，其余显示"加入自选"按钮

### 仪表盘汇总

- 多币种折算为统一展示货币（CNY / HKD / USD）
- 汇率来源：新浪财经（`hq.sinajs.cn`），4 小时缓存；fallback 硬编码兜底（USD=7, HKD=0.85）
- 顶部四卡片：**组合成本**、**当前资产**（持仓总市值）、**未实现盈亏**、**触发提醒**
- 盈利 / 亏损标的计数：仅统计 `Quantity > 0 && CostPrice > 0` 的持仓，纯观察标的不参与

### 设置

| 设置项                                              | 说明                              |
| --------------------------------------------------- | --------------------------------- |
| `RefreshIntervalSeconds`                            | 自动刷新间隔（≥10 秒）            |
| `CNQuoteSource` / `HKQuoteSource` / `USQuoteSource` | 各市场行情源（eastmoney / yahoo） |
| `HotUSSource`                                       | 美股热门榜单来源                  |
| `ThemeMode`                                         | system / light / dark             |
| `ColorTheme`                                        | blue / graphite / forest / sunset |
| `FontPreset`                                        | system / reading / compact        |
| `AmountDisplay`                                     | full / compact                    |
| `CurrencyDisplay`                                   | symbol / code                     |
| `PriceColorScheme`                                  | cn（红涨绿跌）/ intl（绿涨红跌）  |
| `Locale`                                            | system / zh-CN / en-US            |
| `DashboardCurrency`                                 | CNY / HKD / USD                   |
| `DeveloperMode`                                     | 启用 F12 DevTools                 |
| `UseNativeTitleBar`                                 | 显示原生标题栏                    |

### macOS 特性

- 自动读取系统代理（`scutil --proxy`），注入 `HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY`
- 半透明背景（`MacBackdropTranslucent`）
- 可选隐藏原生标题栏（`MacTitleBarHiddenInsetUnified`）
- F12 开发者工具（需 `DeveloperMode=true` + devtools 构建标签）

---

## 数据源

所有数据源均为公开免费接口，字段可能随时变更，代码已实现分层 fallback；适合个人观察，不适合严格交易执行或合规记账。

| 数据               | 主源                           | 备源            |
| ------------------ | ------------------------------ | --------------- |
| A股 / ETF 实时报价 | 东方财富 (push2.eastmoney.com) | Yahoo Finance   |
| 港股实时报价       | 东方财富                       | Yahoo Finance   |
| 美股实时报价       | 东方财富（三交易所并发）       | Yahoo Finance   |
| 历史 K 线图表      | 东方财富 K 线 API              | Yahoo Chart API |
| 热门榜单           | 东方财富 clist API             | Yahoo Search    |
| CNY/HKD/USD 汇率   | 新浪财经 (hq.sinajs.cn)        | 硬编码兜底      |

---

## 项目结构

```
investgo/
├── main.go                        # 应用入口：组装依赖、注册路由、启动 Wails
├── internal/
│   ├── datasource/
│   │   └── endpoints.go           # 所有外部 API 端点常量及 URL 构造工具
│   ├── monitor/                   # 核心业务层
│   │   ├── model.go               # 领域模型（WatchlistItem, DCAEntry, AlertRule, AppSettings 等）
│   │   ├── quotes.go              # QuoteProvider 接口 + QuoteTarget 标的规范化
│   │   ├── history_range.go       # HistoryProvider 接口 + HistoryInterval 枚举
│   │   ├── store.go               # Store 核心（构造、Save、Snapshot）
│   │   ├── store_mutations.go     # CRUD 方法（UpsertItem/DeleteItem/UpsertAlert/...）及定投推算（sanitiseItem）
│   │   ├── store_runtime.go       # 运行时方法（Refresh, ItemHistory）
│   │   ├── store_state.go         # 状态加载/持久化/快照/提醒评估/仪表盘聚合/种子数据
│   │   ├── fxrates.go             # 汇率服务（新浪财经 + 兜底汇率）
│   │   ├── logbook.go             # 日志簿（内存环形缓冲 + 文件 + 终端）
│   │   └── *_test.go              # 单元测试
│   ├── marketdata/                # 行情数据接入层
│   │   ├── quote_sources.go       # EastMoneyQuoteProvider + YahooQuoteProvider + Registry
│   │   ├── yahoo.go               # Yahoo Chart API 工具函数
│   │   ├── history.go             # EastMoneyChartProvider + SmartHistoryProvider 工厂
│   │   ├── history_yahoo.go       # YahooChartProvider
│   │   ├── hot.go                 # HotService（热门榜单、种子池、分页、搜索）
│   │   ├── hot_fallback.go        # 热门榜单降级策略
│   │   ├── helpers.go             # 共享工具函数
│   │   └── *_test.go
│   └── api/
│       ├── http.go                # Handler 构造 + ServeHTTP + JSON 工具
│       ├── routes.go              # 全部 API 路由（15 个端点）
│       └── http_test.go
├── frontend/
│   ├── src/
│   │   ├── main.ts                # Vue 应用入口 + PrimeVue 初始化
│   │   ├── App.vue                # 根组件，协调所有模块和弹窗
│   │   ├── types.ts               # 类型定义（含 DCAEntry / DCAEntryRow）
│   │   ├── forms.ts               # 表单映射与序列化（含定投字段双向映射）
│   │   ├── format.ts              # 格式化工具
│   │   ├── constants.ts           # 常量
│   │   ├── style.css              # 全局样式
│   │   └── components/
│   │       ├── AppHeader.vue      # 顶部应用标题栏
│   │       ├── ModuleTabs.vue     # 模块标签切换
│   │       ├── PriceChart.vue     # 价格走势图表组件
│   │       ├── SummaryStrip.vue   # 仪表盘汇总条（组合成本 / 当前资产 / 盈亏 / 提醒）
│   │       ├── modules/           # 功能模块视图
│   │       │   ├── MarketModule.vue    # 行情详情（K 线图 + 区间选择器 + 持仓卡）
│   │       │   ├── WatchlistModule.vue # 自选列表（含定投徽标列）
│   │       │   ├── HotModule.vue       # 热门榜单（无限滚动 + 排序 + 搜索）
│   │       │   └── AlertsModule.vue    # 提醒规则
│   │       └── dialogs/           # 弹窗组件
│   │           ├── ItemDialog.vue      # 标的编辑弹窗（基础信息 / 定投记录 双标签页）
│   │           ├── DCADetailDialog.vue # 定投明细只读弹窗（汇总 + 逐笔明细）
│   │           ├── AlertDialog.vue     # 提醒规则编辑弹窗
│   │           ├── ConfirmDialog.vue   # 通用确认弹窗
│   │           └── SettingsDialog.vue  # 设置弹窗
│   └── dist/                      # 构建产物（嵌入 Go 二进制，不提交）
├── scripts/
│   ├── build-macos-arm64.sh       # 生产/开发构建脚本
│   └── package-macos-dmg.sh       # macOS .app + .dmg 打包
├── build/
│   └── appicon.png                # 应用图标（嵌入二进制）
├── doc/
│   └── backend-architecture.md    # 后端架构文档
├── go.mod                         # Go 1.25，主要依赖 Wails v3
├── package.json                   # 前端依赖（pnpm/npm）
├── tsconfig.json
├── vite.config.ts
└── AGENTS.md                      # AI 助手协作指南
```

---

## 开发命令

```bash
# 安装前端依赖
pnpm install          # 或 npm install

# 前端开发服务器（http://localhost:5173）
npm run dev

# 前端类型检查
npm run typecheck

# 构建前端（Go build 前必须先执行）
npm run build

# 后端测试
env GOCACHE=/tmp/go-build-cache go test ./...

# 后端构建
env GOCACHE=/tmp/go-build-cache go build ./...

# 开发模式运行（启用终端日志）
go run main.go -dev

# 生产构建 macOS arm64（输出：build/bin/investgo-macos-arm64）
./scripts/build-macos-arm64.sh

# 开发构建（启用 F12 DevTools）
./scripts/build-macos-arm64.sh -dev
```

---

## 构建与打包

```bash
# 打包 .app + .dmg
VERSION=0.1.0 APP_ID=com.example.investgo ./scripts/package-macos-dmg.sh

# 带签名和公证
APPLE_SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" \
NOTARYTOOL_PROFILE="investgo-notary" \
VERSION=0.1.0 APP_ID=com.yourdomain.investgo \
./scripts/package-macos-dmg.sh
```

输出：`build/macos/InvestGo.app` 和 `build/bin/investgo-<version>-macos-arm64.dmg`，DMG 内含 `/Applications` 快捷方式，可直接拖拽安装。

---

## 持久化

- **状态文件**：`~/Library/Application Support/investgo/state.json`
    - 存储：Items（含 DCAEntries）+ Alerts + Settings + UpdatedAt
    - 写入：先写 `.tmp` 临时文件，再原子 rename，防止写入中断损坏
    - 首次启动：自动写入三个示例标的（贵州茅台 / 腾讯控股 / QQQ）和三条示例提醒
    - 向后兼容：`DCAEntries` 使用 `omitempty`，旧 state.json 无需迁移
- **日志文件**：`~/Library/Application Support/investgo/logs/app.log`

---

## 已知限制

- 北交所（CN-BJ）实时行情不支持东方财富，仅可使用 Yahoo Finance
- 价格提醒仅做本地评估，未接 macOS 系统通知中心
- 持久化使用 JSON，未升级到 SQLite
- 历史图表无本地缓存，每次请求都访问外部接口
- 主要针对 macOS arm64 优化，其他平台支持有限

---

## 免责声明 / Disclaimer

### 中文

**重要提示**：本软件仅用于个人学习和投资观察目的，不构成任何形式的投资建议、财务建议或买卖建议。

使用本软件所提供的所有数据、信息和功能，用户应当自行判断其准确性和完整性。作者和贡献者不对以下情况承担任何责任：

1. 因使用本软件而产生的任何投资损失或收益；
2. 本软件所提供数据的准确性、及时性或完整性；
3. 因网络故障、数据源变更或其他技术问题导致的数据中断或错误；
4. 任何基于本软件信息做出的投资决策的结果。

投资有风险，入市需谨慎。用户在使用本软件前应充分了解投资风险，并自行承担所有投资决策的后果。

### English

**IMPORTANT NOTICE**: This software is intended for personal learning and investment observation purposes only and does not constitute any form of investment advice, financial advice, or recommendation to buy or sell.

Users should independently verify the accuracy and completeness of all data, information, and functions provided by this software. The authors and contributors assume no liability for:

1. Any investment losses or gains resulting from the use of this software;
2. The accuracy, timeliness, or completeness of the data provided;
3. Data interruptions or errors caused by network failures, data source changes, or other technical issues;
4. Any outcomes from investment decisions based on information from this software.

Investment involves risks. Users should fully understand the investment risks before using this software and assume full responsibility for all consequences of their investment decisions.

---

## 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。
