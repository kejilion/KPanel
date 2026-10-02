// @vitest-environment jsdom

import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ref, type Ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import DockerView from './DockerView.vue'
import { desktopWindowActiveKey, desktopWindowVisibleKey } from '@/lib/desktopRouteKeys'
import { dockerLiveMetricsInterval } from '@/lib/dockerLiveMetrics'
import type { DockerContainer, DockerContainerStats, DockerInventory } from '@/types/api'

const mocks = vi.hoisted(() => ({
  inventory: vi.fn(),
  backups: vi.fn(),
  environment: vi.fn(),
  jobs: vi.fn(),
  publicNetwork: vi.fn(),
  checkUpdate: vi.fn(),
  stats: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {
    readonly status: number

    constructor(message: string, status = 0) {
      super(message)
      this.status = status
    }
  },
  api: {
    docker: {
      checkUpdate: mocks.checkUpdate,
      inventory: mocks.inventory,
      backups: mocks.backups,
      environment: mocks.environment,
      jobs: mocks.jobs,
      job: vi.fn(),
      action: vi.fn(),
      composeProject: vi.fn(),
      exec: vi.fn(),
      logs: vi.fn(),
      stats: mocks.stats,
      task: vi.fn(),
    },
    system: { publicNetwork: mocks.publicNetwork, action: vi.fn() },
  },
}))

function container(overrides: Partial<DockerContainer> & Pick<DockerContainer, 'id' | 'name'>): DockerContainer {
  return {
    image: 'nginx:alpine',
    state: 'running',
    access: 'managed',
    consistency: 'synced',
    ports: [],
    networks: ['bridge'],
    mounts: [],
    resourceVersion: `sha256:${overrides.id}`,
    allowedActions: ['logs', 'stats', 'exec', 'access', 'restart', 'pause', 'stop', 'remove'],
    ...overrides,
  }
}

const webID = 'a'.repeat(64)
const dbID = 'b'.repeat(64)
const idleID = 'c'.repeat(64)

function inventory(): DockerInventory {
  return {
    available: true,
    version: '28.0.0',
    observedAt: '2026-10-02T00:00:00Z',
    containers: [
      container({ id: webID, name: 'web', project: 'shop', service: 'web' }),
      container({ id: dbID, name: 'db', project: 'shop', service: 'db', image: 'mysql:8' }),
      container({
        id: idleID,
        name: 'idle',
        state: 'exited',
        statusText: 'Exited (0)',
        allowedActions: ['start', 'remove'],
      }),
    ],
    composeProjects: ['shop'],
    images: [],
    networks: [],
    volumes: [],
  }
}

function sample(id: string): DockerContainerStats {
  const web = id === webID
  return {
    containerId: id,
    cpuPercent: web ? 42.5 : 3.2,
    memoryBytes: web ? 256 * 1024 * 1024 : 64 * 1024 * 1024,
    memoryLimitBytes: 1024 * 1024 * 1024,
    memoryPercent: web ? 25 : 6.3,
    networkRxBytes: 4096,
    networkTxBytes: 2048,
    blockReadBytes: 8192,
    blockWriteBytes: 1024,
    pids: web ? 12 : 5,
    collectedAt: new Date().toISOString(),
  }
}

describe('Docker container monitor mode', () => {
  let wrapper: VueWrapper

  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    window.localStorage.clear()
    mocks.inventory.mockResolvedValue(inventory())
    mocks.backups.mockResolvedValue({ items: [] })
    mocks.environment.mockResolvedValue({
      available: true,
      containers: 3,
      images: 0,
      mirrorPreset: 'official',
      registryMirrors: [],
      ipv6Enabled: false,
      daemonConfig: 'valid',
      observedAt: '2026-10-02T00:00:00Z',
    })
    mocks.jobs.mockResolvedValue({ items: [] })
    mocks.publicNetwork.mockResolvedValue({ ipv4: '203.0.113.10' })
    mocks.checkUpdate.mockRejectedValue({ code: 'docker_update_registry_auth' })
    mocks.stats.mockImplementation(async (id: string) => sample(id))
  })

  afterEach(() => {
    wrapper?.unmount()
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  async function mountView(windowState?: { active: Ref<boolean>; visible: Ref<boolean> }): Promise<void> {
    wrapper = mount(DockerView, {
      attachTo: document.body,
      global: windowState
        ? { provide: { [desktopWindowActiveKey]: windowState.active, [desktopWindowVisibleKey]: windowState.visible } }
        : undefined,
    })
    await flushPromises()
  }

  function viewSwitch() {
    const buttons = wrapper.findAll('.docker-view-switch button')
    return { manage: buttons[0]!, monitor: buttons[1]! }
  }

  it('starts in the management view and does not sample anything', async () => {
    await mountView()
    const { manage, monitor } = viewSwitch()
    expect(manage.attributes('aria-pressed')).toBe('true')
    expect(monitor.attributes('aria-pressed')).toBe('false')
    expect(wrapper.find('.docker-table--monitor').exists()).toBe(false)
    expect(wrapper.find('.docker-row-actions').exists()).toBe(true)
    expect(wrapper.text()).toContain('容器日常管理')
    // The switch lives in the controls, not after the title text, so it cannot move between views.
    expect(wrapper.find('.resource-section__heading .docker-view-switch').exists()).toBe(false)
    expect(wrapper.get('.resource-section__controls > .docker-view-switch').element)
      .toBe(wrapper.get('.resource-section__controls').element.lastElementChild)
    expect(wrapper.get('.docker-controls-slot > .docker-live-status').classes()).toContain('is-inactive')
    expect(wrapper.get('.docker-controls-slot > .card-actions').classes()).not.toContain('is-inactive')
    await vi.advanceTimersByTimeAsync(dockerLiveMetricsInterval * 3)
    expect(mocks.stats).not.toHaveBeenCalled()
  })

  it('swaps the list columns for live usage and only samples running containers', async () => {
    await mountView()
    await viewSwitch().monitor.trigger('click')
    await vi.advanceTimersByTimeAsync(0)
    await flushPromises()

    expect(viewSwitch().monitor.attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('.docker-table--monitor').exists()).toBe(true)
    expect(wrapper.text()).toContain('容器资源监控')
    const headers = wrapper.findAll('thead th').map((cell) => cell.text())
    expect(headers).toEqual(['容器', '状态', 'CPU', '内存', '磁盘 I/O', '网络', '进程'])

    // The management-only surface is gone from the same list.
    expect(wrapper.find('.docker-row-actions').exists()).toBe(false)
    expect(wrapper.find('.docker-context-trigger').exists()).toBe(false)
    expect(wrapper.get('.docker-controls-slot > .card-actions').classes()).toContain('is-inactive')
    expect(wrapper.get('.docker-controls-slot > .card-actions').attributes('aria-hidden')).toBe('true')
    expect(wrapper.get('.docker-controls-slot > .docker-live-status').classes()).not.toContain('is-inactive')
    expect(wrapper.text()).not.toContain('管理 Compose')

    const requested = mocks.stats.mock.calls.map(([id]) => id).sort()
    expect(requested).toEqual([webID, dbID].sort())

    const meters = wrapper.findAll('[role="meter"]')
    expect(meters).toHaveLength(4)
    expect(wrapper.text()).toContain('42.5%')
    expect(wrapper.text()).toContain('256.0 MB / 1.0 GB')
    expect(wrapper.text()).toContain('实时 · 更新于')

    // The stopped container keeps its row but has nothing to chart.
    const idleRow = wrapper.findAll('tr.docker-row').find((row) => row.text().includes('idle'))!
    expect(idleRow.findAll('[role="meter"]')).toHaveLength(0)
    expect(idleRow.text()).toContain('—')
  })

  it('samples standalone containers like compose ones and explains a running container it cannot sample', async () => {
    const standaloneID = 'd'.repeat(64)
    const lockedID = 'e'.repeat(64)
    mocks.inventory.mockResolvedValue({
      ...inventory(),
      containers: [
        container({ id: standaloneID, name: 'solo' }),
        container({ id: lockedID, name: 'locked', allowedActions: ['logs'] }),
      ],
      composeProjects: [],
    })
    await mountView()
    await viewSwitch().monitor.trigger('click')
    await vi.advanceTimersByTimeAsync(0)
    await flushPromises()

    expect(mocks.stats.mock.calls.map(([id]) => id)).toEqual([standaloneID])
    const group = wrapper.get('.docker-group__row')
    expect(group.text()).toContain('独立容器')
    expect(group.text()).toContain('2/2 运行中')
    expect(group.text()).toContain('CPU 合计 3.2%')
    const row = (name: string) => wrapper.findAll('tr.docker-row').find((candidate) => candidate.text().includes(name))!
    expect(row('solo').findAll('[role="meter"]')).toHaveLength(2)
    expect(row('locked').text()).toContain('暂不可采样')
    expect(row('locked').findAll('[role="meter"]')).toHaveLength(0)
  })

  it('shows a compose project total in the group header while monitoring', async () => {
    await mountView()
    await viewSwitch().monitor.trigger('click')
    await vi.advanceTimersByTimeAsync(0)
    await flushPromises()
    const group = wrapper.findAll('.docker-group__row').find((row) => row.text().includes('shop'))!
    expect(group.text()).toContain('2/2 运行中')
    expect(group.text()).toContain('CPU 合计 45.7%')
    expect(group.text()).toContain('内存合计 320.0 MB')
  })

  it('keeps refreshing while monitoring and stops as soon as the view switches back', async () => {
    await mountView()
    await viewSwitch().monitor.trigger('click')
    await vi.advanceTimersByTimeAsync(0)
    await flushPromises()
    const afterFirstPass = mocks.stats.mock.calls.length
    expect(afterFirstPass).toBe(2)

    await vi.advanceTimersByTimeAsync(dockerLiveMetricsInterval)
    await flushPromises()
    expect(mocks.stats.mock.calls.length).toBe(afterFirstPass * 2)

    await viewSwitch().manage.trigger('click')
    await flushPromises()
    const afterSwitch = mocks.stats.mock.calls.length
    await vi.advanceTimersByTimeAsync(dockerLiveMetricsInterval * 5)
    expect(mocks.stats.mock.calls.length).toBe(afterSwitch)
    expect(wrapper.find('.docker-table--monitor').exists()).toBe(false)
    expect(wrapper.find('.docker-row-actions').exists()).toBe(true)
    expect(wrapper.find('[role="meter"]').exists()).toBe(false)
  })

  it('pauses sampling while the browser tab is hidden', async () => {
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    await mountView()
    await viewSwitch().monitor.trigger('click')
    await vi.advanceTimersByTimeAsync(0)
    await flushPromises()
    const before = mocks.stats.mock.calls.length

    visibility.mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(dockerLiveMetricsInterval * 5)
    expect(mocks.stats.mock.calls.length).toBe(before)
  })

  it('keeps sampling in a visible desktop window that is not focused and stops once it is hidden', async () => {
    const windowState = { active: ref(false), visible: ref(true) }
    await mountView(windowState)
    await viewSwitch().monitor.trigger('click')
    await vi.advanceTimersByTimeAsync(0)
    await flushPromises()
    const first = mocks.stats.mock.calls.length
    expect(first).toBe(2)

    await vi.advanceTimersByTimeAsync(dockerLiveMetricsInterval)
    await flushPromises()
    expect(mocks.stats.mock.calls.length).toBe(first * 2)

    windowState.visible.value = false
    await flushPromises()
    const hidden = mocks.stats.mock.calls.length
    await vi.advanceTimersByTimeAsync(dockerLiveMetricsInterval * 5)
    expect(mocks.stats.mock.calls.length).toBe(hidden)
    expect(wrapper.find('[role="meter"]').exists()).toBe(false)
  })

  it('re-reads the container inventory every 30 seconds, but only while monitoring', async () => {
    await mountView()
    const initial = mocks.inventory.mock.calls.length
    await vi.advanceTimersByTimeAsync(60_000)
    expect(mocks.inventory.mock.calls.length).toBe(initial)

    await viewSwitch().monitor.trigger('click')
    await vi.advanceTimersByTimeAsync(30_000)
    await flushPromises()
    expect(mocks.inventory.mock.calls.length).toBe(initial + 1)
    // Background refreshes take only the final inventory, so the tab counts never dip to zero.
    expect(mocks.inventory.mock.calls.at(-1)?.[1]).toBeUndefined()

    await viewSwitch().manage.trigger('click')
    await vi.advanceTimersByTimeAsync(60_000)
    await flushPromises()
    expect(mocks.inventory.mock.calls.length).toBe(initial + 1)
  })

  it('reports an unreadable sample instead of showing nothing', async () => {
    mocks.stats.mockRejectedValue(new Error('docker busy'))
    await mountView()
    await viewSwitch().monitor.trigger('click')
    await vi.advanceTimersByTimeAsync(0)
    await flushPromises()
    expect(wrapper.text()).toContain('暂时无法读取容器性能数据，正在重试')
    expect(wrapper.text()).toContain('采样失败')
    expect(wrapper.find('[role="meter"]').exists()).toBe(false)
  })
})
