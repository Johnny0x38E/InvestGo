import type { AlertFormModel, AlertRule, AppSettings, DCAEntry, DCAEntryRow, ItemFormModel, WatchlistItem } from "./types";

// 前端初始化时使用的默认设置，需与后端默认值保持一致。
export const defaultSettings: AppSettings = {
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

// 返回新增标的时使用的空表单模型。
export function emptyItemForm(): ItemFormModel {
    return {
        id: "",
        symbol: "",
        name: "",
        market: "CN-A",
        currency: "CNY",
        quantity: 0,
        costPrice: 0,
        tagsText: "",
        thesis: "",
        currentPrice: 0,
        dcaEntries: [],
    };
}

function isoDateToInputValue(value: string): string {
    if (!value) {
        return "";
    }

    if (/^\d{4}-\d{2}-\d{2}$/.test(value)) {
        return value;
    }

    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) {
        return value.substring(0, 10);
    }

    const year = String(parsed.getFullYear());
    const month = String(parsed.getMonth() + 1).padStart(2, "0");
    const day = String(parsed.getDate()).padStart(2, "0");
    return `${year}-${month}-${day}`;
}

// 把后端标的对象映射为编辑弹窗使用的表单模型。
export function mapItemToForm(item: WatchlistItem): ItemFormModel {
    return {
        id: item.id,
        symbol: item.symbol,
        name: item.name,
        market: item.market,
        currency: item.currency,
        quantity: item.quantity,
        costPrice: item.costPrice,
        tagsText: item.tags.join(", "),
        thesis: item.thesis,
        currentPrice: item.currentPrice,
        dcaEntries: (item.dcaEntries ?? []).map(
            (e): DCAEntryRow => ({
                id: e.id,
                date: isoDateToInputValue(e.date),
                amount: e.amount,
                shares: e.shares,
                price: e.price && e.price > 0 ? e.price : null,
                fee: e.fee && e.fee > 0 ? e.fee : null,
                note: e.note ?? "",
            }),
        ),
    };
}

// 把表单模型序列化为后端可接受的标的载荷。
export function serialiseItemForm(form: ItemFormModel): Omit<
    WatchlistItem,
    "currentPrice" | "previousClose" | "openPrice" | "dayHigh" | "dayLow" | "change" | "changePercent" | "quoteSource" | "quoteUpdatedAt" | "updatedAt" | "tags"
> & {
    tags: string[];
    dcaEntries: DCAEntry[];
} {
    return {
        id: form.id,
        symbol: form.symbol,
        name: form.name,
        market: form.market,
        currency: form.currency,
        quantity: form.quantity || 0,
        costPrice: form.costPrice || 0,
        thesis: form.thesis,
        tags: form.tagsText
            .split(",")
            .map((value) => value.trim())
            .filter(Boolean),
        dcaEntries: form.dcaEntries
            .filter((e) => (e.amount ?? 0) > 0 && (e.shares ?? 0) > 0)
            .map(
                (e): DCAEntry => ({
                    id: e.id.startsWith("tmp-") ? "" : e.id,
                    date: e.date ? new Date(e.date + "T00:00:00").toISOString() : new Date().toISOString(),
                    amount: e.amount ?? 0,
                    shares: e.shares ?? 0,
                    price: e.price && e.price > 0 ? e.price : undefined,
                    fee: e.fee && e.fee > 0 ? e.fee : undefined,
                    note: e.note || undefined,
                }),
            ),
    };
}

// 返回新增提醒时使用的空表单模型。
export function emptyAlertForm(itemId = ""): AlertFormModel {
    return {
        id: "",
        name: "",
        itemId,
        condition: "above",
        threshold: 1,
        enabled: true,
    };
}

// 把后端提醒对象映射为编辑弹窗使用的表单模型。
export function mapAlertToForm(alert: AlertRule): AlertFormModel {
    return {
        id: alert.id,
        name: alert.name,
        itemId: alert.itemId,
        condition: alert.condition,
        threshold: alert.threshold,
        enabled: alert.enabled,
    };
}
