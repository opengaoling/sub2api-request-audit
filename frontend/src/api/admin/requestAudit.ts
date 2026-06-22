import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export interface RequestAuditLog {
  id: number
  request_id?: string
  user_id: number
  user_email?: string
  api_key_id: number
  account_id?: number
  group_id?: number
  platform: string
  endpoint?: string
  model?: string
  stream: boolean
  status_code?: number
  duration_ms?: number
  request_body?: string
  response_body?: string
  request_body_truncated: boolean
  response_body_truncated: boolean
  request_body_bytes: number
  response_body_bytes: number
  is_mocked: boolean
  mock_rule_id?: number
  error_message?: string
  created_at: string
}

export interface RequestAuditQueryParams {
  page?: number
  page_size?: number
  user_id?: number | string
  api_key_id?: number | string
  account_id?: number | string
  group_id?: number | string
  platform?: string
  model?: string
  request_id?: string
  q?: string
  start_date?: string
  end_date?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export async function listRequestAuditLogs(params: RequestAuditQueryParams) {
  const { data } = await apiClient.get<PaginatedResponse<RequestAuditLog>>('/admin/request-audit-logs', { params })
  return data
}

export async function getRequestAuditLog(id: number) {
  const { data } = await apiClient.get<RequestAuditLog>(`/admin/request-audit-logs/${id}`)
  return data
}

export async function cleanupRequestAuditLogs(olderThanHours: number) {
  const { data } = await apiClient.post<{ deleted: number }>('/admin/request-audit-logs/cleanup', null, {
    params: { older_than_hours: olderThanHours },
  })
  return data
}

export default {
  listRequestAuditLogs,
  getRequestAuditLog,
  cleanupRequestAuditLogs,
}
