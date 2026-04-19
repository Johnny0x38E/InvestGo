# 后端架构文档

## 1. 当前后端分层

当前后端可以按四层理解：

```text
main.go
  ├─ internal/api
  ├─ internal/monitor
  ├─ internal/marketdata
  └─ internal/datasource
```

- `main.go`
  负责启动顺序、依赖注入、代理配置、Wails 宿主接线。
- `internal/api`
  纯 HTTP 适配层，负责路由、参数解析、JSON 编解码、错误回写。
- `internal/monitor`
  应用核心层，负责状态、快照、刷新、提醒、概览聚合、持久化协调。
- `internal/marketdata`
  外部数据接入层，负责实时行情、历史走势、热门榜单及其回退链。
- `internal/datasource`
  外部端点常量和 URL 组装工具。

依赖方向保持为：

```text
main -> api -> monitor <- marketdata <- datasource
main -> monitor
main -> marketdata
```

硬约束：

- `monitor` 不依赖 `api` 或 `marketdata`
- `marketdata` 实现 `monitor` 定义的接口
- `api` 不包含业务状态，只协调 `Store` 和服务对象

---

## 2. main.go 启动顺序

当前 `main.go` 的关键顺序是：

1. 创建 `LogBook`
2. 初始化代理 transport 和共享 `http.Client`
3. 构建 `quoteProviders` 与 `quoteSourceOptions`
4. 构建 `Store`
5. 构建 `HistoryProvider`
   这里使用 `NewSmartHistoryProvider(...)`，通过一个延迟读取的 settings getter 让 `HistoryRouter` 始终读取最新设置
6. 创建 `HotService`
7. 创建 `api.Handler`
8. 读取 `store.Snapshot()`，应用启动时代理与标题栏相关设置
9. 启动 Wails 应用

这里最重要的点是：

- 行情源注册表和历史路由是在启动时注入 `Store`
- 历史路由不是静态配置，它会在每次历史请求时重新读取当前设置

---

## 3. monitor 层职责

`internal/monitor` 是当前后端最核心的层。

主要职责：

- 维护持久化状态 `persistedState`
- 提供统一只读快照 `StateSnapshot`
- 提供实时刷新 `Refresh()`
- 提供单标的历史查询 `ItemHistory()`
- 提供概览分析 `OverviewAnalytics()`
- 提供标的、提醒、设置的写操作
- 统一做告警评估、派生字段计算、状态保存

### 3.1 Store 的角色

`Store` 是 monitor 层的总入口。

它内部协调：

- 持久化状态
- `quoteProviders`
- `historyProvider`
- `FxRates`
- `LogBook`

前端大多数写操作和快照读取都经由 `Store`。

### 3.2 StateSnapshot

前端拿到的主要是 `StateSnapshot`，它不是原始持久化结构，而是后端重新组装后的只读输出。

它包含：

- `Dashboard`
- `Items`
- `Alerts`
- `Settings`
- `Runtime`
- `QuoteSources`
- `StoragePath`
- `GeneratedAt`

当前快照构建有两个重要特征：

1. `Items` 会先做派生装饰
   包括 `DCASummary` 和 `PositionSummary`
2. `Runtime` 会在输出前重新汇总当前 quote source 概况和 live count

也就是说，前端看到的不是原始 state，而是已经被后端加工过的一层视图模型。

### 3.3 WatchlistItem 的双重语义

`WatchlistItem` 同时服务于：

- 观察列表
- 我的持仓

区分方式不是不同结构体，而是语义：

- 纯观察项：`Quantity == 0` 且没有有效 `DCAEntries`
- 持仓项：`Quantity > 0` 或存在有效 `DCAEntries`

后端会派生：

- `Position *PositionSummary`
- `DCASummary *DCASummary`

前端应直接使用这些派生字段，不应重新推导。

---

## 4. 三条核心数据链

当前项目需要区分三条数据链，它们不是一个接口顺手返回的。

### 4.1 实时行情链

入口：

- 前端 `POST /api/refresh`
- 后端 `Store.Refresh()`

流程：

1. 复制当前 items，避免长时间持锁
2. 按市场对应的有效 quote source 对 item 分组
3. 分组调用对应 `QuoteProvider.Fetch(...)`
4. 把 quote 回填到 item
5. 如 FX 过期则同步刷新汇率
6. 重新评估提醒
7. 保存状态
8. 返回新的 `StateSnapshot`

这条链只负责实时行情和运行时状态，不负责图表历史。

### 4.2 历史走势链

入口：

- 前端 `GET /api/history?itemId=&interval=`
- 后端 `Store.ItemHistory()`

流程：

1. 根据 `itemId` 找到 item
2. 调用 `historyProvider.Fetch(...)`
3. 返回 `HistorySeries`
4. 同时补一个 `Snapshot` 字段给图表侧边统计使用

这条链是按需拉取的，不打包进 `StateSnapshot`。

### 4.3 概览分析链

入口：

- 前端 `GET /api/overview`
- 后端 `Store.OverviewAnalytics()`

流程：

1. 读取当前 items 和 `DashboardCurrency`
2. 用 `overviewCalculator` 计算 breakdown
3. 用同一个 calculator 拉历史、重放 DCA、构造 trend
4. 输出 `OverviewAnalytics`

这条链是概览模块专用的后端聚合层。

---

## 5. 实时行情 provider 架构

实时行情接口定义在 `internal/monitor/quotes.go`：

- `QuoteProvider`

当前 provider 注册在 `internal/marketdata/quote_sources.go`：

- `eastmoney`
- `yahoo`
- `sina`
- `xueqiu`

设置层面真正生效的是：

- `CNQuoteSource`
- `HKQuoteSource`
- `USQuoteSource`

`Store.Refresh()` 会按市场取有效 source，再按 source 分组批量请求。

这意味着：

- 同一轮刷新里，不同市场可能同时走不同 provider
- `Runtime.QuoteSource` 是汇总结果，不是单一固定源

---

## 6. 历史 provider 架构

历史接口定义在 `internal/monitor/history_range.go`：

- `HistoryProvider`

真正对外暴露的是 `HistoryRouter`，定义在 `internal/marketdata/history_router.go`。

当前历史能力 provider 只有两个：

- `eastmoney`
- `yahoo`

`HistoryRouter` 的规则是：

1. 先根据市场读取当前 quote source 设置
2. 如果该 source 也具备历史能力，就放在第一优先级
3. 否则回到市场默认链

当前默认链：

- CN / HK：`eastmoney -> yahoo`
- US：`yahoo -> eastmoney`

重要结论：

- 当前系统没有单独前端支持的“历史源设置”
- 历史路由跟随当前 quote source 偏好和 provider capability

---

## 7. HotService 架构

热门榜单在 `internal/marketdata/hot.go` 与 `hot_fallback.go`。

它负责：

- 分类
- 搜索
- 排序
- 分页
- 缓存
- provider 选择
- fallback 池补数

### 7.1 分类来源

CN / HK 类别：

- 优先使用 EastMoney / 新浪 / 雪球等源的榜单接口

US 类别：

- 主要依赖维护好的静态池 `hot_us_constituents.go`

### 7.2 US 池当前行为

当前 US 池增加了两层业务化处理：

1. 显示数量规范化
   - `Nasdaq` 按 100 行展示
   - `S&P 500` 按 500 行展示
2. 名称补全
   - 即便实时行情来自 Yahoo
   - 仍可以额外走一次 EastMoney 名称查询，把 US 股票和 ETF 显示成中文名

### 7.3 EastMoney 热门备援

EastMoney 的 US secid 会扩展成多个交易所候选，因此：

- 大池子不能把所有 `secids` 一次性拼进一个请求
- 现在改成了分片请求，再聚合结果

这是为了避免 `S&P 500` 这种大池子因 URL 过长或上游限制而直接返回 `502`

---

## 8. API 层职责

`internal/api` 当前是纯适配层。

主要特点：

- 路由全在 `routes.go`
- handler 本身尽量薄
- 参数解析、错误回写、JSON 输出都留在这一层

关键路由：

- `GET /api/state`
- `GET /api/overview`
- `GET /api/hot`
- `GET /api/history`
- `POST /api/refresh`
- `PUT /api/settings`
- `POST /api/items`
- `PUT /api/items/{id}`
- `PUT /api/items/{id}/pin`
- `DELETE /api/items/{id}`
- `POST /api/alerts`
- `PUT /api/alerts/{id}`
- `DELETE /api/alerts/{id}`
- `GET /api/logs`
- `DELETE /api/logs`
- `POST /api/client-logs`
- `POST /api/open-external`

其中：

- 所有写操作返回新的 `StateSnapshot`
- `/api/overview` 返回 `OverviewAnalytics`
- `/api/history` 返回 `HistorySeries`

---

## 9. 持久化与兼容性

当前持久化通过 `internal/monitor/persistence.Repository` 抽象。

`Store` 不直接依赖某个具体文件格式实现，而是通过 repository 读写：

- `Load`
- `Save`
- `Path`

兼容性策略：

- `normaliseLocked()` 会补齐旧状态文件缺失字段
- 历史遗留字段如 `QuoteSource` 仍用于旧状态兼容
- `HotUSSource` 目前属于兼容/内部镜像语义，不应被当作新的独立前端产品开关

---

## 10. 代理、窗口和平台热点

当前平台敏感逻辑主要放在：

- `internal/platform/proxy.go`
- `internal/platform/window.go`
- `internal/api/open_external.go`

要点：

- 代理设置会影响共享 `http.Client`
- 外链打开已经按 OS 分流
- 原生标题栏与自定义标题栏行为由启动时 snapshot 决定

这些地方是未来 Windows x64 适配的热点，不要把平台逻辑重新散回业务层。

---

## 11. 当前最容易误判的几点

1. `StateSnapshot` 不包含图表历史
   图表历史永远走 `/api/history`
2. `Overview` 不是前端自己拿 `items` 算出来的
   它是后端专门聚合出的 `OverviewAnalytics`
3. 当前没有单独前端支持的 history source 配置
   历史路由跟随 quote source 偏好和 provider 能力
4. US 热门榜单里的中文名不一定来自实时 quote 源本身
   可能来自后续的 EastMoney 名称补全

---

## 12. 扩展建议

如果以后继续扩展：

- 加新 quote provider：
  更新 `quote_sources.go`、设置校验、前端 option 列表、错误本地化
- 加新 history provider：
  更新 `HistoryRouter`、provider 注册表、市场默认链
- 加新热门分类：
  更新 `hot.go`、前端常量、若是 US 类别还要看 `hot_us_constituents.go`
- 加新主题或设置枚举：
  同步更新后端 allowlist、前端类型、常量、i18n、样式

这份文档以当前代码实现为准；如果后续再改动实时/历史/概览链路，应优先同步这里，再同步技能参考。
