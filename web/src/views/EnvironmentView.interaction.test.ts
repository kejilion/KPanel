// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import EnvironmentView from './EnvironmentView.vue'
import { api } from '@/lib/api'
import type { WebEnvironmentJob, WebEnvironmentSummary } from '@/types/api'

vi.mock('@/lib/api', () => ({
  ApiError: class extends Error {},
  api: { webEnvironment: { summary: vi.fn(), jobs: vi.fn(), job: vi.fn(), start: vi.fn() } },
}))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ success: vi.fn(), danger: vi.fn() }) }))
vi.mock('vue-router', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-router')>(), useRoute: () => ({ query: { section: 'protection' } }) }))
vi.mock('@/components/apps/AppInteractiveTerminal.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/sites/SitesSectionTabs.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/i18n/phrase', () => ({ phraseCatalogVersion: { value: 0 }, translatePhrase: (value: string) => value, usePhraseCatalog: vi.fn() }))

const state: WebEnvironmentSummary = {
  protocolVersion: '1', state: 'installed', profile: 'full', health: 'healthy',
  webRoot: '/home/web', diskBytes: 1, siteCount: 1, databaseCount: 1, certificateCount: 1,
  composeValid: true, nginxValid: true, resourceVersion: `sha256:${'a'.repeat(64)}`,
  scriptVersion: 'test', latestBackup: '', portConflicts: [], components: [],
  protection: { fail2ban: true, fail2banInstalled: true, waf: false, cloudflare: false, ddos: false },
  optimization: { mode: 'custom', gzip: false, brotli: false, zstd: false }, observedAt: '2026-10-31T00:00:00Z',
}
const job: WebEnvironmentJob = {
  id: 'a'.repeat(32), action: 'backup.create', target: 'web', status: 'running', stage: 'running',
  progress: 25, message: 'running', createdAt: '2026-10-31T00:00:00Z',
}
let wrapper: VueWrapper | undefined
beforeEach(() => {
  window.localStorage.clear()
  vi.clearAllMocks()
  vi.useFakeTimers()
  vi.mocked(api.webEnvironment.summary).mockResolvedValue(structuredClone(state))
  vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [], total: 0 })
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers(); vi.restoreAllMocks() })
async function open() {
  wrapper = mount(EnvironmentView, { global: { stubs: {
    SitesSectionTabs: true, AppInteractiveTerminal: true, ModalDialog: true, RouterLink: true,
  } } })
  await flushPromises()
  return wrapper
}

describe('environment protection actions and background banner', () => {
  it('keeps installed protection controls available without an install/repair button', async () => {
    const view = await open()
    expect(view.text()).not.toContain('检查迁移并修复防护')
    expect(view.findAll('button').some(button => button.text() === '卸载')).toBe(true)
    expect(view.findAll('button').some(button => button.text() === '开启')).toBe(true)
    expect(api.webEnvironment.start).not.toHaveBeenCalled()
  })
  it('offers installation when protection is absent', async () => {
    vi.mocked(api.webEnvironment.summary).mockResolvedValue({ ...state, protection: { ...state.protection, fail2ban: false, fail2banInstalled: false } })
    const view = await open()
    expect(view.findAll('button').some(button => button.text() === '安装防护')).toBe(true)
  })
  it.each(['succeeded', 'failed', 'needs_attention'] as const)('does not restore an unrelated historical %s task', async (status) => {
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [{ ...job, status }], total: 1 })
    const view = await open()
    expect(view.find('.environment-job-banner').exists()).toBe(false)
  })

  it.each(['succeeded', 'failed', 'needs_attention'] as const)('restores the previously tracked %s result once and dismisses it without a history list', async (status) => {
    localStorage.setItem('kpanel:active-environment-job', job.id)
    const finished = { ...job, status, progress: 100 }
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [finished], total: 1 })
    const view = await open()
    expect(view.find('.environment-job-banner').exists()).toBe(true)
    expect(localStorage.getItem('kpanel:active-environment-job')).toBeNull()
    await view.get('.environment-job-banner button[aria-label="关闭"]').trigger('click')
    await view.findAll('button').find(button => button.text().includes('刷新状态'))!.trigger('click')
    await flushPromises()
    expect(view.find('.environment-job-banner').exists()).toBe(false)
    view.unmount()
    const reopened = await open()
    expect(reopened.find('.environment-job-banner').exists()).toBe(false)
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [{ ...job, id: 'b'.repeat(32) }, finished], total: 2 })
    await reopened.findAll('button').find(button => button.text().includes('刷新状态'))!.trigger('click')
    await flushPromises()
    expect(reopened.find('.environment-job-banner').exists()).toBe(true)
    expect(localStorage.getItem('kpanel:active-environment-job')).toBe('b'.repeat(32))
    expect(api.webEnvironment.start).not.toHaveBeenCalled()
  })

  it('restores running progress on reopen, keeps polling, and allows closing only after completion', async () => {
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [job], total: 1 })
    vi.mocked(api.webEnvironment.job).mockResolvedValue({ ...job, progress: 50 })
    const view = await open()
    expect(view.find('.environment-job-banner .icon-button').exists()).toBe(false)
    expect(localStorage.getItem('kpanel:active-environment-job')).toBe(job.id)
    view.unmount()
    const reopened = await open()
    expect(reopened.find('.environment-job-banner').exists()).toBe(true)
    await vi.advanceTimersByTimeAsync(1200)
    await flushPromises()
    expect(api.webEnvironment.job).toHaveBeenCalledWith(job.id, expect.any(AbortSignal))
    expect(reopened.get('.environment-job-banner').text()).toContain('50%')
    const failed = { ...job, status: 'failed' as const, progress: 100, message: '任务执行失败' }
    vi.mocked(api.webEnvironment.job).mockResolvedValue(failed)
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [failed], total: 1 })
    await vi.advanceTimersByTimeAsync(1200)
    await flushPromises()
    expect(reopened.get('.environment-job-banner').text()).toContain('任务执行失败')
    expect(localStorage.getItem('kpanel:active-environment-job')).toBeNull()
    await reopened.get('.environment-job-banner .icon-button').trigger('click')
    expect(reopened.find('.environment-job-banner').exists()).toBe(false)
  })

  it('keeps waiting-for-input tasks visible and tracked', async () => {
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [{ ...job, status: 'waiting_input' }], total: 1 })
    const view = await open()
    expect(view.get('.environment-job-banner').text()).toContain('等待输入')
    expect(view.find('.environment-job-banner .icon-button').exists()).toBe(false)
    expect(localStorage.getItem('kpanel:active-environment-job')).toBe(job.id)
  })

  it('tracks a newly started task and shows its immediate terminal result', async () => {
    vi.mocked(api.webEnvironment.start).mockResolvedValue({ ...job, status: 'failed', message: '任务执行失败' })
    const view = await open()
    await view.findAll('button').find(button => button.text() === '开启')!.trigger('click')
    await flushPromises()
    expect(api.webEnvironment.start).toHaveBeenCalledOnce()
    expect(view.get('.environment-job-banner').text()).toContain('任务执行失败')
    expect(localStorage.getItem('kpanel:active-environment-job')).toBeNull()
  })

  it('discovers running jobs and closes their results even when storage is blocked', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked') })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('quota') })
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => { throw new Error('blocked') })
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [job], total: 1 })
    const view = await open()
    expect(view.find('.environment-job-banner').exists()).toBe(true)
    const failed = { ...job, status: 'failed' as const }
    vi.mocked(api.webEnvironment.job).mockResolvedValue(failed)
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [failed], total: 1 })
    await vi.advanceTimersByTimeAsync(1200)
    await flushPromises()
    await view.get('.environment-job-banner .icon-button').trigger('click')
    expect(view.find('.environment-job-banner').exists()).toBe(false)
  })

  it.each(['invalid', 'b'.repeat(32)])('does not replace a missing tracked job with unrelated history (%s)', async (saved) => {
    localStorage.setItem('kpanel:active-environment-job', saved)
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [{ ...job, status: 'failed' }], total: 1 })
    const view = await open()
    expect(view.find('.environment-job-banner').exists()).toBe(false)
    expect(localStorage.getItem('kpanel:active-environment-job')).toBeNull()
  })
})
