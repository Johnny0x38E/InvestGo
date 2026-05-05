InvestGo 的前端基于 Vue 3 与 TypeScript，后端基于 Go，两者在 Wails v3 同进程架构下通过本地 HTTP 服务交换数据。与常规 Web 应用不同，这里的网络调用既不跨越公网，也不涉及 CORS，但行情刷新、历史走势加载、设置保存等高频操作依然面临超时、取消、竞态和错误消息本地化等典型问题。本页围绕前端统一的 `api<T>` 请求包装器、后端 Handler 的响应契约，以及贯穿前后端的错误处理与日志体系展开，帮助你理解数据是如何在 Vue 组件与 Go 服务之间可靠流动的。

## 架构定位：统一 API 层的职责边界

在没有统一封装的情况下，每个 Vue Composable 直接调用 `fetch` 将导致超时逻辑重复、取消信号难以协调、HTTP 错误解析不一致，且敏感信息可能未经脱敏就进入日志。InvestGo 的前端通过单个 `api.ts` 模块解决这些问题：它向上为所有业务 Composables 提供类型安全的 `api<T>` 函数，向下聚合超时控制、请求取消、Locale 透传、错误解析和日志上报。后端则由 `internal/api` 包集中处理路由注册、请求解码、响应编码与错误消息本地化。前后端之间的契约极简——成功时返回 JSON 业务对象，失败时返回统一形状的 `{ error, debugError? }`，其余细节均由两侧基础设施消化。

下面这张图展示了请求从 Vue Composable 发出后，在前端包装器、后端 Handler 与 i18n 层之间的完整流转路径。

```mermaid
sequenceDiagram
    participant VC as Vue Composable
    participant AP as api.ts (前端包装器)
    participant BH as Handler (后端)
    participant II as i18n/error_i18n.go
    participant ST as Store / Service

    VC->>AP: api&lt;T&gt;(path, init)
    AP->>AP: 注入 X-InvestGo-Locale<br/>启动 AbortController + 超时
    AP->>BH: fetch → /api/...
    BH->>ST: 执行业务逻辑
    ST-->>BH: 返回 error (如有)
    BH->>II: LocalizeErrorMessage(locale, err.Error())
    II-->>BH: 本地化后的消息
    alt 成功
        BH-->>AP: 200 + JSON payload
        AP-->>VC: 返回 T
    else 失败
        BH-->>AP: 4xx/5xx + {error, debugError}
        AP->>AP: 解析错误、脱敏日志
        AP-->>VC: throw Error (含 debugMessage)
    end
```

Sources: [api.ts](frontend/src/api.ts#L31-L86), [http.go](internal/api/http.go#L45-L138)

## 前端统一请求包装器 `api<T>`

`api<T>` 是整个前端唯一直接调用 `fetch` 的函数。它的签名扩展了标准 `RequestInit`，增加了一个可选的 `timeoutMs` 字段，默认值为 15000 毫秒。

### 超时、取消与信号桥接

每个请求都会创建一个内部 `AbortController`，并通过 `setTimeout` 在超时后主动触发 `abort`。如果调用方传入了外部的 `signal`（例如 Composable 中因用户切换标的而需要取消上一次请求），包装器会将外部信号桥接到同一个内部 `controller` 上：当外部信号触发时，内部请求也会立即终止；反之，超时触发时，内部中止同样生效。这种双向桥接确保了无论哪一侧发起取消，正在进行的 `fetch` 都能被干净地中断。

Sources: [api.ts](frontend/src/api.ts#L31-L47)

### `ApiAbortError` 的语义区分

原生 `fetch` 在被取消时只会抛出 `DOMException` 且 `name === "AbortError"`，无法区分是用户主动取消还是超时触发。InvestGo 定义了 `ApiAbortError`，其 `reason` 字段明确标记 `"timeout"` 或 `"aborted"`。在 `catch` 块中，包装器将原生 `AbortError` 转换为 `ApiAbortError`，并根据触发来源赋予正确的 `reason`。这一设计让调用方可以精确决定是否需要向用户展示错误：超时通常需要提示，而因切换页面或切换标的导致的手动取消则应静默忽略。

Sources: [api.ts](frontend/src/api.ts#L16-L24), [api.ts](frontend/src/api.ts#L69-L76)

### 错误解析与响应结构

当响应状态码非 2xx 时，包装器首先检查 `Content-Type` 是否为 JSON，然后尝试将响应体解析为 `ApiErrorPayload`（即 `{ error?: string; debugError?: string }`）。`error` 字段是面向用户的本地化消息，直接用于抛出 `Error` 的 `message`；`debugError` 则是原始调试信息，附加在错误对象的 `debugMessage` 属性上。如果后端未返回 JSON 错误体，则包装器回退到前端自身的 `translate("api.requestFailed", { status })` 作为兜底消息。

Sources: [api.ts](frontend/src/api.ts#L56-L66)

### 自动日志与敏感信息脱敏

对于非中止类的网络或解析错误，`api<T>` 会在抛出前自动调用 `appendClientLog`，将请求方法与路径、以及调试信息写入前端开发者日志。写入前通过 `redactSensitiveText` 对 API Key 等敏感字段进行脱敏，规则覆盖常见的 key 查询参数和 JSON 字段形式。这些日志随后会通过后台批处理机制上报到后端 `/api/client-logs`，实现前后端日志的统一收集。

Sources: [api.ts](frontend/src/api.ts#L77-L81), [devlog.ts](frontend/src/devlog.ts#L67-L84), [devlog.ts](frontend/src/devlog.ts#L139-L150)

## 后端响应契约与错误编码

后端的 API 入口是 `internal/api` 包中的 `Handler`。它内部持有一个 Go 1.22+ 的 `http.ServeMux`，并显式注册所有业务路由。`Handler.ServeHTTP` 负责剥离 `/api` 前缀后将请求分发给内部路由，同时强制设置响应头 `Content-Type: application/json; charset=utf-8`。

### 统一的成功与失败响应

`writeJSON` 与 `writeError` 是后端唯一的两种响应出口。`writeJSON` 直接将任意 payload 编码为 JSON 并写入指定状态码；`writeError` 则接受一个 `error` 接口值，提取其错误文本后调用 `i18n.LocalizeErrorMessage` 进行本地化，最终输出统一结构：

```json
{
  "error": "面向用户的本地化错误消息",
  "debugError": "原始调试信息（仅在不同于 error 时附加）"
}
```

这种双字段结构让前端在展示错误时优先使用 `error`（用户友好），而在需要排查问题时可以读取 `debugError`（保留原始上下文）。

Sources: [http.go](internal/api/http.go#L119-L138)

### `apiError` 与 Handler 中的错误传播

后端内部通过 `apiError` 结构体显式构造面向 API 层的错误。它与业务层由 `store`、`hot` 等服务返回的具体错误解耦：当业务错误流经 Handler 时，Handler 并不修改错误内容，而是直接将其交给 `writeError`，由 i18n 层负责翻译。例如，当 `handleCreateItem` 中 `decodeJSON` 失败时，包装器会生成 `apiError{message: "Invalid JSON request body"}`，再由 `writeError` 将其翻译为 `"请求体 JSON 无效"` 返回给前端。

Sources: [http.go](internal/api/http.go#L284-L292), [handler.go](frontend/src/api.ts#L194-L208)

## 双语错误消息体系

InvestGo 支持简体中文与英文两种界面语言，错误消息的翻译不仅发生在前端 UI 层，也发生在后端 API 层。原因在于：许多错误起源于后端业务逻辑（如行情 Provider 请求失败、符号格式校验不通过），如果仅在前端翻译，后端返回的动态错误文本（如 `"Yahoo quote request failed: network error"`）将无法被静态字典覆盖。因此，后端在输出错误响应前完成本地化，前端则负责兜底翻译与通用 API 错误提示。

### 后端本地化的三级匹配策略

`internal/api/i18n/error_i18n.go` 实现了 `LocalizeErrorMessage` 函数，它对原始英文错误文本执行三级匹配：

1. **精确匹配**：通过 `localizedExactMessages` 字典直接映射完整句子，如 `"Invalid JSON request body"` → `"请求体 JSON 无效"`。
2. **前缀匹配**：通过 `localizedPrefixMessages` 切片按前缀匹配，并支持 `recursive` 标志对剩余部分递归本地化。这适用于 `"Yahoo quote request failed: ..."` 这类带动态后缀的错误。
3. **正则匹配**：针对少数包含多个动态变量的复杂句式（如 `"Did not receive EastMoney quote for ... (...)"`），使用预编译正则提取变量并重组为中文语序。

如果以上均未命中，则原文返回，确保不会因为翻译缺失而丢失信息。

Sources: [error_i18n.go](internal/api/i18n/error_i18n.go#L8-L217)

### Locale 的识别优先级

后端通过 `requestLocale` 函数确定当前请求的语言环境，优先级如下：首先读取前端注入的请求头 `X-InvestGo-Locale`；若不存在，则回退到标准 `Accept-Language`；最终兜底为 `"en-US"`。前端在每次调用 `api<T>` 时都会通过 `getI18nLocale()` 将当前活跃语言写入该请求头，从而保证前后端语言状态严格同步。

Sources: [http.go](internal/api/http.go#L176-L188), [api.ts](frontend/src/api.ts#L38)

### 前端兜底翻译

对于纯前端产生的通用错误（如请求超时、连接取消、状态码异常），前端 i18n 字典在 `api` 命名空间下提供了独立词条：`api.timeout`、`api.aborted`、`api.requestFailed`。这些词条不依赖后端响应，确保即使在后端完全不可达的情况下，用户依然能看到母语提示。

Sources: [i18n.ts](frontend/src/i18n.ts#L548-L552), [i18n.ts](frontend/src/i18n.ts#L1092-L1096)

## 请求取消与竞态控制

在高频交互场景中（如用户在行情列表中快速切换标的），如果前一次请求尚未返回就发起新请求，就可能出现竞态：旧响应晚于新响应到达，导致界面显示过期数据。InvestGo 通过 `AbortController` 替换模式解决这一问题，最典型的实现位于历史走势加载逻辑中。

### `useHistorySeries` 的取消模式

`useHistorySeries` 维护一个 `inflightController` 引用，指向当前正在进行的请求控制器。当用户切换标的或时间范围时，`loadHistory` 会先调用 `cancelInflightHistory`，用 `ApiAbortError("aborted")` 终止上一次请求，然后创建新的 `AbortController` 并赋给 `inflightController`。在 `fetch` 返回后，代码会检查 `inflightController !== controller`：若不相等，说明该请求已被更新的请求取代，直接丢弃结果。这种模式确保了只有最后一次发起的请求才能更新 `historySeries` 状态。

Sources: [useHistorySeries.ts](frontend/src/composables/useHistorySeries.ts#L22-L84)

### 静默刷新与错误隔离

`loadHistory` 支持 `silent` 参数。在静默模式下，即使请求失败，也不会清空当前已显示的图表数据，而是保持旧数据直至用户主动切换或下一次显式刷新成功。同时，因取消而抛出的 `ApiAbortError` 在 `catch` 块中被直接 `return`，既不打断用户操作，也不触发状态栏错误提示。这种分层策略让网络抖动对用户视觉体验的干扰降到最低。

Sources: [useHistorySeries.ts](frontend/src/composables/useHistorySeries.ts#L99-L117)

## 日志记录与敏感信息脱敏

前端日志体系由 `devlog.ts` 统一管理。它通过劫持 `console.*`、监听 `window.error` 与 `window.unhandledrejection`，将前端所有输出和异常收集到一个容量上限为 200 条的响应式缓冲区中。API 层在捕获到非中止错误时，也会主动调用 `appendClientLog` 将请求路径与错误信息写入该缓冲区。

### 向后端镜像日志

前端日志并非仅停留在内存中。`mirrorClientLog` 通过 1 秒定时器将日志批量发送到 `/api/client-logs`，且在页面卸载前利用 `keepalive: true` 确保最后一批日志也能送达。后端 `handleClientLogs` 接收后将其写入与后端日志同一存储，使开发者模式下的日志视图呈现前后端统一的时间线。

Sources: [devlog.ts](frontend/src/devlog.ts#L95-L118), [handler.go](internal/api/handler.go#L86-L101)

### 脱敏规则

`redactSensitiveText` 实现了三条主要规则：覆盖 JSON 中四个付费 API Key 字段的键值对、覆盖 URL 查询参数中的 `apikey`/`api_key`/`key`，以及对等号前后不同上下文进行安全匹配。所有写入日志的 API 请求信息都会经过此函数处理，防止用户在使用开发者模式时意外泄露私钥。

Sources: [devlog.ts](frontend/src/devlog.ts#L139-L150)

## 调用方错误处理范式

在前端业务层，所有与 API 交互的 Composables（如 `useItemDialog`、`useAlertDialog`、`useDeveloperLogs`）遵循统一的错误处理范式：`try { await api<T>(...) } catch (error) { setStatus(error.message, "error") }`。`setStatus` 是由根组件注入的状态栏报告函数，它将错误消息以 Toast 或状态条形式呈现给用户。由于后端已经返回本地化后的 `error` 字段，因此 Composables 通常不需要额外的翻译逻辑，直接展示 `error.message` 即可。

下表总结了不同来源的错误在前端的处理策略：

| 错误来源 | 典型场景 | 前端处理方式 |
|---------|---------|-------------|
| `ApiAbortError("timeout")` | 行情 Provider 响应慢 | 静默或显示"请求超时"，不记录为异常 |
| `ApiAbortError("aborted")` | 切换标的导致旧请求取消 | 直接 `return`，不更新状态也不提示 |
| HTTP 4xx/5xx | 参数校验失败、Provider 不可用 | 提取 `error` 显示给用户，`debugError` 进日志 |
| 网络不可达 | 后端服务未启动 | 回退到 `translate("api.requestFailed")` 并记录日志 |
| 业务逻辑错误 | 标的已存在、提醒阈值无效 | 后端 i18n 翻译后返回，前端直接展示 |

Sources: [useItemDialog.ts](frontend/src/composables/useItemDialog.ts#L51-L87), [useAlertDialog.ts](frontend/src/composables/useAlertDialog.ts#L29-L59), [App.vue](frontend/src/App.vue#L255-L268)

## 下一步阅读

理解了 API 通信层与错误处理后，你可以继续深入以下相关主题：

- 如果你希望了解 Wails 运行时将后端 HTTP 服务暴露给前端的机制，请参阅 [Wails 运行时桥接与平台适配](16-wails-yun-xing-shi-qiao-jie-yu-ping-tai-gua-pei)。
- 如果你对 `useHistorySeries`、`useItemDialog` 等 Composables 的业务逻辑复用模式感兴趣，请参阅 [Composables 业务逻辑复用](17-composables-ye-wu-luo-ji-fu-yong)。
- 如果你需要研究后端 Store 如何将状态持久化并在刷新后通过 `/api/state` 返回给前端，请参阅 [前后端状态同步与快照机制](22-qian-hou-duan-zhuang-tai-tong-bu-yu-kuai-zhao-ji-zhi)。
- 后端行情 Provider 的具体请求实现与错误产生原因，请参阅 Provider 实现详解系列，例如 [东方财富与新浪行情 Provider](25-dong-fang-cai-fu-yu-xin-lang-xing-qing-provider)。