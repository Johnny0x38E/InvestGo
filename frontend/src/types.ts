// 图表范围采用时间窗口语义，不再包含单独的实时标签。
export type HistoryInterval = "1h" | "1d" | "1w" | "1mo" | "1y" | "3y" | "all";
export type AlertCondition = "above" | "below";
export type ModuleKey = "market" | "hot" | "watchlist" | "alerts";
export type SettingsTabKey = "general" | "display" | "region" | "developer";
export type StatusTone = "success" | "warn" | "error";
export type CardTone = "neutral" | "rise" | "fall" | "warn";
export type DeveloperLogLevel = "debug" | "info" | "warn" | "error";
export type DeveloperLogSource = "backend" | "frontend" | "system";
export type ThemeMode = "system" | "light" | "dark";
export type ColorTheme = "blue" | "graphite" | "forest" | "sunset";

// 统一的市场类型，符合交易所规范
export type MarketType =
    | "CN-A" // 沪深A股（主板）
    | "CN-GEM" // 深交所创业板
    | "CN-STAR" // 上交所科创板
    | "CN-ETF" // 境内ETF/LOF
    | "CN-BJ" // 北交所
    | "HK-MAIN" // 港股主板
    | "HK-GEM" // 港股创业板
    | "HK-ETF" // 港股ETF
    | "US-STOCK" // 美股（NYSE+NASDAQ）
    | "US-ETF"; // 美股ETF

// 热门榜单市场分组
export type HotMarketGroup = "cn" | "hk" | "us";

// 热门榜单详细分类
export type HotCategory =
    | "cn-a" // 沪深A股（主板+创业板+科创板）
    | "cn-etf" // 沪深ETF
    | "hk" // 港股
    | "hk-etf" // 港股ETF
    | "us-sp500" // 标普500
    | "us-nasdaq" // 纳斯达克100
    | "us-dow" // 道琼斯30
    | "us-etf"; // 美股ETF

export type HotSort = "volume" | "gainers" | "losers" | "market-cap" | "price";

export interface DCAEntry {
    id: string;
    date: string; // ISO 8601，如 "2024-01-15T00:00:00Z"
    amount: number; // 本次投入金额
    shares: number; // 本次买入份额
    price?: number; // 手动录入的买入价，0 或缺省表示未填写
    fee?: number; // 手续费/佣金
    note?: string;
}

export interface WatchlistItem {
    id: string;
    symbol: string;
    name: string;
    market: string;
    currency: string;
    quantity: number;
    costPrice: number;
    currentPrice: number;
    previousClose: number;
    openPrice: number;
    dayHigh: number;
    dayLow: number;
    change: number;
    changePercent: number;
    quoteSource: string;
    quoteUpdatedAt?: string;
    thesis: string;
    tags: string[];
    dcaEntries?: DCAEntry[];
    updatedAt: string;
}

export interface AlertRule {
    id: string;
    itemId: string;
    name: string;
    condition: AlertCondition;
    threshold: number;
    enabled: boolean;
    triggered: boolean;
    lastTriggeredAt?: string;
    updatedAt: string;
}

export interface AppSettings {
    refreshIntervalSeconds: number;
    cnQuoteSource: string;
    hkQuoteSource: string;
    usQuoteSource: string;
    hotUSSource: string;
    themeMode: ThemeMode;
    colorTheme: ColorTheme;
    fontPreset: "system" | "compact" | "reading";
    amountDisplay: "full" | "compact";
    currencyDisplay: "symbol" | "code";
    priceColorScheme: "cn" | "intl";
    locale: "system" | "zh-CN" | "en-US";
    developerMode: boolean;
    dashboardCurrency: string;
    useNativeTitleBar: boolean;
}

export interface QuoteSourceOption {
    id: string;
    name: string;
    description: string;
    supportedMarkets: MarketType[];
}

export interface RuntimeStatus {
    lastQuoteAttemptAt?: string;
    lastQuoteRefreshAt?: string;
    lastQuoteError?: string;
    quoteSource: string;
    livePriceCount: number;
}

export interface DashboardSummary {
    totalCost: number;
    totalValue: number;
    totalPnL: number;
    totalPnLPct: number;
    itemCount: number;
    triggeredAlerts: number;
    winCount: number;
    lossCount: number;
    displayCurrency: string;
}

export interface HistoryPoint {
    timestamp: string;
    open: number;
    high: number;
    low: number;
    close: number;
    volume: number;
}

export interface HistorySeries {
    symbol: string;
    name: string;
    market: string;
    currency: string;
    interval: HistoryInterval;
    source: string;
    startPrice: number;
    endPrice: number;
    high: number;
    low: number;
    change: number;
    changePercent: number;
    points: HistoryPoint[];
    generatedAt: string;
}

export interface StateSnapshot {
    dashboard: DashboardSummary;
    items: WatchlistItem[];
    alerts: AlertRule[];
    settings: AppSettings;
    runtime: RuntimeStatus;
    quoteSources: QuoteSourceOption[];
    storagePath: string;
    generatedAt: string;
}

export interface DeveloperLogEntry {
    id: string;
    source: DeveloperLogSource;
    scope: string;
    level: DeveloperLogLevel;
    message: string;
    timestamp: string;
}

export interface DeveloperLogSnapshot {
    entries: DeveloperLogEntry[];
    logFilePath: string;
    generatedAt: string;
}

export interface OptionItem<T = string> {
    label: string;
    value: T;
}

export interface ModuleTab {
    key: ModuleKey;
    label: string;
    icon: string;
}

export interface SettingsTab {
    key: SettingsTabKey;
    label: string;
}

export interface SummaryCard {
    label: string;
    value: string;
    sub: string;
    tone: CardTone;
    currency?: string;
}

export interface MarketMetricCard {
    label: string;
    value: string;
    sub: string;
    tone: Exclude<CardTone, "warn">;
}

export interface DCAEntryRow {
    id: string; // 前端临时 ID（"tmp-xxx"）或后端持久 ID
    date: string; // YYYY-MM-DD 格式
    amount: number | null;
    shares: number | null;
    price: number | null;
    fee: number | null;
    note: string;
}

export interface ItemFormModel {
    id: string;
    symbol: string;
    name: string;
    market: string;
    currency: string;
    quantity: number;
    costPrice: number;
    tagsText: string;
    thesis: string;
    currentPrice: number; // 仅用于定投汇总展示，不序列化提交
    dcaEntries: DCAEntryRow[];
}

export interface AlertFormModel {
    id: string;
    name: string;
    itemId: string;
    condition: AlertCondition;
    threshold: number;
    enabled: boolean;
}

export interface HotItem {
    symbol: string;
    name: string;
    market: string;
    currency: string;
    currentPrice: number;
    change: number;
    changePercent: number;
    volume: number;
    marketCap: number;
    quoteSource: string;
    updatedAt: string;
}

export interface HotListResponse {
    category: HotCategory;
    sort: HotSort;
    page: number;
    pageSize: number;
    total: number;
    hasMore: boolean;
    items: HotItem[];
    generatedAt: string;
}
