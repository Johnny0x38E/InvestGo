import type { AppSettings, HistoryInterval } from "./types";
import { translate } from "./i18n";
import { defaultSettings } from "./forms";

let settings: AppSettings = { ...defaultSettings };

const currencySymbolMap: Record<string, string> = {
    CNY: "¥",
    HKD: "HK$",
    USD: "$",
};

// Formatter cache: keyed by a string describing the options
const formatterCache = new Map<string, Intl.NumberFormat>();

function getCachedFormatter(key: string, factory: () => Intl.NumberFormat): Intl.NumberFormat {
    let formatter = formatterCache.get(key);
    if (!formatter) {
        formatter = factory();
        formatterCache.set(key, formatter);
    }
    return formatter;
}

// Update the global settings snapshot read by formatting functions.
export function setFormatterSettings(next: AppSettings): void {
    settings = next;
    formatterCache.clear();
}

export function formatMoney(value: number, signed = false): string {
    const amount = Number(value || 0);
    const locale = resolvedLocale();
    const compact = settings.amountDisplay === "compact";
    const key = compact ? `money-compact-${locale}` : `money-full-${locale}`;
    const formatter = getCachedFormatter(key, () =>
        compact
            ? new Intl.NumberFormat(locale, { notation: "compact", minimumFractionDigits: 0, maximumFractionDigits: 2 })
            : new Intl.NumberFormat(locale, { minimumFractionDigits: 2, maximumFractionDigits: 2 }),
    );
    const prefix = signed && amount > 0 ? "+" : "";
    return `${prefix}${formatter.format(amount)}`;
}

export function formatNumber(value: number, digits = 2): string {
    const locale = resolvedLocale();
    const key = `number-${locale}-${digits}`;
    const formatter = getCachedFormatter(
        key,
        () => new Intl.NumberFormat(locale, { minimumFractionDigits: digits, maximumFractionDigits: digits }),
    );
    return formatter.format(Number(value || 0));
}

// formatFlexNumber formats value with at least minDigits and at most maxDigits decimal places,
// trimming unnecessary trailing zeros beyond minDigits.
export function formatFlexNumber(value: number, minDigits: number, maxDigits: number): string {
    const locale = resolvedLocale();
    const key = `number-flex-${locale}-${minDigits}-${maxDigits}`;
    const formatter = getCachedFormatter(
        key,
        () => new Intl.NumberFormat(locale, { minimumFractionDigits: minDigits, maximumFractionDigits: maxDigits }),
    );
    return formatter.format(Number(value || 0));
}

export function formatPercent(value: number): string {
    const amount = Number(value || 0);
    const prefix = amount > 0 ? "+" : "";
    return `${prefix}${formatNumber(amount, 2)}%`;
}

// formatUnitPrice formats a price/amount in the given currency.
// Pass maxFractionDigits > 2 (e.g. 4) when the stored value may have more decimal places
// that should be shown in full (e.g. cost-price, DCA buy-price).
export function formatUnitPrice(value: number, currency: string, maxFractionDigits = 2): string {
    const numeric = formatFlexNumber(value, 2, maxFractionDigits);
    if (settings.currencyDisplay === "code") {
        return `${currency} ${numeric}`;
    }
    const symbol = currencySymbolMap[currency] || "";
    return symbol ? `${symbol} ${numeric}` : numeric;
}

// formatShares formats a share / unit count with at most 4 decimal places,
// trimming trailing zeros (100 → "100", 0.1234 → "0.1234", 15.50 → "15.5").
export function formatShares(value: number): string {
    return formatFlexNumber(value, 0, 4);
}

export function formatRange(low: number, high: number): string {
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

// Return the localized display label for a market identifier.
// Falls back to the raw market string if no translation is found.
export function formatMarket(market: string): string {
    const label = translate(`options.market.${market}`);
    return label || market;
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
