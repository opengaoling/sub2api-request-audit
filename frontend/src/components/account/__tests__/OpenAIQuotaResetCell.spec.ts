import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import OpenAIQuotaResetCell from '../OpenAIQuotaResetCell.vue'
import type { Account } from '@/types'

const { queryOpenAIQuota, resetOpenAIQuota } = vi.hoisted(() => ({
  queryOpenAIQuota: vi.fn(),
  resetOpenAIQuota: vi.fn()
}))

vi.mock('@/api/admin/accounts', () => ({
  queryOpenAIQuota,
  resetOpenAIQuota
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const ConfirmDialogStub = {
  props: ['show', 'title', 'message', 'confirmText', 'cancelText', 'danger'],
  emits: ['confirm', 'cancel'],
  template: `
    <div v-if="show" data-test="quota-reset-confirmation">
      <button data-test="confirm-reset" @click="$emit('confirm')">{{ confirmText }}</button>
      <button data-test="cancel-reset" @click="$emit('cancel')">{{ cancelText }}</button>
    </div>
  `
}

const account = {
  id: 42,
  platform: 'openai',
  type: 'oauth'
} as Account

function mountCell(accountToMount: Account = account) {
  return mount(OpenAIQuotaResetCell, {
    props: { account: accountToMount },
    global: {
      stubs: {
        ConfirmDialog: ConfirmDialogStub
      }
    }
  })
}

describe('OpenAIQuotaResetCell', () => {
  beforeEach(() => {
    queryOpenAIQuota.mockReset()
    resetOpenAIQuota.mockReset()
    queryOpenAIQuota.mockResolvedValue({
      rate_limit_reset_credits: {
        available_count: 1
      }
    })
    resetOpenAIQuota.mockResolvedValue({ windows_reset: 1 })
  })

  it('uses the persisted reset count before querying quota', () => {
    const wrapper = mountCell({
      ...account,
      extra: {
        codex_reset_credit_available_count: 3
      }
    } as Account)

    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.count')
    expect(wrapper.text()).toContain('3')
    expect(wrapper.findAll('button')[1].attributes('disabled')).toBeUndefined()
    expect(queryOpenAIQuota).not.toHaveBeenCalled()
  })

  it('requires confirmation before resetting quota', async () => {
    const wrapper = mountCell()

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    const resetButton = wrapper.findAll('button')[1]
    await resetButton.trigger('click')

    expect(wrapper.find('[data-test="quota-reset-confirmation"]').exists()).toBe(true)
    expect(resetOpenAIQuota).not.toHaveBeenCalled()

    await wrapper.find('[data-test="cancel-reset"]').trigger('click')
    expect(wrapper.find('[data-test="quota-reset-confirmation"]').exists()).toBe(false)
    expect(resetOpenAIQuota).not.toHaveBeenCalled()

    await resetButton.trigger('click')
    await wrapper.find('[data-test="confirm-reset"]').trigger('click')
    await flushPromises()

    expect(resetOpenAIQuota).toHaveBeenCalledWith(account.id)
  })
})
