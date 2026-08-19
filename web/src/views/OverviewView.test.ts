import { createSSRApp, ssrContextKey, type ComputedRef, type Ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OverviewView from './OverviewView.vue'
import type { SystemOverview } from '@/types/api'

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
  basicSettings: ComputedRef<Array<{ id: string; title: string; capability: string }>>
  networkTools: ComputedRef<Array<{ id: string; title: string; capability: string }>>
  overviewSystemTools: ComputedRef<Array<{ id: string; title: string; capability: string }>>
  systemCenterSections: ComputedRef<Array<{ id: string; title: string; tools: Array<{ id: string; recommended?: boolean }> }>>
  selectedResourceDialog: Ref<string | undefined>
  toolAvailabilityLabel: (tool: { id: string; capability: string }) => string
  actionForm: { timezone: string; timezonePreset: string }
  openTool: (tool: { id: string }) => void
  load: (silent?: boolean) => Promise<void>
}

function setupView(): OverviewBindings {
  const component = OverviewView as unknown as {
    setup: (props: Record<string, never>, context: { expose: () => void }) => OverviewBindings
  }
  const app = createSSRApp({ render: () => null })
  app.provide(ssrContextKey, { modules: new Set<string>() })
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  try {
    return app.runWithContext(() => component.setup({}, { expose: () => undefined }))
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
  it('streams the first load but applies later refreshes atomically', async () => {
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
        expect(onUpdate).toBeUndefined()
        return new Promise<SystemOverview>((resolve) => {
          finishRefresh = resolve
        })
      })

    const view = setupView()
    await view.load()
    expect(view.data.value).toStrictEqual(initialComplete)

    const refresh = view.load(true)
    expect(view.data.value).toStrictEqual(initialComplete)
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
    expect(
      view.systemCenterSections.value.map((section) => ({
        id: section.id,
        tools: section.tools.map((tool) => tool.id),
      })),
    ).toEqual([
      { id: 'maintenance', tools: ['system-update', 'system-cleanup', 'system-reboot'] },
      { id: 'basic', tools: ['swap', 'hostname', 'timezone', 'mirror', 'cron'] },
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
  })
})
