// @vitest-environment jsdom
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BackupCenter from './BackupCenter.vue'
import { backups } from '@/lib/backup'

vi.mock('@/lib/backup', () => ({ backups: { list: vi.fn(), inventory: vi.fn(), export: vi.fn(), import: vi.fn(), preview: vi.fn(), restore: vi.fn(), recover: vi.fn(), delete: vi.fn(), download: (id: string) => `/api/v1/backups/${id}/download` } }))
const record = { id: 'a'.repeat(32), action: 'import' as const, status: 'ready', stage: 'ready', modules: ['panel'] as ['panel'], createdAt: '2026-09-12T01:00:00Z', size: 123, targetRevision: 'initial' }
let wrapper: ReturnType<typeof mount>
function render() {
  wrapper = mount(BackupCenter, { global: { stubs: { ModalDialog: { props: ['open', 'title'], template: '<div v-if="open" role="dialog"><h3>{{ title }}</h3><slot /></div>' } } } })
}
const submit = () => wrapper.get('button[type="submit"]')
beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(backups.list).mockResolvedValue({ items: [], maxBytes: 50 * 2 ** 30 })
  vi.mocked(backups.inventory).mockResolvedValue({ revision: 'x', panelBytes: 128, maxBytes: 50 * 2 ** 30, hostAvailable: true, host: { revision: 'host', modules: [{ id: 'apps', bytes: 200, containers: 1, requires: ['docker'] }, { id: 'web', bytes: 300, containers: 1, requires: [] }, { id: 'docker', bytes: 400, containers: 1, requires: [] }] } })
})
afterEach(() => wrapper?.unmount())

describe('backup center user flow', () => {
  it('requires the category that shares selected data', async () => {
    render(); await flushPromises()
    await wrapper.get('.backup-actions button').trigger('click'); await flushPromises()
    const passwords = wrapper.findAll('input[type="password"]')
    await passwords[0]!.setValue('long-password'); await passwords[1]!.setValue('long-password')
    expect(submit().attributes('disabled')).toBeUndefined()
    await wrapper.get('input[value="docker"]').setValue(false)
    expect(submit().attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit')
    expect(backups.export).not.toHaveBeenCalled()
    await wrapper.get('input[value="docker"]').setValue(true)
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(backups.export).toHaveBeenCalledWith(['panel', 'apps', 'web', 'docker'], 'long-password', 'host')
  })

  it('never restores after a failed preview', async () => {
    vi.mocked(backups.list).mockResolvedValue({ items: [record], maxBytes: 50 * 2 ** 30 })
    vi.mocked(backups.preview).mockRejectedValue(new Error('preview unavailable'))
    render(); await flushPromises()
    await wrapper.get('.backup-record-status button').trigger('click'); await flushPromises()
    expect(submit().attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit')
    expect(backups.restore).not.toHaveBeenCalled()
  })

  it('uses the refreshed revision only after confirmation', async () => {
    vi.mocked(backups.list).mockResolvedValue({ items: [record], maxBytes: 50 * 2 ** 30 })
    vi.mocked(backups.preview).mockResolvedValue({ ...record, targetRevision: 'fresh' })
    render(); await flushPromises()
    await wrapper.get('.backup-record-status button').trigger('click'); await flushPromises()
    expect(backups.restore).not.toHaveBeenCalled()
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(backups.restore).toHaveBeenCalledWith(expect.objectContaining({ targetRevision: 'fresh' }), ['panel'])
  })

  it('downloads by streaming through the authenticated URL', async () => {
    vi.mocked(backups.list).mockResolvedValue({ items: [{ ...record, action: 'export', status: 'completed' }], maxBytes: 50 * 2 ** 30 })
    render(); await flushPromises()
    expect(wrapper.get('a[download]').attributes('href')).toBe('/api/v1/backups/' + record.id + '/download')
  })

  it('resolves interruption only through recovery after confirmation', async () => {
    vi.mocked(backups.list).mockResolvedValue({ items: [{ ...record, action: 'restore', status: 'failed', errorCode: 'cleanup_pending', completedModules: ['panel'] }], maxBytes: 50 * 2 ** 30 })
    render(); await flushPromises()
    await wrapper.get('.backup-record-status button').trigger('click'); await flushPromises()
    expect(backups.recover).not.toHaveBeenCalled()
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(backups.recover).toHaveBeenCalledWith(record.id)
    expect(backups.restore).not.toHaveBeenCalled()
    expect(backups.import).not.toHaveBeenCalled()
  })
})
