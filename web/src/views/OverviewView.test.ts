import { createSSRApp, ssrContextKey, type ComputedRef, type Ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OverviewView from './OverviewView.vue'
import type { SystemOverview } from '@/types/api'
import { pendingOverview } from '@/lib/overviewState'

const mocks = vi.hoisted(() => ({
  overviewGet: vi.fn(),
  setAgent: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {},
  api: {
    overview: { get: mocks.overviewGet },
    system: { action: vi.fn(), maintenance: vi.fn() },
  },
}))

vi.mock('@/stores/panel', () => ({
  usePanelState: () => ({ setAgent: mocks.setAgent }),
}))

vi.mock('@/stores/toast', () => ({
  useToast: () => ({ success: vi.fn(), danger: vi.fn() }),
}))

interface OverviewBindings {
  data: Ref<SystemOverview | undefined>
  contentReady: ComputedRef<boolean>
  loading: Ref<boolean>
  error: Ref<string>
  basicSettings: ComputedRef<Array<{ id: string; title: string; capability: string }>>
  networkTools: ComputedRef<Array<{ id: string; title: string; capability: string }>>
  overviewSystemTools: ComputedRef<Array<{ id: string; title: string; capability: string }>>
  systemCenterSections: ComputedRef<Array<{ id: string; title: string; tools: Array<{ id: string; recommended?: boolean }> }>>
  selectedResourceDialog: Ref<string | undefined>
  toolAvailabilityLabel: (tool: { id: string; capability: string }) => string
  maintenanceActionFor: (toolID: string) => string | undefined
  actionForm: { timezone: string; timezonePreset: string }
  openTool: (tool: { id: string }) => void
  toolReadState: (tool: { id: string }) => string
  toolCanOpen: (tool: { id: string }) => boolean
  load: (silent?: boolean) => Promise<void>
}

function setupView(systemCenterOnly = false): OverviewBindings {
  const component = OverviewView as unknown as {
    setup: (props: { systemCenterOnly: boolean }, context: { expose: () => void }) => OverviewBindings
  }
  const app = createSSRApp({ render: () => null })
  app.provide(ssrContextKey, { modules: new Set<string>() })
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  try {
    return app.runWithContext(() => component.setup({ systemCenterOnly }, { expose: () => undefined }))
  } finally {
    warn.mockRestore()
  }
}

function overview(id: string): SystemOverview {
  return {
    agent: { version: id },
    publicNetwork: {},
    management: {
      ssh: { ports: [], defense: { enabled: false } },
      dns: { servers: [] },
      swap: {},
      packageSources: [],
      kernelOptimization: { enabled: false },
      bbr: { enabled: false },
      bbrv3: { installed: false },
      maintenance: { state: 'idle', progress: 0 },
      capabilities: {},
    },
  } as unknown as SystemOverview
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('OverviewView refresh stability', () => {
  it('keeps observed resources during partial refresh, but accepts genuine empty completion', async () => {
    const initial = pendingOverview()
    initial.reads!.runtime = { state: 'ready' }
    initial.services = [{ id: 'test', name: 'Test', state: 'running' }]
    initial.sites = { total: 4, healthy: 4, drifted: 0 }
    initial.publicNetwork = { ipv4: '192.0.2.1', source: 'test' }
    mocks.overviewGet.mockResolvedValueOnce(initial)
    const view = setupView()
    await view.load()
    let update!: (value: SystemOverview) => void
    let complete!: (value: SystemOverview) => void
    mocks.overviewGet.mockImplementationOnce((_signal, onUpdate) => {
      update = onUpdate
      return new Promise<SystemOverview>((resolve) => { complete = resolve })
    })
    const request = view.load(true)
    const partial = pendingOverview()
    partial.reads!.runtime = { state: 'ready' }
    partial.cpu.value = 47
    update(partial)
    expect(view.data.value!.services).toEqual(initial.services)
    expect(view.data.value!.sites).toEqual(initial.sites)
    expect(view.data.value!.publicNetwork).toEqual(initial.publicNetwork)
    expect(view.data.value!.cpu.value).toBe(47)
    expect(view.contentReady.value).toBe(true)
    complete(partial)
    await request
    expect(view.data.value!.services).toEqual([])
    expect(view.data.value!.sites).toBeUndefined()
    expect(view.data.value!.publicNetwork).toEqual({})
  })

  it('reports returned runtime errors, retaining prior content only if it was observed', async () => {
    const failed = pendingOverview()
    failed.reads!.runtime = { state: 'error' }
    mocks.overviewGet.mockImplementationOnce(async (_signal, update) => { update(failed); return failed })
    const view = setupView()
    await view.load()
    expect(view.contentReady.value).toBe(false)
    expect(view.loading.value).toBe(false)
    expect(view.error.value).not.toBe('')
    mocks.overviewGet.mockResolvedValueOnce(overview('retry'))
    await view.load()
    expect(view.contentReady.value).toBe(true)
    expect(view.error.value).toBe('')
    mocks.overviewGet.mockImplementationOnce(async (_signal, update) => { update(failed); return failed })
    await view.load(true)
    expect(view.contentReady.value).toBe(true)
    expect(view.data.value!.agent.version).toBe('retry')
    expect(view.error.value).not.toBe('')
  })

  it('keeps the overview skeleton until runtime, then reveals ordered content without waiting for management', async () => {
    let update!: (value: SystemOverview) => void
    let complete!: (value: SystemOverview) => void
    mocks.overviewGet.mockImplementationOnce((_signal, onUpdate) => {
      update = onUpdate
      return new Promise<SystemOverview>((resolve) => { complete = resolve })
    })
    const view = setupView()
    const request = view.load()
    expect(view.contentReady.value).toBe(false)
    expect(view.loading.value).toBe(true)
    const capabilitiesOnly = pendingOverview()
    capabilitiesOnly.reads!.capabilities = { state: 'ready' }
    update(capabilitiesOnly)
    expect(view.contentReady.value).toBe(false)
    expect(view.loading.value).toBe(true)
    const runtime = pendingOverview()
    runtime.reads!.runtime = { state: 'ready' }
    update(runtime)
    expect(view.contentReady.value).toBe(true)
    expect(view.loading.value).toBe(false)
    expect(view.toolReadState({ id: 'dns' })).toBe('loading')
    complete(runtime)
    await request
  })

  it('keeps the system center catalog immediate and leaves runtime failure recoverable on overview', async () => {
    expect(setupView(true).contentReady.value).toBe(true)
    mocks.overviewGet.mockRejectedValueOnce(new Error('runtime unavailable'))
    const view = setupView()
    await view.load()
    expect(view.contentReady.value).toBe(false)
    expect(view.loading.value).toBe(false)
    mocks.overviewGet.mockResolvedValueOnce(overview('retry'))
    await view.load()
    expect(view.contentReady.value).toBe(true)
  })

  it('renders every tool title before network data and protects unknown action state', () => {
    const view = setupView()
    expect(view.basicSettings.value.length).toBeGreaterThan(0)
    expect(view.networkTools.value.length).toBeGreaterThan(0)
    expect(view.systemCenterSections.value.map((section) => section.id)).toEqual(['maintenance', 'basic', 'security', 'network', 'performance'])
    expect(view.toolReadState({ id: 'bbrv3' })).toBe('loading')
    expect(view.toolCanOpen({ id: 'bbrv3' })).toBe(false)
    expect(view.toolCanOpen({ id: 'swap' })).toBe(false)
    expect(view.toolAvailabilityLabel({ id: 'bbrv3', capability: '' })).toBe('状态加载中')
    view.data.value!.reads!.capabilities = { state: 'ready' }
    view.data.value!.reads!.config = { state: 'ready' }
    view.data.value!.reads!.bbrv3 = { state: 'error' }
    expect(view.toolAvailabilityLabel({ id: 'bbrv3', capability: '' })).toBe('状态读取失败')
    expect(view.toolCanOpen({ id: 'bbrv3' })).toBe(false)
    expect(view.toolCanOpen({ id: 'dns' })).toBe(true)
    expect(view.toolCanOpen({ id: 'system-logs' })).toBe(true)
  })

  it('ignores stale completion and callbacks after a newer refresh', async () => {
    let oldUpdate!: (value: SystemOverview) => void
    let oldFinish!: (value: SystemOverview) => void
    mocks.overviewGet.mockImplementationOnce((_signal, update) => {
      oldUpdate = update
      return new Promise<SystemOverview>((resolve) => { oldFinish = resolve })
    }).mockResolvedValueOnce(overview('new'))
    const view = setupView()
    const old = view.load()
    await view.load(true)
    oldUpdate(overview('stale-partial'))
    oldFinish(overview('stale-final'))
    await old
    expect(view.data.value).toStrictEqual(overview('new'))
  })

  it('streams initial and subsequent refreshes without waiting for optional reads', async () => {
    const initialPartial = overview('initial-partial')
    const initialComplete = overview('initial-complete')
    const refreshedComplete = overview('refreshed-complete')
    let finishRefresh: ((value: SystemOverview) => void) | undefined
    mocks.overviewGet
      .mockImplementationOnce(async (_signal: AbortSignal, onUpdate?: (value: SystemOverview) => void) => {
        expect(onUpdate).toEqual(expect.any(Function))
        onUpdate?.(initialPartial)
        return initialComplete
      })
      .mockImplementationOnce((_signal: AbortSignal, onUpdate?: (value: SystemOverview) => void) => {
        expect(onUpdate).toEqual(expect.any(Function))
        onUpdate?.(overview('refresh-partial'))
        return new Promise<SystemOverview>((resolve) => {
          finishRefresh = resolve
        })
      })

    const view = setupView()
    await view.load()
    expect(view.data.value).toStrictEqual(initialComplete)

    const refresh = view.load(true)
    expect(view.data.value).toStrictEqual(overview('refresh-partial'))
    finishRefresh?.(refreshedComplete)
    await refresh
    expect(view.data.value).toStrictEqual(refreshedComplete)
    expect(mocks.overviewGet).toHaveBeenCalledTimes(2)
  })

  it('does not present Shanghai when the Agent cannot identify the host timezone', () => {
    const view = setupView()
    view.data.value = overview('unknown-timezone')

    view.openTool({ id: 'timezone' })

    expect(view.actionForm.timezone).toBe('')
    expect(view.actionForm.timezonePreset).toBe('__custom__')
  })

  it('groups basic settings and network tools for the overview and system center', () => {
    const view = setupView()
    view.data.value = overview('grouping')

    expect(view.basicSettings.value.map((tool) => tool.id)).toEqual([
      'hostname',
      'ssh-port',
      'ssh-defense',
      'timezone',
      'swap',
	  'disk-partitions',
      'mirror',
	  'system-tuning',
	  'accounts',
      'cron',
    ])
    expect(view.basicSettings.value.find((tool) => tool.id === 'mirror')?.title).toBe('系统更新源')
    expect(view.networkTools.value.map((tool) => tool.id)).toEqual([
      'dns',
      'hosts',
      'network-interfaces',
      'firewall',
	  'port-usage',
	  'traffic-shutdown',
      'ip-preference',
      'kernel',
      'bbr',
      'bbrv3',
    ])
    expect(view.toolAvailabilityLabel(view.networkTools.value[1]!)).toBe('适配器未就绪')
    expect(view.overviewSystemTools.value.map((tool) => tool.id)).toEqual([
      'swap',
      'ssh-port',
      'dns',
      'ip-preference',
      'bbr',
      'system-tuning',
    ])
    expect(view.overviewSystemTools.value.map((tool) => tool.title)).toEqual([
      '虚拟内存',
      'SSH 端口',
      'DNS 优化',
      'V4 / V6 优先',
      'BBR 管理',
      '综合调优',
    ])
    expect(view.maintenanceActionFor('system-logs')).toBe('log-cleanup')
    expect(
      view.systemCenterSections.value.map((section) => ({
        id: section.id,
        tools: section.tools.map((tool) => tool.id),
      })),
    ).toEqual([
      { id: 'maintenance', tools: ['system-update', 'system-cleanup', 'system-logs', 'system-reboot'] },
      { id: 'basic', tools: ['swap', 'disk-partitions', 'hostname', 'timezone', 'mirror', 'cron'] },
      { id: 'security', tools: ['ssh-port', 'ssh-defense', 'accounts', 'firewall'] },
      {
        id: 'network',
        tools: [
          'dns',
          'port-usage',
          'network-interfaces',
          'ip-preference',
          'hosts',
          'traffic-shutdown',
        ],
      },
      { id: 'performance', tools: ['system-tuning', 'bbr', 'kernel', 'bbrv3'] },
    ])
    expect(
      view.systemCenterSections.value
        .flatMap((section) => section.tools)
        .filter((tool) => tool.recommended)
        .map((tool) => tool.id),
    ).toEqual(['system-update', 'system-cleanup', 'swap', 'ssh-defense', 'dns', 'bbr'])

    view.openTool({ id: 'hosts' })
    expect(view.selectedResourceDialog.value).toBe('hosts')
    view.openTool({ id: 'disk-partitions' })
    expect(view.selectedResourceDialog.value).toBe('disk-partitions')
    view.openTool({ id: 'system-logs' })
    expect(view.selectedResourceDialog.value).toBe('system-logs')
  })
})
