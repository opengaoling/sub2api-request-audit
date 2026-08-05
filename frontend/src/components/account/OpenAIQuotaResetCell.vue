<template>
  <div v-if="visible" class="space-y-1">
    <div class="flex flex-wrap items-center gap-1.5">
      <slot name="pre-actions" />
      <button
        type="button"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        :disabled="loading || resetting"
        @click="query"
      >
        <span :class="{ 'animate-spin': loading }">↻</span>
        {{ t('admin.accounts.openaiQuotaReset.count') }}<span v-if="hasAvailableCount"> {{ availableCount }}</span>
      </button>
      <button
        type="button"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-orange-600 transition-colors hover:bg-orange-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-orange-400 dark:hover:bg-orange-900/30"
        :disabled="loading || resetting || !canReset"
        @click="requestReset"
      >
        <span :class="{ 'animate-spin': resetting }">↻</span>
        {{ t('admin.accounts.openaiQuotaReset.reset') }}
      </button>
    </div>
    <div v-if="error" class="text-[10px] text-red-600 dark:text-red-400" :title="error">
      {{ error }}
    </div>
    <div v-else-if="success" class="text-[10px] text-emerald-600 dark:text-emerald-400">
      {{ success }}
    </div>
  </div>

  <ConfirmDialog
    :show="showResetConfirmation"
    :title="t('admin.accounts.openaiQuotaReset.confirmTitle')"
    :message="t('admin.accounts.openaiQuotaReset.confirmMessage')"
    :confirm-text="t('admin.accounts.openaiQuotaReset.reset')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    @confirm="performReset"
    @cancel="showResetConfirmation = false"
  />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import {
  queryOpenAIQuota,
  resetOpenAIQuota,
  type OpenAIQuotaResetResult,
  type OpenAIQuotaUsage
} from '@/api/admin/accounts'

const props = defineProps<{ account: Account }>()
const { t } = useI18n()
const visible = computed(() => props.account.platform === 'openai' && props.account.type === 'oauth')
const loading = ref(false)
const resetting = ref(false)
const error = ref<string | null>(null)
const success = ref<string | null>(null)
const data = ref<OpenAIQuotaUsage | null>(null)
const showResetConfirmation = ref(false)
const persistedAvailableCount = computed(() => {
  const value = props.account.extra?.codex_reset_credit_available_count
  return typeof value === 'number' && Number.isFinite(value) ? value : null
})
const hasAvailableCount = computed(() => data.value !== null || persistedAvailableCount.value !== null)
const availableCount = computed(() => data.value?.rate_limit_reset_credits?.available_count ?? persistedAvailableCount.value ?? 0)
const canReset = computed(() => availableCount.value > 0)

const errorMessage = (value: unknown) => {
  const candidate = value as { message?: string; reason?: string; response?: { data?: { message?: string } } }
  return candidate?.message || candidate?.reason || candidate?.response?.data?.message || t('common.error')
}

const query = async () => {
  if (loading.value) return
  loading.value = true
  error.value = null
  success.value = null
  try {
    data.value = await queryOpenAIQuota(props.account.id)
  } catch (value) {
    error.value = errorMessage(value)
  } finally {
    loading.value = false
  }
}

const requestReset = () => {
  if (resetting.value || !canReset.value) return
  showResetConfirmation.value = true
}

const performReset = async () => {
  showResetConfirmation.value = false
  if (resetting.value || !canReset.value) return
  resetting.value = true
  error.value = null
  success.value = null
  try {
    const result: OpenAIQuotaResetResult = await resetOpenAIQuota(props.account.id)
    await query()
    success.value = t('admin.accounts.openaiQuotaReset.resetSuccess', { windows: result.windows_reset })
  } catch (value) {
    error.value = errorMessage(value)
  } finally {
    resetting.value = false
  }
}

watch(() => props.account.id, () => {
  data.value = null
  error.value = null
  success.value = null
  showResetConfirmation.value = false
})
</script>
