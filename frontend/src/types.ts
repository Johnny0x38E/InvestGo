// History intervals use time-window semantics; there is no separate real-time label.
export type HistoryInterval = "1h" | "1d" | "1w" | "1mo" | "1y" | "3y" | "all";
export type AlertCondition = "above" | "below";
export type ModuleKey = "overview" | "watchlist" | "hot" | "holdings" | "alerts" | "settings";
export type SettingsTabKey = "general" | "display" | "region" | "network" | "developer" | "about";
export type StatusTone = "success" | "warn" | "error";
export type CardTone = "neutral" | "rise" | "fall" | "warn";
export type DeveloperLogLevel = "debug" | "info" | "warn" | "error";
export type DeveloperLogSource = "backend" | "frontend" | "system";
export type ThemeMode = "system" | "light" | "dark";
export type ColorTheme = "blue" | "graphite" | "forest" | "sunset" | "rose" | "violet" | "amber";

// Unified market types conforming to exchange conventions
export type MarketType =
    | "CN-A" // CSI A-shares (Main Board)
    | "CN-GEM" // SZSE ChiNext
    | "CN-STAR" // SSE STAR Market
    | "CN-ETF" // Onshore ETF/LOF
    | "CN-BJ" // Beijing Stock Exchange
    | "HK-MAIN" // HK Main Board
    | "HK-GEM" // HK GEM (Growth Enterprise Market)
    | "HK-ETF" // HK-listed ETF
    | "US-STOCK" // US stocks (NYSE + NASDAQ)
    | "US-ETF"; // US-listed ETF

// Hot list market groups
export type HotMarketGroup = "cn" | "hk" | "us";

// Hot list detailed categories
export type HotCategory =
    | "cn-a" // CSI A-shares (Main Board + ChiNext + STAR)
    | "cn-etf" // CSI ETFs
    | "hk" // Hong Kong stocks
    | "hk-etf" // HK-listed ETFs
    | "us-sp500" // S&P 500
    | "us-nasdaq" // NASDAQ-100
    | "us-dow" // Dow Jones 30
    | "us-etf"; // US-listed ETFs

export type HotSort = "volume" | "gainers" | "losers" | "market-cap" | "price";

export interface DCAEntry {
    id: string;
    date: string; // ISO 8601, e.g. "2024-01-15T00:00:00Z"
    amount: number; // Investment amount for this entry
    shares: number; // Shares purchased this time
    price?: number; // Manually entered buy price; 0 or omitted means not filled
    fee?: number; // Commission / fee
    note?: string;
    effectivePrice?: number;
}

export interface DCASummary {
    count: number;
    totalAmount: number;
    totalShares: number;
    totalFees: number;
    averageCost: number;
    currentValue: number;
    pnl: number;
    pnlPct: number;
    hasCurrentPrice: boolean;
}

export interface PositionSummary {
    costBasis: number;
    marketValue: number;
    unrealisedPnL: number;
    unrealisedPnLPct: number;
    hasPosition: boolean;
}

export interface WatchlistItem {
    id: string;
    symbol: string;
    name: string;
    market: string;
    currency: string;
    quantity: number;
    costPrice: number;
    acquiredAt?: string;
    currentPrice: number;
    previousClose: number;
    openPrice: number;
    dayHigh: number;
    dayLow: number;
    change: number;
    changePercent: number;
    quoteSource: string;
    quoteUpdatedAt?: string;
    pinnedAt?: string;
    thesis: string;
    tags: string[];
    dcaEntries?: DCAEntry[];
    dcaSummary?: DCASummary;
    position?: PositionSummary;
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
    hotCacheTTLSeconds: number;
    cnQuoteSource: string;
    hkQuoteSource: string;
    usQuoteSource: string;
    themeMode: ThemeMode;
    colorTheme: ColorTheme;
    fontPreset: "system" | "compact" | "reading";
    amountDisplay: "full" | "compact";
    currencyDisplay: "symbol" | "code";
    priceColorScheme: "cn" | "intl";
    locale: "system" | "zh-CN" | "en-US";
    proxyMode: "none" | "system" | "custom";
    proxyURL: string;
    alphaVantageApiKey: string;
    twelveDataApiKey: string;
    finnhubApiKey: string;
    polygonApiKey: string;
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
    appVersion: string;
    lastFxError?: string;
    lastFxRefreshAt?: string;
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

export interface OverviewHoldingSlice {
    itemId: string;
    symbol: string;
    name: string;
    market: string;
    currency: string;
    value: number;
    weight: number;
}

export interface OverviewTrendSeries {
    itemId: string;
    symbol: string;
    name: string;
    market: string;
    currency: string;
    latestValue: number;
    firstBuyDate: string;
    values: number[];
}

export interface OverviewTrend {
    startDate?: string;
    endDate?: string;
    dates: string[];
    series: OverviewTrendSeries[];
    totalValue: number;
}

export interface OverviewAnalytics {
    displayCurrency: string;
    breakdown: OverviewHoldingSlice[];
    trend: OverviewTrend;
    cached: boolean;
    cacheExpiresAt?: string;
    generatedAt: string;
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
    snapshot?: {
        livePrice: number;
        effectiveChange: number;
        effectiveChangePct: number;
        previousClose: number;
        openPrice: number;
        rangeHigh: number;
        rangeLow: number;
        amplitudePct: number;
        positionValue: number;
        positionBaseline: number;
        positionPnL: number;
        positionPnLPct: number;
    };
    cached: boolean;
    cacheExpiresAt?: string;
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
    id: string; // Frontend temporary ID ("tmp-xxx") or backend persistent ID
    date: string; // YYYY-MM-DD format
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
    acquiredAt: string;
    tagsText: string;
    thesis: string;
    currentPrice: number; // Used only for DCA summary display; not serialized on submit
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
    cached: boolean;
    cacheExpiresAt?: string;
    generatedAt: string;
}
