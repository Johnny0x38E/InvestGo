import type { AppSettings, HistoryInterval } from "./types";

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

// 更新格式化函数读取的全局设置快照。
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

// 判断某个图表范围是否应该显示为日内时间刻度。
function isIntradayHistoryRange(interval: HistoryInterval): boolean {
    return interval === "1h" || interval === "1d";
}

// 将图表点位格式化为适合当前范围的时间轴文本。
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

// 返回图表范围在摘要区使用的简短中文标签。
export function historyRangeLabel(interval: HistoryInterval): string {
    switch (interval) {
        case "1h":
            return "1小时";
        case "1d":
            return "1天";
        case "1w":
            return "1周";
        case "1mo":
            return "1月";
        case "1y":
            return "1年";
        case "3y":
            return "3年";
        case "all":
            return "全部";
        default:
            return "区间";
    }
}
