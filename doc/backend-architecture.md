# 后端架构文档

## 目录

- [整体结构](#整体结构)
- [包职责划分](#包职责划分)
- [依赖方向](#依赖方向)
- [领域模型](#领域模型)
- [接口契约](#接口契约)
- [internal/monitor 详解](#internalmonitor-详解)
- [internal/marketdata 详解](#internalmarketdata-详解)
- [internal/api 详解](#internalapi-详解)
- [internal/datasource 详解](#internaldatasource-详解)
- [运行时流程](#运行时流程)
- [持久化机制](#持久化机制)
- [日志系统](#日志系统)
- [汇率服务](#汇率服务)
- [macOS 代理探测](#macos-代理探测)
- [扩展指南](#扩展指南)

---

## 整体结构

后端按职责分为四层：

```
main.go
  ├── internal/api          HTTP 适配层：路由、参数解析、JSON 编解码
  ├── internal/monitor      业务核心层：状态管理、CRUD、提醒评估、快照输出
  ├── internal/marketdata   数据接入层：行情/历史/热门的外部实现
  └── internal/datasource   基础设施：端点常量与 URL 构造工具
```

`main.go` 负责：

- 创建 `LogBook`、`QuoteProvider` 注册表、`Store`、`HotService`
- 把这些实例注入 `api.NewHandler`
- 注册 `/api/*` → `api.Handler`、`/` → 前端静态资源
- 读取持久化设置，决定标题栏样式
- 在 macOS 上自动探测并注入系统代理
- 注册关机回调，在 Wails 退出时 `Store.Save()`

---

## 包职责划分

### `internal/monitor`

这里是应用自己的规则，与任何外部服务无关：

| 文件                 | 职责                                                                                                     |
| -------------------- | -------------------------------------------------------------------------------------------------------- |
| `model.go`           | 所有领域类型定义（见[领域模型](#领域模型)）                                                              |
| `quotes.go`          | `QuoteProvider` 接口、`Quote` 结构、`QuoteTarget` 规范化逻辑                                             |
| `history_range.go`   | `HistoryProvider` 接口、`HistoryInterval` 枚举                                                           |
| `store.go`           | `Store` 结构体、`NewStore`、`Save`、`Snapshot`、`CurrentSettings`                                        |
| `store_mutations.go` | `UpsertItem`、`DeleteItem`、`UpsertAlert`、`DeleteAlert`、`UpdateSettings`、所有 sanitise/normalise 函数 |
| `store_runtime.go`   | `Refresh`（行情刷新）、`ItemHistory`（历史走势委托）、行情回填辅助函数                                   |
| `store_state.go`     | 状态加载/保存/快照/提醒评估/内部查找/种子数据/工具函数                                                   |
| `fxrates.go`         | `FxRates` 汇率服务（新浪财经拉取 + 兜底）                                                                |
| `logbook.go`         | `LogBook` 日志聚合器                                                                                     |

这个包**不依赖** `internal/api` 和 `internal/marketdata`，只暴露接口让外层实现。

### `internal/marketdata`

这里是"怎么跟外部源打交道"的全部实现：

| 文件               | 职责                                                                         |
| ------------------ | ---------------------------------------------------------------------------- |
| `quote_sources.go` | `EastMoneyQuoteProvider`、`YahooQuoteProvider`、`DefaultQuoteSourceRegistry` |
| `yahoo.go`         | Yahoo Chart API 工具函数（`fetchYahooChart`、`buildHistoryPoints` 等）       |
| `history.go`       | `EastMoneyChartProvider`（东方财富 K 线）、`NewSmartHistoryProvider` 工厂    |
| `history_yahoo.go` | `YahooChartProvider`（Yahoo Chart API 历史数据）                             |
| `hot.go`           | `HotService`（热门榜单、种子池、分页、搜索、缓存）                           |
| `hot_fallback.go`  | 热门榜单降级策略                                                             |
| `helpers.go`       | 共享工具函数（`collectQuoteTargets`、`buildQuote`、`collapseProblems` 等）   |

这个包实现 `monitor` 定义的接口，依赖 `internal/monitor` 的类型，反向不成立。

### `internal/api`

纯适配层，无状态，不保存任何数据：

| 文件           | 职责                                                       |
| -------------- | ---------------------------------------------------------- |
| `http.go`      | `Handler` 结构体、`NewHandler`、`ServeHTTP`、JSON 工具函数 |
| `routes.go`    | 全部 15 个路由注册及其 handler 实现                        |
| `http_test.go` | HTTP 层集成测试                                            |

### `internal/datasource`

仅包含一个文件 `endpoints.go`，统一维护所有外部 API 端点：

```
EastMoneyQuoteAPI     push2.eastmoney.com/api/qt/ulist.np/get
EastMoneyHotAPI       push2.eastmoney.com/api/qt/clist/get
EastMoneyHistoryAPI   push2his.eastmoney.com/api/qt/stock/kline/get
YahooChartHosts       query1/query2.finance.yahoo.com  (轮询)
YahooSearchHosts      query1/query2.finance.yahoo.com  (轮询)
YahooScreenerListAPI  query1.finance.yahoo.com/v1/finance/screener/predefined/saved
YahooScreenerAPI      query2.finance.yahoo.com/v1/finance/screener
SinaFXRatesAPI        hq.sinajs.cn/list=usdcny,hkdcny
```

---

## 依赖方向

```
main ──► api ──► monitor ◄── marketdata ◄── datasource
  │                 ▲
  └────────────────►┘
  └──► marketdata
  └──► datasource (间接，通过 marketdata)
```

硬性约束：

- `monitor` **不能** import `api` 或 `marketdata`
- `api` 可以 import `monitor` 和 `marketdata`（因为它是组装适配层）
- `marketdata` 可以 import `monitor`（实现其接口）和 `datasource`（读取端点）
- `datasource` 不 import 任何内部包

---

## 领域模型

### WatchlistItem

用户自选标的。市价字段（CurrentPrice、PreviousClose 等）由行情刷新回填，不由用户直接写入。


```go
type WatchlistItem struct {
    ID             string           // 唯一标识
    Symbol         string           // 规范化后的代码，如 600519.SH、00700.HK、QQQ
    Name           string
    Market         string           // 见市场枚举
    Currency       string           // CNY / HKD / USD
    Quantity       float64          // 持仓数量（0 表示仅观察）
    CostPrice      float64          // 成本价（单价）
    AcquiredAt     *time.Time       // 首次建仓时间（仅持仓项有效；纯观察项由 sanitiseItem 清除）
    CurrentPrice   float64          // 最新价（行情回填）
    PreviousClose  float64
    OpenPrice      float64
    DayHigh        float64
    DayLow         float64
    Change         float64
    ChangePercent  float64
    QuoteSource    string
    QuoteUpdatedAt *time.Time
    PinnedAt       *time.Time       // 非 nil 时表示已置顶
    Thesis         string           // 投资逻辑备注
    Tags           []string         // 自定义标签（去重、去空）
    DCAEntries     []DCAEntry       // 定投记录（纯观察项为空）
    DCASummary     *DCASummary      // 由 DCAEntries 聚合，非 nil 表示有有效定投记录
    Position       *PositionSummary // 后端派生的持仓指标；前端应使用 Position.HasPosition 而非自行判断
    UpdatedAt      time.Time
}
```

> **watchOnly 语义**：Quantity == 0 且 DCAEntries 为空的项为纯观察项。`sanitiseItem` 在保存纯观察项时会主动清除 `AcquiredAt`，使其不参与 Overview 趋势计算。

**市场枚举**：

| 值         | 含义             |
| ---------- | ---------------- |
| `CN-A`     | 沪深主板 A 股    |
| `CN-GEM`   | 创业板           |
| `CN-STAR`  | 科创板（688xxx） |
| `CN-ETF`   | 沪深 ETF         |
| `CN-BJ`    | 北交所           |
| `HK-MAIN`  | 港股主板         |
| `HK-GEM`   | 港股创业板       |
| `HK-ETF`   | 港股 ETF         |
| `US-STOCK` | 美股             |
| `US-ETF`   | 美股 ETF         |

### DCAEntry / DCASummary / PositionSummary

定投记录和持仓派生指标：

```go
// DCAEntry 记录一次定投操作。
type DCAEntry struct {
    ID             string
    Date           time.Time
    Amount         float64    // 投入金额
    Shares         float64    // 买入份额
    Price          float64    // 期望价（可选）
    Fee            float64    // 手续费（可选）
    Note           string     // 备注（可选）
    EffectivePrice float64    // 实际成交均价（Amount / Shares）
}

// DCASummary 汇总 DCAEntries 的聚合指标，由后端在快照时计算。
type DCASummary struct {
    Count           int
    TotalAmount     float64
    TotalShares     float64
    TotalFees       float64
    AverageCost     float64
    CurrentValue    float64
    PnL             float64
    PnLPct          float64
    HasCurrentPrice bool
}

// PositionSummary 由后端基于 Quantity / CostPrice 派生，随快照下发给前端。
// 前端通过 item.position?.hasPosition 判断是否有持仓，不在客户端重新计算。
type PositionSummary struct {
    CostBasis        float64
    MarketValue      float64
    UnrealisedPnL    float64
    UnrealisedPnLPct float64
    HasPosition      bool
}
```

### AlertRule

```go
type AlertRule struct {
    ID              string
    ItemID          string
    Name            string
    Condition       AlertCondition  // "above" 或 "below"
    Threshold       float64         // 必须 > 0
    Enabled         bool
    Triggered       bool            // 每次行情刷新时重新计算，不持久化
    LastTriggeredAt *time.Time
    UpdatedAt       time.Time
}
```

### AppSettings

```go
type AppSettings struct {
    RefreshIntervalSeconds int     // 最小 10 秒
    CNQuoteSource          string  // "eastmoney" | "yahoo"
    HKQuoteSource          string
    USQuoteSource          string
    HotUSSource            string  // "eastmoney" | "yahoo"
    ThemeMode              string  // "system" | "light" | "dark"
    ColorTheme             string  // "blue" | "graphite" | "forest" | "sunset" | "rose" | "violet" | "amber"
    FontPreset             string  // "system" | "reading" | "compact"
    AmountDisplay          string  // "full" | "compact"
    CurrencyDisplay        string  // "symbol" | "code"
    PriceColorScheme       string  // "cn" | "intl"
    Locale                 string  // "system" | "zh-CN" | "en-US"
    DashboardCurrency      string  // "CNY" | "HKD" | "USD"
    DeveloperMode          bool
    UseNativeTitleBar      bool
    QuoteSource            string  // legacy 字段，兼容旧 state.json
}
```

### StateSnapshot

前端消费的只读快照，每次调用 `Snapshot()` 或写操作结束时重新构建。

```go
type StateSnapshot struct {
    Dashboard    DashboardSummary
    Items        []WatchlistItem     // 按 UpdatedAt 倒序
    Alerts       []AlertRule         // 已触发的排在前面，其次按 UpdatedAt 倒序
    Settings     AppSettings
    Runtime      RuntimeStatus
    QuoteSources []QuoteSourceOption
    StoragePath  string
    GeneratedAt  time.Time
}
```

### DashboardSummary

仪表盘聚合，所有金额统一折算为 `DisplayCurrency`：

```go
type DashboardSummary struct {
    TotalCost       float64
    TotalValue      float64
    TotalPnL        float64
    TotalPnLPct     float64
    ItemCount       int
    TriggeredAlerts int
    WinCount        int
    LossCount       int
    DisplayCurrency string
}
```

### HistoryPoint / HistorySeries

```go
type HistoryPoint struct {
    Timestamp time.Time
    Open, High, Low, Close, Volume float64
}

type HistorySeries struct {
    Symbol, Name, Market, Currency string
    Interval      HistoryInterval
    Source        string
    StartPrice    float64  // Points[0].Close
    EndPrice      float64  // Points[last].Close
    High          float64  // 区间最高点
    Low           float64  // 区间最低点
    Change        float64
    ChangePercent float64
    Points        []HistoryPoint
    GeneratedAt   time.Time
}
```

### HotItem / HotListResponse

```go
type HotItem struct {
    Symbol, Name, Market, Currency string
    CurrentPrice, Change, ChangePercent float64
    Volume, MarketCap                   float64
    QuoteSource string
    UpdatedAt   time.Time
}

type HotListResponse struct {
    Category    HotCategory
    Sort        HotSort
    Page, PageSize, Total int
    HasMore     bool
    Items       []HotItem
    GeneratedAt time.Time
}
```

---

## 接口契约

### QuoteProvider

```go
type QuoteProvider interface {
    Fetch(ctx context.Context, items []WatchlistItem) (map[string]Quote, error)
    Name() string
}
```

返回的 `map` 以 `QuoteTarget.Key` 为索引。Key 的格式：

- A 股 / ETF：`600519.SH`、`000001.SZ`
- 港股：`00700.HK`（5 位，左补零）
- 美股：`QQQ`（大写，`.` 替换为 `-`）
- 北交所：`430090.BJ`

### HistoryProvider

```go
type HistoryProvider interface {
    Fetch(ctx context.Context, item WatchlistItem, interval HistoryInterval) (HistorySeries, error)
    Name() string
}
```

`HistoryInterval` 枚举：`1h / 1d / 1w / 1mo / 1y / 3y / all`

---

## internal/monitor 详解

### Store

`Store` 是应用的中心协调器，内部用 `sync.RWMutex` 保护状态：

```go
type Store struct {
    mu                 sync.RWMutex
    path               string
    quoteProviders     map[string]QuoteProvider
    quoteSourceOptions []QuoteSourceOption
    historyProviders   map[string]HistoryProvider  // 由 NewSmartHistoryProvider 注入
    logs               *LogBook
    state              persistedState
    runtime            RuntimeStatus
    fxRates            *FxRates
}
```

#### 锁策略

- 网络请求（行情拉取、汇率刷新）**不持有** Store 锁，在锁外完成
- 写操作取得 `mu.Lock()` 后，才更新 state、评估提醒、调用 `saveLocked()`
- `Snapshot()` 先在锁外刷新汇率（若过期），再取读锁构建快照副本
- 快照中的切片（Items、Alerts、QuoteSources）都是独立副本，不共享底层数组

#### UpsertItem 流程

1. `sanitiseItem`：规范化 Symbol/Market/Currency，检验 Quantity/Price 非负
2. 读锁内取出 provider 和旧条目（若是更新）
3. 若是更新，调用 `inheritLiveFields` 保留盘口字段
4. 锁外请求一跳行情（8s 超时）
5. 写锁内执行 append（新增）或下标替换（更新），调用 `evaluateAlertsLocked`，写盘

#### Refresh 流程

1. 读锁内复制 Items 切片，按市场分组（每个市场取对应 provider）
2. 锁外批量请求各 provider
3. 写锁内回填行情、更新 runtime、评估提醒、写盘

#### 提醒评估（evaluateAlertsLocked）

- 先按 ItemID 建价格索引
- 遍历所有 AlertRule：若 Enabled=false 或 CurrentPrice<=0，Triggered=false
- `AlertAbove`：price >= Threshold → Triggered=true
- `AlertBelow`：price <= Threshold → Triggered=true
- 每次 Refresh / UpsertItem / UpsertAlert / UpdateSettings 后都会重新执行

#### historyProviderCandidatesLocked

根据市场和用户偏好组装 provider 候选列表（按优先级排序，Store.ItemHistory 依次 fallback）：

- 美股：用户偏好源 → yahoo → eastmoney
- 其他市场：用户偏好源 → eastmoney → yahoo

#### 设置规范化（sanitiseSettings）

- 空字符串字段不覆盖当前值（Boolean 除外，Boolean 显式采用输入值）
- `QuoteSource`（legacy）在三个市场来源都空时作为共同回退
- 每个市场来源最终通过 `normaliseQuoteSourceIDLocked` 确保 provider 存在且支持对应市场
- 返回 error 时不修改 state

### QuoteTarget 规范化

`resolveQuoteTarget` 把各种用户输入格式收敛到系统内部 Key：

| 输入示例                                 | 解析结果                            |
| ---------------------------------------- | ----------------------------------- |
| `600519`、`600519.SH`、`SH600519`        | `600519.SH` / `CN-A`                |
| `688001`、`688001.SH`                    | `688001.SH` / `CN-STAR`             |
| `300750`、`300750.SZ`                    | `300750.SZ` / `CN-GEM`              |
| `159915`、`510300.SH`                    | `xxx.SZ` 或 `.SH` / `CN-ETF`        |
| `430090`、`430090.BJ`、`BJ430090`        | `430090.BJ` / `CN-BJ`               |
| `700`、`00700`、`00700.HK`、`HK00700`    | `00700.HK` / `HK-MAIN`（5位左补零） |
| `QQQ`、`AAPL`、`GB_AAPL`                 | `QQQ` / `US-ETF` 或 `US-STOCK`      |
| `BRK-B`（含连字符，内部 `.` 替换为 `-`） | `BRK-B` / `US-STOCK`                |

---

## internal/marketdata 详解

### EastMoneyQuoteProvider

东方财富实时行情主源，使用 `push2.eastmoney.com/api/qt/ulist.np/get` 批量查询。

SecID 映射规则：

- 沪市（`.SH`）：`1.{code}`
- 深市（`.SZ`）：`0.{code}`
- 港股（`.HK`）：`116.{5位code}`
- 美股：同时请求 `105.{ticker}`、`106.{ticker}`、`107.{ticker}`（NASDAQ/NYSE/NYSE Arca）
- 北交所（`.BJ`）：**不支持**，返回 error

### YahooQuoteProvider

通过 Yahoo Chart API（`/v8/finance/chart/{symbol}`，range=5d，interval=1d）拉取最近两个交易日数据，取最后一根 K 线作为实时快照。

Yahoo Symbol 映射：

- A 股：`600519.SS`（SH）、`000001.SZ`
- 港股：`0700.HK`
- 北交所：不支持
- 美股：原始 ticker（`-` 替换为 `空格` 或保持）

### EastMoneyChartProvider

通过 `push2his.eastmoney.com/api/qt/stock/kline/get` 拉取 K 线数据。

K 线周期映射：

| HistoryInterval | klt | 备注                      |
| --------------- | --- | ------------------------- |
| `1h`            | 60  | 分钟级，取最近 1 小时数据 |
| `1d`            | 60  | 分钟级，取最近 24 小时    |
| `1w`            | 101 | 日 K，最多 10 根          |
| `1mo`           | 101 | 日 K，最多 35 根          |
| `1y`            | 101 | 日 K，最多 270 根         |
| `3y`            | 102 | 周 K，最多 160 根         |
| `all`           | 103 | 月 K，999 根              |

时间戳解析使用 `CST`（Asia/Shanghai UTC+8）。

### YahooChartProvider

使用 Yahoo Chart API，`query1` 和 `query2` 主机轮询（若一个失败尝试另一个）。

`HistoryInterval` → Yahoo Chart 参数映射：

| 区间  | range | interval |
| ----- | ----- | -------- |
| `1h`  | `1d`  | `1h`     |
| `1d`  | `5d`  | `1d`     |
| `1w`  | `1mo` | `1d`     |
| `1mo` | `3mo` | `1d`     |
| `1y`  | `1y`  | `1wk`    |
| `3y`  | `5y`  | `1mo`    |
| `all` | `max` | `3mo`    |

### HotService

```go
type HotService struct {
    client *http.Client
    mu     sync.Mutex
    cache  map[string]cachedHotPage  // key = hotSearchCacheKey(category, sort, keyword)
}
```

`List` 方法流程：

1. 检查内存缓存（TTL 5 分钟）
2. 若缓存未命中，按分类路由：
    - A 股 / 港股：调用 `listEastMoney`（东方财富 clist API）
    - 美股（非 ETF，HotUSSource=eastmoney）：`listEastMoney`
    - 美股（HotUSSource=yahoo）：`listAllSearchableItems` 从 Yahoo Screener 拉取
    - US-ETF：`listAllSearchableItems` 从 Yahoo 搜索 + 静态种子池
3. 关键词搜索：`search` 方法从缓存或 `listAllSearchableItems` 中过滤
4. `sortHotItems` 按指定维度排序
5. `paginateHotItems` 分页后返回

---

## internal/api 详解

### API 路由表

所有路由均在 `/api/` 前缀下，由 `api.Handler.ServeHTTP` 剥离前缀后匹配：

| 方法   | 路径           | Handler                | 说明                                            |
| ------ | -------------- | ---------------------- | ----------------------------------------------- |
| GET    | `/state`       | `handleState`          | 返回完整状态快照                                |
| GET    | `/logs`        | `handleLogs`           | 返回开发日志快照（?limit=N）                    |
| DELETE | `/logs`        | `handleClearLogs`      | 清空日志                                        |
| POST   | `/client-logs` | `handleClientLogs`     | 前端上报日志                                    |
| GET    | `/hot`         | `handleHot`            | 热门榜单（?category=&sort=&q=&page=&pageSize=） |
| GET    | `/history`     | `handleHistory`        | 历史走势（?itemId=&interval=）                  |
| POST   | `/refresh`     | `handleRefresh`        | 触发全量行情刷新                                |
| PUT    | `/settings`    | `handleUpdateSettings` | 更新设置                                        |
| POST   | `/items`       | `handleCreateItem`     | 新增标的                                        |
| PUT    | `/items/{id}`  | `handleUpdateItem`     | 更新标的                                        |
| DELETE | `/items/{id}`  | `handleDeleteItem`     | 删除标的                                        |
| POST   | `/alerts`      | `handleCreateAlert`    | 新增提醒                                        |
| PUT    | `/alerts/{id}` | `handleUpdateAlert`    | 更新提醒                                        |
| DELETE | `/alerts/{id}` | `handleDeleteAlert`    | 删除提醒                                        |

所有写操作（POST / PUT / DELETE）返回更新后的 `StateSnapshot`。

### 路由匹配

Handler 使用自实现的轻量路由（不依赖 `net/http` 的模式匹配）：

- 按 `{name}` 单段参数规则匹配，不支持通配符
- 匹配后提取路径参数存入 `routeParams`
- 未匹配时返回 404 + `{"error": "接口不存在: /xxx"}`

### 响应规范

- 成功：HTTP 200 + JSON body
- 客户端错误（参数、校验）：HTTP 400 + `{"error": "..."}`
- 服务端错误（存储失败）：HTTP 500 + `{"error": "..."}`
- 服务不可用（HotService 为 nil）：HTTP 503 + `{"error": "热门服务不可用"}`

---

## 运行时流程

### 应用启动

```
main()
  ├─ 创建 LogBook（容量 400 条）
  ├─ 按需启用终端日志（--dev 参数或构建注入 defaultTerminalLogging=1）
  ├─ 配置日志文件（~/Library/Application Support/investgo/logs/app.log）
  ├─ applySystemProxy()：读 macOS 系统代理并注入环境变量
  ├─ DefaultQuoteSourceRegistry()：创建 EastMoney + Yahoo provider 注册表
  ├─ NewStore()：加载 state.json（或写入种子状态）
  ├─ NewHotService()
  ├─ 构建 HTTP mux（/api/* → api.Handler，/ → 前端静态资源）
  └─ 启动 Wails 窗口
```

### 行情刷新

```
前端 POST /api/refresh
  └─ api.Handler.handleRefresh
       └─ store.Refresh(ctx)
            ├─ [读锁] 复制 Items，按市场分组，确定各市场 provider
            ├─ [无锁] 各 provider.Fetch() 并发请求（当前为串行按 sourceID 分批）
            └─ [写锁] applyQuoteToItem 回填行情
                    → evaluateAlertsLocked()
                    → saveLocked()
                    → snapshotLocked()  ──► 返回 StateSnapshot
```

### 历史走势请求

```
前端 GET /api/history?itemId=xxx&interval=1w
  └─ api.Handler.handleHistory
       └─ store.ItemHistory(ctx, itemID, interval)
            ├─ [读锁] 找到 Item，调用 historyProviderCandidatesLocked() 得候选列表
            └─ [无锁] 按顺序尝试每个 provider.Fetch()，首个成功者直接返回
                    候选顺序（美股）：用户偏好 → yahoo → eastmoney
                    候选顺序（其他）：用户偏好 → eastmoney → yahoo
```

### 标的新增/更新

```
前端 POST /api/items  或  PUT /api/items/{id}
  └─ api.Handler.handleCreateItem / handleUpdateItem
       └─ store.UpsertItem(input)
            ├─ sanitiseItem()：规范化代码、市场、货币，校验数值；纯观察项（Quantity=0 且无 DCA）自动清除 AcquiredAt
            ├─ [读锁] 取当前 provider，查旧条目（更新时）
            ├─ inheritLiveFields()：保留已有盘口数据
            ├─ [无锁] provider.Fetch() 补一跳行情（8s 超时）
            └─ [写锁] 写入 state
                    → evaluateAlertsLocked()
                    → saveLocked()
                    → snapshotLocked()  ──► 返回 StateSnapshot
```

### 热门榜单请求

```
前端 GET /api/hot?category=us-nasdaq&sort=gainers&page=1&pageSize=20
  └─ api.Handler.handleHot
       └─ hot.List(ctx, category, sort, keyword, page, pageSize)
            ├─ 检查内存缓存（TTL 5min）
            ├─ 未命中：按分类路由到 listEastMoney / listAllSearchableItems
            ├─ sortHotItems()
            └─ paginateHotItems()  ──► 返回 HotListResponse
```

---

## 持久化机制

状态文件路径：`~/Library/Application Support/investgo/state.json`（通过 `os.UserConfigDir()` 解析）

### 持久化内容

```go
type persistedState struct {
    Items     []WatchlistItem
    Alerts    []AlertRule
    Settings  AppSettings
    UpdatedAt time.Time
}
```

`Triggered`、`Runtime`、实时行情字段不参与持久化（Triggered 在下次 evaluate 时重算，行情字段通过刷新恢复）。

实际上，`CurrentPrice`、`PreviousClose` 等行情字段**会随 Items 一起持久化**，这样应用重启后仍能显示上次刷新的价格，不至于全部归零。

### 写入策略

```go
func (s *Store) saveLocked() error {
    payload, _ := json.MarshalIndent(s.state, "", "  ")
    tempPath := s.path + ".tmp"
    os.WriteFile(tempPath, payload, 0o644)
    return os.Rename(tempPath, s.path)  // 原子替换
}
```

先写 `.tmp` 临时文件，再通过 `os.Rename` 原子替换正式文件，防止写入中断导致状态损坏。

### 状态加载与规范化

`store.load()` 在 `NewStore` 时调用：

- 文件不存在 → 写入种子状态（3 个示例标的 + 3 条示例提醒）
- 文件存在 → `json.Unmarshal` 后调用 `normaliseLocked()`

`normaliseLocked()` 的职责：

- 为缺字段的旧版本补齐默认值（Settings 各字段、Items ID/Name/UpdatedAt 等）
- 把 legacy `QuoteSource` 迁移为 `CNQuoteSource`/`HKQuoteSource`/`USQuoteSource`
- 对每个 Item / Alert 调用 sanitise 函数重新规范化
- 调用 `evaluateAlertsLocked()` 重建提醒状态

---

## 日志系统

### LogBook

```go
type LogBook struct {
    entries    []DeveloperLogEntry  // 固定容量环形覆盖（默认 400 条）
    maxEntries int
    sequence   atomic.Uint64        // 单调递增 ID
    file       *os.File             // 追加写，可选
    console    io.Writer            // 终端输出，可选
}
```

三条输出通路：**内存**（环形缓冲）+ **文件**（追加写）+ **终端**（stderr，--dev 模式启用）

日志等级：`debug` / `info` / `warn` / `error`

日志记录格式（文件/终端）：

```
2025-01-01T12:00:00Z [INFO] backend/storage loaded state from /path/to/state.json
```

### 集成点

| 调用方              | 写入方式                                                            |
| ------------------- | ------------------------------------------------------------------- |
| `main()` stdlib log | `logs.Writer("backend", "stdlib", DeveloperLogError)`               |
| Wails 框架 slog     | `logs.NewSlogLogger("system", slog.LevelInfo)`                      |
| `Store` 方法        | `logs.Info/Warn/Error("backend", scope, message)`                   |
| 前端前端上报        | `POST /api/client-logs` → `logs.Log(source, scope, level, message)` |

`GET /api/logs` 返回 `DeveloperLogSnapshot`（含日志文件路径和倒序条目）。

---

## 汇率服务

```go
type FxRates struct {
    rates   map[string]float64  // 1 单位外币 = X CNY
    validAt time.Time
    client  *http.Client
}
```

数据源：新浪财经 `hq.sinajs.cn/list=usdcny,hkdcny`

行为：

- `IsStale()`：缓存超过 4 小时则判断为过期
- `Fetch(ctx)`：3 秒超时，失败时保留既有汇率，不报错
- `Convert(value, from, to)`：以 CNY 为中间货币进行两步折算
- 兜底汇率：USD=7、HKD=0.85（初始值，新浪拉取成功后覆盖）

`Snapshot()` 会在取读锁前检查汇率是否过期，若过期则用 3 秒超时尝试刷新。

---

## macOS 代理探测

`applySystemProxy()` 在 `main()` 启动时调用（仅 darwin），流程：

1. 若环境变量 `HTTPS_PROXY` / `HTTP_PROXY` 已设置，跳过
2. 执行 `scutil --proxy` 读取系统代理配置
3. `parseScutilProxy()` 解析 KV 映射和排除列表
4. 优先采用 HTTPS 代理（`HTTPSEnable=1`），否则回退 HTTP 代理
5. 设置 `HTTPS_PROXY`、`HTTP_PROXY`、`NO_PROXY` 环境变量

之后所有 `net/http` 客户端通过 `http.ProxyFromEnvironment` 自动走代理，无需额外配置。

---

## 扩展指南

### 新增实时行情源

1. 在 `internal/marketdata/` 创建新文件，实现 `monitor.QuoteProvider` 接口
2. 在 `DefaultQuoteSourceRegistry()` 中实例化并注册，追加到 `options` 和 `providers` map
3. 在 `QuoteSourceOption.SupportedMarkets` 中列出支持的市场
4. 在 `sanitiseSettings` 的市场来源校验中追加新 ID

### 新增历史行情源

1. 在 `internal/marketdata/` 创建新文件，实现 `monitor.HistoryProvider` 接口
2. 在 `NewSmartHistoryProvider()` 返回的 map 中注册
3. 若需调整候选顺序，修改 `store_state.go` 中的 `historyProviderCandidatesLocked()`

### 新增 API 端点

1. 在 `internal/api/routes.go` 中追加 `route` 条目和对应 handler
2. 若涉及业务规则或状态修改，先在 `internal/monitor/store_mutations.go` 或 `store_runtime.go` 中增加 `Store` 方法
3. 纯外部数据可直接在 handler 内调用 `HotService` 或新建服务

### 新增持久化字段

1. 在 `model.go` 的对应类型（通常 `AppSettings` 或 `WatchlistItem`）追加字段
2. 在 `store_state.go` 的 `normaliseLocked()` 中为旧 state 文件补齐默认值
3. 在 `store_mutations.go` 的 `sanitiseSettings` / `sanitiseItem` 中增加校验

### 新增市场类型

1. 在 `model.go` 中追加 `HotCategory` 常量
2. 在 `quotes.go` 的 `normaliseMarketLabel()` 和 `inferCNMarketAndExchange()` 中处理新市场
3. 在 `store_state.go` 的 `marketGroupForMarket()` 中归组
4. 在 `quote_sources.go` 的 `resolveAllEastMoneySecIDs()` 和 `SupportedMarkets` 中处理新市场的 secid 映射

### 新增配色主题

新增一个 `ColorTheme` 值需要同步修改以下五处，缺一不可：

1. `frontend/src/types.ts` — 扩展 `ColorTheme` 联合类型
2. `frontend/src/constants.ts` — 在 `getColorThemeOptions()` 追加选项，并在 `COLOR_THEME_SWATCHES` 补充代表色
3. `frontend/src/style.css` — 添加两个 CSS 规则块（亮色选择器 + `html[data-theme="dark"]` 组合选择器）
4. `frontend/src/theme.ts` — 在 `themeSeeds` 中添加 PrimeVue 调色板种子色
5. `internal/monitor/settings_rules.go` — 在 `sanitiseSettings` 的 `switch settings.ColorTheme` 中加入新值，并同步更新 `internal/monitor/error_i18n.go` 中对应的错误提示
