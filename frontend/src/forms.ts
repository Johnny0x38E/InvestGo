import type { AlertFormModel, AlertRule, AppSettings, ItemFormModel, WatchlistItem } from "./types";

export const defaultSettings: AppSettings = {
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

export function emptyItemForm(): ItemFormModel {
    return {
        id: "",
        symbol: "",
        name: "",
        market: "A-Share",
        currency: "CNY",
        quantity: 0,
        costPrice: 0,
        currentPrice: 0,
        tagsText: "",
        thesis: "",
    };
}

export function mapItemToForm(item: WatchlistItem): ItemFormModel {
    return {
        id: item.id,
        symbol: item.symbol,
        name: item.name,
        market: item.market,
        currency: item.currency,
        quantity: item.quantity,
        costPrice: item.costPrice,
        currentPrice: item.currentPrice,
        tagsText: item.tags.join(", "),
        thesis: item.thesis,
    };
}

export function serialiseItemForm(
    form: ItemFormModel,
): Omit<WatchlistItem, "previousClose" | "openPrice" | "dayHigh" | "dayLow" | "change" | "changePercent" | "quoteSource" | "quoteUpdatedAt" | "updatedAt" | "tags"> & { tags: string[] } {
    return {
        id: form.id,
        symbol: form.symbol,
        name: form.name,
        market: form.market,
        currency: form.currency,
        quantity: form.quantity || 0,
        costPrice: form.costPrice || 0,
        currentPrice: form.currentPrice || 0,
        thesis: form.thesis,
        tags: form.tagsText
            .split(",")
            .map((value) => value.trim())
            .filter(Boolean),
    };
}

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
