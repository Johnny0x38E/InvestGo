import { reactive, ref } from "vue";
import { api } from "../api";
import {
    emptyItemForm,
    hotItemToPositionForm,
    hotItemToWatchForm,
    mapItemToForm,
    serialiseItemForm,
} from "../forms";
import { translate } from "../i18n";
import type { HotItem, ItemFormModel, StateSnapshot, StatusTone, WatchlistItem } from "../types";

type StatusReporter = (message: string, tone: StatusTone) => void;

export function useItemDialog(
    applySnapshot: (snapshot: StateSnapshot) => void,
    clearHistoryCache: () => void,
    setStatus: StatusReporter,
) {
    const itemDialogVisible = ref(false);
    const itemDialogInitialTab = ref<"basic" | "dca">("basic");
    const itemDialogWatchOnly = ref(false);
    const savingItem = ref(false);
    const itemForm = reactive<ItemFormModel>(emptyItemForm());

    // Open the item editor dialog, optionally pre-filling from an existing item.
    function openItemDialog(item?: WatchlistItem, initialTab: "basic" | "dca" = "basic"): void {
        Object.assign(itemForm, item ? mapItemToForm(item) : emptyItemForm());
        itemDialogInitialTab.value = initialTab;
        itemDialogWatchOnly.value = false;
        itemDialogVisible.value = true;
    }

    // Open the item dialog pre-filled from a hot list item in watch-only mode.
    function openHotWatchDialog(item: HotItem): void {
        Object.assign(itemForm, hotItemToWatchForm(item));
        itemDialogInitialTab.value = "basic";
        itemDialogWatchOnly.value = true;
        itemDialogVisible.value = true;
    }

    // Open the item dialog pre-filled from a hot list item in open-position mode.
    function openHotPositionDialog(item: HotItem): void {
        Object.assign(itemForm, hotItemToPositionForm(item));
        itemDialogInitialTab.value = "basic";
        itemDialogWatchOnly.value = false;
        itemDialogVisible.value = true;
    }

    // Save the item and refresh cached data so the active chart stays aligned with the latest state.
    async function saveItem(): Promise<void> {
        savingItem.value = true;
        try {
            const payload = serialiseItemForm(itemForm);
            // In watch-only mode, clear all position fields so the item is saved as a pure watchlist entry.
            if (itemDialogWatchOnly.value) {
                payload.quantity = 0;
                payload.costPrice = 0;
                payload.acquiredAt = undefined;
                payload.dcaEntries = [];
            }
            const path = itemForm.id ? `/api/items/${itemForm.id}` : "/api/items";
            const method = itemForm.id ? "PUT" : "POST";
            const snapshot = await api<StateSnapshot>(path, {
                method,
                body: JSON.stringify(payload),
            });
            clearHistoryCache();
            applySnapshot(snapshot);
            itemDialogVisible.value = false;
            setStatus(
                itemForm.id
                    ? translate("app.itemUpdated")
                    : translate("app.itemAdded"),
                "success",
            );
        } catch (error) {
            setStatus(
                error instanceof Error
                    ? error.message
                    : translate("app.itemSaveFailed"),
                "error",
            );
        } finally {
            savingItem.value = false;
        }
    }

    // Quickly add an instrument from the hot list into the watchlist.
    // isAlreadyTracked is computed by the caller (App.vue) and passed in so this
    // composable does not need a direct reference to the full tracked-key list.
    async function quickAddHotItem(item: HotItem, isAlreadyTracked: boolean): Promise<void> {
        if (isAlreadyTracked) {
            setStatus(translate("app.itemAlreadyTracked"), "warn");
            return;
        }

        try {
            // Quick add only writes the baseline holding fields; the current price is still
            // backfilled by the unified quote source.
            const snapshot = await api<StateSnapshot>("/api/items", {
                method: "POST",
                body: JSON.stringify({
                    symbol: item.symbol,
                    name: item.name,
                    market: item.market,
                    currency: item.currency,
                    quantity: 0,
                    costPrice: item.currentPrice || 0,
                    tags: [translate("app.quickAddTag")],
                    thesis: translate("app.quickAddThesis"),
                }),
            });
            applySnapshot(snapshot);
            setStatus(
                translate("app.hotItemAdded", { symbol: item.symbol }),
                "success",
            );
        } catch (error) {
            setStatus(
                error instanceof Error
                    ? error.message
                    : translate("app.addItemFailed"),
                "error",
            );
        }
    }

    async function toggleItemPinned(item: WatchlistItem): Promise<void> {
        try {
            const snapshot = await api<StateSnapshot>(`/api/items/${item.id}/pin`, {
                method: "PUT",
                body: JSON.stringify({ pinned: !item.pinnedAt }),
            });
            applySnapshot(snapshot);
            setStatus(
                item.pinnedAt
                    ? translate("app.itemUnpinned")
                    : translate("app.itemPinned"),
                "success",
            );
        } catch (error) {
            setStatus(
                error instanceof Error ? error.message : translate("app.pinFailed"),
                "error",
            );
        }
    }

    // Clear related history cache entries when an item is deleted.
    async function performDeleteItem(id: string): Promise<void> {
        try {
            const snapshot = await api<StateSnapshot>(`/api/items/${id}`, {
                method: "DELETE",
            });
            clearHistoryCache();
            applySnapshot(snapshot);
            setStatus(translate("app.itemDeleted"), "success");
        } catch (error) {
            setStatus(
                error instanceof Error
                    ? error.message
                    : translate("app.deleteFailed"),
                "error",
            );
        }
    }

    return {
        itemDialogVisible,
        itemDialogInitialTab,
        itemDialogWatchOnly,
        savingItem,
        itemForm,
        openItemDialog,
        openHotWatchDialog,
        openHotPositionDialog,
        saveItem,
        quickAddHotItem,
        toggleItemPinned,
        performDeleteItem,
    };
}
