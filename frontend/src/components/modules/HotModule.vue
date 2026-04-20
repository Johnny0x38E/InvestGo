<script setup lang="ts">
import { computed, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref, watch } from "vue";
import Button from "primevue/button";
import InputText from "primevue/inputtext";
import Tag from "primevue/tag";

import { ApiAbortError, api } from "../../api";
import { getHotCategoryOptions } from "../../constants";
import { formatDateTime, formatMoney, formatPercent, formatUnitPrice } from "../../format";
import { useI18n } from "../../i18n";
import type { HotCategory, HotItem, HotListResponse, HotMarketGroup } from "../../types";

type SortField = "volume" | "changePercent" | "marketCap" | "currentPrice" | null;
type SortDirection = "asc" | "desc";

const props = defineProps<{
    trackedKeys: string[];
    marketGroup: HotMarketGroup;
}>();

defineEmits<{
    (event: "watch-item", item: HotItem): void;
    (event: "unwatch-item", item: HotItem): void;
    (event: "open-position", item: HotItem): void;
    (event: "update:marketGroup", value: HotMarketGroup): void;
}>();

const category = ref<HotCategory>("cn-a");
const searchKeyword = ref("");
const activeKeyword = ref("");
const sortField = ref<SortField>("volume");
const sortDirection = ref<SortDirection>("desc");
const items = ref<HotItem[]>([]);
const page = ref(1);
const total = ref(0);
const hasMore = ref(true);
const cached = ref(false);
const cacheExpiresAt = ref("");
const loading = ref(false);
const loadingMore = ref(false);
const error = ref("");
const sentinelRef = ref<HTMLElement | null>(null);
let observer: IntersectionObserver | null = null;
let inflightController: AbortController | null = null;
let searchDebounceTimer: ReturnType<typeof setTimeout> | null = null;
const { t } = useI18n();

const trackedSet = computed(() => new Set(props.trackedKeys));
const hotCategoryOptions = computed(() => getHotCategoryOptions());
const categoryOptions = computed(() => hotCategoryOptions.value[props.marketGroup]);

const sortedItems = computed(() => {
    const result = [...items.value];
    if (!sortField.value) {
        return result;
    }

    const dir = sortDirection.value === "asc" ? 1 : -1;

    // Apply a secondary sort on the client while the backend remains responsible for pagination and baseline filtering.
    return result.sort((a, b) => {
        const valA = a[sortField.value!] as number;
        const valB = b[sortField.value!] as number;
        return (valA - valB) * dir;
    });
});

const emptyMessage = computed(() => {
    if (error.value && !items.value.length) {
        return error.value;
    }
    if (activeKeyword.value) {
        return t("hot.noMatch");
    }
    return t("hot.noData");
});

const sourceSummary = computed(() => {
    const distinct = Array.from(new Set(items.value.map((item) => item.quoteSource).filter(Boolean)));
    if (!distinct.length) {
        return t("common.notAvailable");
    }
    return distinct.join(" / ");
});

const updatedAtSummary = computed(() => {
    const timestamps = items.value.map((item) => item.updatedAt).filter(Boolean);
    if (!timestamps.length) {
        return "";
    }
    return timestamps.reduce((latest, current) => (new Date(current).getTime() > new Date(latest).getTime() ? current : latest));
});

const cacheSummary = computed(() => (cached.value ? t("hot.cacheHit") : t("hot.cacheMiss")));

function handleSort(field: SortField): void {
    if (sortField.value === field) {
        sortDirection.value = sortDirection.value === "asc" ? "desc" : "asc";
    } else {
        sortField.value = field;
        sortDirection.value = "desc";
    }
}

function getSortIcon(field: SortField): string {
    if (sortField.value !== field) {
        return "pi pi-sort-alt";
    }
    return sortDirection.value === "asc" ? "pi pi-sort-amount-up" : "pi pi-sort-amount-down";
}

watch(() => props.marketGroup, async (next, previous) => {
    if (next === previous) {
        return;
    }

    const nextCategory = firstCategoryForGroup(next);
    if (!categoryBelongsToGroup(category.value, next)) {
        category.value = nextCategory;
        return;
    }

    await resetAndLoad();
});

watch(category, async (next, previous) => {
    if (next === previous) {
        return;
    }
    await resetAndLoad();
});

watch(searchKeyword, (next, previous) => {
    if (next.trim() === previous.trim()) {
        return;
    }
    clearSearchDebounce();
    searchDebounceTimer = setTimeout(() => {
        activeKeyword.value = searchKeyword.value.trim();
        searchDebounceTimer = null;
    }, 280);
});

watch(activeKeyword, async (next, previous) => {
    if (next === previous) {
        return;
    }
    await resetAndLoad();
});

onMounted(async () => {
    category.value = normalizeCategory(category.value);
    bindObserver();
    await ensureInitialLoad();
});

onBeforeUnmount(() => {
    unbindObserver();
    clearSearchDebounce();
    cancelInflightRequest(true);
});

onActivated(async () => {
    bindObserver();
    await ensureInitialLoad();
});

onDeactivated(() => {
    unbindObserver();
    clearSearchDebounce();
    cancelInflightRequest(true);
});

function hotKey(item: HotItem): string {
    return `${item.market}:${item.symbol}`;
}

function isTracked(item: HotItem): boolean {
    return trackedSet.value.has(hotKey(item));
}

function cancelInflightRequest(resetLoading = false): void {
    inflightController?.abort(new ApiAbortError("aborted"));
    inflightController = null;
    if (resetLoading) {
        loading.value = false;
        loadingMore.value = false;
    }
}

function clearSearchDebounce(): void {
    if (searchDebounceTimer) {
        clearTimeout(searchDebounceTimer);
        searchDebounceTimer = null;
    }
}

function firstCategoryForGroup(group: HotMarketGroup): HotCategory {
    return hotCategoryOptions.value[group][0]?.value ?? "cn-a";
}

function categoryBelongsToGroup(next: HotCategory, group: HotMarketGroup): boolean {
    return hotCategoryOptions.value[group].some((entry) => entry.value === next);
}

function normalizeCategory(next: HotCategory): HotCategory {
    return categoryBelongsToGroup(next, props.marketGroup) ? next : firstCategoryForGroup(props.marketGroup);
}

function selectCategory(next: HotCategory): void {
    const normalized = normalizeCategory(next);
    if (normalized !== category.value) {
        category.value = normalized;
    }
}

async function resetAndLoad(): Promise<void> {
    await resetAndLoadWithOptions(false);
}

async function resetAndLoadWithOptions(forceRefresh: boolean): Promise<void> {
    // Reset to page one whenever any filter changes so stale pagination results are not reused.
    cancelInflightRequest(true);
    items.value = [];
    page.value = 1;
    total.value = 0;
    hasMore.value = true;
    cached.value = false;
    cacheExpiresAt.value = "";
    error.value = "";
    await loadPage(1, false, forceRefresh);
}

async function ensureInitialLoad(): Promise<void> {
    if (items.value.length || loading.value || loadingMore.value) {
        return;
    }
    await loadPage(1, false, false);
}

async function loadPage(nextPage: number, append: boolean, forceRefresh: boolean): Promise<void> {
    if ((loading.value && !append) || (loadingMore.value && append)) {
        return;
    }

    if (append) {
        loadingMore.value = true;
    } else {
        loading.value = true;
    }

    const controller = new AbortController();
    inflightController = controller;

    try {
        const params = new URLSearchParams({
            category: normalizeCategory(category.value),
            page: String(nextPage),
            pageSize: "20",
        });
        if (activeKeyword.value) {
            params.set("q", activeKeyword.value);
        }
        if (forceRefresh) {
            params.set("force", "1");
        }

        const payload = await api<HotListResponse>(`/api/hot?${params.toString()}`, {
            signal: controller.signal,
            timeoutMs: 15000,
        });
        // When categories switch rapidly, only accept the response from the most recent still-active request.
        if (inflightController !== controller) {
            return;
        }
        items.value = append ? [...items.value, ...payload.items] : payload.items;
        page.value = payload.page;
        total.value = payload.total;
        hasMore.value = payload.hasMore;
        cached.value = payload.cached;
        cacheExpiresAt.value = payload.cacheExpiresAt ?? "";
        error.value = "";
    } catch (requestError) {
        if (requestError instanceof ApiAbortError) {
            return;
        }
        error.value = requestError instanceof Error ? requestError.message : t("hot.loadFailed");
    } finally {
        if (inflightController === controller) {
            inflightController = null;
            loading.value = false;
            loadingMore.value = false;
        }
    }
}

async function loadMore(): Promise<void> {
    if (!hasMore.value || loading.value || loadingMore.value) {
        return;
    }
    await loadPage(page.value + 1, true, false);
}

async function refreshHot(forceRefresh = false): Promise<void> {
    await resetAndLoadWithOptions(forceRefresh);
}

function bindObserver(): void {
    if (!sentinelRef.value || typeof IntersectionObserver === "undefined") {
        return;
    }

    // Infinite scrolling only watches the bottom sentinel and loads the next page after it enters the viewport.
    observer?.disconnect();
    observer = new IntersectionObserver(
        (entries) => {
            for (const entry of entries) {
                if (entry.isIntersecting) {
                    void loadMore();
                }
            }
        },
        {
            rootMargin: "120px 0px",
            threshold: 0.1,
        },
    );
    observer.observe(sentinelRef.value);
}

function unbindObserver(): void {
    observer?.disconnect();
    observer = null;
}
</script>

<template>
    <section class="module-content hot-module">
        <div class="panel-header">
            <div>
                <h3 class="title">{{ t("hot.title") }}</h3>
            </div>
            <div class="hot-toolbar">
                <div class="hot-actions">
                    <Button size="small" text icon="pi pi-refresh" :label="t('hot.refresh')" @click="refreshHot(true)" />
                </div>
                <div class="hot-category-tabs" role="tablist" :aria-label="t('hot.ariaCategoryTabs')">
                    <button
                        v-for="entry in categoryOptions"
                        :key="entry.value"
                        class="hot-category-tab"
                        :class="{ active: category === entry.value }"
                        :aria-selected="category === entry.value"
                        role="tab"
                        type="button"
                        @click="selectCategory(entry.value)"
                    >
                        {{ entry.label }}
                    </button>
                </div>
                <InputText v-model="searchKeyword" class="search-input" :placeholder="t('hot.searchPlaceholder')" />
            </div>
        </div>

        <div class="hot-summary">
            <span v-if="activeKeyword">{{ t("hot.searchResults", { count: items.length, total }) }}</span>
            <span v-else>{{ t("hot.loadedSummary", { count: items.length, total }) }}</span>
            <span>{{ t("hot.meta.source", { source: sourceSummary }) }}</span>
            <span>{{ t("hot.meta.updatedAt", { time: updatedAtSummary ? formatDateTime(updatedAtSummary) : t("common.notAvailable") }) }}</span>
            <span>{{ t("hot.meta.cache", { state: cacheSummary }) }}</span>
            <span v-if="cacheExpiresAt">{{ t("hot.meta.cacheFreshUntil", { time: formatDateTime(cacheExpiresAt) }) }}</span>
        </div>

        <div class="hot-table-shell">
            <table class="hot-table">
                <thead>
                    <tr>
                        <th>{{ t("hot.table.item") }}</th>
                        <th @click="handleSort('currentPrice')" class="sortable">
                            {{ t("hot.table.currentPrice") }}
                            <span :class="getSortIcon('currentPrice')"></span>
                        </th>
                        <th @click="handleSort('changePercent')" class="sortable">
                            {{ t("hot.table.changePercent") }}
                            <span :class="getSortIcon('changePercent')"></span>
                        </th>
                        <th @click="handleSort('marketCap')" class="sortable">
                            {{ t("hot.table.marketCap") }}
                            <span :class="getSortIcon('marketCap')"></span>
                        </th>
                        <th class="hot-table-sticky hot-table-sticky-volume" @click="handleSort('volume')">
                            {{ t("hot.table.volume") }}
                            <span :class="getSortIcon('volume')"></span>
                        </th>
                        <th class="hot-table-sticky hot-table-sticky-actions"></th>
                    </tr>
                </thead>
                <tbody v-if="sortedItems.length">
                    <tr v-for="item in sortedItems" :key="hotKey(item)">
                        <td>
                            <div class="item-block">
                                <strong>{{ item.name }}</strong>
                                <span>{{ item.market }} · {{ item.symbol }}</span>
                            </div>
                        </td>
                        <td>
                            <div class="value-stack">
                                <strong>{{ formatUnitPrice(item.currentPrice, item.currency) }}</strong>
                                <span>{{ item.currency }}</span>
                            </div>
                        </td>
                        <td>
                            <div class="value-stack">
                                <strong :class="item.changePercent > 0 ? 'tone-rise' : item.changePercent < 0 ? 'tone-fall' : ''">{{ formatPercent(item.changePercent) }}</strong>
                                <span :class="item.change > 0 ? 'tone-rise' : item.change < 0 ? 'tone-fall' : ''">{{ formatMoney(item.change, true) }}</span>
                            </div>
                        </td>
                        <td>
                            <div class="value-stack">
                                <strong>{{ formatMoney(item.marketCap) }}</strong>
                                <span>{{ t("hot.totalMarketCap") }}</span>
                            </div>
                        </td>
                        <td class="hot-table-sticky hot-table-sticky-volume">
                            <div class="value-stack">
                                <strong>{{ formatMoney(item.volume) }}</strong>
                                <span>{{ t("hot.tradedVolume") }}</span>
                            </div>
                        </td>
                        <td class="table-action-cell hot-table-sticky hot-table-sticky-actions">
                            <div class="action-stack table-action-stack" @click.stop>
                                <template v-if="isTracked(item)">
                                    <Button
                                        size="small"
                                        text
                                        rounded
                                        icon="pi pi-bookmark-fill"
                                        class="hot-watched-button"
                                        style="color: var(--accent);"
                                        :aria-label="t('hot.unwatchItem')"
                                        :title="t('hot.unwatchItem')"
                                        @click="$emit('unwatch-item', item)"
                                    />
                                </template>
                                <template v-else>
                                    <Button
                                        size="small"
                                        text
                                        rounded
                                        icon="pi pi-bookmark"
                                        class="hot-watch-button"
                                        :aria-label="t('hot.watchItem')"
                                        :title="t('hot.watchItem')"
                                        @click="$emit('watch-item', item)"
                                    />
                                    <Button
                                        size="small"
                                        text
                                        rounded
                                        icon="pi pi-wallet"
                                        class="hot-position-button"
                                        :aria-label="t('hot.openPosition')"
                                        :title="t('hot.openPosition')"
                                        @click="$emit('open-position', item)"
                                    />
                                </template>
                            </div>
                        </td>
                    </tr>
                </tbody>
                <tbody v-else-if="!loading">
                    <tr>
                        <td colspan="6" class="empty-row">{{ emptyMessage }}</td>
                    </tr>
                </tbody>
            </table>

            <div v-if="loading" class="hot-feedback">{{ t("hot.loading") }}</div>
            <div v-else-if="error && items.length" class="hot-feedback hot-feedback-error">{{ error }}</div>
            <div ref="sentinelRef" class="hot-sentinel">
                <span v-if="loadingMore">{{ t("hot.loadingMore") }}</span>
                <span v-else-if="hasMore">{{ t("hot.scrollToLoad") }}</span>
                <span v-else-if="items.length">{{ t("hot.allLoaded") }}</span>
            </div>
        </div>
    </section>
</template>

<style scoped>
.hot-toolbar {
    display: flex;
    align-items: center;
    gap: 10px;
    justify-content: flex-end;
    flex-wrap: nowrap;
    min-width: 0;
}

.hot-actions {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    flex: 0 0 auto;
}

.hot-actions :deep(.p-button) {
    flex-wrap: nowrap;
    white-space: nowrap;
}

.hot-actions :deep(.p-button-label) {
    white-space: nowrap;
}

.hot-toolbar .search-input {
    height: 40px;
    flex: 0 0 220px;
    min-width: 180px;
}

.hot-category-tabs {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px;
    border: 1px solid var(--border);
    border-radius: calc(var(--radius-control) + 2px);
    background: var(--panel-soft);
    box-shadow: var(--shadow-soft);
    flex: 0 0 auto;
}

.hot-category-tab {
    min-height: 32px;
    padding: 0 12px;
    border-radius: calc(var(--radius-control) - 4px);
    border: 1px solid transparent;
    background: transparent;
    color: var(--muted);
    font: 600 12px/1 var(--font-ui);
    cursor: pointer;
    transition:
        background 140ms ease,
        border-color 140ms ease,
        color 140ms ease,
        box-shadow 140ms ease;
}

.hot-category-tab:hover {
    color: var(--ink);
    background: color-mix(in srgb, var(--accent-soft) 52%, var(--panel-strong));
}

.hot-category-tab.active {
    color: var(--accent-strong);
    border-color: color-mix(in srgb, var(--accent) 18%, var(--border));
    background: linear-gradient(180deg, color-mix(in srgb, var(--accent-soft) 86%, var(--panel-strong)) 0%, color-mix(in srgb, var(--accent-soft) 34%, var(--panel-strong)) 100%);
    box-shadow: var(--shadow-soft);
}

.hot-summary {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
    color: var(--muted);
    font-size: 11px;
}

.hot-table-shell {
    min-height: 0;
    flex: 1 1 auto;
    overflow: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius-panel);
    background: var(--panel-strong);
    box-shadow: var(--shadow-soft);
}

.hot-table {
    min-width: 780px;
}

.hot-feedback,
.hot-sentinel {
    padding: 12px 14px;
    text-align: center;
    font-size: 12px;
    color: var(--muted);
}

.hot-feedback-error {
    color: var(--fall);
}

.sortable {
    cursor: pointer;
    user-select: none;
    white-space: nowrap;
}

.sortable span {
    margin-left: 4px;
    opacity: 0.8;
    font-size: 12px;
    line-height: 1;
    display: inline-flex;
    align-items: center;
}

.hot-table th.hot-table-sticky-volume .pi {
    font-size: 11px;
    line-height: 1;
}

.hot-table th:first-child,
.hot-table td:first-child {
    width: 42%;
}

.hot-table th:nth-child(2),
.hot-table td:nth-child(2) {
    width: 102px;
}

.hot-table th:nth-child(3),
.hot-table td:nth-child(3) {
    width: 106px;
}

.hot-table th:nth-child(4),
.hot-table td:nth-child(4) {
    width: 108px;
}

.hot-table th.hot-table-sticky-volume,
.hot-table td.hot-table-sticky-volume {
    right: 88px;
    width: 108px;
    min-width: 108px;
    max-width: 108px;
}

.hot-table th.hot-table-sticky-actions,
.hot-table td.hot-table-sticky-actions {
    right: 0;
    width: 88px;
    min-width: 88px;
    max-width: 88px;
}

.hot-table td.table-action-cell {
    padding-left: 12px;
    padding-right: 12px;
}

.hot-table .table-action-stack {
    width: 100%;
    justify-content: center;
    gap: 4px;
}

.hot-table .table-action-cell :deep(.p-button) {
    width: 28px;
    height: 28px;
    min-width: 28px;
    padding: 0;
}

.hot-table .table-action-cell :deep(.hot-add-button.p-button) {
    width: auto;
    min-width: 0;
    height: 28px;
    padding: 0.25rem 0.7rem;
    gap: 0.35rem;
}

.hot-table .table-action-cell :deep(.hot-add-button .p-button-label) {
    font-size: 11px;
    white-space: nowrap;
}

.hot-table .table-action-cell :deep(.hot-position-button.p-button) {
    color: var(--accent);
}


@media (max-width: 880px) {
    .hot-summary {
        align-items: stretch;
        flex-direction: column;
    }

    .hot-module .panel-header,
    .hot-module .hot-toolbar {
        align-items: center;
        flex-direction: row;
    }
}
</style>
