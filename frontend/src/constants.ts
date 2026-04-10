import type { AlertCondition, AppSettings, HotCategory, HotMarketGroup, HotSort, HistoryInterval, MarketType, ModuleTab, OptionItem, SettingsTab } from "./types";

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

// 市场模块使用的图表范围选项。
export const historyRangeOptions: OptionItem<HistoryInterval>[] = [
    { value: "1h", label: "1小时" },
    { value: "1d", label: "1天" },
    { value: "1w", label: "1周" },
    { value: "1mo", label: "1月" },
    { value: "1y", label: "1年" },
    { value: "3y", label: "3年" },
    { value: "all", label: "全部" },
];

// 统一的市场类型选项
export const marketOptions: OptionItem<MarketType>[] = [
    { label: "沪深A股（主板）", value: "CN-A" },
    { label: "创业板", value: "CN-GEM" },
    { label: "科创板", value: "CN-STAR" },
    { label: "境内ETF/LOF", value: "CN-ETF" },
    { label: "港股主板", value: "HK-MAIN" },
    { label: "港股创业板", value: "HK-GEM" },
    { label: "港股ETF", value: "HK-ETF" },
    { label: "美股", value: "US-STOCK" },
    { label: "美股ETF", value: "US-ETF" },
];

export const currencyOptions: OptionItem[] = [
    { label: "CNY", value: "CNY" },
    { label: "HKD", value: "HKD" },
    { label: "USD", value: "USD" },
];

export const fontPresetOptions: OptionItem<AppSettings["fontPreset"]>[] = [
    { label: "系统默认", value: "system" },
    { label: "桌面无衬线", value: "compact" },
    { label: "阅读衬线", value: "reading" },
];

export const themeModeOptions: OptionItem<AppSettings["themeMode"]>[] = [
    { label: "跟随系统", value: "system" },
    { label: "始终亮色", value: "light" },
    { label: "始终暗色", value: "dark" },
];

export const colorThemeOptions: OptionItem<AppSettings["colorTheme"]>[] = [
    { label: "系统蓝", value: "blue" },
    { label: "石墨灰", value: "graphite" },
    { label: "森林绿", value: "forest" },
    { label: "日落橙", value: "sunset" },
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

// 热门榜单市场分组
export const hotMarketOptions: OptionItem<HotMarketGroup>[] = [
    { label: "A股", value: "cn" },
    { label: "港股", value: "hk" },
    { label: "美股", value: "us" },
];

// 热门榜单详细分类
export const hotCategoryOptions: Record<HotMarketGroup, OptionItem<HotCategory>[]> = {
    cn: [
        { label: "沪深A股", value: "cn-a" },
        { label: "ETF", value: "cn-etf" },
    ],
    hk: [
        { label: "港股", value: "hk" },
        { label: "港股ETF", value: "hk-etf" },
    ],
    us: [
        { label: "标普500", value: "us-sp500" },
        { label: "纳斯达克", value: "us-nasdaq" },
        { label: "道琼斯", value: "us-dow" },
        { label: "ETF", value: "us-etf" },
    ],
};

export const hotSortOptions: OptionItem<HotSort>[] = [
    { label: "交易量", value: "volume" },
    { label: "涨幅", value: "gainers" },
    { label: "跌幅", value: "losers" },
    { label: "市值", value: "market-cap" },
    { label: "股价", value: "price" },
];

export const dashboardCurrencyOptions: OptionItem[] = [
    { label: "人民币 (CNY)", value: "CNY" },
    { label: "港元 (HKD)", value: "HKD" },
    { label: "美元 (USD)", value: "USD" },
];

// hotUSSourceOptions removed - US hot lists now always use constituent pools
