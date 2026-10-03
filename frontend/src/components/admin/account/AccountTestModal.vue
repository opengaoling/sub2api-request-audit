<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.testAccountConnection')"
    :width="activeTab === 'all' ? 'extra-wide' : 'normal'"
    @close="handleClose"
  >
    <div class="space-y-4">
      <!-- Account Info Card -->
      <div
        v-if="account"
        class="flex items-center justify-between rounded-xl border border-gray-200 bg-gradient-to-r from-gray-50 to-gray-100 p-3 dark:border-dark-500 dark:from-dark-700 dark:to-dark-600"
      >
        <div class="flex items-center gap-3">
          <div
            class="flex h-10 w-10 items-center justify-center rounded-lg bg-gradient-to-br from-primary-500 to-primary-600"
          >
            <Icon name="play" size="md" class="text-white" :stroke-width="2" />
          </div>
          <div>
            <div class="font-semibold text-gray-900 dark:text-gray-100">{{ account.name }}</div>
            <div class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
              <span
                class="rounded bg-gray-200 px-1.5 py-0.5 text-[10px] font-medium uppercase dark:bg-dark-500"
              >
                {{ account.type }}
              </span>
              <span>{{ t('admin.accounts.account') }}</span>
            </div>
          </div>
        </div>
        <span
          :class="[
            'rounded-full px-2.5 py-1 text-xs font-semibold',
            account.status === 'active'
              ? 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400'
              : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400'
          ]"
        >
          {{ account.status }}
        </span>
      </div>

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
          {{ t('admin.accounts.allModelsTest.tabSingle') }}
        </button>
        <button
          type="button"
          :class="[
            'px-4 py-2 text-sm font-medium border-b-2 transition-colors -mb-px flex items-center gap-1.5',
            activeTab === 'all'
              ? 'border-primary-500 text-primary-600 dark:text-primary-400'
              : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
          ]"
          @click="activeTab = 'all'"
        >
          <span>{{ t('admin.accounts.allModelsTest.tabAll') }}</span>
          <span
            v-if="availableModels.length"
            class="rounded-full bg-gray-100 dark:bg-dark-600 px-2 py-0.5 text-xs text-gray-600 dark:text-gray-300 font-mono"
          >
            {{ availableModels.length }}
          </span>
        </button>
      </div>

      <!-- Single Model Test Tab -->
      <div v-show="activeTab === 'single'" class="space-y-4">
        <div class="space-y-1.5">
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.accounts.selectTestModel') }}
          </label>
          <Select
            v-model="selectedModelId"
            :options="availableModels"
            :disabled="loadingModels || status === 'connecting'"
            value-key="id"
            label-key="display_name"
            :placeholder="loadingModels ? t('common.loading') + '...' : t('admin.accounts.selectTestModel')"
          />
        </div>

        <div v-if="isOpenAIAccount" class="space-y-1.5">
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.accounts.openai.testMode') }}
          </label>
          <Select
            v-model="testMode"
            :options="openAITestModeOptions"
            :disabled="status === 'connecting'"
          />
        </div>

        <div v-if="supportsImageTest" class="space-y-1.5">
          <TextArea
            v-model="testPrompt"
            :label="t('admin.accounts.imagePromptLabel')"
            :placeholder="t('admin.accounts.imagePromptPlaceholder')"
            :hint="t('admin.accounts.imageTestHint')"
            :disabled="status === 'connecting'"
            rows="3"
          />
        </div>

        <!-- Terminal Output -->
        <div class="group relative">
          <div
            ref="terminalRef"
            class="max-h-[240px] min-h-[120px] overflow-y-auto rounded-xl border border-gray-700 bg-gray-900 p-4 font-mono text-sm dark:border-gray-800 dark:bg-black"
          >
            <!-- Status Line -->
            <div v-if="status === 'idle'" class="flex items-center gap-2 text-gray-500">
              <Icon name="play" size="sm" :stroke-width="2" />
              <span>{{ t('admin.accounts.readyToTest') }}</span>
            </div>
            <div v-else-if="status === 'connecting'" class="flex items-center gap-2 text-yellow-400">
              <Icon name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
              <span>{{ t('admin.accounts.connectingToApi') }}</span>
            </div>

            <!-- Output Lines -->
            <div v-for="(line, index) in outputLines" :key="index" :class="line.class">
              {{ line.text }}
            </div>

            <!-- Streaming Content -->
            <div v-if="streamingContent" class="text-green-400">
              {{ streamingContent }}<span class="animate-pulse">_</span>
            </div>

            <!-- Result Status -->
            <div
              v-if="status === 'success'"
              class="mt-3 flex items-center gap-2 border-t border-gray-700 pt-3 text-green-400"
            >
              <Icon name="check" size="sm" :stroke-width="2" />
              <span>{{ t('admin.accounts.testCompleted') }}</span>
            </div>
            <div
              v-else-if="status === 'error'"
              class="mt-3 flex items-center gap-2 border-t border-gray-700 pt-3 text-red-400"
            >
              <Icon name="x" size="sm" :stroke-width="2" />
              <span>{{ errorMessage }}</span>
            </div>
          </div>

          <!-- Copy Button -->
          <button
            v-if="outputLines.length > 0"
            @click="copyOutput"
            class="absolute right-2 top-2 rounded-lg bg-gray-800/80 p-1.5 text-gray-400 opacity-0 transition-all hover:bg-gray-700 hover:text-white group-hover:opacity-100"
            :title="t('admin.accounts.copyOutput')"
          >
            <Icon name="link" size="sm" :stroke-width="2" />
          </button>
        </div>

        <div v-if="generatedImages.length > 0" class="space-y-2">
          <div class="text-xs font-medium text-gray-600 dark:text-gray-300">
            {{ t('admin.accounts.imagePreview') }}
          </div>
          <div class="flex flex-wrap justify-center gap-3">
            <div
              v-for="(image, index) in generatedImages"
              :key="`${image.url}-${index}`"
              class="group/img relative cursor-pointer overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm transition hover:border-primary-300 hover:shadow-md dark:border-dark-500 dark:bg-dark-700"
              @click="previewImageUrl = image.url"
            >
              <img :src="image.url" :alt="`test-image-${index + 1}`" class="max-h-[360px] w-full object-contain" />
              <div class="absolute inset-0 flex items-center justify-center bg-black/0 transition-colors group-hover/img:bg-black/20">
                <Icon name="eye" size="lg" class="text-white opacity-0 drop-shadow-lg transition-opacity group-hover/img:opacity-100" :stroke-width="2" />
              </div>
              <div class="border-t border-gray-100 px-3 py-1.5 text-xs text-gray-500 dark:border-dark-500 dark:text-gray-300">
                {{ image.mimeType || 'image/*' }}
              </div>
            </div>
          </div>
        </div>

        <!-- Image Lightbox -->
        <Teleport to="body">
          <Transition name="fade">
            <div
              v-if="previewImageUrl"
              class="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 p-4"
              @click.self="previewImageUrl = ''"
            >
              <button
                class="absolute right-4 top-4 rounded-full bg-black/50 p-2 text-white transition-colors hover:bg-black/70"
                @click="previewImageUrl = ''"
              >
                <Icon name="x" size="lg" :stroke-width="2" />
              </button>
              <img
                :src="previewImageUrl"
                alt="preview"
                class="max-h-[90vh] max-w-[90vw] rounded-lg object-contain shadow-2xl"
              />
            </div>
          </Transition>
        </Teleport>

        <!-- Test Info -->
        <div class="flex items-center justify-between px-1 text-xs text-gray-500 dark:text-gray-400">
          <div class="flex items-center gap-3">
            <span class="flex items-center gap-1">
              <Icon name="grid" size="sm" :stroke-width="2" />
              {{ t('admin.accounts.testModel') }}
            </span>
          </div>
          <span class="flex items-center gap-1">
            <Icon name="chat" size="sm" :stroke-width="2" />
            {{
              supportsImageTest
                ? t('admin.accounts.imageTestMode')
                : t('admin.accounts.testPrompt')
            }}
          </span>
        </div>
      </div>

      <!-- All Models Test Tab -->
      <div v-show="activeTab === 'all'" class="space-y-4">
        <!-- Controls & Summary Bar -->
        <div class="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-gray-50 dark:bg-dark-700 p-3">
          <div class="flex flex-wrap items-center gap-3">
            <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
              <span class="font-medium">{{ t('admin.accounts.batchTest.resultFilter') }}:</span>
              <select v-model="allModelsFilter" class="input py-1 px-2 text-sm">
                <option value="all">{{ t('admin.accounts.allModelsTest.filterAll') }}</option>
                <option value="success">{{ t('admin.accounts.allModelsTest.filterSuccess') }}</option>
                <option value="failed">{{ t('admin.accounts.allModelsTest.filterFailed') }}</option>
                <option value="mismatch">{{ t('admin.accounts.allModelsTest.filterMismatch') }}</option>
              </select>
            </label>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.allModelsTest.summary', {
                total: allModelRows.length,
                success: allModelsSuccessCount,
                failed: allModelsFailedCount,
                mismatchText: allModelsMismatchCount > 0 ? t('admin.accounts.allModelsTest.mismatchAlert', { count: allModelsMismatchCount }) : ''
              }) }}
            </div>
          </div>
          <div class="flex items-center gap-2">
            <button
              v-if="allModelsRunning"
              @click="stopAllModelsTest"
              class="btn btn-secondary btn-sm"
            >
              {{ t('admin.accounts.allModelsTest.stop') }}
            </button>
            <button
              @click="startAllModelsTest"
              :disabled="allModelsRunning || !allModelRows.length"
              class="btn btn-primary btn-sm flex items-center gap-1.5"
            >
              <Icon v-if="allModelsRunning" name="refresh" size="sm" class="animate-spin" />
              <span>{{ allModelsRunning ? t('admin.accounts.allModelsTest.testingAll') : t('admin.accounts.allModelsTest.startAll') }}</span>
            </button>
          </div>
        </div>

        <!-- Progress bar if running or completed -->
        <div v-if="allModelsRunning || allModelsCompletedCount > 0" class="space-y-1">
          <div class="flex justify-between text-xs text-gray-500">
            <span>{{ t('admin.accounts.batchTest.progress', { completed: allModelsCompletedCount, total: allModelRows.length }) }}</span>
            <span>{{ Math.round((allModelsCompletedCount / (allModelRows.length || 1)) * 100) }}%</span>
          </div>
          <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
            <div
              class="h-full bg-primary-500 transition-all duration-300"
              :style="{ width: `${(allModelsCompletedCount / (allModelRows.length || 1)) * 100}%` }"
            ></div>
          </div>
        </div>

        <!-- Models Table -->
        <div class="max-h-[50vh] overflow-auto rounded-lg border border-gray-200 dark:border-dark-600">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
            <thead class="bg-gray-50 dark:bg-dark-700 sticky top-0 z-10">
              <tr>
                <th class="px-3 py-2 text-left font-medium">{{ t('admin.accounts.allModelsTest.model') }}</th>
                <th class="px-3 py-2 text-left font-medium">{{ t('admin.accounts.allModelsTest.upstreamModel') }}</th>
                <th class="px-3 py-2 text-left font-medium">{{ t('admin.accounts.allModelsTest.status') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.accounts.allModelsTest.firstByte') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.accounts.allModelsTest.totalLatency') }}</th>
                <th class="px-3 py-2 text-left font-medium">{{ t('admin.accounts.allModelsTest.error') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-if="!allModelRows.length">
                <td colspan="7" class="py-8 text-center text-sm text-gray-500">
                  {{ loadingModels ? t('common.loading') : t('admin.accounts.allModelsTest.noModels') }}
                </td>
              </tr>
              <tr v-for="row in visibleAllModelRows" :key="row.modelId" class="hover:bg-gray-50/50 dark:hover:bg-dark-700/50">
                <td class="px-3 py-2 font-mono font-medium text-gray-900 dark:text-gray-100">
                  {{ row.modelId }}
                </td>
                <td class="px-3 py-2 font-mono text-xs">
                  <template v-if="row.upstreamModel">
                    <span :class="row.upstreamModel === row.modelId ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400 font-semibold'">
                      {{ row.upstreamModel }}
                    </span>
                    <span v-if="row.upstreamModel !== row.modelId" class="ml-1 text-[10px] text-red-500 font-normal">
                      ({{ t('admin.accounts.batchTest.modelMismatch') }})
                    </span>
                  </template>
                  <span v-else class="text-gray-400">-</span>
                </td>
                <td class="px-3 py-2">
                  <span :class="getAllModelStatusClass(row.status)">
                    {{ getAllModelStatusLabel(row.status) }}
                  </span>
                </td>
                <td class="px-3 py-2 text-right font-mono text-xs">
                  {{ formatLatency(row.firstByteLatencyMs) }}
                </td>
                <td class="px-3 py-2 text-right font-mono text-xs">
                  {{ formatLatency(row.latencyMs) }}
                </td>
                <td class="max-w-64 truncate px-3 py-2 text-xs text-red-600 dark:text-red-400" :title="row.error">
                  {{ row.error || '-' }}
                </td>
                <td class="px-3 py-2 text-right">
                  <button
                    @click="retestSingleModel(row.modelId)"
                    :disabled="allModelsRunning || row.status === 'testing'"
                    class="btn btn-secondary btn-xs"
                    :title="t('admin.accounts.allModelsTest.retest')"
                  >
                    {{ t('admin.accounts.allModelsTest.retest') }}
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button
          @click="handleClose"
          class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
        >
          {{ t('common.close') }}
        </button>
        <button
          v-if="activeTab === 'single'"
          @click="startTest"
          :disabled="status === 'connecting' || !selectedModelId"
          :class="[
            'flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-all',
            status === 'connecting' || !selectedModelId
              ? 'cursor-not-allowed bg-primary-400 text-white'
              : status === 'success'
                ? 'bg-green-500 text-white hover:bg-green-600'
                : status === 'error'
                  ? 'bg-orange-500 text-white hover:bg-orange-600'
                  : 'bg-primary-500 text-white hover:bg-primary-600'
          ]"
        >
          <Icon
            v-if="status === 'connecting'"
            name="refresh"
            size="sm"
            class="animate-spin"
            :stroke-width="2"
          />
          <Icon v-else-if="status === 'idle'" name="play" size="sm" :stroke-width="2" />
          <Icon v-else name="refresh" size="sm" :stroke-width="2" />
          <span>
            {{
              status === 'connecting'
                ? t('admin.accounts.testing')
                : status === 'idle'
                  ? t('admin.accounts.startTest')
                  : t('admin.accounts.retry')
            }}
          </span>
        </button>
        <button
          v-else
          @click="allModelsRunning ? stopAllModelsTest() : startAllModelsTest()"
          :disabled="!allModelRows.length"
          :class="[
            'flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-all',
            allModelsRunning
              ? 'bg-orange-500 text-white hover:bg-orange-600'
              : 'bg-primary-500 text-white hover:bg-primary-600'
          ]"
        >
          <Icon v-if="allModelsRunning" name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
          <Icon v-else name="play" size="sm" :stroke-width="2" />
          <span>
            {{ allModelsRunning ? t('admin.accounts.allModelsTest.stop') : t('admin.accounts.allModelsTest.startAll') }}
          </span>
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import TextArea from '@/components/common/TextArea.vue'
import { Icon } from '@/components/icons'
import { useClipboard } from '@/composables/useClipboard'
import { adminAPI } from '@/api/admin'
import { accountsAPI } from '@/api/admin/accounts'
import type { Account, ClaudeModel } from '@/types'

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

interface OutputLine {
  text: string
  class: string
}

interface PreviewImage {
  url: string
  mimeType?: string
}

type AllModelRow = {
  modelId: string
  upstreamModel: string
  status: 'ready' | 'testing' | 'success' | 'failed'
  firstByteLatencyMs: number
  latencyMs: number
  error: string
}

const props = defineProps<{
  show: boolean
  account: Account | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const activeTab = ref<'single' | 'all'>('single')
const terminalRef = ref<HTMLElement | null>(null)
const status = ref<'idle' | 'connecting' | 'success' | 'error'>('idle')
const outputLines = ref<OutputLine[]>([])
const streamingContent = ref('')
const errorMessage = ref('')
const availableModels = ref<ClaudeModel[]>([])
const selectedModelId = ref('')
const testPrompt = ref('')
const loadingModels = ref(false)
let abortController: AbortController | null = null
const generatedImages = ref<PreviewImage[]>([])
const previewImageUrl = ref('')
const testMode = ref<'default' | 'compact'>('default')

// All models test state
const allModelRows = ref<AllModelRow[]>([])
const allModelsFilter = ref<'all' | 'success' | 'failed' | 'mismatch'>('all')
const allModelsRunning = ref(false)
const allModelsCompletedCount = ref(0)
let allModelsAbortController: AbortController | null = null

const allModelsSuccessCount = computed(() => allModelRows.value.filter((r) => r.status === 'success').length)
const allModelsFailedCount = computed(() => allModelRows.value.filter((r) => r.status === 'failed').length)
const allModelsMismatchCount = computed(
  () =>
    allModelRows.value.filter(
      (r) => r.status === 'success' && r.upstreamModel && r.upstreamModel !== r.modelId
    ).length
)

const visibleAllModelRows = computed(() => {
  if (allModelsFilter.value === 'success') {
    return allModelRows.value.filter((r) => r.status === 'success')
  }
  if (allModelsFilter.value === 'failed') {
    return allModelRows.value.filter((r) => r.status === 'failed')
  }
  if (allModelsFilter.value === 'mismatch') {
    return allModelRows.value.filter(
      (r) => r.upstreamModel && r.upstreamModel !== r.modelId
    )
  }
  return allModelRows.value
})

const isOpenAIAccount = computed(() => props.account?.platform === 'openai')
const openAITestModeOptions = computed(() => [
  { value: 'default', label: t('admin.accounts.openai.testModeDefault') },
  { value: 'compact', label: t('admin.accounts.openai.testModeCompact') }
])
const prioritizedGeminiModels = ['gemini-3.1-flash-image', 'gemini-2.5-flash-image', 'gemini-3.5-flash', 'gemini-2.5-flash', 'gemini-2.5-pro', 'gemini-3-flash-preview', 'gemini-3-pro-preview', 'gemini-2.0-flash']
const supportsGeminiImageTest = computed(() => {
  const modelID = selectedModelId.value.toLowerCase()
  if (!modelID.startsWith('gemini-') || !modelID.includes('-image')) return false

  return props.account?.platform === 'gemini' || (props.account?.platform === 'antigravity' && props.account?.type === 'apikey')
})

const supportsOpenAIImageTest = computed(() => {
  const modelID = selectedModelId.value.toLowerCase()
  if (!modelID.startsWith('gpt-image-')) return false
  return props.account?.platform === 'openai'
})

const supportsImageTest = computed(() => supportsGeminiImageTest.value || supportsOpenAIImageTest.value)

const sortTestModels = (models: ClaudeModel[]) => {
  const priorityMap = new Map(prioritizedGeminiModels.map((id, index) => [id, index]))

  return [...models].sort((a, b) => {
    const aPriority = priorityMap.get(a.id) ?? Number.MAX_SAFE_INTEGER
    const bPriority = priorityMap.get(b.id) ?? Number.MAX_SAFE_INTEGER
    if (aPriority !== bPriority) return aPriority - bPriority
    return 0
  })
}

const initAllModelRows = () => {
  allModelRows.value = availableModels.value.map((m) => ({
    modelId: m.id,
    upstreamModel: '',
    status: 'ready',
    firstByteLatencyMs: 0,
    latencyMs: 0,
    error: ''
  }))
  allModelsCompletedCount.value = 0
}

// Load available models when modal opens
watch(
  () => props.show,
  async (newVal) => {
    if (newVal && props.account) {
      testPrompt.value = ''
      testMode.value = 'default'
      resetState()
      await loadAvailableModels()
    } else {
      abortStream()
      stopAllModelsTest()
    }
  }
)

watch(selectedModelId, () => {
  if (supportsImageTest.value && !testPrompt.value.trim()) {
    testPrompt.value = t('admin.accounts.imagePromptDefault')
  }
})

const loadAvailableModels = async () => {
  if (!props.account) return

  loadingModels.value = true
  selectedModelId.value = '' // Reset selection before loading
  try {
    const models = await adminAPI.accounts.getAvailableModels(props.account.id)
    availableModels.value = props.account.platform === 'gemini' || props.account.platform === 'antigravity'
      ? sortTestModels(models)
      : models
    // Default selection by platform
    if (availableModels.value.length > 0) {
      if (props.account.platform === 'gemini') {
        selectedModelId.value = availableModels.value[0].id
      } else {
        // Try to select Sonnet as default, otherwise use first model
        const sonnetModel = availableModels.value.find((m) => m.id.includes('sonnet'))
        selectedModelId.value = sonnetModel?.id || availableModels.value[0].id
      }
    }
    initAllModelRows()
  } catch (error) {
    console.error('Failed to load available models:', error)
    // Fallback to empty list
    availableModels.value = []
    selectedModelId.value = ''
    allModelRows.value = []
  } finally {
    loadingModels.value = false
  }
}

const resetState = () => {
  status.value = 'idle'
  outputLines.value = []
  streamingContent.value = ''
  errorMessage.value = ''
  generatedImages.value = []
  previewImageUrl.value = ''
  stopAllModelsTest()
}

const handleClose = () => {
  abortStream()
  stopAllModelsTest()
  emit('close')
}

const abortStream = () => {
  if (abortController) {
    abortController.abort()
    abortController = null
  }
}

const addLine = (text: string, className: string = 'text-gray-300') => {
  outputLines.value.push({ text, class: className })
  scrollToBottom()
}

const scrollToBottom = async () => {
  await nextTick()
  if (terminalRef.value) {
    terminalRef.value.scrollTop = terminalRef.value.scrollHeight
  }
}

const startTest = async () => {
  if (!props.account || !selectedModelId.value) return

  resetState()
  status.value = 'connecting'
  addLine(t('admin.accounts.startingTestForAccount', { name: props.account.name }), 'text-blue-400')
  addLine(t('admin.accounts.testAccountTypeLabel', { type: props.account.type }), 'text-gray-400')
  addLine('', 'text-gray-300')

  abortStream()

  abortController = new AbortController()

  try {
    const requestBody: {
      model_id: string
      prompt: string
      mode?: 'default' | 'compact'
    } = {
      model_id: selectedModelId.value,
      prompt: supportsImageTest.value ? testPrompt.value.trim() : ''
    }
    if (isOpenAIAccount.value) {
      requestBody.mode = testMode.value
    }

    // Create EventSource for SSE
    const url = `/api/v1/admin/accounts/${props.account.id}/test`

    // Use fetch with streaming for SSE since EventSource doesn't support POST
    const response = await fetch(url, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${localStorage.getItem('auth_token')}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(requestBody),
      signal: abortController.signal
    })

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`)
    }

    const reader = response.body?.getReader()
    if (!reader) {
      throw new Error('No response body')
    }

    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const jsonStr = line.slice(6).trim()
          if (jsonStr) {
            try {
              const event = JSON.parse(jsonStr)
              handleEvent(event)
            } catch (e) {
              console.error('Failed to parse SSE event:', e)
            }
          }
        }
      }
    }
  } catch (error: unknown) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      status.value = 'idle'
      return
    }
    status.value = 'error'
    const msg = error instanceof Error ? error.message : 'Unknown error'
    errorMessage.value = msg
    addLine(`Error: ${msg}`, 'text-red-400')
  }
}

const handleEvent = (event: {
  type: string
  text?: string
  model?: string
  upstream_model?: string
  success?: boolean
  error?: string
  image_url?: string
  mime_type?: string
}) => {
  switch (event.type) {
    case 'test_start':
      addLine(t('admin.accounts.connectedToApi'), 'text-green-400')
      if (event.model) {
        addLine(t('admin.accounts.usingModel', { model: event.model }), 'text-cyan-400')
      }
      addLine(
        supportsImageTest.value
            ? t('admin.accounts.sendingImageRequest')
            : t('admin.accounts.sendingTestMessage'),
        'text-gray-400'
      )
      addLine('', 'text-gray-300')
      addLine(t('admin.accounts.response'), 'text-yellow-400')
      break

    case 'upstream_model':
      if (event.upstream_model) {
        const isMismatch = event.upstream_model !== selectedModelId.value
        addLine(
          t('admin.accounts.upstreamModelLabel', { model: event.upstream_model }) +
            (isMismatch ? ` (${t('admin.accounts.batchTest.modelMismatch')})` : ''),
          isMismatch ? 'text-red-400 font-bold' : 'text-purple-400'
        )
      }
      break

    case 'content':
      if (event.text) {
        streamingContent.value += event.text
        scrollToBottom()
      }
      break

    case 'image':
      if (event.image_url) {
        generatedImages.value.push({
          url: event.image_url,
          mimeType: event.mime_type
        })
        addLine(t('admin.accounts.imageReceived', { count: generatedImages.value.length }), 'text-purple-300')
      }
      break

    case 'status':
      if (event.text) {
        addLine(event.text, 'text-cyan-300')
      }
      break

    case 'test_complete':
      // Move streaming content to output lines
      if (streamingContent.value) {
        addLine(streamingContent.value, 'text-green-300')
        streamingContent.value = ''
      }
      if (event.success) {
        status.value = 'success'
      } else {
        status.value = 'error'
        errorMessage.value = event.error || 'Test failed'
      }
      break

    case 'error':
      status.value = 'error'
      errorMessage.value = event.error || 'Unknown error'
      if (streamingContent.value) {
        addLine(streamingContent.value, 'text-green-300')
        streamingContent.value = ''
      }
      break
  }
}

const copyOutput = () => {
  const text = outputLines.value.map((l) => l.text).join('\n')
  copyToClipboard(text, t('admin.accounts.outputCopied'))
}

// All Models Test methods
const startAllModelsTest = async () => {
  if (!props.account || allModelsRunning.value || !allModelRows.value.length) return
  allModelsRunning.value = true
  allModelsCompletedCount.value = 0
  allModelRows.value.forEach((r) => {
    r.status = 'ready'
    r.upstreamModel = ''
    r.firstByteLatencyMs = 0
    r.latencyMs = 0
    r.error = ''
  })

  allModelsAbortController = new AbortController()

  try {
    await accountsAPI.testAccountAllModels(
      props.account.id,
      (event) => {
        if (event.type === 'model_started' && event.model_id) {
          const row = allModelRows.value.find((r) => r.modelId === event.model_id)
          if (row) row.status = 'testing'
        } else if (event.type === 'model_result' && event.model_id) {
          const row = allModelRows.value.find((r) => r.modelId === event.model_id)
          if (row) {
            row.status = event.status === 'success' ? 'success' : 'failed'
            row.upstreamModel = event.upstream_model || ''
            row.firstByteLatencyMs = event.first_byte_latency_ms || 0
            row.latencyMs = event.latency_ms || 0
            row.error = event.error || ''
          }
          allModelsCompletedCount.value = event.completed || allModelsCompletedCount.value + 1
        }
      },
      undefined,
      allModelsAbortController.signal
    )
  } catch (error) {
    if (!(error instanceof DOMException && error.name === 'AbortError')) {
      console.error('All models test error:', error)
    }
  } finally {
    allModelsRunning.value = false
    allModelsAbortController = null
  }
}

const stopAllModelsTest = () => {
  if (allModelsAbortController) {
    allModelsAbortController.abort()
    allModelsAbortController = null
  }
  allModelsRunning.value = false
}

const retestSingleModel = async (modelId: string) => {
  if (!props.account || allModelsRunning.value) return
  const row = allModelRows.value.find((r) => r.modelId === modelId)
  if (!row) return
  row.status = 'testing'
  row.upstreamModel = ''
  row.firstByteLatencyMs = 0
  row.latencyMs = 0
  row.error = ''

  try {
    await accountsAPI.testAccountAllModels(
      props.account.id,
      (event) => {
        if (event.type === 'model_result' && event.model_id === modelId) {
          row.status = event.status === 'success' ? 'success' : 'failed'
          row.upstreamModel = event.upstream_model || ''
          row.firstByteLatencyMs = event.first_byte_latency_ms || 0
          row.latencyMs = event.latency_ms || 0
          row.error = event.error || ''
        }
      },
      [modelId]
    )
  } catch (error) {
    row.status = 'failed'
    row.error = String(error)
  }
}

const getAllModelStatusLabel = (s: AllModelRow['status']) => {
  switch (s) {
    case 'ready':
      return t('admin.accounts.batchTest.status.ready')
    case 'testing':
      return t('admin.accounts.batchTest.status.testing')
    case 'success':
      return t('admin.accounts.batchTest.status.success')
    case 'failed':
      return t('admin.accounts.batchTest.status.failed')
    default:
      return s
  }
}

const getAllModelStatusClass = (s: AllModelRow['status']) => {
  switch (s) {
    case 'ready':
      return 'text-gray-500 dark:text-gray-400'
    case 'testing':
      return 'text-yellow-600 dark:text-yellow-400 font-medium'
    case 'success':
      return 'text-green-600 dark:text-green-400 font-medium'
    case 'failed':
      return 'text-red-600 dark:text-red-400 font-medium'
    default:
      return 'text-gray-500'
  }
}

const formatLatency = (val?: number) => {
  if (!val || val <= 0) return '-'
  return `${val} ms`
}
</script>

<style>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
