import type { AlertCondition, AppSettings, HotCategory, HotMarketGroup, HotSort, HistoryInterval, ModuleTab, OptionItem, SettingsTab } from "./types";

export const moduleTabs: ModuleTab[] = [
    { key: "market", label: "市场行情", icon: "pi pi-chart-line" },
    { key: "hot", label: "热门榜单", icon: "pi pi-bolt" },
    { key: "watchlist", label: "自选列表", icon: "pi pi-table" },
    { key: "alerts", label: "提醒规则", icon: "pi pi-bell" },
];

export const settingsTabs: SettingsTab[] = [
    { key: "general", label: "常规" },
    { key: "display", label: "显示" },
    { key: "region", label: "区域" },
    { key: "developer", label: "开发" },
];

export const historyIntervals: OptionItem<HistoryInterval>[] = [
    { value: "live", label: "实时" },
    { value: "1h", label: "1小时" },
    { value: "6h", label: "6小时" },
    { value: "day", label: "日线" },
    { value: "week", label: "周线" },
    { value: "month", label: "月线" },
];

export const marketOptions: OptionItem[] = [
    { label: "A-Share", value: "A-Share" },
    { label: "HK", value: "HK" },
    { label: "US", value: "US" },
    { label: "US ETF", value: "US ETF" },
    { label: "BJ", value: "BJ" },
];

export const currencyOptions: OptionItem[] = [
    { label: "CNY", value: "CNY" },
    { label: "HKD", value: "HKD" },
    { label: "USD", value: "USD" },
];

export const priceModeOptions: OptionItem<AppSettings["priceMode"]>[] = [
    { label: "实时行情", value: "live" },
    { label: "手动价格", value: "manual" },
];

export const fontPresetOptions: OptionItem<AppSettings["fontPreset"]>[] = [
    { label: "系统默认", value: "system" },
    { label: "桌面无衬线", value: "compact" },
    { label: "阅读衬线", value: "reading" },
];

export const amountDisplayOptions: OptionItem<AppSettings["amountDisplay"]>[] = [
    { label: "完整数值", value: "full" },
    { label: "紧凑缩写", value: "compact" },
];

export const currencyDisplayOptions: OptionItem<AppSettings["currencyDisplay"]>[] = [
    { label: "货币符号", value: "symbol" },
    { label: "货币代码", value: "code" },
];

export const priceColorOptions: OptionItem<AppSettings["priceColorScheme"]>[] = [
    { label: "红涨绿跌", value: "cn" },
    { label: "绿涨红跌", value: "intl" },
];

export const localeOptions: OptionItem<AppSettings["locale"]>[] = [
    { label: "跟随系统", value: "system" },
    { label: "简体中文", value: "zh-CN" },
    { label: "English (US)", value: "en-US" },
];

export const alertConditionOptions: OptionItem<AlertCondition>[] = [
    { label: "价格高于阈值", value: "above" },
    { label: "价格低于阈值", value: "below" },
];

export const hotMarketOptions: OptionItem<HotMarketGroup>[] = [
    { label: "美股", value: "us" },
    { label: "ETF", value: "etf" },
    { label: "港股", value: "hk" },
];

export const hotCategoryOptions: Record<HotMarketGroup, OptionItem<HotCategory>[]> = {
    us: [
        { label: "标普500", value: "us-sp500" },
        { label: "纳斯达克100", value: "us-nasdaq100" },
        { label: "道琼斯30", value: "us-dow30" },
    ],
    etf: [
        { label: "宽基 ETF", value: "etf-broad" },
        { label: "行业 ETF", value: "etf-sector" },
        { label: "收益与防守 ETF", value: "etf-income" },
    ],
    hk: [{ label: "港股主板", value: "hk-main" }],
};

export const hotSortOptions: OptionItem<HotSort>[] = [
    { label: "交易量", value: "volume" },
    { label: "涨幅", value: "gainers" },
    { label: "跌幅", value: "losers" },
    { label: "市值", value: "market-cap" },
    { label: "股价", value: "price" },
];
