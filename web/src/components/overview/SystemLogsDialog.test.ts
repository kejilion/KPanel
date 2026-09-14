// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import englishSharedCatalog from '@/i18n/pages/shared/en-US'
import { registerPhraseCatalog } from '@/i18n/phrase'
import SystemLogsDialog from './SystemLogsDialog.vue'

let unregisterCatalog: (() => void) | undefined

const mocks = vi.hoisted(() => ({
  logsSummary: vi.fn(),
  logs: vi.fn(),
  action: vi.fn(),
  maintenance: vi.fn(),
  success: vi.fn(),
  danger: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {},
  api: {
    system: {
      logsSummary: mocks.logsSummary,
      logs: mocks.logs,
      action: mocks.action,
      maintenance: mocks.maintenance,
    },
  },
}))

vi.mock('@/stores/toast', () => ({
  useToast: () => ({ success: mocks.success, danger: mocks.danger }),
}))

const idleMaintenance = {
  state: 'idle' as const,
  progress: 0,
  rebootRequired: false,
}

const summary = {
  observedAt: '2026-08-24T08:00:00Z',
  varLog: { available: true, bytes: 3_145_728 },
  journal: { available: true, bytes: 1_048_576 },
  sources: {
    system: { available: true, supportsPriority: true },
    journal: { available: true },
    security: { available: true },
    login: { available: true },
  },
  authSource: '/var/log/auth.log',
  maintenance: idleMaintenance,
}

const systemEntries = {
  source: 'system' as const,
  entries: [
    { timestamp: '2026-08-24T07:59:00Z', priority: 'info', identifier: 'kernel', message: 'older boot event' },
    { timestamp: '2026-08-24T08:00:00Z', priority: 'warning', unit: 'nginx.service', pid: 42, message: 'newer nginx event' },
  ],
  truncated: false,
  observedAt: '2026-08-24T08:00:00Z',
}

function mountDialog(open = true, readable = true) {
  return mount(SystemLogsDialog, {
    props: { open, readable, writable: true, unavailableReason: '日志适配器不可用' },
    global: { stubs: { teleport: true } },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.logsSummary.mockResolvedValue(summary)
  mocks.logs.mockResolvedValue(systemEntries)
  mocks.action.mockResolvedValue({
    action: 'log-cleanup',
    status: 'accepted',
    changed: true,
    taskId: 'cleanup-task-a',
    maintenancePolicy: 'retain-7d',
    message: 'queued',
    appliedAt: '2026-08-24T08:01:00Z',
  })
  mocks.maintenance.mockResolvedValue(idleMaintenance)
  vi.stubGlobal('confirm', vi.fn(() => true))
})

afterEach(() => {
  unregisterCatalog?.()
  unregisterCatalog = undefined
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('SystemLogsDialog', () => {
  it('loads the summary and bounded entries only after the readable dialog opens', async () => {
    const wrapper = mountDialog(false)
    await flushPromises()
    expect(mocks.logsSummary).not.toHaveBeenCalled()
    expect(mocks.logs).not.toHaveBeenCalled()

    await wrapper.setProps({ open: true })
    await flushPromises()

    expect(mocks.logsSummary).toHaveBeenCalledWith(expect.any(AbortSignal))
    expect(mocks.logs).toHaveBeenCalledWith({
      source: 'system',
      limit: 100,
      priority: 'all',
    }, expect.any(AbortSignal))
    const output = wrapper.find('pre.system-log-output').text()
    expect(output.indexOf('older boot event')).toBeLessThan(output.indexOf('newer nginx event'))
    expect(wrapper.text()).toContain('/var/log 总占用')
    expect(wrapper.text()).toContain('两项不能相加')
    wrapper.unmount()
  })

  it('uses fixed syslog on OpenRC without exposing unsupported priority filters', async () => {
    mocks.logsSummary.mockResolvedValueOnce({
      ...summary,
      journal: { available: false, reason: 'OpenRC 主机不使用 systemd journal' },
      sources: {
        ...summary.sources,
        system: { available: true },
        journal: { available: false, reason: 'OpenRC 主机不使用 systemd journal' },
      },
      authSource: '/var/log/messages',
    })
    const wrapper = mountDialog()
    await flushPromises()

    expect(mocks.logs).toHaveBeenCalledWith({
      source: 'system',
      limit: 100,
      priority: 'all',
    }, expect.any(AbortSignal))
    expect(wrapper.find('.system-log-priority').exists()).toBe(false)
    expect(wrapper.find('pre.system-log-output').exists()).toBe(true)
    wrapper.unmount()
  })

  it('reads all service logs and lets users search them directly without choosing a unit', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.findAll('.system-log-source-switch button')[1]!.trigger('click')
    await flushPromises()
    expect(mocks.logs).toHaveBeenLastCalledWith({
      source: 'service',
      limit: 100,
      priority: 'all',
    }, expect.any(AbortSignal))

    expect(wrapper.find('.system-log-query-bar').text()).toContain('搜索服务日志')
    expect(wrapper.find('.system-log-query-bar').text()).not.toContain('系统服务')
    expect(wrapper.find('input[type="search"]').attributes('placeholder')).toBe('输入服务名或日志关键字')
    await wrapper.findAll('.system-log-priority button')[1]!.trigger('click')
    const limitSelect = wrapper.findAll('.system-log-control')
      .find((control) => control.text().includes('读取行数'))!
      .find('select')
    await limitSelect.setValue('50')
    await flushPromises()
    expect(mocks.logs).toHaveBeenLastCalledWith({
      source: 'service',
      limit: 50,
      priority: 'warning',
    }, expect.any(AbortSignal))

    const callsBeforeSearch = mocks.logs.mock.calls.length
    await wrapper.find('input[type="search"]').setValue('nginx')
    expect(wrapper.find('pre.system-log-output').text()).toContain('newer nginx event')
    expect(wrapper.find('pre.system-log-output').text()).not.toContain('older boot event')
    expect(mocks.logs).toHaveBeenCalledTimes(callsBeforeSearch)
    wrapper.unmount()
  })

  it('uses structured priority colors and highlights literal search matches', async () => {
    mocks.logs.mockResolvedValueOnce({
      ...systemEntries,
      entries: [
        ...systemEntries.entries,
        { timestamp: '2026-08-24T08:01:00Z', priority: 'error', unit: 'docker.service', message: 'container start failed' },
      ],
    })
    const wrapper = mountDialog()
    await flushPromises()

    const levels = wrapper.findAll('.system-log-level')
    expect(levels[0]!.classes()).toContain('is-neutral')
    expect(levels[1]!.classes()).toContain('is-warning')
    expect(levels[2]!.classes()).toContain('is-danger')
    expect(wrapper.find('.system-log-time').text()).toBe('2026-08-24T07:59:00Z')
    expect(wrapper.findAll('.system-log-identity')[1]!.text()).toBe('nginx.service[42]')

    await wrapper.find('input[type="search"]').setValue('.')
    const matches = wrapper.findAll('.system-log-highlight')
    expect(matches).toHaveLength(2)
    expect(matches.every((match) => match.text() === '.')).toBe(true)
    expect(wrapper.find('pre.system-log-output').text()).toContain('newer nginx event')
    wrapper.unmount()
  })

  it('serializes summary and entry reads through the shared Agent gate', async () => {
    let resolveFirst: ((value: typeof systemEntries) => void) | undefined
    mocks.logs
      .mockImplementationOnce(() => new Promise<typeof systemEntries>((resolve) => {
        resolveFirst = resolve
      }))
      .mockResolvedValue(systemEntries)
    const wrapper = mountDialog()
    await flushPromises()
    expect(mocks.logs).toHaveBeenCalledTimes(1)

    await wrapper.findAll('.system-log-source-switch button')[1]!.trigger('click')
    await Promise.resolve()
    expect(mocks.logs).toHaveBeenCalledTimes(1)
    expect(wrapper.find('button[title="刷新日志概览和当前日志"]').attributes('disabled')).toBeDefined()

    resolveFirst?.(systemEntries)
    await flushPromises()
    expect(mocks.logs).toHaveBeenCalledTimes(2)
    expect(mocks.logs).toHaveBeenLastCalledWith({
      source: 'service',
      limit: 100,
      priority: 'all',
    }, expect.any(AbortSignal))
    wrapper.unmount()
  })

  it('runs one three-second live request at a time and stops immediately when closed', async () => {
    vi.useFakeTimers()
    const wrapper = mountDialog()
    await flushPromises()

    let resolveLive: ((value: typeof systemEntries) => void) | undefined
    mocks.logs.mockImplementationOnce(() => new Promise<typeof systemEntries>((resolve) => {
      resolveLive = resolve
    }))
    await wrapper.find('.system-log-realtime').trigger('click')
    vi.advanceTimersByTime(3_000)
    await Promise.resolve()
    expect(mocks.logs).toHaveBeenCalledTimes(2)

    vi.advanceTimersByTime(9_000)
    await Promise.resolve()
    expect(mocks.logs).toHaveBeenCalledTimes(2)

    await wrapper.setProps({ open: false })
    resolveLive?.(systemEntries)
    await Promise.resolve()
    vi.advanceTimersByTime(9_000)
    await Promise.resolve()
    expect(mocks.logs).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('ignores observedAt-only refreshes and signals only when the latest entry changes', async () => {
    vi.useFakeTimers()
    const wrapper = mountDialog()
    await flushPromises()

    let resolveSame: ((value: typeof systemEntries) => void) | undefined
    mocks.logs.mockImplementationOnce(() => new Promise<typeof systemEntries>((resolve) => {
      resolveSame = resolve
    }))
    await wrapper.find('.system-log-realtime').trigger('click')
    vi.advanceTimersByTime(3_000)
    await Promise.resolve()
    ;(wrapper.vm as unknown as { followLatest: boolean }).followLatest = false
    resolveSame?.({ ...systemEntries, observedAt: '2026-08-24T08:00:03Z' })
    await flushPromises()
    expect(wrapper.find('.system-log-latest').exists()).toBe(false)

    let resolveChanged: ((value: typeof systemEntries) => void) | undefined
    mocks.logs.mockImplementationOnce(() => new Promise<typeof systemEntries>((resolve) => {
      resolveChanged = resolve
    }))
    vi.runOnlyPendingTimers()
    await Promise.resolve()
    expect(mocks.logs).toHaveBeenCalledTimes(3)
    expect(resolveChanged).toBeDefined()
    resolveChanged?.({
      ...systemEntries,
      observedAt: '2026-08-24T08:00:06Z',
      entries: [
        systemEntries.entries[0]!,
        { ...systemEntries.entries[1]!, message: 'latest entry changed' },
      ],
    })
    await flushPromises()
    expect(wrapper.find('pre.system-log-output').text()).toContain('latest entry changed')
    expect(wrapper.find('.system-log-latest').exists()).toBe(true)
    wrapper.unmount()
  })

  it('submits a fixed cleanup policy, polls its own maintenance task, and refreshes facts', async () => {
    mocks.logsSummary
      .mockResolvedValueOnce(summary)
      .mockResolvedValueOnce({
        ...summary,
        maintenance: {
          id: 'cleanup-task-a',
          state: 'succeeded',
          action: 'log-cleanup',
          policy: 'retain-7d',
          progress: 100,
          message: 'released space',
          rebootRequired: false,
        },
      })
    const wrapper = mountDialog()
    await flushPromises()
    vi.useFakeTimers()
    mocks.maintenance
      .mockResolvedValueOnce({
        id: 'cleanup-task-a',
        state: 'running',
        action: 'log-cleanup',
        policy: 'retain-7d',
        progress: 40,
        message: 'vacuuming',
        rebootRequired: false,
      })
      .mockResolvedValueOnce({
        id: 'cleanup-task-a',
        state: 'succeeded',
        action: 'log-cleanup',
        policy: 'retain-7d',
        progress: 100,
        message: 'released space',
        rebootRequired: false,
      })

    await wrapper.find('.system-log-realtime').trigger('click')
    const cleanup = wrapper.findAll('button').find((button) => button.text().includes('确认清理旧系统日志'))!
    await cleanup.trigger('click')
    await Promise.resolve()
    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining('保留最近 7 天'))
    expect(mocks.action).toHaveBeenCalledWith({
      action: 'log-cleanup',
      maintenancePolicy: 'retain-7d',
    })

    await vi.advanceTimersByTimeAsync(800)
    expect(mocks.maintenance).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(2_000)
    expect(mocks.maintenance).toHaveBeenCalledTimes(2)
    expect(mocks.success).toHaveBeenCalledWith('日志清理已完成', 'released space')
    expect(mocks.logsSummary).toHaveBeenCalledTimes(2)
    expect(mocks.logs).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(6_000)
    expect(mocks.logs).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('stops polling when another tab replaces the submitted cleanup task', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    vi.useFakeTimers()
    mocks.action.mockResolvedValueOnce({
      action: 'log-cleanup',
      status: 'accepted',
      changed: true,
      taskId: 'cleanup-task-a',
      message: 'queued A',
      appliedAt: '2026-08-24T08:01:00Z',
    })
    mocks.maintenance.mockResolvedValueOnce({
      id: 'cleanup-task-b',
      state: 'succeeded',
      action: 'log-cleanup',
      policy: 'retain-7d',
      progress: 100,
      message: 'B completed',
      rebootRequired: false,
    })

    const cleanup = wrapper.findAll('button').find((button) => button.text().includes('确认清理旧系统日志'))!
    await cleanup.trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(800)

    expect(wrapper.text()).toContain('日志清理任务身份已变化，请刷新确认真实状态。')
    expect(mocks.success).not.toHaveBeenCalledWith('日志清理已完成', expect.anything())
    expect(mocks.maintenance).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(8_000)
    expect(mocks.maintenance).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('fails closed when the cleanup action response omits taskId', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    vi.useFakeTimers()
    mocks.action.mockResolvedValueOnce({
      action: 'log-cleanup',
      status: 'accepted',
      changed: true,
      maintenancePolicy: 'retain-7d',
      message: 'possibly queued',
      appliedAt: '2026-08-24T08:01:00Z',
    })

    const cleanup = wrapper.findAll('button').find((button) => button.text().includes('确认清理旧系统日志'))!
    await cleanup.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Agent 未返回日志清理任务身份；任务可能已提交，请刷新确认真实状态。')
    expect(mocks.danger).toHaveBeenCalledWith(
      '日志清理任务身份缺失',
      'Agent 未返回日志清理任务身份；任务可能已提交，请刷新确认真实状态。',
    )
    expect(mocks.success).not.toHaveBeenCalledWith('日志清理任务已提交', expect.anything())
    await vi.advanceTimersByTimeAsync(8_000)
    expect(mocks.maintenance).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('fails closed when the cleanup action response returns another policy', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    vi.useFakeTimers()
    mocks.action.mockResolvedValueOnce({
      action: 'log-cleanup',
      status: 'accepted',
      changed: true,
      taskId: 'cleanup-task-a',
      maintenancePolicy: 'retain-3d',
      message: 'queued with a different policy',
      appliedAt: '2026-08-24T08:01:00Z',
    })

    const cleanup = wrapper.findAll('button').find((button) => button.text().includes('确认清理旧系统日志'))!
    await cleanup.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Agent 返回的日志清理策略与提交内容不一致；任务可能已提交，请刷新确认真实状态。')
    expect(mocks.danger).toHaveBeenCalledWith(
      '日志清理任务身份异常',
      'Agent 返回的日志清理策略与提交内容不一致；任务可能已提交，请刷新确认真实状态。',
    )
    expect(mocks.success).not.toHaveBeenCalledWith('日志清理任务已提交', expect.anything())
    await vi.advanceTimersByTimeAsync(8_000)
    expect(mocks.maintenance).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it.each([
    ['another action', { action: 'cleanup', status: 'accepted' }],
    ['a non-accepted status', { action: 'log-cleanup', status: 'completed' }],
  ])('fails closed when the cleanup action response reports %s', async (_case, response) => {
    const wrapper = mountDialog()
    await flushPromises()
    vi.useFakeTimers()
    mocks.action.mockResolvedValueOnce({
      ...response,
      changed: true,
      taskId: 'cleanup-task-a',
      maintenancePolicy: 'retain-7d',
      message: 'unexpected action response',
      appliedAt: '2026-08-24T08:01:00Z',
    })

    const cleanup = wrapper.findAll('button').find((button) => button.text().includes('确认清理旧系统日志'))!
    await cleanup.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('日志清理任务身份已变化，请刷新确认真实状态。')
    expect(mocks.danger).toHaveBeenCalledWith(
      '日志清理任务身份异常',
      '日志清理任务身份已变化，请刷新确认真实状态。',
    )
    expect(mocks.success).not.toHaveBeenCalledWith('日志清理任务已提交', expect.anything())
    await vi.advanceTimersByTimeAsync(8_000)
    expect(mocks.maintenance).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('does not treat task B as task A after closing and reopening the dialog', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    vi.useFakeTimers()

    const cleanup = wrapper.findAll('button').find((button) => button.text().includes('确认清理旧系统日志'))!
    await cleanup.trigger('click')
    await flushPromises()
    await wrapper.setProps({ open: false })

    mocks.logsSummary.mockResolvedValueOnce({
      ...summary,
      maintenance: {
        id: 'cleanup-task-b',
        state: 'succeeded',
        action: 'log-cleanup',
        policy: 'retain-7d',
        progress: 100,
        message: 'B completed while closed',
        rebootRequired: false,
      },
    })
    await wrapper.setProps({ open: true })
    await flushPromises()

    expect(wrapper.text()).toContain('日志清理任务身份已变化，请刷新确认真实状态。')
    expect(mocks.success).not.toHaveBeenCalledWith('日志清理已完成', expect.anything())
    await vi.advanceTimersByTimeAsync(8_000)
    expect(mocks.maintenance).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('resets a stale running state on reopen and never accepts another maintenance action as cleanup success', async () => {
    vi.useFakeTimers()
    mocks.logsSummary
      .mockResolvedValueOnce({
        ...summary,
        maintenance: {
          state: 'running',
          action: 'log-cleanup',
          progress: 25,
          rebootRequired: false,
        },
      })
      .mockResolvedValue(summary)
    const wrapper = mountDialog()
    await flushPromises()
    expect(wrapper.findAll('button').find((button) => button.text().includes('清理任务进行中'))?.attributes('disabled')).toBeDefined()

    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    await flushPromises()
    expect(wrapper.findAll('button').find((button) => button.text().includes('确认清理旧系统日志'))?.attributes('disabled')).toBeUndefined()

    mocks.maintenance.mockResolvedValueOnce({
      state: 'succeeded',
      action: 'cleanup',
      progress: 100,
      rebootRequired: false,
    })
    const cleanup = wrapper.findAll('button').find((button) => button.text().includes('确认清理旧系统日志'))!
    await cleanup.trigger('click')
    await Promise.resolve()
    await vi.advanceTimersByTimeAsync(800)
    expect(wrapper.text()).toContain('日志清理任务身份已变化，请刷新确认真实状态。')
    expect(mocks.success).not.toHaveBeenCalledWith('日志清理已完成', expect.anything())
    wrapper.unmount()
  })

  it('restores a cleanup failure that finished while the dialog was closed', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.setProps({ open: false })
    mocks.logsSummary.mockResolvedValueOnce({
      ...summary,
      maintenance: {
        state: 'failed',
        action: 'log-cleanup',
        progress: 100,
        message: 'journal cleanup failed on host',
        rebootRequired: false,
      },
    })

    await wrapper.setProps({ open: true })
    await flushPromises()
    expect(wrapper.text()).toContain('journal cleanup failed on host')
    expect(mocks.success).not.toHaveBeenCalledWith('日志清理已完成', expect.anything())
    wrapper.unmount()
  })

  it('shows the unavailable reason without touching the host', async () => {
    const wrapper = mountDialog(true, false)
    await flushPromises()
    expect(mocks.logsSummary).not.toHaveBeenCalled()
    expect(mocks.logs).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('日志适配器不可用')
    wrapper.unmount()
  })

  it('localizes the teleported modal explicitly while preserving real log content', async () => {
    unregisterCatalog = registerPhraseCatalog(englishSharedCatalog)
    mocks.logs.mockResolvedValueOnce({
      ...systemEntries,
      entries: [{ timestamp: '2026-08-24T08:00:00Z', message: '关闭' }],
    })
    const wrapper = mount(SystemLogsDialog, {
      attachTo: document.body,
      props: { open: true, readable: true, writable: true },
    })
    await flushPromises()

    const bodyText = document.body.textContent || ''
    expect(bodyText).toContain('System log management')
    expect(bodyText).toContain('System messages')
    expect(bodyText).toContain('Lines')
    expect(bodyText).toContain('Search logs')
    expect(bodyText).toContain('Cleanup policy')
    expect(bodyText).toContain('Clean old system logs')
    expect(document.querySelector('[aria-label="Log source"]')).not.toBeNull()
    expect(document.querySelector<HTMLInputElement>('input[type="search"]')?.placeholder).toBe('Keyword, service, PID, or message')
    expect(document.querySelector('pre.system-log-output')?.textContent).toContain('关闭')
    expect(bodyText).not.toContain('系统日志管理')
    expect(bodyText).not.toContain('系统消息')
    expect(bodyText).not.toContain('读取行数')
    expect(bodyText).not.toContain('清理策略')
    wrapper.unmount()
  })
})
