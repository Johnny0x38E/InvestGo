export type HistoryInterval = "live" | "1h" | "6h" | "day" | "week" | "month";
export type AlertCondition = "above" | "below";
export type ModuleKey = "market" | "hot" | "watchlist" | "alerts";
export type SettingsTabKey = "general" | "display" | "region" | "developer";
export type StatusTone = "success" | "warn" | "error";
export type CardTone = "neutral" | "rise" | "fall" | "warn";
export type DeveloperLogLevel = "debug" | "info" | "warn" | "error";
export type DeveloperLogSource = "backend" | "frontend" | "system";
export type HotMarketGroup = "us" | "etf" | "hk";
export type HotCategory = "us-sp500" | "us-nasdaq100" | "us-dow30" | "etf-broad" | "etf-sector" | "etf-income" | "hk-main";
export type HotSort = "volume" | "gainers" | "losers" | "market-cap" | "price";

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
    priceMode: "live" | "manual";
    refreshIntervalSeconds: number;
    quoteSource: string;
    fontPreset: "system" | "compact" | "reading";
    amountDisplay: "full" | "compact";
    currencyDisplay: "symbol" | "code";
    priceColorScheme: "cn" | "intl";
    locale: "system" | "zh-CN" | "en-US";
    developerMode: boolean;
}

export interface QuoteSourceOption {
    id: string;
    name: string;
    description: string;
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
}

export interface MarketMetricCard {
    label: string;
    value: string;
    sub: string;
    tone: Exclude<CardTone, "warn">;
}

export interface ItemFormModel {
    id: string;
    symbol: string;
    name: string;
    market: string;
    currency: string;
    quantity: number;
    costPrice: number;
    currentPrice: number;
    tagsText: string;
    thesis: string;
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
