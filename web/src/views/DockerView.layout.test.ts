import { readFileSync } from 'node:fs'
import { createSSRApp, nextTick, ref, ssrContextKey, type Ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import DockerView from './DockerView.vue'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import type { DockerContainer, DockerContainerStats, DockerMaintenanceJob } from '@/types/api'

const mocks = vi.hoisted(() => ({
  job: vi.fn(),
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
      job: mocks.job,
      stats: mocks.stats,
    },
  },
}))

const dockerSource = readFileSync(new URL('./DockerView.vue', import.meta.url), 'utf8')
const deploymentEditorSource = readFileSync(new URL('../components/docker/DockerDeploymentEditor.vue', import.meta.url), 'utf8')

interface DockerBindings {
  stats: Ref<DockerContainerStats | undefined>
  startJobPolling: (job: DockerMaintenanceJob) => void
  restoreBackgroundJob: () => Promise<void>
  refreshJob: (id: string) => Promise<void>
  showStats: (container: DockerContainer) => void
  refreshStats: () => Promise<void>
}

function setupView(windowActive: Ref<boolean>): DockerBindings {
  const component = DockerView as unknown as {
    setup: (props: Record<string, never>, context: { expose: () => void }) => DockerBindings
  }
  const app = createSSRApp({ render: () => null })
  app.provide(ssrContextKey, { modules: new Set<string>() })
  app.provide(desktopWindowActiveKey, windowActive)
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  try {
    return app.runWithContext(() => component.setup({}, { expose: () => undefined }))
  } finally {
    warn.mockRestore()
  }
}

function runningJob(): DockerMaintenanceJob {
  return {
    id: 'd'.repeat(32),
    action: 'prune',
    status: 'running',
    stage: 'running',
    progress: 20,
    createdAt: '2026-08-08T00:00:00Z',
  }
}

function container(): DockerContainer {
  return {
    id: 'c'.repeat(64),
    name: 'web',
    image: 'nginx:latest',
    state: 'running',
    access: 'managed',
    consistency: 'synced',
    ports: [],
    networks: ['bridge'],
    mounts: [],
  }
}

function sampleStats(cpuPercent: number): DockerContainerStats {
  return {
    containerId: 'c'.repeat(64),
    cpuPercent,
    memoryBytes: 1,
    memoryLimitBytes: 2,
    memoryPercent: 50,
    networkRxBytes: 3,
    networkTxBytes: 4,
    blockReadBytes: 5,
    blockWriteBytes: 6,
    pids: 7,
    collectedAt: '2026-08-08T00:00:00Z',
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  const storage = new Map<string, string>()
  vi.stubGlobal('window', {
    localStorage: {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
    },
    setTimeout: vi.fn(() => 1),
    clearTimeout: vi.fn(),
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Docker resource toolbar layout', () => {
  it('gives Docker text and select controls a consistent KPanel treatment', () => {
    expect(dockerSource).toMatch(/\.text-input,\s*\.select-input\s*\{[^}]*border-radius:\s*11px;[^}]*background-color:\s*var\(--surface-subtle\);/)
    expect(dockerSource).toMatch(/\.select-input\s*\{[^}]*appearance:\s*none;[^}]*background-image:\s*url\(/)
    expect(dockerSource).toMatch(/\.text-input:focus,\s*\.select-input:focus\s*\{[^}]*border-color:\s*var\(--brand\);[^}]*box-shadow:/)
  })

  it('uses a light deployment editor by default and preserves the dark terminal treatment', () => {
    expect(deploymentEditorSource).toMatch(/\.deployment-editor__surface\s*\{[\s\S]*?background:\s*var\(--surface-subtle\);/)
    expect(deploymentEditorSource).toMatch(/:global\(:root\[data-theme='dark'\]\) \.deployment-editor__surface\s*\{[^}]*background:\s*var\(--terminal-shell-background, #0b1214\);/)
    expect(deploymentEditorSource).toContain('setSelectionRange(item.from, Math.max(item.from + 1, item.to))')
  })

  it('does not repeat generic invalid-state copy when precise diagnostics are visible', () => {
    expect(dockerSource).toContain(`v-if="createAnalysis.kind !== 'invalid' || !createDiagnostics.length"`)
    expect(dockerSource).toContain(`v-if="composeAnalysis.kind !== 'invalid' || !composeDiagnostics.length"`)
  })

  it('keeps resource actions aligned to the right on desktop', () => {
    expect(dockerSource).toContain('.workspace-card > header:not(.resource-section__header)')
    expect(dockerSource).toMatch(/\.resource-section__header\s*\{[^}]*align-items:\s*center;/)
    expect(dockerSource).toMatch(/\.resource-section__header > \.card-actions\s*\{[^}]*margin-left:\s*auto;[^}]*justify-content:\s*flex-end;/)
  })

  it('lets resource actions wrap below the title on narrower screens', () => {
    expect(dockerSource).toMatch(/@media \(max-width: 1000px\)[\s\S]*?\.resource-section__header > \.card-actions\s*\{[^}]*width:\s*100%;[^}]*margin-left:\s*0;[^}]*flex-wrap:\s*wrap;/)
  })

  it('keeps resource titles and descriptions stacked instead of running together', () => {
    expect(dockerSource).toMatch(
      /\.resource-section__header > div:first-child > div\s*\{[^}]*display:\s*grid;[^}]*gap:\s*3px;/,
    )
  })

  it('exposes right-click menus for every daily Docker resource', () => {
    expect(dockerSource).toContain('@contextmenu="showContainerContext($event, container)"')
    expect(dockerSource).toContain('@contextmenu="showImageContext($event, image)"')
    expect(dockerSource).toContain('@contextmenu="showNetworkContext($event, network)"')
    expect(dockerSource).toContain('@contextmenu="showVolumeContext($event, volume)"')
    expect(dockerSource).toContain('class="docker-context-menu"')
    expect(dockerSource).toContain('role="menu"')
  })

  it('groups Compose containers and exposes project configuration management', () => {
    expect(dockerSource).toContain('v-for="group in containerGroups"')
    expect(dockerSource).toContain(':aria-expanded="!isContainerGroupCollapsed(group.key)"')
    expect(dockerSource).toContain('@click="toggleContainerGroup(group.key)"')
    expect(dockerSource).toContain('<TransitionGroup name="docker-group-row">')
    expect(dockerSource).toContain('v-for="container in visibleContainerGroupRows(group)"')
    expect(dockerSource).toContain(':style="containerGroupStyle(group)"')
    expect(dockerSource).toContain('var(--docker-group-accent, var(--brand))')
    expect(dockerSource).toMatch(/\.docker-group-row-enter-active,[\s\S]*?transition:\s*opacity \.12s linear, transform \.12s cubic-bezier\(\.2, \.8, \.2, 1\);/)
    expect(dockerSource).toContain('transform: translate3d(0, -3px, 0)')
    expect(dockerSource).not.toContain('scaleY(.97)')
    expect(dockerSource).toMatch(/@media \(prefers-reduced-motion: reduce\)[\s\S]*?\.docker-group-row-enter-active,[\s\S]*?transition:\s*none;/)
    expect(dockerSource).toContain('管理 Compose')
    expect(dockerSource).toContain('保存并重新部署')
    expect(dockerSource).toContain('composeEnvironmentVariables')
    expect(dockerSource).toContain('composeEnvironment: createComposeEnvironmentSource()')
    expect(dockerSource).toContain('composeEnvironment: environmentSource')
    expect(dockerSource).toContain('项目变量 <code data-i18n-ignore>.env</code>')
    expect(dockerSource).toContain("createComposeEnvironmentRevealed ? 'text' : 'password'")
    expect(dockerSource).not.toContain('1panel.env')
    expect(dockerSource).toMatch(/\.docker-table\s*\{\s*min-width:\s*1240px;/)
    expect(dockerSource).toMatch(/\.docker-table \.docker-row > td:last-child\s*\{[^}]*min-width:\s*372px;[^}]*max-width:\s*372px;/)
  })
})

describe('Docker desktop polling', () => {
  it('throttles a running maintenance job while the window is inactive', async () => {
    const job = runningJob()
    const windowActive = ref(false)
    mocks.job.mockResolvedValue(job)
    const view = setupView(windowActive)

    view.startJobPolling(job)

    expect(mocks.job).not.toHaveBeenCalled()
    expect(window.setTimeout).toHaveBeenLastCalledWith(expect.any(Function), 15_000)

    windowActive.value = true
    await nextTick()

    expect(mocks.job).toHaveBeenCalledOnce()
  })

  it('does not overlap a slow maintenance-job request', async () => {
    let resolveJob: ((value: DockerMaintenanceJob) => void) | undefined
    mocks.job.mockReturnValueOnce(
      new Promise<DockerMaintenanceJob>((resolve) => {
        resolveJob = resolve
      }),
    )
    const view = setupView(ref(true))
    const job = runningJob()

    view.startJobPolling(job)
    await view.refreshJob(job.id)

    expect(mocks.job).toHaveBeenCalledOnce()
    resolveJob?.(job)
    await Promise.resolve()
    await Promise.resolve()
    expect(window.setTimeout).toHaveBeenLastCalledWith(expect.any(Function), 1_500)
  })

  it('keeps a persisted maintenance job after a transient restore failure', async () => {
    const job = runningJob()
    window.localStorage.setItem('kpanel.active-docker-job', job.id)
    mocks.job.mockRejectedValueOnce(new Error('temporary network failure'))
    const view = setupView(ref(false))

    await view.restoreBackgroundJob()

    expect(window.localStorage.getItem('kpanel.active-docker-job')).toBe(job.id)
    expect(window.setTimeout).toHaveBeenLastCalledWith(expect.any(Function), 15_000)
  })

  it('aborts a slow stats request on blur and ignores its late response', async () => {
    let resolveStats: ((value: DockerContainerStats) => void) | undefined
    mocks.stats.mockReturnValueOnce(
      new Promise<DockerContainerStats>((resolve) => {
        resolveStats = resolve
      }),
    )
    const windowActive = ref(true)
    const view = setupView(windowActive)

    view.showStats(container())
    await view.refreshStats()

    expect(mocks.stats).toHaveBeenCalledOnce()
    const signal = mocks.stats.mock.calls[0]?.[1] as AbortSignal

    windowActive.value = false
    await nextTick()
    expect(signal.aborted).toBe(true)

    resolveStats?.(sampleStats(99))
    await Promise.resolve()
    await Promise.resolve()
    expect(view.stats.value).toBeUndefined()
  })
})
