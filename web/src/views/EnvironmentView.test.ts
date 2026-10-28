import { createSSRApp, ssrContextKey, type Ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import EnvironmentView from './EnvironmentView.vue'
import { api } from '@/lib/api'
import type { WebEnvironmentBackup, WebEnvironmentJob, WebEnvironmentSummary } from '@/types/api'

const mocks = vi.hoisted(() => ({
  start: vi.fn(),
  toastSuccess: vi.fn(),
  toastDanger: vi.fn(),
  route: { query: {} as Record<string, unknown> },
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {},
  api: {
    webEnvironment: {
      summary: vi.fn(),
      catalog: vi.fn(),
      backups: vi.fn(),
      jobs: vi.fn(),
      job: vi.fn(),
      terminal: vi.fn(),
      terminalInput: vi.fn(),
      start: mocks.start,
      backupDownloadURL: vi.fn(),
    },
  },
}))

vi.mock('@/stores/toast', () => ({
  useToast: () => ({
    success: mocks.toastSuccess,
    danger: mocks.toastDanger,
  }),
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return { ...actual, useRoute: () => mocks.route }
})

interface EnvironmentBindings {
  section: Ref<string>
  wafLabel: Readonly<Ref<string>>
  wafBadge: Readonly<Ref<string>>
  fail2banInstalled: Readonly<Ref<boolean>>
  fail2banActionLabel: Readonly<Ref<string>>
  banMigrationLabel: Readonly<Ref<string>>
  cloudflareLabel: Readonly<Ref<string>>
  cloudflareBadge: Readonly<Ref<string>>
  cloudflareShieldLabel: Readonly<Ref<string>>
  cloudflareShieldBadge: Readonly<Ref<string>>
  ddosLabel: Readonly<Ref<string>>
  ddosBadge: Readonly<Ref<string>>
  summary: Ref<WebEnvironmentSummary | undefined>
  backups: Ref<WebEnvironmentBackup[]>
  jobs: Ref<WebEnvironmentJob[]>
  error: Ref<string>
  auxiliaryWarning: Readonly<Ref<string>>
  terminalOpen: Ref<boolean>
  terminalJob: Ref<WebEnvironmentJob | undefined>
  load: (silent?: boolean) => Promise<void>
  loadBackups: (force?: boolean) => Promise<void>
  start: (input: Record<string, unknown>) => Promise<void>
  stopJobPolling: () => void
}

function deferred<T>(): { promise: Promise<T>; resolve: (value: T) => void } {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((next) => {
    resolve = next
  })
  return { promise, resolve }
}

function setupView(): EnvironmentBindings {
  const component = EnvironmentView as unknown as {
    setup: (props: Record<string, never>, context: { expose: () => void }) => EnvironmentBindings
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

function summary(): WebEnvironmentSummary {
  return {
    protocolVersion: '1',
    state: 'installed',
    profile: 'full',
    health: 'healthy',
    webRoot: '/home/web',
    diskBytes: 1,
    siteCount: 1,
    databaseCount: 1,
    certificateCount: 1,
    composeValid: true,
    nginxValid: true,
    resourceVersion: `sha256:${'a'.repeat(64)}`,
    scriptVersion: 'test',
    latestBackup: '',
    portConflicts: [],
    components: [],
    protection: { fail2ban: false, waf: false, cloudflare: false, ddos: false },
    optimization: { mode: 'custom', gzip: false, brotli: false, zstd: false },
    observedAt: '2026-07-28T00:00:00Z',
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.route.query = {}
  vi.useFakeTimers()
  vi.stubGlobal('window', {
    setTimeout: globalThis.setTimeout,
    clearTimeout: globalThis.clearTimeout,
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('EnvironmentView background jobs', () => {
  it('opens the protection section from a focused environment link', () => {
    mocks.route.query = { section: 'protection' }

    expect(setupView().section.value).toBe('protection')
  })

  it('distinguishes detection, mixed and unknown WAF state from blocking', () => {
    const view = setupView()
    view.summary.value = summary()
    view.summary.value.protection.wafMode = 'detect'
    expect(view.wafLabel.value).toBe('检测模式')
    expect(view.wafBadge.value).toBe('warning')
    view.summary.value.protection.wafMode = 'mixed'
    expect(view.wafLabel.value).toBe('按站点配置')
    view.summary.value.protection.wafMode = 'unknown'
    expect(view.wafBadge.value).toBe('unknown')
    view.summary.value.protection.wafMode = 'constructor' as WebEnvironmentSummary['protection']['wafMode']
    expect(view.wafLabel.value).toBe('状态待核验')
    expect(view.wafBadge.value).toBe('unknown')
    view.summary.value.protection.wafMode = 'on'
    expect(view.wafLabel.value).toBe('拦截模式')
    expect(view.wafBadge.value).toBe('healthy')
  })

  it('shows the running and cleanup state for each defense control', () => {
    const view = setupView()
    view.summary.value = summary()
    view.summary.value.protection.fail2banInstalled = true
    view.summary.value.protection.cloudflareStatus = {
      mode: 'cloudflare', configured: true, running: true, healthy: true,
      pending: true, cleanupPending: false, activeBans: 4,
    }
    view.summary.value.protection.cloudflareShield = { available: true, scheduled: true, scriptPresent: true }
    view.summary.value.protection.ddosRulesAvailable = true
    view.summary.value.protection.ddosRuleCount = 5
    view.summary.value.protection.ddosRuleExpected = 8
    view.summary.value.protection.banMigration = {
      state: 'legacy-imported', importedRecords: 12, apiJailEnabled: true,
      legacyCFDetected: true, cfReentryRequired: true, legacyReconcilerStopped: true,
    }

    expect(view.fail2banInstalled.value).toBe(true)
    expect(view.fail2banActionLabel.value).toBe('检查迁移并修复防护')
    expect(view.banMigrationLabel.value).toBe('旧封禁记录已迁移：12')
    expect(view.cloudflareLabel.value).toBe('等待同步')
    expect(view.cloudflareBadge.value).toBe('warning')
    expect(view.cloudflareShieldLabel.value).toBe('已安排')
    expect(view.cloudflareShieldBadge.value).toBe('healthy')
    expect(view.ddosLabel.value).toBe('规则未完整 (5/8)')
    expect(view.ddosBadge.value).toBe('warning')

    view.summary.value.protection.cloudflareStatus.cleanupPending = true
    expect(view.cloudflareLabel.value).toBe('关闭清理待重试')
    view.summary.value.protection.fail2ban = true
    expect(view.fail2banActionLabel.value).toBe('检查迁移并修复防护')
    view.summary.value.protection.cloudflareShield.scriptPresent = false
    expect(view.cloudflareShieldBadge.value).toBe('warning')
  })

  it('renders the environment summary without requesting heavy section data', async () => {
    vi.mocked(api.webEnvironment.summary).mockResolvedValueOnce(summary())
    vi.mocked(api.webEnvironment.jobs).mockImplementationOnce(() => new Promise(() => undefined))
    const view = setupView()

    await view.load()

    expect(view.summary.value).toEqual(summary())
    expect(view.error.value).toBe('')
    expect(api.webEnvironment.catalog).not.toHaveBeenCalled()
    expect(api.webEnvironment.backups).not.toHaveBeenCalled()
  })

  it('keeps environment management available when a lazy backup list fails', async () => {
    vi.mocked(api.webEnvironment.backups).mockRejectedValueOnce(new Error('backup unavailable'))
    const view = setupView()

    await view.loadBackups()

    expect(view.backups.value).toEqual([])
    expect(view.auxiliaryWarning.value).toBe('备份列表暂时无法读取')
  })

  it('always submits the current environment resource version and reopens the terminal', async () => {
    const job: WebEnvironmentJob = {
      id: 'b'.repeat(32),
      action: 'backup.create',
      target: 'web',
      status: 'running',
      stage: 'running',
      progress: 1,
      message: 'running',
      createdAt: '2026-07-28T00:00:00Z',
    }
    mocks.start.mockResolvedValueOnce(job)
    const view = setupView()
    view.summary.value = summary()

    await view.start({ action: 'backup.create' })

    expect(mocks.start).toHaveBeenCalledWith({
      action: 'backup.create',
      expectedResourceVersion: view.summary.value?.resourceVersion,
    })
    expect(view.jobs.value).toEqual([job])
    expect(view.terminalJob.value).toEqual(job)
    expect(view.terminalOpen.value).toBe(true)
    expect(mocks.toastSuccess).toHaveBeenCalledWith(
      '任务已转入后台',
      '关闭终端或刷新页面都不会中断执行。',
    )
  })

  it('does not restart job polling after lifecycle cleanup while a request is in flight', async () => {
    const job: WebEnvironmentJob = {
      id: 'c'.repeat(32),
      action: 'backup.create',
      target: 'web',
      status: 'running',
      stage: 'running',
      progress: 1,
      message: 'running',
      createdAt: '2026-07-28T00:00:00Z',
    }
    const pending = deferred<WebEnvironmentJob>()
    let requestSignal: AbortSignal | undefined
    mocks.start.mockResolvedValueOnce(job)
    vi.mocked(api.webEnvironment.job).mockImplementationOnce((_id, signal) => {
      requestSignal = signal
      return pending.promise
    })
    const view = setupView()
    view.summary.value = summary()
    await view.start({ action: 'backup.create' })

    await vi.advanceTimersByTimeAsync(1_200)
    expect(api.webEnvironment.job).toHaveBeenCalledTimes(1)

    view.stopJobPolling()
    expect(requestSignal?.aborted).toBe(true)
    pending.resolve({ ...job, progress: 20 })
    await Promise.resolve()
    await vi.advanceTimersByTimeAsync(2_300)

    expect(api.webEnvironment.job).toHaveBeenCalledTimes(1)
  })
})
