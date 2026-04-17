import type { AppSettings, HistoryInterval } from "./types";
import { translate } from "./i18n";

let settings: AppSettings = {
    refreshIntervalSeconds: 60,
    cnQuoteSource: "tencent",
    hkQuoteSource: "eastmoney",
    usQuoteSource: "yahoo",
    hotUSSource: "eastmoney",
    themeMode: "system",
    colorTheme: "blue",
    fontPreset: "system",
    amountDisplay: "full",
    currencyDisplay: "symbol",
    priceColorScheme: "cn",
    locale: "system",
    developerMode: false,
    dashboardCurrency: "CNY",
    useNativeTitleBar: false,
};

const currencySymbolMap: Record<string, string> = {
    CNY: "¥",
    HKD: "HK$",
    USD: "$",
};

// Update the global settings snapshot read by formatting functions.
export function setFormatterSettings(next: AppSettings): void {
    settings = next;
}

export function formatMoney(value: number, signed = false): string {
    const amount = Number(value || 0);
    const formatter =
        settings.amountDisplay === "compact"
            ? new Intl.NumberFormat(resolvedLocale(), {
                  notation: "compact",
                  minimumFractionDigits: 0,
                  maximumFractionDigits: 2,
              })
            : new Intl.NumberFormat(resolvedLocale(), {
                  minimumFractionDigits: 2,
                  maximumFractionDigits: 2,
              });

    const prefix = signed && amount > 0 ? "+" : "";
    return `${prefix}${formatter.format(amount)}`;
}

export function formatNumber(value: number, digits = 2): string {
    return new Intl.NumberFormat(resolvedLocale(), {
        minimumFractionDigits: digits,
        maximumFractionDigits: digits,
    }).format(Number(value || 0));
}

export function formatPercent(value: number): string {
    const amount = Number(value || 0);
    const prefix = amount > 0 ? "+" : "";
    return `${prefix}${formatNumber(amount, 2)}%`;
}

export function formatUnitPrice(value: number, currency: string): string {
    const numeric = formatNumber(value, 2);
    if (settings.currencyDisplay === "code") {
        return `${currency} ${numeric}`;
    }
    const symbol = currencySymbolMap[currency] || "";
    return symbol ? `${symbol} ${numeric}` : numeric;
}

export function formatRange(low: number, high: number, currency: string): string {
    if (!(low > 0) || !(high > 0)) {
        return "-";
    }

    const fmt = (v: number) => formatNumber(v, 2);
    return `${fmt(low)} - ${fmt(high)}`;
}

export function formatDateTime(value?: string): string {
    if (!value) {
        return "-";
    }

    return new Intl.DateTimeFormat(resolvedLocale(), {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour12: false,
        hour: "2-digit",
        minute: "2-digit",
    }).format(new Date(value));
}

export function formatShortTime(value?: string): string {
    if (!value) {
        return "-";
    }

    return new Intl.DateTimeFormat(resolvedLocale(), {
        hour12: false,
        hour: "2-digit",
        minute: "2-digit",
    }).format(new Date(value));
}

// Determine whether a chart interval should display intraday time ticks.
function isIntradayHistoryRange(interval: HistoryInterval): boolean {
    return interval === "1h" || interval === "1d";
}

// Format a chart data point into a time-axis label suitable for the given interval.
export function formatHistoryTick(value: string, interval: HistoryInterval): string {
    let options: Intl.DateTimeFormatOptions;
    if (isIntradayHistoryRange(interval)) {
        options = { hour12: false, hour: "2-digit", minute: "2-digit" };
    } else if (interval === "1w" || interval === "1mo") {
        options = { month: "2-digit", day: "2-digit" };
    } else if (interval === "1y" || interval === "3y" || interval === "all") {
        options = { year: "2-digit", month: "2-digit" };
    } else {
        options = { month: "2-digit", day: "2-digit" };
    }

    return new Intl.DateTimeFormat(resolvedLocale(), options).format(new Date(value));
}

export function resolvedLocale(): string {
    return settings.locale === "system" ? navigator.language || "zh-CN" : settings.locale;
}

// Return a short localized label for the chart interval used in the summary area.
export function historyRangeLabel(interval: HistoryInterval): string {
    switch (interval) {
        case "1h":
            return translate("options.historyRange.1h");
        case "1d":
            return translate("options.historyRange.1d");
        case "1w":
            return translate("options.historyRange.1w");
        case "1mo":
            return translate("options.historyRange.1mo");
        case "1y":
            return translate("options.historyRange.1y");
        case "3y":
            return translate("options.historyRange.3y");
        case "all":
            return translate("options.historyRange.all");
        default:
            return translate("options.historyRange.fallback");
    }
}
