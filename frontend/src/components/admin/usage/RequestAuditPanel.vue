<template>
  <div class="space-y-4">
    <div class="card p-4">
      <div class="flex flex-wrap items-end justify-between gap-3">
        <div class="w-full sm:w-56">
          <label class="input-label">清理范围</label>
          <select v-model.number="cleanupOlderThanHours" class="input">
            <option :value="1">1 小时前</option>
            <option :value="6">6 小时前</option>
            <option :value="24">24 小时前</option>
            <option :value="24 * 7">7 天前</option>
            <option :value="24 * 30">30 天前</option>
          </select>
        </div>
        <button type="button" class="btn btn-danger" :disabled="cleanupLoading" @click="cleanupLogs">
          {{ cleanupLoading ? '清理中...' : '清理请求审计' }}
        </button>
      </div>
    </div>

    <div class="card overflow-hidden">
      <div v-if="loading" class="p-8 text-center text-gray-500 dark:text-gray-400">加载中...</div>
      <div v-else-if="logs.length === 0" class="p-8 text-center text-gray-500 dark:text-gray-400">暂无请求审计记录</div>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-800">
            <tr>
              <th class="th">时间</th>
              <th class="th">平台</th>
              <th class="th">模型</th>
              <th class="th">用户 / API Key</th>
              <th class="th">账号</th>
              <th class="th">状态</th>
              <th class="th">请求</th>
              <th class="th">响应</th>
              <th class="th">耗时</th>
              <th class="th">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="item in logs" :key="item.id" class="hover:bg-gray-50 dark:hover:bg-dark-800/60">
              <td class="td whitespace-nowrap">{{ formatTime(item.created_at) }}</td>
              <td class="td">{{ item.platform }}</td>
              <td class="td max-w-[220px] truncate" :title="item.model">{{ item.model || '-' }}</td>
              <td class="td whitespace-nowrap">
                <span :title="item.user_email || undefined">{{ item.user_email || '-' }}</span>
                <span class="text-gray-400"> / #{{ item.api_key_id }}</span>
              </td>
              <td class="td whitespace-nowrap">{{ item.account_id ? `#${item.account_id}` : '-' }}</td>
              <td class="td whitespace-nowrap">
                <span :class="statusClass(item.status_code)">{{ item.status_code || '-' }}</span>
              </td>
              <td class="td whitespace-nowrap">{{ formatBytes(item.request_body_bytes) }}<span v-if="item.request_body_truncated" class="ml-1 text-amber-600">截断</span></td>
              <td class="td whitespace-nowrap">{{ formatBytes(item.response_body_bytes) }}<span v-if="item.response_body_truncated" class="ml-1 text-amber-600">截断</span></td>
              <td class="td whitespace-nowrap">{{ item.duration_ms ?? '-' }}ms</td>
              <td class="td whitespace-nowrap">
                <button class="text-primary-600 hover:underline dark:text-primary-400" @click="openDetail(item.id)">查看内容</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <Pagination
      v-if="pagination.total > 0"
      :page="pagination.page"
      :total="pagination.total"
      :page-size="pagination.page_size"
      @update:page="handlePageChange"
      @update:pageSize="handlePageSizeChange"
    />

    <div v-if="detail" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click.self="detail = null">
      <div class="max-h-[90vh] w-full max-w-6xl overflow-hidden rounded-xl bg-white shadow-xl dark:bg-dark-900">
        <div class="flex items-center justify-between border-b border-gray-200 p-4 dark:border-dark-700">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">请求详情 #{{ detail.id }}</h2>
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ detail.request_id || '无 request_id' }}</p>
          </div>
          <button class="text-gray-500 hover:text-gray-900 dark:hover:text-white" @click="detail = null">✕</button>
        </div>
        <div class="max-h-[75vh] overflow-y-auto p-4">
          <div class="mb-4 grid gap-3 text-sm text-gray-700 dark:text-gray-300 md:grid-cols-3">
            <div>平台：{{ detail.platform }}</div>
            <div>模型：{{ detail.model || '-' }}</div>
            <div>Endpoint：{{ detail.endpoint || '-' }}</div>
            <div>用户：<span :title="detail.user_email || undefined">{{ detail.user_email || '-' }}</span></div>
            <div>API Key：#{{ detail.api_key_id }}</div>
            <div>账号：{{ detail.account_id ? `#${detail.account_id}` : '-' }}</div>
            <div>状态：{{ detail.status_code || '-' }}</div>
            <div>耗时：{{ detail.duration_ms ?? '-' }}ms</div>
            <div>流式：{{ detail.stream ? '是' : '否' }}</div>
          </div>
          <div class="grid gap-4 lg:grid-cols-2">
            <section>
              <div class="mb-2 flex items-center justify-between">
                <h3 class="font-medium text-gray-900 dark:text-white">Request Body</h3>
                <span v-if="detail.request_body_truncated" class="text-sm text-amber-600">已截断</span>
              </div>
              <pre class="code-block">{{ pretty(detail.request_body) }}</pre>
            </section>
            <section>
              <div class="mb-2 flex items-center justify-between">
                <h3 class="font-medium text-gray-900 dark:text-white">Response Body</h3>
                <span v-if="detail.response_body_truncated" class="text-sm text-amber-600">已截断</span>
              </div>
              <pre class="code-block">{{ pretty(detail.response_body) }}</pre>
            </section>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import Pagination from '@/components/common/Pagination.vue'
import requestAuditAPI, { type RequestAuditLog, type RequestAuditQueryParams } from '@/api/admin/requestAudit'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import type { AdminUsageQueryParams } from '@/api/admin/usage'

const props = defineProps<{
  startDate: string
  endDate: string
  filters: AdminUsageQueryParams
}>()

const loading = ref(false)
const cleanupLoading = ref(false)
const cleanupOlderThanHours = ref(24)
const logs = ref<RequestAuditLog[]>([])
const detail = ref<RequestAuditLog | null>(null)
const pagination = reactive({ page: 1, page_size: getPersistedPageSize(20), total: 0 })

watch(() => [props.startDate, props.endDate, props.filters], () => {
  applyFilters()
}, { deep: true })

async function loadData() {
  loading.value = true
  try {
    const params: RequestAuditQueryParams = {
      user_id: props.filters.user_id,
      api_key_id: props.filters.api_key_id,
      account_id: props.filters.account_id,
      group_id: props.filters.group_id,
      model: props.filters.model || undefined,
      start_date: props.startDate,
      end_date: props.endDate,
      page: pagination.page,
      page_size: pagination.page_size,
      sort_by: 'created_at',
      sort_order: 'desc',
    }
    const res = await requestAuditAPI.listRequestAuditLogs(params)
    logs.value = res.items || []
    pagination.total = res.total || 0
    pagination.page = res.page || pagination.page
    pagination.page_size = res.page_size || pagination.page_size
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  pagination.page = 1
  loadData()
}
function handlePageChange(page: number) {
  pagination.page = page
  loadData()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  loadData()
}

async function openDetail(id: number) {
  detail.value = await requestAuditAPI.getRequestAuditLog(id)
}

async function cleanupLogs() {
  if (!confirm(`确认清理 ${cleanupOlderThanHours.value} 小时前的请求审计日志？`)) return
  cleanupLoading.value = true
  try {
    const result = await requestAuditAPI.cleanupRequestAuditLogs(cleanupOlderThanHours.value)
    alert(`已清理 ${result.deleted || 0} 条请求审计日志`)
    pagination.page = 1
    await loadData()
  } finally {
    cleanupLoading.value = false
  }
}

function pretty(value?: string) {
  if (!value) return ''
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

function formatTime(value: string) {
  return new Date(value).toLocaleString()
}

function formatBytes(value: number) {
  if (!value) return '0 B'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}

function statusClass(status?: number) {
  if (!status) return 'text-gray-500'
  if (status >= 200 && status < 300) return 'text-green-600 dark:text-green-400'
  if (status >= 400) return 'text-red-600 dark:text-red-400'
  return 'text-gray-700 dark:text-gray-300'
}

onMounted(loadData)

defineExpose({
  refreshData: loadData,
})
</script>

<style scoped>
.th { @apply px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400; }
.td { @apply px-4 py-3 text-gray-700 dark:text-gray-200; }
.code-block { @apply max-h-[52vh] overflow-auto rounded-lg bg-gray-950 p-3 text-xs text-gray-100; }
</style>
