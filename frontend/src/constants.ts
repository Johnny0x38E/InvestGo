import { translate } from "./i18n";
import type { AlertCondition, AppSettings, ColorTheme, HotCategory, HotMarketGroup, HotSort, HistoryInterval, MarketType, ModuleTab, OptionItem, SettingsTab } from "./types";

export const projectMeta = {
    repositoryUrl: "https://github.com/Johnny0x38E/InvestGo",
} as const;

export function getModuleTabs(): ModuleTab[] {
    return [
        { key: "overview", label: translate("modules.overview"), icon: "pi pi-home" },
        { key: "watchlist", label: translate("modules.watchlist"), icon: "pi pi-chart-line" },
        { key: "holdings", label: translate("modules.holdings"), icon: "pi pi-wallet" },
        { key: "hot", label: translate("modules.hot"), icon: "pi pi-bolt" },
        { key: "alerts", label: translate("modules.alerts"), icon: "pi pi-bell" },
    ];
}

export function getSettingsTabs(): SettingsTab[] {
    return [
        { key: "general", label: translate("settings.tabs.general") },
        { key: "display", label: translate("settings.tabs.display") },
        { key: "region", label: translate("settings.tabs.region") },
        { key: "network", label: translate("settings.tabs.network") },
        { key: "developer", label: translate("settings.tabs.developer") },
        { key: "about", label: translate("settings.tabs.about") },
    ];
}

export function getHistoryRangeOptions(): OptionItem<HistoryInterval>[] {
    return [
        { value: "1h", label: translate("options.historyRange.1h") },
        { value: "1d", label: translate("options.historyRange.1d") },
        { value: "1w", label: translate("options.historyRange.1w") },
        { value: "1mo", label: translate("options.historyRange.1mo") },
        { value: "1y", label: translate("options.historyRange.1y") },
        { value: "3y", label: translate("options.historyRange.3y") },
        { value: "all", label: translate("options.historyRange.all") },
    ];
}

export function getMarketOptions(): OptionItem<MarketType>[] {
    // Only the six canonical user-facing categories are offered in forms.
    // Sub-markets (CN-GEM, CN-STAR, CN-BJ, HK-GEM) are handled by the backend
    // automatically via symbol-prefix inference and are not exposed as choices.
    return [
        { label: translate("options.market.CN-A"),     value: "CN-A" },
        { label: translate("options.market.CN-ETF"),   value: "CN-ETF" },
        { label: translate("options.market.HK-MAIN"),  value: "HK-MAIN" },
        { label: translate("options.market.HK-ETF"),   value: "HK-ETF" },
        { label: translate("options.market.US-STOCK"), value: "US-STOCK" },
        { label: translate("options.market.US-ETF"),   value: "US-ETF" },
    ];
}

export const currencyOptions: OptionItem[] = [
    { label: "CNY", value: "CNY" },
    { label: "HKD", value: "HKD" },
    { label: "USD", value: "USD" },
];

export function getFontPresetOptions(): OptionItem<AppSettings["fontPreset"]>[] {
    return [
        { label: translate("options.fontPreset.system"), value: "system" },
        { label: translate("options.fontPreset.compact"), value: "compact" },
        { label: translate("options.fontPreset.reading"), value: "reading" },
    ];
}

export function getThemeModeOptions(): OptionItem<AppSettings["themeMode"]>[] {
    return [
        { label: translate("options.themeMode.system"), value: "system" },
        { label: translate("options.themeMode.light"), value: "light" },
        { label: translate("options.themeMode.dark"), value: "dark" },
    ];
}

export function getColorThemeOptions(): OptionItem<AppSettings["colorTheme"]>[] {
    return [
        { label: translate("options.colorTheme.blue"), value: "blue" },
        { label: translate("options.colorTheme.graphite"), value: "graphite" },
        { label: translate("options.colorTheme.forest"), value: "forest" },
        { label: translate("options.colorTheme.sunset"), value: "sunset" },
        { label: translate("options.colorTheme.rose"), value: "rose" },
        { label: translate("options.colorTheme.violet"), value: "violet" },
        { label: translate("options.colorTheme.amber"), value: "amber" },
    ];
}

/** Representative accent swatch for each color theme (always light-mode tones for visibility). */
export const COLOR_THEME_SWATCHES: Record<ColorTheme, string> = {
    blue:     "#355f96",
    graphite: "#627588",
    forest:   "#2f7d69",
    sunset:   "#c36f37",
    rose:     "#b84c6e",
    violet:   "#6b4fc8",
    amber:    "#a87928",
};

export function getAmountDisplayOptions(): OptionItem<AppSettings["amountDisplay"]>[] {
    return [
        { label: translate("options.amountDisplay.full"), value: "full" },
        { label: translate("options.amountDisplay.compact"), value: "compact" },
    ];
}

export function getCurrencyDisplayOptions(): OptionItem<AppSettings["currencyDisplay"]>[] {
    return [
        { label: translate("options.currencyDisplay.symbol"), value: "symbol" },
        { label: translate("options.currencyDisplay.code"), value: "code" },
    ];
}

export function getPriceColorOptions(): OptionItem<AppSettings["priceColorScheme"]>[] {
    return [
        { label: translate("options.priceColorScheme.cn"), value: "cn" },
        { label: translate("options.priceColorScheme.intl"), value: "intl" },
    ];
}

export function getLocaleOptions(): OptionItem<AppSettings["locale"]>[] {
    return [
        { label: translate("options.locale.system"), value: "system" },
        { label: translate("options.locale.zh-CN"), value: "zh-CN" },
        { label: translate("options.locale.en-US"), value: "en-US" },
    ];
}

export function getAlertConditionOptions(): OptionItem<AlertCondition>[] {
    return [
        { label: translate("options.alertCondition.above"), value: "above" },
        { label: translate("options.alertCondition.below"), value: "below" },
    ];
}

export function getHotMarketOptions(): OptionItem<HotMarketGroup>[] {
    return [
        { label: translate("options.hotMarket.cn"), value: "cn" },
        { label: translate("options.hotMarket.hk"), value: "hk" },
        { label: translate("options.hotMarket.us"), value: "us" },
    ];
}

export function getHotCategoryOptions(): Record<HotMarketGroup, OptionItem<HotCategory>[]> {
    return {
        cn: [
            { label: translate("options.hotCategory.cn-a"), value: "cn-a" },
            { label: translate("options.hotCategory.cn-etf"), value: "cn-etf" },
        ],
        hk: [
            { label: translate("options.hotCategory.hk"), value: "hk" },
            { label: translate("options.hotCategory.hk-etf"), value: "hk-etf" },
        ],
        us: [
            { label: translate("options.hotCategory.us-sp500"), value: "us-sp500" },
            { label: translate("options.hotCategory.us-nasdaq"), value: "us-nasdaq" },
            { label: translate("options.hotCategory.us-dow"), value: "us-dow" },
            { label: translate("options.hotCategory.us-etf"), value: "us-etf" },
        ],
    };
}

export function getHotSortOptions(): OptionItem<HotSort>[] {
    return [
        { label: translate("options.hotSort.volume"), value: "volume" },
        { label: translate("options.hotSort.gainers"), value: "gainers" },
        { label: translate("options.hotSort.losers"), value: "losers" },
        { label: translate("options.hotSort.market-cap"), value: "market-cap" },
        { label: translate("options.hotSort.price"), value: "price" },
    ];
}

export function getDashboardCurrencyOptions(): OptionItem[] {
    return [
        { label: translate("options.dashboardCurrency.CNY"), value: "CNY" },
        { label: translate("options.dashboardCurrency.HKD"), value: "HKD" },
        { label: translate("options.dashboardCurrency.USD"), value: "USD" },
    ];
}

export function getProxyModeOptions(): OptionItem<AppSettings["proxyMode"]>[] {
    return [
        { label: translate("options.proxyMode.none"), value: "none" },
        { label: translate("options.proxyMode.system"), value: "system" },
        { label: translate("options.proxyMode.custom"), value: "custom" },
    ];
}
