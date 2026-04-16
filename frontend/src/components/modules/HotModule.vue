<script setup lang="ts">
import { computed, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref, watch } from "vue";
import Button from "primevue/button";
import InputText from "primevue/inputtext";
import Select from "primevue/select";
import Tag from "primevue/tag";

import { ApiAbortError, api } from "../../api";
import { getHotCategoryOptions, getHotMarketOptions } from "../../constants";
import { formatDateTime, formatMoney, formatPercent, formatUnitPrice } from "../../format";
import { useI18n } from "../../i18n";
import type { HotCategory, HotItem, HotListResponse, HotMarketGroup } from "../../types";

type SortField = "volume" | "changePercent" | "marketCap" | "currentPrice" | null;
type SortDirection = "asc" | "desc";

const props = defineProps<{
    trackedKeys: string[];
}>();

const emit = defineEmits<{
    (event: "add-item", item: HotItem): void;
}>();

const category = ref<HotCategory>("cn-a");
const marketGroup = ref<HotMarketGroup>("cn");
const searchKeyword = ref("");
const activeKeyword = ref("");
const sortField = ref<SortField>("volume");
const sortDirection = ref<SortDirection>("desc");
const items = ref<HotItem[]>([]);
const page = ref(1);
const total = ref(0);
const hasMore = ref(true);
const loading = ref(false);
const loadingMore = ref(false);
const error = ref("");
const sentinelRef = ref<HTMLElement | null>(null);
let observer: IntersectionObserver | null = null;
let inflightController: AbortController | null = null;
let searchDebounceTimer: ReturnType<typeof setTimeout> | null = null;
const { t } = useI18n();

const trackedSet = computed(() => new Set(props.trackedKeys));
const normalizedKeyword = computed(() => activeKeyword.value);
const hotMarketOptions = computed(() => getHotMarketOptions());
const hotCategoryOptions = computed(() => getHotCategoryOptions());
const categoryOptions = computed(() => hotCategoryOptions.value[marketGroup.value]);

const sortedItems = computed(() => {
    const result = [...items.value];
    if (!sortField.value) {
        return result;
    }

    const dir = sortDirection.value === "asc" ? 1 : -1;

    // 这里在前端做二次排序，后端继续负责分页和基础过滤。
    return result.sort((a, b) => {
        const valA = a[sortField.value!] as number;
        const valB = b[sortField.value!] as number;
        return (valA - valB) * dir;
    });
});

const visibleItems = computed(() => {
    return sortedItems.value;
});

const emptyMessage = computed(() => {
    if (error.value && !items.value.length) {
        return error.value;
    }
    if (normalizedKeyword.value) {
        return t("hot.noMatch");
    }
    return t("hot.noData");
});

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

watch(marketGroup, async (next, previous) => {
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
    return categoryBelongsToGroup(next, marketGroup.value) ? next : firstCategoryForGroup(marketGroup.value);
}

function selectCategory(next: HotCategory): void {
    const normalized = normalizeCategory(next);
    if (normalized !== category.value) {
        category.value = normalized;
    }
}

async function resetAndLoad(): Promise<void> {
    // 任何筛选条件变化都从第一页重拉，避免沿用旧分页结果。
    cancelInflightRequest(true);
    items.value = [];
    page.value = 1;
    total.value = 0;
    hasMore.value = true;
    error.value = "";
    await loadPage(1, false);
}

async function ensureInitialLoad(): Promise<void> {
    if (items.value.length || loading.value || loadingMore.value) {
        return;
    }
    await loadPage(1, false);
}

async function loadPage(nextPage: number, append: boolean): Promise<void> {
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

        const payload = await api<HotListResponse>(`/api/hot?${params.toString()}`, {
            signal: controller.signal,
            timeoutMs: 15000,
        });
        // 多次快速切换分类时，只接收最后一次仍然有效的请求结果。
        if (inflightController !== controller) {
            return;
        }
        items.value = append ? [...items.value, ...payload.items] : payload.items;
        page.value = payload.page;
        total.value = payload.total;
        hasMore.value = payload.hasMore;
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
    await loadPage(page.value + 1, true);
}

function bindObserver(): void {
    if (!sentinelRef.value || typeof IntersectionObserver === "undefined") {
        return;
    }

    // 无限滚动只观察底部哨兵元素，进入可视区后再按页加载更多。
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
                <Select v-model="marketGroup" :options="hotMarketOptions" option-label="label" option-value="value" class="compact-select hot-market-select" />
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
            <span v-if="normalizedKeyword">{{ t("hot.searchResults", { count: items.length, total }) }}</span>
            <span v-else>{{ t("hot.loadedSummary", { count: items.length, total }) }}</span>
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
                        <th @click="handleSort('volume')" class="sortable">
                            {{ t("hot.table.volume") }}
                            <span :class="getSortIcon('volume')"></span>
                        </th>
                        <th>{{ t("hot.table.source") }}</th>
                        <th></th>
                    </tr>
                </thead>
                <tbody v-if="visibleItems.length">
                    <tr v-for="item in visibleItems" :key="hotKey(item)">
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
                                <strong :class="item.changePercent >= 0 ? 'tone-rise' : 'tone-fall'">{{ formatPercent(item.changePercent) }}</strong>
                                <span :class="item.change >= 0 ? 'tone-rise' : 'tone-fall'">{{ formatMoney(item.change, true) }}</span>
                            </div>
                        </td>
                        <td>
                            <div class="value-stack">
                                <strong>{{ formatMoney(item.marketCap) }}</strong>
                                <span>{{ t("hot.totalMarketCap") }}</span>
                            </div>
                        </td>
                        <td>
                            <div class="value-stack">
                                <strong>{{ formatMoney(item.volume) }}</strong>
                                <span>{{ t("hot.tradedVolume") }}</span>
                            </div>
                        </td>
                        <td>
                            <div class="value-stack">
                                <strong>{{ item.quoteSource }}</strong>
                                <span>{{ formatDateTime(item.updatedAt) }}</span>
                            </div>
                        </td>
                        <td class="table-action-cell">
                            <div class="action-stack table-action-stack" @click.stop>
                                <Tag v-if="isTracked(item)" :value="t('hot.added')" severity="success" />
                                <Button v-else size="small" outlined icon="pi pi-plus" :label="t('hot.addToWatchlist')" :aria-label="t('hot.addToWatchlist')" @click="$emit('add-item', item)" />
                            </div>
                        </td>
                    </tr>
                </tbody>
                <tbody v-else-if="!loading">
                    <tr>
                        <td colspan="7" class="empty-row">{{ emptyMessage }}</td>
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
.sortable {
    cursor: pointer;
    user-select: none;
}

.sortable span {
    margin-left: 4px;
    opacity: 0.8;
    font-size: 0.8em;
}
</style>
