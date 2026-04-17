import type { AlertFormModel, AlertRule, AppSettings, DCAEntry, DCAEntryRow, HotItem, ItemFormModel, WatchlistItem } from "./types";

// Default settings used during frontend initialization; must stay in sync with backend defaults.
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

// Return an empty form model for creating a new watchlist item.
export function emptyItemForm(): ItemFormModel {
    return {
        id: "",
        symbol: "",
        name: "",
        market: "CN-A",
        currency: "CNY",
        quantity: 0,
        costPrice: 0,
        acquiredAt: "",
        tagsText: "",
        thesis: "",
        currentPrice: 0,
        dcaEntries: [],
    };
}
function todayDateString(): string {
    const d = new Date();
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, "0");
    const day = String(d.getDate()).padStart(2, "0");
    return `${y}-${m}-${day}`;
}

// Pre-fill a form for "关注" (watch only, no position) from a hot list item.
export function hotItemToWatchForm(item: HotItem): ItemFormModel {
    return {
        id: "",
        symbol: item.symbol,
        name: item.name,
        market: item.market,
        currency: item.currency,
        quantity: 0,
        costPrice: 0,
        acquiredAt: "",
        tagsText: "",
        thesis: "",
        currentPrice: item.currentPrice,
        dcaEntries: [],
    };
}

// Pre-fill a form for "建仓" (open position) from a hot list item.
// acquiredAt defaults to today; costPrice defaults to currentPrice.
export function hotItemToPositionForm(item: HotItem): ItemFormModel {
    return {
        id: "",
        symbol: item.symbol,
        name: item.name,
        market: item.market,
        currency: item.currency,
        quantity: 0,
        costPrice: item.currentPrice,
        acquiredAt: todayDateString(),
        tagsText: "",
        thesis: "",
        currentPrice: item.currentPrice,
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

// Map a backend item object to the form model used in the edit dialog.
export function mapItemToForm(item: WatchlistItem): ItemFormModel {
    return {
        id: item.id,
        symbol: item.symbol,
        name: item.name,
        market: item.market,
        currency: item.currency,
        quantity: item.quantity,
        costPrice: item.costPrice,
        acquiredAt: item.acquiredAt ? isoDateToInputValue(item.acquiredAt) : "",
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

// Serialize the form model into a backend-compatible item payload.
export function serialiseItemForm(form: ItemFormModel): Omit<
    WatchlistItem,
    | "currentPrice"
    | "previousClose"
    | "openPrice"
    | "dayHigh"
    | "dayLow"
    | "change"
    | "changePercent"
    | "quoteSource"
    | "quoteUpdatedAt"
    | "pinnedAt"
    | "updatedAt"
    | "tags"
    | "dcaSummary"
    | "position"
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
        acquiredAt: form.acquiredAt ? new Date(form.acquiredAt + "T00:00:00").toISOString() : undefined,
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

// Return an empty form model for creating a new alert.
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

// Map a backend alert object to the form model used in the edit dialog.
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
