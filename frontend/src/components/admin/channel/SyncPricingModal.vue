<template>
  <BaseDialog
    :show="show"
    :title="t('admin.channels.syncPricing.title', '同步模型价格')"
    width="extra-wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <!-- Description -->
      <p class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.channels.syncPricing.description', '从上游定价源（如 models.dev 或官方镜像）同步最新模型及其输入、输出与缓存读取价格，可对比差异并选择性应用。') }}
      </p>

      <!-- Source & Filter Controls -->
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <!-- Source Selection -->
        <div>
          <label class="block text-xs font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.channels.syncPricing.source', '同步数据源') }}
          </label>
          <select
            v-model="selectedSource"
            class="input mt-1 w-full text-xs"
          >
            <option
              v-for="s in sources"
              :key="s.id"
              :value="s.id"
            >
              {{ s.name }}
            </option>
          </select>
        </div>

        <!-- Platform Filter -->
        <div>
          <label class="block text-xs font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.channels.syncPricing.platform', '平台筛选') }}
          </label>
          <select
            v-model="selectedPlatform"
            class="input mt-1 w-full text-xs"
          >
            <option value="all">{{ t('admin.channels.syncPricing.allPlatforms', '全部平台') }}</option>
            <option value="openai">OpenAI</option>
            <option value="anthropic">Anthropic</option>
            <option value="gemini">Google Gemini</option>
            <option value="deepseek">DeepSeek</option>
          </select>
        </div>

        <!-- Action Button -->
        <div class="flex items-end">
          <button
            type="button"
            @click="handlePreview"
            :disabled="loadingPreview"
            class="btn btn-primary flex w-full items-center justify-center gap-1.5 py-2 text-xs"
          >
            <Icon
              name="refresh"
              size="sm"
              :class="{ 'animate-spin': loadingPreview }"
            />
            {{ loadingPreview ? t('common.loading', '加载中...') : t('admin.channels.syncPricing.checkDiff', '检查价格变动并预览') }}
          </button>
        </div>
      </div>

      <!-- Custom URL Input (if custom source selected) -->
      <div v-if="selectedSource === 'custom'">
        <label class="block text-xs font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.channels.syncPricing.customUrl', '自定义上游 JSON 地址') }}
        </label>
        <input
          v-model="customUrl"
          type="url"
          placeholder="https://..."
          class="input mt-1 w-full text-xs"
        />
      </div>

      <!-- Preview Summary Stats -->
      <div v-if="previewResult" class="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <div class="rounded-lg border border-gray-200 bg-gray-50 p-2.5 dark:border-dark-700 dark:bg-dark-800">
          <div class="text-xs text-gray-500">{{ t('admin.channels.syncPricing.totalUpstream', '上游模型总数') }}</div>
          <div class="text-base font-semibold text-gray-900 dark:text-gray-100">
            {{ previewResult.items.length }}
          </div>
        </div>
        <div class="rounded-lg border border-emerald-200 bg-emerald-50/50 p-2.5 dark:border-emerald-900/40 dark:bg-emerald-950/20">
          <div class="text-xs text-emerald-600 dark:text-emerald-400">{{ t('admin.channels.syncPricing.added', '新增模型') }}</div>
          <div class="text-base font-semibold text-emerald-700 dark:text-emerald-300">
            {{ previewResult.added_count }}
          </div>
        </div>
        <div class="rounded-lg border border-sky-200 bg-sky-50/50 p-2.5 dark:border-sky-900/40 dark:bg-sky-950/20">
          <div class="text-xs text-sky-600 dark:text-sky-400">{{ t('admin.channels.syncPricing.updated', '价格变动') }}</div>
          <div class="text-base font-semibold text-sky-700 dark:text-sky-300">
            {{ previewResult.updated_count }}
          </div>
        </div>
        <div class="rounded-lg border border-gray-200 bg-gray-50/50 p-2.5 dark:border-dark-700 dark:bg-dark-800/40">
          <div class="text-xs text-gray-500">{{ t('admin.channels.syncPricing.unchanged', '价格一致') }}</div>
          <div class="text-base font-semibold text-gray-600 dark:text-gray-300">
            {{ previewResult.unchanged_count }}
          </div>
        </div>
      </div>

      <!-- Preview Filter & Search Bar -->
      <div v-if="previewResult" class="flex flex-wrap items-center justify-between gap-2 border-t border-gray-100 pt-2 dark:border-dark-700">
        <!-- View Filter Buttons -->
        <div class="flex items-center gap-1">
          <button
            type="button"
            @click="filterStatus = 'changed'"
            :class="[
              'rounded-md px-2 py-1 text-xs font-medium transition-colors',
              filterStatus === 'changed'
                ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/40 dark:text-primary-300'
                : 'text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-700'
            ]"
          >
            {{ t('admin.channels.syncPricing.showChangedOnly', '仅看变动与新增') }} ({{ previewResult.added_count + previewResult.updated_count }})
          </button>
          <button
            type="button"
            @click="filterStatus = 'all'"
            :class="[
              'rounded-md px-2 py-1 text-xs font-medium transition-colors',
              filterStatus === 'all'
                ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/40 dark:text-primary-300'
                : 'text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-700'
            ]"
          >
            {{ t('admin.channels.syncPricing.showAll', '查看全部') }} ({{ previewResult.items.length }})
          </button>
        </div>

        <!-- Search input -->
        <div class="relative w-48">
          <input
            v-model="searchQuery"
            type="text"
            :placeholder="t('common.search', '搜索模型名称...')"
            class="input w-full py-1 text-xs"
          />
        </div>
      </div>

      <!-- Table View -->
      <div v-if="previewResult && filteredItems.length > 0" class="max-h-72 overflow-y-auto rounded-lg border border-gray-200 dark:border-dark-700">
        <table class="min-w-full divide-y divide-gray-200 text-left text-xs dark:divide-dark-700">
          <thead class="sticky top-0 bg-gray-50 dark:bg-dark-800">
            <tr>
              <th scope="col" class="w-8 px-3 py-2">
                <input
                  type="checkbox"
                  :checked="isAllSelected"
                  @change="toggleSelectAll"
                  class="rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600"
                />
              </th>
              <th scope="col" class="px-3 py-2 font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.channels.form.model', '模型') }}
              </th>
              <th scope="col" class="px-3 py-2 font-medium text-gray-500 dark:text-gray-400">
                {{ t('common.status', '状态') }}
              </th>
              <th scope="col" class="px-3 py-2 font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.channels.form.inputPrice', '输入价格') }} ($/MTok)
              </th>
              <th scope="col" class="px-3 py-2 font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.channels.form.outputPrice', '输出价格') }} ($/MTok)
              </th>
              <th scope="col" class="px-3 py-2 font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.channels.form.cacheReadPrice', '缓存读取') }} ($/MTok)
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-750 dark:bg-dark-900">
            <tr
              v-for="item in filteredItems"
              :key="item.model"
              :class="[
                'hover:bg-gray-50 dark:hover:bg-dark-800/50',
                selectedModelNames.has(item.model) ? 'bg-primary-50/20 dark:bg-primary-950/10' : ''
              ]"
            >
              <td class="px-3 py-2">
                <input
                  type="checkbox"
                  :checked="selectedModelNames.has(item.model)"
                  @change="toggleSelectModel(item.model)"
                  class="rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600"
                />
              </td>
              <td class="px-3 py-2">
                <div class="font-medium text-gray-900 dark:text-gray-100">
                  {{ item.model }}
                </div>
                <div class="text-[10px] text-gray-400">
                  {{ item.platform }}
                </div>
              </td>
              <td class="px-3 py-2">
                <span
                  v-if="item.status === 'added'"
                  class="inline-flex rounded-full bg-emerald-100 px-2 py-0.5 text-[10px] font-medium text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300"
                >
                  {{ t('admin.channels.syncPricing.statusAdded', '新增') }}
                </span>
                <span
                  v-else-if="item.status === 'updated'"
                  class="inline-flex rounded-full bg-sky-100 px-2 py-0.5 text-[10px] font-medium text-sky-800 dark:bg-sky-900/40 dark:text-sky-300"
                >
                  {{ t('admin.channels.syncPricing.statusUpdated', '变动') }}
                </span>
                <span
                  v-else
                  class="inline-flex rounded-full bg-gray-100 px-2 py-0.5 text-[10px] font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-400"
                >
                  {{ t('admin.channels.syncPricing.statusUnchanged', '一致') }}
                </span>
              </td>
              <td class="px-3 py-2">
                <div v-if="item.status === 'added'" class="text-emerald-600 dark:text-emerald-400">
                  ${{ formatMTok(item.upstream_input_price) }}
                </div>
                <div v-else-if="item.status === 'updated'" class="flex items-center gap-1">
                  <span class="text-gray-400 line-through">${{ formatMTok(item.current_input_price) }}</span>
                  <span class="font-medium text-sky-600 dark:text-sky-400">→ ${{ formatMTok(item.upstream_input_price) }}</span>
                </div>
                <div v-else class="text-gray-600 dark:text-gray-300">
                  ${{ formatMTok(item.upstream_input_price) }}
                </div>
              </td>
              <td class="px-3 py-2">
                <div v-if="item.status === 'added'" class="text-emerald-600 dark:text-emerald-400">
                  ${{ formatMTok(item.upstream_output_price) }}
                </div>
                <div v-else-if="item.status === 'updated'" class="flex items-center gap-1">
                  <span class="text-gray-400 line-through">${{ formatMTok(item.current_output_price) }}</span>
                  <span class="font-medium text-sky-600 dark:text-sky-400">→ ${{ formatMTok(item.upstream_output_price) }}</span>
                </div>
                <div v-else class="text-gray-600 dark:text-gray-300">
                  ${{ formatMTok(item.upstream_output_price) }}
                </div>
              </td>
              <td class="px-3 py-2">
                <div v-if="item.status === 'added'" class="text-emerald-600 dark:text-emerald-400">
                  ${{ formatMTok(item.upstream_cache_read_price) }}
                </div>
                <div v-else-if="item.status === 'updated'" class="flex items-center gap-1">
                  <span class="text-gray-400 line-through">${{ formatMTok(item.current_cache_read_price) }}</span>
                  <span class="font-medium text-sky-600 dark:text-sky-400">→ ${{ formatMTok(item.upstream_cache_read_price) }}</span>
                </div>
                <div v-else class="text-gray-600 dark:text-gray-300">
                  ${{ formatMTok(item.upstream_cache_read_price) }}
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div
        v-else-if="previewResult && filteredItems.length === 0"
        class="rounded-lg border border-dashed border-gray-200 py-8 text-center text-xs text-gray-400 dark:border-dark-700"
      >
        {{ t('admin.channels.syncPricing.noMatchingItems', '没有符合过滤条件的模型') }}
      </div>
    </div>

    <!-- Footer -->
    <template #footer>
      <div class="flex items-center justify-between w-full">
        <div class="text-xs text-gray-500">
          <span v-if="selectedModelNames.size > 0">
            {{ t('admin.channels.syncPricing.selectedCount', { count: selectedModelNames.size }) }}
          </span>
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="btn btn-secondary text-xs"
            @click="emit('close')"
          >
            {{ t('common.cancel', '取消') }}
          </button>
          <button
            v-if="previewResult"
            type="button"
            class="btn btn-primary flex items-center gap-1.5 text-xs"
            :disabled="loadingApply || selectedModelNames.size === 0"
            @click="handleApply"
          >
            <Icon
              name="check"
              size="sm"
              :class="{ 'animate-spin': loadingApply }"
            />
            {{ loadingApply ? t('common.saving') : t('admin.channels.syncPricing.applySync', { count: selectedModelNames.size }) }}
          </button>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import channelsAPI, {
  type PricingSyncSource,
  type PricingSyncPreviewResult,
  type PricingSyncDiffItem,
} from '@/api/admin/channels'
import { perTokenToMTok } from './types'

const props = defineProps<{
  show: boolean
  platform?: string
}>()

const emit = defineEmits<{
  close: []
  synced: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

const sources = ref<PricingSyncSource[]>([])
const selectedSource = ref<string>('models.dev')
const customUrl = ref<string>('')
const selectedPlatform = ref<string>(props.platform || 'all')

const loadingPreview = ref(false)
const loadingApply = ref(false)
const previewResult = ref<PricingSyncPreviewResult | null>(null)

const filterStatus = ref<'changed' | 'all'>('changed')
const searchQuery = ref('')
const selectedModelNames = ref<Set<string>>(new Set())

onMounted(async () => {
  try {
    const list = await channelsAPI.getPricingSyncSources()
    if (list && list.length > 0) {
      sources.value = list
    }
  } catch {
    // 降级使用内置源
    sources.value = [
      {
        id: 'models.dev',
        name: 'models.dev 价格预设',
        url: 'https://models.dev/api.json',
        format: 'models.dev',
        description: 'models.dev'
      },
      {
        id: 'official',
        name: '官方镜像源 (LiteLLM)',
        url: 'https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/main/model_prices_and_context_window.json',
        format: 'litellm',
        description: 'LiteLLM'
      }
    ]
  }
})

watch(() => props.show, (newVal) => {
  if (newVal) {
    if (props.platform) {
      selectedPlatform.value = props.platform
    }
    // 默认打开时自动进行一次预览
    if (!previewResult.value) {
      handlePreview()
    }
  }
})

function formatMTok(perToken: number): string {
  const mtok = perTokenToMTok(perToken)
  if (mtok === null || mtok === undefined) return '0'
  return mtok.toFixed(mtok < 0.01 ? 4 : 2)
}

const filteredItems = computed<PricingSyncDiffItem[]>(() => {
  if (!previewResult.value) return []
  return previewResult.value.items.filter(item => {
    if (filterStatus.value === 'changed') {
      if (item.status === 'unchanged') return false
    }
    if (searchQuery.value.trim()) {
      const q = searchQuery.value.toLowerCase().trim()
      if (!item.model.toLowerCase().includes(q) && !item.platform.toLowerCase().includes(q)) {
        return false
      }
    }
    return true
  })
})

const isAllSelected = computed(() => {
  if (filteredItems.value.length === 0) return false
  return filteredItems.value.every(item => selectedModelNames.value.has(item.model))
})

function toggleSelectAll() {
  if (isAllSelected.value) {
    // 取消当前筛选中的全选
    filteredItems.value.forEach(item => selectedModelNames.value.delete(item.model))
  } else {
    filteredItems.value.forEach(item => selectedModelNames.value.add(item.model))
  }
}

function toggleSelectModel(model: string) {
  if (selectedModelNames.value.has(model)) {
    selectedModelNames.value.delete(model)
  } else {
    selectedModelNames.value.add(model)
  }
}

async function handlePreview() {
  loadingPreview.value = true
  try {
    const res = await channelsAPI.previewPricingSync({
      source: selectedSource.value,
      url: customUrl.value,
      platform: selectedPlatform.value === 'all' ? '' : selectedPlatform.value,
    })
    previewResult.value = res
    // 默认勾选所有变动与新增项
    selectedModelNames.value = new Set(
      res.items.filter(i => i.status !== 'unchanged').map(i => i.model)
    )
    if (res.added_count + res.updated_count === 0) {
      filterStatus.value = 'all'
    } else {
      filterStatus.value = 'changed'
    }
  } catch (err: any) {
    appStore.showError(err?.response?.data?.message || err?.message || '获取上游价格预览失败')
  } finally {
    loadingPreview.value = false
  }
}

async function handleApply() {
  if (selectedModelNames.value.size === 0) return
  loadingApply.value = true
  try {
    const res = await channelsAPI.applyPricingSync({
      source: selectedSource.value,
      url: customUrl.value,
      platform: selectedPlatform.value === 'all' ? '' : selectedPlatform.value,
      models: Array.from(selectedModelNames.value),
    })
    appStore.showSuccess(res.message || '模型价格同步成功')
    emit('synced')
    emit('close')
  } catch (err: any) {
    appStore.showError(err?.response?.data?.message || err?.message || '同步模型价格失败')
  } finally {
    loadingApply.value = false
  }
}
</script>
