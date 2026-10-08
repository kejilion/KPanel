// @vitest-environment jsdom
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BackupCenter from './BackupCenter.vue'
import { backups } from '@/lib/backup'

vi.mock('@/lib/backup', () => ({ backupMessage: (reason: unknown) => reason instanceof Error ? reason.message : '操作未完成，请刷新后重试。', backups: { settings: vi.fn(), files: vi.fn(), remoteImport: vi.fn(), upload: vi.fn(), list: vi.fn(), inventory: vi.fn(), export: vi.fn(), import: vi.fn(), preview: vi.fn(), restore: vi.fn(), recover: vi.fn(), delete: vi.fn(), download: (id: string) => `/api/v1/backups/${id}/download` } }))
const record = { id: 'a'.repeat(32), action: 'import' as const, status: 'ready', stage: 'ready', modules: ['panel'] as ['panel'], createdAt: '2026-09-12T01:00:00Z', size: 123, targetRevision: 'initial' }
let wrapper: ReturnType<typeof mount>
function render() {
  wrapper = mount(BackupCenter, { global: { stubs: { ModalDialog: { props: ['open', 'title'], template: '<div v-if="open" role="dialog"><h3>{{ title }}</h3><slot /></div>' } } } })
}
const submit = () => wrapper.get('button[type="submit"]')
beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(backups.settings).mockResolvedValue({ revision: 'r', storages: [{ id: 'storage', name: 'NAS', kind: 'webdav', endpoint: 'https://nas.example', prefix: 'backup' }], schedule: { enabled: false, modules: ['panel'], storageId: '', frequency: 'daily', hour: 3, minute: 0, weekday: 0, day: 1, timezone: 'UTC', keep: 7 } })
  vi.mocked(backups.list).mockResolvedValue({ items: [], maxBytes: 50 * 2 ** 30 })
  vi.mocked(backups.inventory).mockResolvedValue({ revision: 'x', panelBytes: 128, maxBytes: 50 * 2 ** 30, hostAvailable: true, host: { revision: 'host', modules: [{ id: 'apps', bytes: 200, containers: 1, requires: ['docker'] }, { id: 'web', bytes: 300, containers: 1, requires: [] }, { id: 'docker', bytes: 400, containers: 1, requires: [] }] } })
})
afterEach(() => wrapper?.unmount())

describe('backup center user flow', () => {
  it('shows a missed window while retaining the last successful receipt', async () => {
    const value = await backups.settings()
    vi.mocked(backups.settings).mockResolvedValue({ ...value, schedule: { ...value.schedule, enabled: true, nextRun: '2026-10-09T03:00:00Z' }, health: { state: 'missed', lastSuccessAt: '2026-10-07T03:02:00Z', errorCode: 'schedule_missed' } })
    render(); await flushPromises()
    expect(wrapper.text()).toContain('自动备份错过执行窗口')
    expect(wrapper.text()).toContain('最近成功的自动备份（开始时间）：')
    expect(wrapper.text()).toContain('下次自动备份：')
    expect(wrapper.text()).not.toContain('暂无成功记录')
    expect(wrapper.text()).not.toContain('自动备份最近一次已完成')
  })
  it('does not show a stale healthy status after a settings refresh fails', async () => {
    const value = await backups.settings()
    vi.mocked(backups.settings).mockResolvedValueOnce({ ...value, health: { state: 'healthy', lastSuccessAt: '2026-10-07T03:02:00Z' } }).mockRejectedValueOnce(new Error('unavailable'))
    render(); await flushPromises()
    expect(wrapper.text()).toContain('自动备份最近一次已完成')
    await wrapper.get('.backup-refresh').trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('自动备份状态暂不可用')
    expect(wrapper.text()).not.toContain('自动备份最近一次已完成')
    expect(wrapper.text()).not.toContain('最近成功的自动备份（开始时间）：')
  })
  it('shows expiry instead of offering a missing local fallback', async () => {
    vi.mocked(backups.list).mockResolvedValue({ items: [{ ...record, action: 'export', status: 'expired', localReady: false, errorCode: 'remote_upload_failed' }], maxBytes: 1000 })
    render(); await flushPromises()
    expect(wrapper.text()).toContain('备份文件已过期')
    expect(wrapper.text()).not.toContain('本地备份可下载')
    expect(wrapper.find('a[download]').exists()).toBe(false)
  })
  it('fetches a remote package for inspection without restoring it', async () => {
    vi.mocked(backups.files).mockResolvedValue({ items: [{ key: 'archive.kpb', size: 128, modified: '2026-09-29T00:00:00Z' }] })
    render(); await flushPromises()
    await wrapper.findAll('.backup-command__actions button')[1]!.trigger('click')
    await wrapper.get('select').setValue('storage'); await flushPromises()
    await wrapper.findAll('select')[1]!.setValue('archive.kpb')
    await wrapper.get('input[type="password"]').setValue('long-password')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(backups.remoteImport).toHaveBeenCalledWith('storage', 'archive.kpb', 'long-password')
    expect(backups.restore).not.toHaveBeenCalled()
    expect(backups.import).not.toHaveBeenCalled()
  })

  it('retains the local download and retries the existing package after upload failure', async () => {
    vi.mocked(backups.list).mockResolvedValue({ items: [{ ...record, action: 'export', status: 'failed', localReady: true, errorCode: 'remote_upload_failed', remote: { storageId: 'storage', storageName: 'NAS', key: 'archive.kpb', status: 'failed' } }], maxBytes: 1000 })
    render(); await flushPromises()
    expect(wrapper.find('a[download]').exists()).toBe(true)
    await wrapper.get('.backup-record-status button').trigger('click'); await flushPromises()
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(backups.upload).toHaveBeenCalledWith(record.id, 'storage')
    expect(backups.export).not.toHaveBeenCalled()
  })

  it('does not submit a remote import after a failed directory listing', async () => {
    vi.mocked(backups.files).mockRejectedValue(new Error('offline'))
    render(); await flushPromises()
    await wrapper.findAll('.backup-command__actions button')[1]!.trigger('click')
    await wrapper.get('select').setValue('storage'); await flushPromises()
    await wrapper.get('input[type="password"]').setValue('long-password')
    expect(submit().attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit')
    expect(backups.remoteImport).not.toHaveBeenCalled()
  })

  it('uses the shared settings hierarchy and groups record controls', async () => {
    render(); await flushPromises()

    expect(wrapper.get('.backup-center').classes()).toContain('settings-section')
    expect(wrapper.get('.backup-center > header').classes()).toContain('settings-section__header')
    expect(wrapper.get('.backup-command__actions').findAll('button')[1]?.classes()).toContain('button--secondary')
    expect(wrapper.get('.backup-history__header button').classes()).toContain('button--secondary')
    expect(wrapper.get('.backup-empty').attributes('role')).toBe('status')
  })

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
    expect(backups.export).toHaveBeenCalledWith(['panel', 'apps', 'web', 'docker'], 'long-password', 'host', '')
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
    expect(wrapper.get('a[download]').classes()).toContain('button--secondary')
    expect(wrapper.get('.button--danger-text').classes()).toContain('button--ghost')
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
