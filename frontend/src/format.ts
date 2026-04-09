import type { AppSettings, HistoryInterval } from "./types";

let settings: AppSettings = {
    priceMode: "live",
    refreshIntervalSeconds: 20,
    quoteSource: "tx-sina",
    fontPreset: "system",
    amountDisplay: "full",
    currencyDisplay: "symbol",
    priceColorScheme: "cn",
    locale: "system",
    developerMode: false,
};

const currencySymbolMap: Record<string, string> = {
    CNY: "¥",
    HKD: "HK$",
    USD: "$",
};

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
    return `${currencySymbolMap[currency] || ""}${numeric}`;
}

export function formatRange(low: number, high: number, currency: string): string {
    if (!(low > 0) || !(high > 0)) {
        return "-";
    }

    return `${formatUnitPrice(low, currency)} - ${formatUnitPrice(high, currency)}`;
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

export function formatHistoryTick(value: string, interval: HistoryInterval): string {
    let options: Intl.DateTimeFormatOptions;
    if (interval === "live" || interval === "1h" || interval === "6h") {
        options = { hour12: false, hour: "2-digit", minute: "2-digit" };
    } else if (interval === "month") {
        options = { year: "numeric", month: "short" };
    } else if (interval === "week") {
        options = { year: "2-digit", month: "2-digit", day: "2-digit" };
    } else {
        options = { month: "2-digit", day: "2-digit" };
    }

    return new Intl.DateTimeFormat(resolvedLocale(), options).format(new Date(value));
}

export function resolvedLocale(): string {
    return settings.locale === "system" ? navigator.language || "zh-CN" : settings.locale;
}

export function intervalWindowLabel(interval: HistoryInterval): string {
    switch (interval) {
        case "live":
            return "实时窗口";
        case "1h":
            return "1 小时窗口";
        case "6h":
            return "6 小时窗口";
        case "day":
            return "日线窗口";
        case "week":
            return "周线窗口";
        case "month":
            return "月线窗口";
        default:
            return "价格窗口";
    }
}
