行情刷新是 InvestGo 最核心的数据流之一。它既响应用户手动点击，也受前端定时器驱动自动执行；既要兼顾多市场 Provider 的并发批处理，又要在后端维护多级缓存与告警重算。本文档从**前端调度策略**、**后端 Store 刷新引擎**、**Provider 批处理与路由**、**缓存失效机制**四个维度，完整拆解行情数据从上游 Provider 到前端界面的全链路流程。

## 整体架构概览

行情刷新链路横跨前端 Vue 应用、Wails HTTP API、后端 Store 状态机，以及多个市场数据 Provider。前端以模块（Module）为单位实施差异化的刷新策略，后端 Store 负责按市场分组批量请求、统一落盘、重算告警并构建快照。

```mermaid
flowchart TD
    A[前端 Vue App] -->|POST /api/refresh| B[API Handler]
    A -->|POST /api/items/{id}/refresh| B
    B --> C[Store.Refresh]
    B --> D[Store.RefreshItem]
    C --> E[refreshQuotesForItems]
    D --> E
    E --> F{按市场分组}
    F --> G[CN Provider]
    F --> H[HK Provider]
    F --> I[US Provider]
    G --> J[应用报价到 Item]
    H --> J
    I --> J
    J --> K[evaluateAlertsLocked]
    K --> L[持久化 & 构建 Snapshot]
    L --> M[返回前端]
    M --> N[更新 UI & 图表]
```

Sources: [App.vue](frontend/src/App.vue#L258-L312), [handler.go](internal/api/handler.go#L123-L148), [runtime.go](internal/core/store/runtime.go#L1-L170)

## 后端刷新引擎：Store 层的两条刷新路径

Store 暴露了两条语义清晰的刷新接口，分别对应**全量刷新**与**单品种刷新**。两者在内部共享同一套 `refreshQuotesForItems` 批处理内核，但在锁粒度、缓存键和副作用范围上有所区别。

**`Refresh(ctx, force)`** 负责刷新全部自选股。它首先在无锁状态下拷贝当前 `Items` 切片，避免长时间占用读锁进行网络 I/O；随后将批量报价结果写回持久化状态，调用 `invalidatePriceCachesLocked` 清除报价相关缓存，最后触发告警重算与快照构建。若 `force` 为 `false`，会优先返回 `refreshCache` 中以 `"all"` 为键的缓存快照，从而在短时间内多次调用时避免重复请求上游。

**`RefreshItem(ctx, itemID, force)`** 仅刷新当前选中的品种，供 Watchlist 模块的局部刷新使用。它通过 `findItemIndexLocked` 定位目标品种，同样走 `refreshQuotesForItems` 内核，但写回时只更新单条记录，并将结果缓存到以 `itemID` 为键的 `itemRefreshCache` 中。该路径避免了把整份自选股列表发送给上游 Provider，对大型 Watchlist 尤为关键。

Sources: [runtime.go](internal/core/store/runtime.go#L1-L170)

## Provider 批处理与市场路由

`refreshQuotesForItems` 是刷新引擎的真正执行核心。它并非简单地把所有品种塞进同一个 Provider，而是依据用户为各市场独立设置的 Quote Source 进行**动态分组**。

首先，方法遍历待刷新品种，通过 `activeQuoteSourceIDLocked` 与 `activeQuoteProviderLocked` 解析每个品种所属市场当前生效的 Provider ID。市场分组逻辑在 `marketGroupForMarket` 中统一收敛为 `cn`、`hk`、`us` 三大组，再分别读取 `AppSettings` 中的 `CNQuoteSource`、`HKQuoteSource`、`USQuoteSource`。随后品种被归入 `map[string][]WatchlistItem`，同一 Provider 下的品种合并为一次 `Fetch` 调用。若 FX 汇率已过期，还会**顺带（opportunistically）**刷新汇率，确保组合概览中的跨币种市值换算始终基于最新牌价。

Provider 返回的报价以 `QuoteTarget.Key`（如 `000001.SZ`、`00700.HK`）作为全局统一索引键。Store 通过 `core.ResolveQuoteTarget` 将用户输入的符号规范化为同一套 Key 体系，从而匹配并写入对应品种。

Sources: [runtime.go](internal/core/store/runtime.go#L171-L294), [state.go](internal/core/store/state.go#L240-L322), [quote_resolver.go](internal/core/quote_resolver.go#L18-L52)

## 价格应用、快照构建与缓存策略

当 Provider 批量返回 `map[string]core.Quote` 后，`applyQuoteToItem` 将报价字段逐一映射到 `WatchlistItem`：覆盖 `CurrentPrice`、`PreviousClose`、`OpenPrice`、`DayHigh`、`DayLow`、`Change`、`ChangePercent`，同时记录报价来源 `QuoteSource` 与时间戳 `QuoteUpdatedAt`。如果 Provider 返回了中文名称，也会同步写入 `Name` 字段。

报价写完后，Store 执行以下副作用链：
1. **`evaluateAlertsLocked`**：基于最新 `CurrentPrice` 遍历全部 `AlertRule`，重算 `Triggered` 状态与 `LastTriggeredAt`。
2. **`invalidatePriceCachesLocked`**：清空 `refreshCache`、`itemRefreshCache`、`overviewCache` 与 `snapshotCache`，确保下一次读取拿到最新数据；但**不触碰** `historyCache`，因为历史 OHLCV 数据不受单点价格跳动影响。
3. **`saveLocked`**：将状态原子写入磁盘。
4. **`snapshotLocked`**：构建前端所需完整 `StateSnapshot`。快照本身也带有一层以 `state.UpdatedAt` 为时间戳的 `snapshotCache` 优化，避免在只读请求（如 `GET /api/state`）中重复排序与派生字段计算。

后端缓存 TTL 由统一设置 `HotCacheTTLSeconds` 控制，通过 `derivedCacheTTLLocked` 读取，最低不小于 10 秒，缺省 60 秒。需要强调的是，**自动刷新在前端始终携带 `force=true`，因此不会命中后端 TTL 缓存**，只有手动触发的非强制刷新或页面切换后的被动刷新才可能走缓存捷径。

Sources: [helper.go](internal/core/store/helper.go#L10-L25), [state.go](internal/core/store/state.go#L131-L164), [cache.go](internal/core/store/cache.go#L1-L60), [snapshot.go](internal/core/store/snapshot.go#L12-L60)

## 前端自动刷新调度与模块差异化策略

前端自动刷新的中枢位于 `App.vue`。应用在 `onMounted` 时调用 `scheduleAutoRefresh()`，该方法根据当前 `settings.hotCacheTTLSeconds` 计算间隔（毫秒），通过 `window.setInterval` 注册周期任务。当用户修改 `hotCacheTTLSeconds` 时，Watcher 会自动重新排程。

`runAutoRefresh` 是实际执行的回调。它通过 `autoRefreshInFlight` 标志实现简单的防重入保护，随后依据当前**激活模块**走三条不同分支：

| 激活模块 | 调用方法 | silent | refreshHistory | force | 设计意图 |
|---------|---------|--------|---------------|-------|---------|
| watchlist | `refreshSelectedItem` | `true` | `true` | `true` | 仅保持当前选中品种 alive，减少 Provider 负载 |
| hot | `refreshQuotes` | `true` | `false` | `true` | 热门榜单不需要历史走势图 |
| overview / 其他 | `refreshQuotes` | `true` | `false` | `true` | 全量刷新报价，更新组合概览 |

`refreshQuotes` 与 `refreshSelectedItem` 均先拼接 `?force=1` 查询串，再发起 `POST` 请求；拿到 `StateSnapshot` 后通过 `applySnapshot` 回填前端全部响应式状态。若当前在 Watchlist 模块且选中品种未变，还会**联动刷新**历史走势图，使侧边栏报价与 K 线叠加指标保持对齐。

Sources: [App.vue](frontend/src/App.vue#L161-L173), [App.vue](frontend/src/App.vue#L258-L312)

## 模块进入时的定向刷新

除了定时自动刷新，InvestGo 还在**模块切换瞬间**触发一次定向刷新，以最小的网络开销让用户立刻看到最新数据。`App.vue` 中监听 `activeModule` 的 Watcher 实现了这一策略：

- **进入 `watchlist`**：调用 `refreshSelectedItem(true, false)`，即静默刷新当前品种但不刷新历史图。这保证了用户从其他页签切回时，行情面板即刻呈现最新价，而不触发昂贵的历史数据重载。
- **进入 `overview`**：调用 `refreshQuotes(true, false)`，即静默全量刷新。因为组合概览需要聚合全部持仓的跨币种市值与盈亏分布，单品种刷新无法满足。

这种“进入即刷新”与“定时自动刷新”形成互补：前者解决**切换延迟**，后者解决**驻留期间的时效性**。

Sources: [App.vue](frontend/src/App.vue#L120-L136)

## 手动刷新与自动刷新的差异

虽然两者最终都走到同一组 API，但在前端参数与用户体验上存在系统性差异：

| 维度 | 手动刷新（点击按钮） | 自动刷新（定时器） |
|------|---------------------|-------------------|
| `silent` | `false`（显示状态栏文案） | `true`（静默后台执行） |
| `force` | `true` | `true` |
| `refreshHistory` | `true` | 模块相关（watchlist 为 `true`，其余为 `false`） |
| `forceHistory` | `true` | `false`（仅 watchlist 自动刷新时） |
| 防重入 | 无（用户可重复点击） | 有（`autoRefreshInFlight` 标志） |
| 后端缓存 | 强制绕过 | 强制绕过 |
| 状态栏反馈 | “同步行情中…” / “行情已同步” | 无 |

手动刷新按钮位于 `WatchlistModule.vue` 的工具栏，事件通过 `@refresh` 冒泡到 `AppWorkspace`，最终由 `App.vue` 根组件根据当前模块统一分发。这种集中式事件处理确保所有刷新入口共享同一套错误处理与快照应用逻辑。

Sources: [App.vue](frontend/src/App.vue#L439-L442), [WatchlistModule.vue](frontend/src/components/modules/WatchlistModule.vue#L188-L196)

## API 端点与路由映射

后端通过 Go 1.22 的 `http.ServeMux` 路径参数提供两个刷新端点：

- `POST /api/refresh` → `handleRefresh` → `Store.Refresh(ctx, force)`
- `POST /api/items/{id}/refresh` → `handleRefreshItem` → `Store.RefreshItem(ctx, id, force)`

Handler 统一使用 `parseBoolQuery` 解析 `?force=1` 或 `?force=true`。刷新完成后，返回的 `StateSnapshot` 会经过 `localizeSnapshot` 处理：将 `LastQuoteError`、`LastFxError` 及 Provider 名称翻译为当前请求语言，再序列化为 JSON 响应。

Sources: [http.go](internal/api/http.go#L44-L55), [handler.go](internal/api/handler.go#L123-L148)

## 相关阅读与下一步

理解行情刷新流程后，建议继续深入以下关联主题：

- [行情数据 Provider 注册与路由机制](7-xing-qing-shu-ju-provider-zhu-ce-yu-lu-you-ji-zhi)：了解 Provider 如何注册到 Registry，以及 `activeQuoteProviderLocked` 的动态解析规则。
- [缓存策略与失效机制](12-huan-cun-ce-lue-yu-shi-xiao-ji-zhi)：深入 `TTLCache` 的实现细节，以及 `invalidateAllCachesLocked` 与 `invalidatePriceCachesLocked` 的精确边界。
- [前后端状态同步与快照机制](22-qian-hou-duan-zhuang-tai-tong-bu-yu-kuai-zhao-ji-zhi)：理解 `StateSnapshot` 的完整字段构成与 `applySnapshot` 在前端的响应式回填逻辑。
- [历史走势图数据加载与缓存](24-li-shi-zou-shi-tu-shu-ju-jia-zai-yu-huan-cun)：探究 `loadHistory` 与 `historyCache` 如何与行情刷新联动，以及 `MarketSnapshot` 的派生计算。