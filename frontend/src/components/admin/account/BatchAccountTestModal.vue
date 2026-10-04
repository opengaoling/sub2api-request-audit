<template>
  <BaseDialog :show="show" :title="t('admin.accounts.batchTest.title')" width="extra-wide" @close="handleClose">
    <div class="space-y-4">
      <!-- Tab Switcher -->
      <div class="flex border-b border-gray-200 dark:border-dark-600">
        <button
          type="button"
          :class="[
            'px-4 py-2 text-sm font-medium border-b-2 transition-colors -mb-px',
            activeTab === 'single'
              ? 'border-primary-500 text-primary-600 dark:text-primary-400'
              : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
          ]"
          @click="activeTab = 'single'"
        >
          {{ t('admin.accounts.batchTest.tabSingle') }}
        </button>
        <button
          type="button"
          :class="[
            'px-4 py-2 text-sm font-medium border-b-2 transition-colors -mb-px flex items-center gap-1.5',
            activeTab === 'allModels'
              ? 'border-primary-500 text-primary-600 dark:text-primary-400'
              : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
          ]"
          @click="activeTab = 'allModels'"
        >
          {{ t('admin.accounts.batchTest.tabAllModels') }}
        </button>
      </div>

      <!-- ======================== Single Model Test Tab ======================== -->
      <div v-show="activeTab === 'single'" class="space-y-4">
        <div class="flex flex-wrap items-end gap-3">
          <label class="min-w-64 flex-1">
            <span class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.accounts.batchTest.model') }}</span>
            <select v-model="selectedModel" class="input w-full" :disabled="loadingModels || running || !modelOptions.length">
              <option value="" disabled>{{ loadingModels ? t('admin.accounts.batchTest.loadingModels') : t('admin.accounts.batchTest.selectModel') }}</option>
              <option v-for="option in modelOptions" :key="option.id" :value="option.id">
                {{ option.id }} ({{ option.supported }}/{{ rows.length }})
              </option>
            </select>
          </label>
          <div class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.batchTest.summary', { supported: supportedCount, skipped: skippedCount }) }}
          </div>
          <label class="w-36">
            <span class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.accounts.batchTest.resultFilter') }}</span>
            <select v-model="resultFilter" class="input w-full" :aria-label="t('admin.accounts.batchTest.resultFilter')">
              <option value="all">{{ t('admin.accounts.batchTest.filter.all') }}</option>
              <option value="success">{{ t('admin.accounts.batchTest.filter.success') }}</option>
              <option value="failed">{{ t('admin.accounts.batchTest.filter.failed') }}</option>
            </select>
          </label>
          <button class="btn btn-primary" data-testid="batch-test-start" :disabled="running || !selectedModel || supportedCount === 0" @click="startTest">
            {{ running ? t('admin.accounts.batchTest.testing') : t('admin.accounts.batchTest.start') }}
          </button>
          <button v-if="snapshotAvailable" class="btn" data-testid="batch-test-view-snapshot" :disabled="running" @click="restoreSnapshot">
            {{ t('admin.accounts.batchTest.viewSnapshot') }}
          </button>
        </div>

        <div v-if="running || completedCount" class="text-sm text-gray-600 dark:text-gray-300">
          {{ t('admin.accounts.batchTest.progress', { completed: completedCount, total: rows.length }) }}
        </div>

        <div class="max-h-[55vh] overflow-auto rounded-lg border border-gray-200 dark:border-dark-600">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
            <thead class="bg-gray-50 dark:bg-dark-700">
              <tr>
                <th class="px-3 py-2 text-left">{{ t('admin.accounts.batchTest.account') }}</th>
                <th class="px-3 py-2 text-left">{{ t('admin.accounts.batchTest.modelStatus') }}</th>
                <th class="px-3 py-2 text-left">{{ t('admin.accounts.batchTest.result') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.accounts.batchTest.firstByte') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.accounts.batchTest.totalLatency') }}</th>
                <th class="px-3 py-2 text-left">{{ t('admin.accounts.batchTest.error') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="row in visibleRows" :key="row.id">
                <td class="px-3 py-2">
                  <div class="font-medium text-gray-900 dark:text-gray-100">{{ row.name }}</div>
                  <div class="text-xs text-gray-500">{{ row.platform }} #{{ row.id }}</div>
                </td>
                <td class="px-3 py-2">
                  <span :class="row.supportedModels === null ? 'text-gray-500' : row.supportedModels.includes(selectedModel) ? 'text-green-600' : 'text-amber-600'">
                    {{ modelSupportLabel(row) }}
                  </span>
                  <div class="mt-1">{{ row.requestedModel || selectedModel || '-' }}</div>
                  <div v-if="row.upstreamModel" :class="row.upstreamModel === (row.requestedModel || selectedModel) ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'">
                    ↳ {{ t('admin.accounts.batchTest.upstreamResponse') }}: {{ row.upstreamModel }}
                    <span v-if="row.upstreamModel !== (row.requestedModel || selectedModel)">（{{ t('admin.accounts.batchTest.modelMismatch') }}）</span>
                  </div>
                </td>
                <td class="px-3 py-2">
                  <span :class="resultClass(row.status)">{{ resultLabel(row.status) }}</span>
                </td>
                <td class="px-3 py-2 text-right">{{ formatLatency(row.firstByteLatencyMs) }}</td>
                <td class="px-3 py-2 text-right">{{ formatLatency(row.latencyMs) }}</td>
                <td class="max-w-72 truncate px-3 py-2 text-red-600" :title="row.error">{{ row.error || '-' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- ======================== All Models Batch Test Tab ======================== -->
      <div v-show="activeTab === 'allModels'" class="space-y-4">
        <!-- Controls Bar -->
        <div class="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-gray-50 dark:bg-dark-700 p-3">
          <div class="flex flex-wrap items-center gap-3">
            <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
              <span class="font-medium">{{ t('admin.accounts.batchTest.resultFilter') }}:</span>
              <select v-model="allModelsAccountFilter" class="input py-1 px-2 text-sm">
                <option value="all">{{ t('admin.accounts.batchTest.filter.all') }}</option>
                <option value="success">{{ t('admin.accounts.batchTest.allModels.filterFullSuccess') }}</option>
                <option value="partial">{{ t('admin.accounts.batchTest.allModels.filterPartial') }}</option>
                <option value="failed">{{ t('admin.accounts.batchTest.allModels.filterAllFailed') }}</option>
              </select>
            </label>
            <div v-if="allModelsGlobalProgress.total > 0" class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.batchTest.allModels.globalProgress', {
                accountsDone: allModelsGlobalProgress.accountsDone,
                accountsTotal: allModelsGlobalProgress.accountsTotal,
                modelsDone: allModelsGlobalProgress.modelsDone,
                modelsTotal: allModelsGlobalProgress.total
              }) }}
            </div>
          </div>
          <div class="flex items-center gap-2">
            <button
              v-if="allModelsRunning"
              @click="stopAllModelsBatch"
              class="btn btn-secondary btn-sm"
            >
              {{ t('admin.accounts.batchTest.allModels.stop') }}
            </button>
            <button
              @click="startAllModelsBatch"
              :disabled="allModelsRunning || allModelsBatchRows.length === 0"
              class="btn btn-primary btn-sm flex items-center gap-1.5"
            >
              <span v-if="allModelsRunning" class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
              {{ allModelsRunning ? t('admin.accounts.batchTest.allModels.testing') : t('admin.accounts.batchTest.allModels.start') }}
            </button>
          </div>
        </div>

        <!-- Global Progress Bar -->
        <div v-if="allModelsRunning || allModelsGlobalProgress.modelsDone > 0" class="space-y-1">
          <div class="flex justify-between text-xs text-gray-500">
            <span>{{ t('admin.accounts.batchTest.progress', { completed: allModelsGlobalProgress.modelsDone, total: allModelsGlobalProgress.total || 1 }) }}</span>
            <span>{{ Math.round((allModelsGlobalProgress.modelsDone / (allModelsGlobalProgress.total || 1)) * 100) }}%</span>
          </div>
          <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
            <div
              class="h-full bg-primary-500 transition-all duration-300"
              :style="{ width: `${(allModelsGlobalProgress.modelsDone / (allModelsGlobalProgress.total || 1)) * 100}%` }"
            ></div>
          </div>
        </div>

        <!-- Loading state -->
        <div v-if="allModelsLoadingAccounts" class="py-8 text-center text-sm text-gray-500">
          {{ t('admin.accounts.batchTest.loadingModels') }}
        </div>

        <!-- Account rows -->
        <div v-else class="max-h-[55vh] overflow-auto rounded-lg border border-gray-200 dark:border-dark-600">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
            <thead class="bg-gray-50 dark:bg-dark-700 sticky top-0 z-10">
              <tr>
                <th class="px-3 py-2 text-left font-medium w-8"></th>
                <th class="px-3 py-2 text-left font-medium">{{ t('admin.accounts.batchTest.account') }}</th>
                <th class="px-3 py-2 text-left font-medium">{{ t('admin.accounts.batchTest.allModels.modelAvailability') }}</th>
                <th class="px-3 py-2 text-left font-medium">{{ t('admin.accounts.batchTest.allModels.overallStatus') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <template v-if="!visibleAllModelsBatchRows.length">
                <tr>
                  <td colspan="4" class="py-8 text-center text-sm text-gray-500">
                    {{ t('admin.accounts.batchTest.allModels.noAccounts') }}
                  </td>
                </tr>
              </template>
              <template v-for="row in visibleAllModelsBatchRows" :key="row.id">
                <!-- Account summary row -->
                <tr
                  class="hover:bg-gray-50/60 dark:hover:bg-dark-700/50 cursor-pointer"
                  @click="toggleAccountExpanded(row.id)"
                >
                  <td class="px-3 py-2 text-gray-400">
                    <svg
                      class="h-4 w-4 transition-transform"
                      :class="expandedAccounts.has(row.id) ? 'rotate-90' : ''"
                      fill="none" stroke="currentColor" viewBox="0 0 24 24"
                    >
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                    </svg>
                  </td>
                  <td class="px-3 py-2">
                    <div class="font-medium text-gray-900 dark:text-gray-100">{{ row.name }}</div>
                    <div class="text-xs text-gray-500">{{ row.platform }} #{{ row.id }}</div>
                  </td>
                  <td class="px-3 py-2">
                    <div v-if="row.status === 'pending'" class="text-xs text-gray-400">
                      {{ t('admin.accounts.batchTest.allModels.waitingToStart') }}
                    </div>
                    <div v-else-if="row.status === 'loading'" class="text-xs text-gray-400">
                      {{ t('admin.accounts.batchTest.loadingModels') }}
                    </div>
                    <div v-else-if="row.status === 'error'" class="text-xs text-red-500">
                      {{ row.loadError }}
                    </div>
                    <div v-else class="flex flex-wrap items-center gap-2">
                      <div class="flex items-center gap-1">
                        <span class="h-2 w-2 rounded-full bg-green-500"></span>
                        <span class="text-xs text-green-600 dark:text-green-400">{{ row.successCount }}</span>
                      </div>
                      <div class="flex items-center gap-1">
                        <span class="h-2 w-2 rounded-full bg-red-400"></span>
                        <span class="text-xs text-red-600 dark:text-red-400">{{ row.failedCount }}</span>
                      </div>
                      <div v-if="row.testingCount > 0" class="flex items-center gap-1">
                        <span class="h-2 w-2 animate-pulse rounded-full bg-blue-400"></span>
                        <span class="text-xs text-blue-500">{{ row.testingCount }}</span>
                      </div>
                      <div class="text-xs text-gray-400">
                        / {{ row.modelRows.length }} {{ t('admin.accounts.batchTest.allModels.modelsUnit') }}
                      </div>
                      <!-- mini progress bar -->
                      <div v-if="row.status === 'testing'" class="w-24 h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                        <div
                          class="h-full bg-primary-400 transition-all duration-200"
                          :style="{ width: `${((row.successCount + row.failedCount) / (row.modelRows.length || 1)) * 100}%` }"
                        ></div>
                      </div>
                    </div>
                  </td>
                  <td class="px-3 py-2">
                    <span v-if="row.status === 'pending'" class="text-xs text-gray-400">-</span>
                    <span v-else-if="row.status === 'loading'" class="text-xs text-gray-400">{{ t('admin.accounts.batchTest.status.checking') }}</span>
                    <span v-else-if="row.status === 'testing'" class="text-xs text-blue-500">{{ t('admin.accounts.batchTest.status.testing') }}</span>
                    <span v-else-if="row.status === 'error'" class="text-xs text-red-500">{{ t('admin.accounts.batchTest.status.failed') }}</span>
                    <span v-else-if="row.status === 'done'" :class="[
                      'text-xs font-medium',
                      row.failedCount === 0 ? 'text-green-600 dark:text-green-400' :
                      row.successCount === 0 ? 'text-red-600 dark:text-red-400' :
                      'text-amber-600 dark:text-amber-400'
                    ]">
                      {{
                        row.failedCount === 0
                          ? t('admin.accounts.batchTest.allModels.allPassed')
                          : row.successCount === 0
                            ? t('admin.accounts.batchTest.allModels.allFailed')
                            : t('admin.accounts.batchTest.allModels.partial', { success: row.successCount, total: row.modelRows.length })
                      }}
                    </span>
                  </td>
                </tr>
                <!-- Expanded model detail rows -->
                <tr v-if="expandedAccounts.has(row.id) && row.modelRows.length > 0" :key="`${row.id}-detail`">
                  <td colspan="4" class="bg-gray-50/60 dark:bg-dark-800/40 px-3 pb-2 pt-1">
                    <div class="max-h-48 overflow-auto rounded border border-gray-100 dark:border-dark-700">
                      <table class="min-w-full text-xs">
                        <thead class="bg-gray-100 dark:bg-dark-700">
                          <tr>
                            <th class="px-2 py-1.5 text-left font-medium">{{ t('admin.accounts.allModelsTest.model') }}</th>
                            <th class="px-2 py-1.5 text-left font-medium">{{ t('admin.accounts.allModelsTest.upstreamModel') }}</th>
                            <th class="px-2 py-1.5 text-left font-medium">{{ t('admin.accounts.allModelsTest.status') }}</th>
                            <th class="px-2 py-1.5 text-right font-medium">{{ t('admin.accounts.allModelsTest.firstByte') }}</th>
                            <th class="px-2 py-1.5 text-right font-medium">{{ t('admin.accounts.allModelsTest.totalLatency') }}</th>
                            <th class="px-2 py-1.5 text-left font-medium">{{ t('admin.accounts.allModelsTest.error') }}</th>
                          </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                          <tr v-for="mr in row.modelRows" :key="mr.modelId" class="hover:bg-gray-50 dark:hover:bg-dark-700/50">
                            <td class="px-2 py-1.5 font-mono">{{ mr.modelId }}</td>
                            <td class="px-2 py-1.5 font-mono">
                              <span v-if="mr.upstreamModel" :class="mr.upstreamModel === mr.modelId ? 'text-green-600 dark:text-green-400' : 'text-red-500'">
                                {{ mr.upstreamModel }}
                              </span>
                              <span v-else class="text-gray-400">-</span>
                            </td>
                            <td class="px-2 py-1.5">
                              <span :class="getAllModelStatusClass(mr.status)">{{ getAllModelStatusLabel(mr.status) }}</span>
                            </td>
                            <td class="px-2 py-1.5 text-right font-mono">{{ formatLatency(mr.firstByteLatencyMs) }}</td>
                            <td class="px-2 py-1.5 text-right font-mono">{{ formatLatency(mr.latencyMs) }}</td>
                            <td class="max-w-48 truncate px-2 py-1.5 text-red-500" :title="mr.error">{{ mr.error || '-' }}</td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI } from '@/api/admin'
import type { Account } from '@/types'
import type { BatchTestAccountEvent, TestAllModelsEvent } from '@/api/admin/accounts'

const props = defineProps<{ show: boolean; accountIds: number[]; accounts: Account[] }>()
const emit = defineEmits<{ (event: 'close'): void }>()
const { t } = useI18n()

// ==================== Tab ====================
const activeTab = ref<'single' | 'allModels'>('single')

// ==================== Single model test (existing logic) ====================
type Row = {
  id: number
  name: string
  platform: string
  supportedModels: string[] | null
  status: 'checking' | 'ready' | 'skipped' | 'testing' | 'success' | 'failed'
  firstByteLatencyMs: number
  latencyMs: number
  error: string
  requestedModel: string
  upstreamModel: string
}

const rows = ref<Row[]>([])
const selectedModel = ref('')
const loadingModels = ref(false)
const running = ref(false)
const completedCount = ref(0)
const resultFilter = ref<'all' | 'success' | 'failed'>('all')
const snapshotAvailable = ref(false)
const SNAPSHOT_KEY = 'sub2api:batch-account-test-snapshot'
let loadVersion = 0

type Snapshot = { model: string; savedAt: string; rows: Row[] }
const validStatuses = new Set<Row['status']>(['checking', 'ready', 'skipped', 'testing', 'success', 'failed'])
const readSnapshot = (): Snapshot | null => {
  try {
    const parsed = JSON.parse(localStorage.getItem(SNAPSHOT_KEY) || 'null') as Partial<Snapshot> | null
    if (!parsed || typeof parsed.model !== 'string' || !Array.isArray(parsed.rows) || !parsed.rows.length) return null
    if (!parsed.rows.every(row => row && typeof row.id === 'number' && typeof row.name === 'string' && typeof row.platform === 'string' && (row.supportedModels === null || Array.isArray(row.supportedModels)) && validStatuses.has(row.status))) return null
    return parsed as Snapshot
  } catch {
    return null
  }
}

const modelOptions = computed(() => {
  const counts = new Map<string, number>()
  for (const row of rows.value) {
    for (const model of row.supportedModels || []) counts.set(model, (counts.get(model) || 0) + 1)
  }
  return [...counts.entries()].map(([id, supported]) => ({ id, supported })).sort((a, b) => a.id.localeCompare(b.id))
})
const supportedCount = computed(() => rows.value.filter(row => row.supportedModels?.includes(selectedModel.value)).length)
const skippedCount = computed(() => rows.value.length - supportedCount.value)
const visibleRows = computed(() => {
  const filtered = resultFilter.value === 'all'
    ? rows.value
    : rows.value.filter(row => row.status === resultFilter.value)
  return [...filtered].sort((a, b) => {
    const rank = (status: Row['status']) => status === 'success' ? 0 : status === 'failed' ? 1 : 2
    const rankDiff = rank(a.status) - rank(b.status)
    if (rankDiff !== 0 || a.status !== 'success' || b.status !== 'success') return rankDiff
    if (a.firstByteLatencyMs === 0) return b.firstByteLatencyMs === 0 ? 0 : 1
    if (b.firstByteLatencyMs === 0) return -1
    return a.firstByteLatencyMs - b.firstByteLatencyMs
  })
})

const loadModels = async () => {
  const version = ++loadVersion
  loadingModels.value = true
  selectedModel.value = ''
  resultFilter.value = 'all'
  completedCount.value = 0
  const known = new Map(props.accounts.map(account => [account.id, account]))
  rows.value = props.accountIds.map(id => {
    const account = known.get(id)
    return { id, name: account?.name || `Account ${id}`, platform: account?.platform || '-', supportedModels: null, status: 'checking', firstByteLatencyMs: 0, latencyMs: 0, error: '', requestedModel: '', upstreamModel: '' }
  })
  await Promise.all(rows.value.map(async row => {
    try {
      if (!known.has(row.id)) known.set(row.id, await adminAPI.accounts.getById(row.id))
      const account = known.get(row.id)
      if (account) { row.name = account.name; row.platform = account.platform }
      row.supportedModels = (await adminAPI.accounts.getAvailableModels(row.id)).map(model => model.id)
      row.status = 'ready'
    } catch (error) {
      row.status = 'skipped'
      row.error = String(error)
    }
  }))
  if (version !== loadVersion || !props.show) return
  if (!selectedModel.value) selectedModel.value = modelOptions.value.find(option => option.id === 'gpt-5.6-sol')?.id || modelOptions.value[0]?.id || ''
  loadingModels.value = false
}

const handleEvent = (event: BatchTestAccountEvent) => {
  if (event.type === 'account_started' && event.account_id) {
    const row = rows.value.find(item => item.id === event.account_id)
    if (row) row.status = 'testing'
  }
  if (event.type !== 'account_result' || !event.account_id) return
  const row = rows.value.find(item => item.id === event.account_id)
  if (!row) return
  row.status = event.status === 'success' ? 'success' : 'failed'
  row.requestedModel = event.model_id || selectedModel.value
  row.upstreamModel = event.upstream_model || ''
  row.firstByteLatencyMs = event.first_byte_latency_ms || 0
  row.latencyMs = event.latency_ms || 0
  row.error = event.error || ''
  completedCount.value = event.completed || completedCount.value + 1
  persistSnapshot()
}

const startTest = async () => {
  if (!selectedModel.value) return
  const ids = rows.value.filter(row => row.supportedModels?.includes(selectedModel.value)).map(row => row.id)
  rows.value.forEach(row => { if (!ids.includes(row.id)) row.status = 'skipped' })
  completedCount.value = 0
  running.value = true
  try {
    await adminAPI.accounts.batchTestAccounts(ids, selectedModel.value, handleEvent)
  } catch (error) {
    rows.value.filter(row => row.status === 'testing' || row.status === 'ready').forEach(row => { row.status = 'failed'; row.error = String(error) })
  } finally {
    running.value = false
    persistSnapshot()
  }
}

const persistSnapshot = () => {
  if (running.value || !completedCount.value || !selectedModel.value) return
  try {
    localStorage.setItem(SNAPSHOT_KEY, JSON.stringify({ model: selectedModel.value, savedAt: new Date().toISOString(), rows: rows.value }))
    snapshotAvailable.value = true
  } catch {
    snapshotAvailable.value = false
  }
}

const restoreSnapshot = () => {
  const snapshot = readSnapshot()
  if (!snapshot) { snapshotAvailable.value = false; return }
  rows.value = snapshot.rows
  selectedModel.value = snapshot.model
  completedCount.value = rows.value.filter(row => row.status === 'success' || row.status === 'failed').length
  resultFilter.value = 'all'
  running.value = false
}

// ==================== All Models Batch Test (new feature) ====================
type ModelRow = {
  modelId: string
  upstreamModel: string
  status: 'ready' | 'testing' | 'success' | 'failed'
  firstByteLatencyMs: number
  latencyMs: number
  error: string
}

type AllModelsBatchAccountRow = {
  id: number
  name: string
  platform: string
  status: 'pending' | 'loading' | 'testing' | 'done' | 'error'
  loadError: string
  modelRows: ModelRow[]
  successCount: number
  failedCount: number
  testingCount: number
}

const allModelsBatchRows = ref<AllModelsBatchAccountRow[]>([])
const allModelsRunning = ref(false)
const allModelsLoadingAccounts = ref(false)
const allModelsAccountFilter = ref<'all' | 'success' | 'partial' | 'failed'>('all')
const expandedAccounts = ref<Set<number>>(new Set())
let allModelsBatchAbortController: AbortController | null = null

const allModelsGlobalProgress = computed(() => {
  let total = 0, modelsDone = 0, accountsDone = 0
  for (const row of allModelsBatchRows.value) {
    total += row.modelRows.length
    modelsDone += row.successCount + row.failedCount
    if (row.status === 'done' || row.status === 'error') accountsDone++
  }
  return { total, modelsDone, accountsDone, accountsTotal: allModelsBatchRows.value.length }
})

const visibleAllModelsBatchRows = computed(() => {
  if (allModelsAccountFilter.value === 'all') return allModelsBatchRows.value
  if (allModelsAccountFilter.value === 'success') {
    return allModelsBatchRows.value.filter(r => r.status === 'done' && r.failedCount === 0 && r.modelRows.length > 0)
  }
  if (allModelsAccountFilter.value === 'partial') {
    return allModelsBatchRows.value.filter(r => r.status === 'done' && r.failedCount > 0 && r.successCount > 0)
  }
  if (allModelsAccountFilter.value === 'failed') {
    return allModelsBatchRows.value.filter(r => (r.status === 'done' && r.successCount === 0) || r.status === 'error')
  }
  return allModelsBatchRows.value
})

const toggleAccountExpanded = (id: number) => {
  if (expandedAccounts.value.has(id)) {
    expandedAccounts.value.delete(id)
  } else {
    expandedAccounts.value.add(id)
  }
}

const initAllModelsBatchRows = async () => {
  const known = new Map(props.accounts.map(account => [account.id, account]))
  allModelsBatchRows.value = props.accountIds.map(id => {
    const account = known.get(id)
    return {
      id,
      name: account?.name || `Account ${id}`,
      platform: account?.platform || '-',
      status: 'pending',
      loadError: '',
      modelRows: [],
      successCount: 0,
      failedCount: 0,
      testingCount: 0
    }
  })
}

const stopAllModelsBatch = () => {
  if (allModelsBatchAbortController) {
    allModelsBatchAbortController.abort()
    allModelsBatchAbortController = null
  }
  allModelsRunning.value = false
  // Mark remaining pending/testing rows as interrupted
  for (const row of allModelsBatchRows.value) {
    if (row.status === 'pending' || row.status === 'loading' || row.status === 'testing') {
      row.status = 'done'
      // mark unfinished model rows as failed
      for (const mr of row.modelRows) {
        if (mr.status === 'ready' || mr.status === 'testing') {
          mr.status = 'failed'
          mr.error = 'Stopped'
          row.failedCount++
        }
      }
      row.testingCount = 0
    }
  }
}

const startAllModelsBatch = async () => {
  if (allModelsRunning.value) return

  allModelsRunning.value = true
  allModelsAccountFilter.value = 'all'
  allModelsBatchAbortController = new AbortController()
  const signal = allModelsBatchAbortController.signal

  // Reset all rows to pending
  await initAllModelsBatchRows()

  // Process accounts with concurrency limit (2 accounts at a time)
  const CONCURRENCY = 2
  const queue = [...allModelsBatchRows.value]
  let activeCount = 0
  let queueIndex = 0

  const processAccount = async (row: AllModelsBatchAccountRow): Promise<void> => {
    if (signal.aborted) return

    // Step 1: load models
    row.status = 'loading'
    try {
      const models = await adminAPI.accounts.getAvailableModels(row.id)
      if (signal.aborted) return
      row.modelRows = models.map(m => ({
        modelId: m.id,
        upstreamModel: '',
        status: 'ready',
        firstByteLatencyMs: 0,
        latencyMs: 0,
        error: ''
      }))
    } catch (err) {
      row.status = 'error'
      row.loadError = String(err)
      return
    }

    if (row.modelRows.length === 0) {
      row.status = 'done'
      return
    }

    // Step 2: test all models via SSE
    row.status = 'testing'
    row.testingCount = 0
    row.successCount = 0
    row.failedCount = 0

    try {
      await adminAPI.accounts.testAccountAllModels(
        row.id,
        (event: TestAllModelsEvent) => {
          if (event.type === 'model_started' && event.model_id) {
            const mr = row.modelRows.find(m => m.modelId === event.model_id)
            if (mr) { mr.status = 'testing'; row.testingCount++ }
          } else if (event.type === 'model_result' && event.model_id) {
            const mr = row.modelRows.find(m => m.modelId === event.model_id)
            if (mr) {
              mr.status = event.status === 'success' ? 'success' : 'failed'
              mr.upstreamModel = event.upstream_model || ''
              mr.firstByteLatencyMs = event.first_byte_latency_ms || 0
              mr.latencyMs = event.latency_ms || 0
              mr.error = event.error || ''
              if (mr.status === 'success') row.successCount++
              else row.failedCount++
              if (row.testingCount > 0) row.testingCount--
            }
          }
        },
        [],
        signal
      )
    } catch (err) {
      if (!signal.aborted) {
        // Mark remaining unfinished models as failed
        for (const mr of row.modelRows) {
          if (mr.status === 'ready' || mr.status === 'testing') {
            mr.status = 'failed'
            mr.error = String(err)
            row.failedCount++
          }
        }
        row.testingCount = 0
      }
    }

    if (!signal.aborted) {
      row.status = 'done'
      row.testingCount = 0
    }
  }

  // Concurrency pool
  await new Promise<void>((resolve) => {
    const tryNext = () => {
      if (signal.aborted) { resolve(); return }
      while (activeCount < CONCURRENCY && queueIndex < queue.length) {
        const row = queue[queueIndex++]
        activeCount++
        processAccount(row).finally(() => {
          activeCount--
          if (queueIndex >= queue.length && activeCount === 0) {
            resolve()
          } else {
            tryNext()
          }
        })
      }
      if (queueIndex >= queue.length && activeCount === 0) resolve()
    }
    tryNext()
  })

  allModelsRunning.value = false
  allModelsBatchAbortController = null
}

// ==================== Common helpers ====================
const handleClose = () => {
  if (!running.value) {
    stopAllModelsBatch()
    emit('close')
  }
}

const modelSupportLabel = (row: Row) => row.supportedModels === null ? (row.status === 'skipped' ? t('admin.accounts.batchTest.status.skipped') : t('admin.accounts.batchTest.checking')) : row.supportedModels.includes(selectedModel.value) ? t('admin.accounts.batchTest.supported') : t('admin.accounts.batchTest.unsupported')
const resultLabel = (status: Row['status']) => t(`admin.accounts.batchTest.status.${status}`)
const resultClass = (status: Row['status']) => status === 'success' ? 'text-green-600 dark:text-green-400' : status === 'failed' ? 'text-red-600 dark:text-red-400' : 'text-gray-600 dark:text-gray-300'
const formatLatency = (value: number) => value > 0 ? `${value} ms` : '-'

const getAllModelStatusClass = (status: ModelRow['status']) => {
  if (status === 'success') return 'text-green-600 dark:text-green-400'
  if (status === 'failed') return 'text-red-600 dark:text-red-400'
  if (status === 'testing') return 'text-blue-500 dark:text-blue-400'
  return 'text-gray-400'
}
const getAllModelStatusLabel = (status: ModelRow['status']) => {
  if (status === 'success') return t('admin.accounts.batchTest.status.success')
  if (status === 'failed') return t('admin.accounts.batchTest.status.failed')
  if (status === 'testing') return t('admin.accounts.batchTest.status.testing')
  return t('admin.accounts.batchTest.status.ready')
}

watch(() => props.show, async (value) => {
  if (value) {
    snapshotAvailable.value = !!readSnapshot()
    activeTab.value = 'single'
    void loadModels()
    await initAllModelsBatchRows()
  } else {
    stopAllModelsBatch()
  }
})
</script>
