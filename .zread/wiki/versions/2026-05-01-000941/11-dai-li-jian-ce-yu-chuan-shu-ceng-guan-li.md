InvestGo 面向全球多市场金融数据 API，上游服务端常部署于不同网络环境，因此应用必须优雅地处理直连、系统代理与自定义代理三种网络场景。本页聚焦后端代理检测逻辑与共享 HTTP 传输层的实现，阐述 macOS 系统代理读取、基于 `utls` 的 TLS 指纹伪装、运行时动态切换等核心机制，以及这些能力如何贯穿应用启动与设置变更的全生命周期。

## 架构概览

代理子系统由两层组成：**检测层**负责在启动时将 macOS 系统代理写入进程环境变量；**传输层**则封装了一个全局共享的 `http.Transport`，通过动态代理回调与 TLS 指纹拨号器，为行情 Provider、热门榜单、汇率转换等所有出站 HTTP 请求提供统一出口。当前端用户在设置面板中切换代理模式时，后端通过 `ProxyTransport.Update` 在运行时热更新配置，无需重启应用。

```mermaid
flowchart TB
    subgraph Frontend["前端设置面板"]
        SM[SettingsModule.vue<br/>proxyMode / proxyURL]
    end

    subgraph Backend["后端代理子系统"]
        direction TB
        PT[ProxyTransport<br/>mode / url / inner *http.Transport]
        ASP[ApplySystemProxy<br/>scutil --proxy]
        CTD[chromeTLSDialer<br/>utls HelloChrome_Auto]
    end

    subgraph Consumers["HTTP 请求消费者"]
        REG[marketdata.Registry]
        HOT[hot.HotService]
        FX[store.Store / FX 汇率]
    end

    SM -->|PUT /settings| API[api.Handler]
    API -->|Update(mode, url)| PT
    ASP -->|设置 env| PT
    PT -->|RoundTrip| REG
    PT -->|RoundTrip| HOT
    PT -->|RoundTrip| FX
    PT -.->|DialTLSContext| CTD
```

上图展示了代理配置的数据流向：用户在前端修改设置后，请求经 `api.Handler` 到达 `ProxyTransport`；启动阶段则通过 `ApplySystemProxy` 将系统代理注入环境变量，供 `http.ProxyFromEnvironment` 后续读取。所有下游服务共享同一个 `ProxyTransport` 实例，确保策略变更全局生效。Sources: [proxy.go](internal/platform/proxy.go#L1-L98), [proxy_transport.go](internal/platform/proxy_transport.go#L1-L158), [http.go](internal/api/http.go#L1-L200)

## 系统代理检测（macOS）

InvestGo 在 `system` 代理模式下，不会简单依赖 Go 标准库的 `http.ProxyFromEnvironment` 去读取环境变量，因为在 macOS 上用户通常通过系统偏好设置配置代理，而这些信息不会自动同步到 shell 环境变量中。为此，平台层提供了 `ApplySystemProxy` 函数，专门调用 `scutil --proxy` 读取系统级代理配置，并将其注入当前进程的环境变量。

函数首先检查 `HTTPS_PROXY`/`HTTP_PROXY` 是否已存在，若存在则跳过，避免覆盖用户手动指定的环境。随后通过 `exec.Command("scutil", "--proxy").Output()` 获取原始输出，交由 `parseScutilProxy` 解析为键值对和例外列表。解析器采用状态机方式扫描文本：遇到 `ExceptionsList : <array>` 时进入数组解析状态，收集代理例外域名；其余行按 `key : value` 形式提取。最终，若系统启用了 HTTPS 代理，则将其写入 `HTTPS_PROXY` 与 `HTTP_PROXY`；否则回退到 HTTP 代理。例外列表则拼接为逗号分隔字符串写入 `NO_PROXY`，确保局域网或内网地址绕过代理。

这一设计仅针对 `darwin` 平台生效，Windows 与 Linux 则直接依赖现有环境变量或用户显式配置。Sources: [proxy.go](internal/platform/proxy.go#L18-L63), [proxy.go](internal/platform/proxy.go#L66-L97)

## 可动态配置的传输层

`ProxyTransport` 是对标准库 `http.Transport` 的轻量级封装，核心目标是在不重建 HTTP 客户端的前提下，支持运行时安全地切换代理策略。结构体内含一个 `sync.RWMutex`，保护 `mode`（取值为 `system`、`custom`、`none`）与 `url`（仅在 `custom` 模式下非空）两个字段。

内部 `http.Transport` 在构造时通过 `Proxy: pt.proxyFunc` 注册动态代理回调。该回调先加读锁，再根据模式返回不同的代理地址： `none` 模式直接返回 `nil` 表示直连；`custom` 模式返回预设的 `url.URL`；`system` 模式则委托给 `http.ProxyFromEnvironment`，由标准库读取环境变量（包括启动阶段 `ApplySystemProxy` 注入的 macOS 系统代理）。`Update` 方法在写锁保护下更新模式与地址，随后调用 `inner.CloseIdleConnections()` 强制关闭所有空闲连接，使后续请求立即使用新的代理配置，而非复用旧连接的池化对象。

传输层还针对金融数据 API 的高并发场景配置了连接池参数：最大空闲连接数 64、单主机最大空闲连接数 8、空闲超时 60 秒、响应头超时 12 秒、TLS 最低版本 1.2。这些参数确保了在大量行情刷新请求并发时，TCP 连接仍能被高效复用，同时避免对故障上游的长时等待。Sources: [proxy_transport.go](internal/platform/proxy_transport.go#L22-L65), [proxy_transport.go](internal/platform/proxy_transport.go#L113-L149)

## TLS 指纹伪装与 HTTP/1.1 限制

部分金融数据服务端会基于 JA3/JA4 指纹识别客户端 TLS 握手特征，并拒绝或重置来自 Go 默认 TLS 库的连接。为绕过此类检测，`ProxyTransport` 的 `DialTLSContext` 被替换为自定义的 `chromeTLSDialer`。

该拨号器基于 `github.com/refraction-networking/utls` 实现，使用 `utls.HelloChrome_Auto` 自动匹配最新的 Chrome ClientHello 指纹，使服务端将连接识别为来自真实 Chrome 浏览器。具体实现中，先通过 `net.Dialer` 建立 TCP 连接，再用 `utls.UClient` 包装为 TLS 连接，并应用预设的 Chrome 规格完成握手。由于 Go 的 `http.Transport` 一旦使用自定义 `DialTLSContext`，便无法同时兼容内置的 HTTP/2 协商逻辑，因此代码显式遍历 TLS 扩展列表，找到 `ALPNExtension` 后将其协议列表强制覆盖为仅 `http/1.1`。若保留默认的 `h2` 宣告，服务端可能协商 HTTP/2，而 Transport 只会发送 HTTP/1.x 请求帧，导致出现 `malformed HTTP response` 错误。

值得留意的是，当请求通过 HTTP 代理隧道（CONNECT）访问 HTTPS 目标时，Go 标准库会在隧道建立后使用默认 TLS 握手，而非调用自定义的 `DialTLSContext`。这意味着 `chromeTLSDialer` 仅在直连 HTTPS 场景下生效，代理场景下的 TLS 伪装由标准库处理。对于 InvestGo 所对接的绝大多数行情 API 而言，HTTP/1.1 已完全满足需求，因此省略 `ForceAttemptHTTP2` 是合理的工程取舍。Sources: [proxy_transport.go](internal/platform/proxy_transport.go#L67-L111)

## 运行时更新与生命周期集成

代理配置并非静态常量，而是与用户设置同步演化的运行时状态。应用在 `main.go` 中的初始化分为两个阶段，确保 Store 加载前后 HTTP 消费者始终可用。

第一阶段在 `Store` 初始化之前，以默认的 `system` 模式创建 `ProxyTransport` 与共享 `http.Client`，并立即注入 `marketdata.DefaultRegistry`、`store.NewStore` 和 `hot.NewHotService`。这使得行情注册表、状态存储和热门榜单服务在启动早期就能发起网络请求。第二阶段在 `Store` 完成磁盘状态加载后，读取持久化快照中的 `ProxyMode` 与 `ProxyURL`：若模式为 `system`，则调用 `ApplySystemProxy` 将 macOS 系统代理写入环境变量；若模式为 `custom` 且地址非空，则记录日志。最后调用 `proxyTransport.Update(proxyMode, proxyURL)` 将实际配置同步到传输层。

当前端用户在设置面板中修改代理模式或地址并提交 `PUT /settings` 请求时，`api.Handler.handleUpdateSettings` 会先委托 `Store.UpdateSettings` 完成校验与持久化，再调用同一 `proxyTransport.Update` 方法，使新配置在毫秒级时间内全局生效。整个过程中，所有共享该 `http.Client` 的 Provider 与 Service 无需重启或重新初始化，真正实现了配置热更新。Sources: [main.go](main.go#L52-L89), [handler.go](internal/api/handler.go#L173-L191)

## 设置校验与持久化

代理相关字段属于 `core.AppSettings` 的一部分，与主题、行情源等设置统一存储在 `state.json` 中。`ProxyMode` 为字符串类型，支持 `none`、`system`、`custom` 三种取值；`ProxyURL` 仅在 `custom` 模式下有效。

`store.sanitiseSettings` 负责合并用户输入与当前配置并执行统一校验。对于代理字段，校验逻辑如下：若输入的 `ProxyMode` 非空，则小写化并进入分支校验；`none` 和 `system` 模式会自动清空 `ProxyURL`；`custom` 模式则要求 `ProxyURL` 必须非空，并通过 `url.Parse` 验证其包含有效的 scheme 与 host。任何非法模式或无效地址都会立即返回错误，阻止脏数据写入状态文件。Store 更新成功后还会触发 `invalidateAllCachesLocked()`，使行情缓存立即失效，确保下一批刷新请求走新的网络出口。

默认种子设置中，`ProxyMode` 被初始化为 `system`、`ProxyURL` 为空字符串，保证新用户首次启动时自动尝试读取操作系统代理，降低网络配置门槛。Sources: [model.go](internal/core/model.go#L117-L138), [settings_sanitize.go](internal/core/store/settings_sanitize.go#L47-L52), [settings_sanitize.go](internal/core/store/settings_sanitize.go#L155-L174), [seed.go](internal/core/store/seed.go#L77-L78)

## 前端设置界面

前端通过 `SettingsModule.vue` 暴露代理配置界面，使用单选组切换 `none`、`system`、`custom` 三种模式。当模式为 `custom` 时，动态显示代理地址输入框，绑定到 `settingsDraft.proxyURL`。提交后，`api.ts` 中的统一请求封装层将 `PUT /settings` 请求发送至后端，携带包含代理字段的完整 `AppSettings` 对象。前后端类型严格对应，`frontend/src/types.ts` 将 `proxyMode` 定义为字面量联合类型 `"none" | "system" | "custom"`，与后端的字符串校验形成双重约束。

由于所有 HTTP 请求消费者共享同一个 `ProxyTransport`，前端无需关心具体哪些 Provider 会受影响；设置保存后，下一次行情刷新、热门榜单加载或汇率查询将自动走新的代理通道。Sources: [SettingsModule.vue](frontend/src/components/modules/SettingsModule.vue#L392-L404), [types.ts](frontend/src/types.ts#L124-L125), [constants.ts](frontend/src/constants.ts#L181-L185)

## 总结

InvestGo 的代理子系统通过**平台检测 + 动态传输层 + 全局共享客户端**的三层设计，实现了跨平台兼容、运行时热切换与 TLS 指纹伪装。macOS 用户可享受系统自动代理的无缝对接；需要自定义代理的用户则通过设置面板输入地址，后端即时校验并生效；`none` 模式确保在完全隔离的网络环境中也能直连访问。`utls` 提供的 Chrome 指纹伪装则为访问对 JA3/JA4 敏感的上游 API 增加了额外的连通性保障。

如需进一步了解所有行情 Provider 如何消费该共享 HTTP 客户端，请参阅 [行情数据 Provider 注册与路由机制](7-xing-qing-shu-ju-provider-zhu-ce-yu-lu-you-ji-zhi) 与 [东方财富与新浪行情 Provider](25-dong-fang-cai-fu-yu-xin-lang-xing-qing-provider) 等后续章节。若要探究状态存储如何将代理设置持久化到磁盘，可参考 [Store 核心状态管理与持久化](6-store-he-xin-zhuang-tai-guan-li-yu-chi-jiu-hua)。