<script setup lang="ts">
import { computed, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref, watch } from "vue";
import Button from "primevue/button";
import Select from "primevue/select";
import Tag from "primevue/tag";

import { ApiAbortError, api } from "../../api";
import { hotCategoryOptions, hotMarketOptions, hotSortOptions } from "../../constants";
import { formatDateTime, formatMoney, formatPercent, formatUnitPrice } from "../../format";
import type { HotCategory, HotItem, HotListResponse, HotMarketGroup, HotSort } from "../../types";

const props = defineProps<{
    trackedKeys: string[];
}>();

const emit = defineEmits<{
    (event: "add-item", item: HotItem): void;
}>();

const marketGroup = ref<HotMarketGroup>("us");
const category = ref<HotCategory>("us-sp500");
const sortBy = ref<HotSort>("volume");
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

const categoryOptions = computed(() => hotCategoryOptions[marketGroup.value]);
const trackedSet = computed(() => new Set(props.trackedKeys));

watch(marketGroup, (value) => {
    const nextCategory = hotCategoryOptions[value][0]?.value;
    if (nextCategory && nextCategory !== category.value) {
        category.value = nextCategory;
        return;
    }
    void resetAndLoad();
});

watch(category, async (next, previous) => {
    if (next === previous) {
        return;
    }
    await resetAndLoad();
});

watch(sortBy, async (next, previous) => {
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
    cancelInflightRequest(true);
});

onActivated(async () => {
    bindObserver();
    await ensureInitialLoad();
});

onDeactivated(() => {
    unbindObserver();
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

async function resetAndLoad(): Promise<void> {
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
        const payload = await api<HotListResponse>(
            `/api/hot?category=${encodeURIComponent(category.value)}&sort=${encodeURIComponent(sortBy.value)}&page=${encodeURIComponent(String(nextPage))}&pageSize=20`,
            {
                signal: controller.signal,
                timeoutMs: 15000,
            },
        );
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
        error.value = requestError instanceof Error ? requestError.message : "热门列表加载失败";
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
                <h3 class="title">热门</h3>
            </div>
            <div class="hot-toolbar">
                <Select v-model="marketGroup" :options="hotMarketOptions" option-label="label" option-value="value" class="compact-select" />
                <Select v-model="category" :options="categoryOptions" option-label="label" option-value="value" class="compact-select" />
                <Select v-model="sortBy" :options="hotSortOptions" option-label="label" option-value="value" class="compact-select" />
            </div>
        </div>

        <div class="hot-summary">
            <span>每次请求 20 条，默认按交易量降序。</span>
            <span>当前已加载 {{ items.length }} / {{ total }}</span>
        </div>

        <div class="hot-table-shell">
            <table class="hot-table">
                <thead>
                    <tr>
                        <th>标的</th>
                        <th>现价</th>
                        <th>涨跌幅</th>
                        <th>市值</th>
                        <th>交易量</th>
                        <th>来源</th>
                        <th></th>
                    </tr>
                </thead>
                <tbody v-if="items.length">
                    <tr v-for="item in items" :key="hotKey(item)">
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
                                <span>总市值</span>
                            </div>
                        </td>
                        <td>
                            <div class="value-stack">
                                <strong>{{ formatMoney(item.volume) }}</strong>
                                <span>成交量</span>
                            </div>
                        </td>
                        <td>
                            <div class="value-stack">
                                <strong>{{ item.quoteSource }}</strong>
                                <span>{{ formatDateTime(item.updatedAt) }}</span>
                            </div>
                        </td>
                        <td>
                            <div class="action-stack">
                                <Tag v-if="isTracked(item)" value="已添加" severity="success" />
                                <Button v-else size="small" icon="pi pi-plus" label="加入自选" outlined @click="$emit('add-item', item)" />
                            </div>
                        </td>
                    </tr>
                </tbody>
                <tbody v-else-if="!loading">
                    <tr>
                        <td colspan="7" class="empty-row">{{ error || "暂无热门数据。" }}</td>
                    </tr>
                </tbody>
            </table>

            <div v-if="loading" class="hot-feedback">正在加载热门列表…</div>
            <div v-else-if="error && items.length" class="hot-feedback hot-feedback-error">{{ error }}</div>
            <div ref="sentinelRef" class="hot-sentinel">
                <span v-if="loadingMore">正在加载更多…</span>
                <span v-else-if="hasMore">下滑继续加载</span>
                <span v-else-if="items.length">已加载全部</span>
            </div>
        </div>
    </section>
</template>
