import { readFileSync } from 'node:fs'
import { createSSRApp, nextTick, ref, ssrContextKey, type ComputedRef, type Ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import DiagnosticsView from './DiagnosticsView.vue'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import type { DiagnosticCatalog, DiagnosticCheck, DiagnosticJob } from '@/types/api'

const mocks = vi.hoisted(() => ({
  job: vi.fn(),
  toastDanger: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {},
  api: {
    diagnostics: {
      catalog: vi.fn(),
      jobs: vi.fn(),
      job: mocks.job,
      start: vi.fn(),
    },
  },
}))

vi.mock('@/stores/toast', () => ({
  useToast: () => ({
    success: vi.fn(),
    danger: mocks.toastDanger,
  }),
}))

interface MetricParts {
  amount: string
  unit: string
  pending: boolean
}

interface DiagnosticBindings {
  catalog: Ref<DiagnosticCatalog | undefined>
  jobs: Ref<DiagnosticJob[]>
  activeJob: Ref<DiagnosticJob | undefined>
  runningJob: Ref<DiagnosticJob | undefined>
  selectedCheck: Ref<DiagnosticCheck | undefined>
  hasActiveJob: ComputedRef<boolean>
  testedCheckIDs: ComputedRef<Set<string>>
  refreshJob: (id: string) => Promise<void>
  startPolling: (job: DiagnosticJob) => void
  selectCheck: (check: DiagnosticCheck) => void
  reportMetrics: ComputedRef<Record<'cpu' | 'memory' | 'diskRead' | 'diskWrite' | 'latency' | 'download' | 'upload', MetricParts>>
  scoreMeterValue: ComputedRef<number | undefined>
}

function setupView(windowActive?: Ref<boolean>): DiagnosticBindings {
  const component = DiagnosticsView as unknown as {
    setup: (props: Record<string, never>, context: { expose: () => void }) => DiagnosticBindings
  }
  const app = createSSRApp({ render: () => null })
  app.provide(ssrContextKey, { modules: new Set<string>() })
  if (windowActive) app.provide(desktopWindowActiveKey, windowActive)
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  try {
    return app.runWithContext(() => component.setup({}, { expose: () => undefined }))
  } finally {
    warn.mockRestore()
  }
}

function runningJob(): DiagnosticJob {
  return {
    id: 'a'.repeat(32),
    checkId: 'yabs',
    checkName: 'YABS 性能测试',
    category: 'hardware',
    sourceUrl: 'https://yabs.sh',
    estimatedMinutes: 30,
    impact: 'intensive',
    status: 'running',
    stage: 'running',
    progress: 10,
    logs: [],
    createdAt: '2026-07-27T12:00:00Z',
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('window', {
    setTimeout: vi.fn(() => 1),
    clearTimeout: vi.fn(),
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DiagnosticsView polling', () => {
  it('marks only checks with finished jobs as tested', () => {
    const view = setupView()
    const job = runningJob()
    view.jobs.value = [
      { ...job, id: 'b'.repeat(32), checkId: 'completed', status: 'succeeded' },
      { ...job, id: 'c'.repeat(32), checkId: 'failed', status: 'failed' },
      { ...job, id: 'd'.repeat(32), checkId: 'queued', status: 'queued' },
      job,
    ]

    expect([...view.testedCheckIDs.value]).toEqual(['completed', 'failed'])
  })

  it('keeps the interactive terminal mounted in unfocused windows; it pauses itself when hidden', () => {
    const source = readFileSync(new URL('./DiagnosticsView.vue', import.meta.url), 'utf8')
    expect(source).toContain('v-if="activeJob?.interactive"')
    expect(source).not.toContain('activeJob?.interactive && windowActive')
  })

  it('does not cancel or duplicate a slow in-flight refresh', async () => {
    let resolveJob: ((job: DiagnosticJob) => void) | undefined
    mocks.job.mockReturnValueOnce(
      new Promise<DiagnosticJob>((resolve) => {
        resolveJob = resolve
      }),
    )
    const view = setupView()

    const first = view.refreshJob('a'.repeat(32))
    const second = view.refreshJob('a'.repeat(32))

    expect(mocks.job).toHaveBeenCalledOnce()
    resolveJob?.(runningJob())
    await Promise.all([first, second])
  })

  it('keeps a background task from stealing the terminal after another check is selected', async () => {
    const background = runningJob()
    const selected: DiagnosticJob = {
      ...background,
      id: 'b'.repeat(32),
      checkId: 'ip-quality',
      checkName: 'IP 质量体检',
      status: 'succeeded',
      progress: 100,
    }
    mocks.job.mockResolvedValue({ ...background, progress: 42 })
    const view = setupView()
    view.jobs.value = [background]
    view.runningJob.value = background
    view.activeJob.value = selected

    await view.refreshJob(background.id)

    expect(view.activeJob.value?.id).toBe(selected.id)
    expect(view.jobs.value[0]?.progress).toBe(42)
  })

  it('shows a newly selected check without hiding the background task lock', () => {
    const view = setupView()
    const background = runningJob()
    const check: DiagnosticCheck = {
      id: 'ip-quality',
      category: 'access',
      name: 'IP 质量体检',
      description: '检查 IP 质量',
      sourceUrl: 'https://example.com',
      estimatedMinutes: 1,
      impact: 'light',
    }
    view.jobs.value = [background]
    view.runningJob.value = background
    view.activeJob.value = background

    view.selectCheck(check)

    expect(view.selectedCheck.value).toMatchObject(check)
    expect(view.activeJob.value).toBeUndefined()
    expect(view.hasActiveJob.value).toBe(true)
  })

  it('tolerates two transient failures and reports the third', async () => {
    mocks.job.mockRejectedValue(new Error('temporary network failure'))
    const view = setupView()

    await view.refreshJob('a'.repeat(32))
    await view.refreshJob('a'.repeat(32))
    expect(mocks.toastDanger).not.toHaveBeenCalled()

    await view.refreshJob('a'.repeat(32))
    expect(mocks.toastDanger).toHaveBeenCalledWith(
      '体检进度刷新中断',
      '后台任务可能仍在运行，请稍后点击刷新重新连接。',
    )
  })

  it('uses a low-frequency background poll and refreshes immediately after focus returns', async () => {
    const job = runningJob()
    const windowActive = ref(false)
    mocks.job.mockResolvedValue(job)
    const view = setupView(windowActive)

    view.startPolling(job)

    expect(mocks.job).not.toHaveBeenCalled()
    expect(window.setTimeout).toHaveBeenLastCalledWith(expect.any(Function), 15_000)

    windowActive.value = true
    await nextTick()

    expect(mocks.job).toHaveBeenCalledOnce()
  })
})

describe('DiagnosticsView report values', () => {
  const nativeCheck: DiagnosticCheck = {
    id: 'native-comprehensive',
    category: 'core',
    name: 'KPanel 核心综合体检',
    description: '原生综合体检',
    sourceUrl: '',
    provider: 'native',
    estimatedMinutes: 3,
    impact: 'network',
  }

  function nativeJob(overrides: Partial<DiagnosticJob> = {}): DiagnosticJob {
    return {
      ...runningJob(),
      checkId: nativeCheck.id,
      checkName: nativeCheck.name,
      category: nativeCheck.category,
      sourceUrl: '',
      provider: 'native',
      ...overrides,
    }
  }

  it('splits amounts from units, keeps non-numeric values whole and marks missing values pending', () => {
    const view = setupView()
    view.catalog.value = { categories: [], items: [nativeCheck] }
    view.jobs.value = [nativeJob({
      status: 'succeeded',
      progress: 100,
      summary: {
        parser: 'kpanel-native-v1',
        dimensions: {
          performance: { metrics: [
            { key: 'cpu_score', value: '1842 KPS' },
            { key: 'memory_score', value: 'N/A' },
            { key: 'disk_write', value: '486.00 MiB/s' },
          ] },
          latency: { metrics: [{ key: 'average', value: '38.24ms' }] },
        },
      },
    })]

    expect(view.reportMetrics.value.cpu).toEqual({ amount: '1842', unit: 'KPS', pending: false })
    expect(view.reportMetrics.value.diskWrite).toEqual({ amount: '486.00', unit: 'MiB/s', pending: false })
    expect(view.reportMetrics.value.latency).toEqual({ amount: '38.24', unit: 'ms', pending: false })
    expect(view.reportMetrics.value.memory).toEqual({ amount: 'N/A', unit: '', pending: false })
    expect(view.reportMetrics.value.upload).toEqual({ amount: '等待检测', unit: '', pending: true })
  })

  it('fills the total score bar with live progress while the benchmark runs', () => {
    const view = setupView()
    view.catalog.value = { categories: [], items: [nativeCheck] }
    view.jobs.value = [nativeJob({ status: 'running', progress: 42 })]

    expect(view.scoreMeterValue.value).toBe(42)

    view.jobs.value = [nativeJob({ status: 'failed', progress: 60 })]
    expect(view.scoreMeterValue.value).toBeUndefined()
  })
})
