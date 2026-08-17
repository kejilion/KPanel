import { readFileSync } from 'node:fs'
import { createSSRApp, ref, ssrContextKey, type ComputedRef, type Ref } from 'vue'
import { renderToString, type SSRContext } from 'vue/server-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AppsView from './AppsView.vue'
import { ApiError } from '@/lib/api'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import type { AppInstallJob, AppMarketInventory, Site } from '@/types/api'

const mocks = vi.hoisted(() => ({
  createSite: vi.fn(),
  listSites: vi.fn(),
  removeSite: vi.fn(),
  inventory: vi.fn(),
  install: vi.fn(),
  installPort: vi.fn(),
  action: vi.fn(),
  checkUpdate: vi.fn(),
  job: vi.fn(),
  cancelJob: vi.fn(),
  publicNetwork: vi.fn(),
  toastSuccess: vi.fn(),
  toastDanger: vi.fn(),
  routerReplace: vi.fn(),
  reloadPanelInterface: vi.fn(),
  route: {
    name: 'apps',
    fullPath: '/apps',
    query: {} as Record<string, string>,
  },
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => mocks.route,
    useRouter: () => ({ replace: mocks.routerReplace }),
  }
})

vi.mock('@/lib/pageLifecycle', () => ({
  reloadPanelInterface: mocks.reloadPanelInterface,
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {
    readonly status: number
    readonly code: string

    constructor(message: string, status = 0, code = 'request_failed') {
      super(message)
      this.status = status
      this.code = code
    }
  },
  api: {
    apps: {
      inventory: mocks.inventory,
      install: mocks.install,
      installPort: mocks.installPort,
      action: mocks.action,
      checkUpdate: mocks.checkUpdate,
      job: mocks.job,
      cancelJob: mocks.cancelJob,
      jobs: vi.fn(),
    },
    sites: {
      create: mocks.createSite,
      list: mocks.listSites,
      remove: mocks.removeSite,
    },
    system: {
      publicNetwork: mocks.publicNetwork,
    },
  },
}))

vi.mock('@/stores/toast', () => ({
  useToast: () => ({
    success: mocks.toastSuccess,
    danger: mocks.toastDanger,
    show: vi.fn(),
  }),
}))

interface AppsBindings {
  inventory: Ref<AppMarketInventory | undefined>
  appSearchCatalog: ComputedRef<Array<{
    item: AppMarketInventory['items'][number]
    searchText: string
  }>>
  filteredApps: ComputedRef<AppMarketInventory['items']>
  selected: ComputedRef<AppMarketInventory['items'][number] | undefined>
  sites: Ref<Site[]>
  search: Ref<string>
  category: Ref<string>
  source: Ref<'all' | 'builtin' | 'thirdparty'>
  status: Ref<'all' | 'installed' | 'running' | 'adapted'>
  appGrid: Ref<HTMLElement | undefined>
  recentInstalledID: Ref<string>
  selectedID: Ref<string>
  installOpen: Ref<boolean>
  installPort: Ref<number>
  domain: Ref<string>
  domainError: Ref<string>
  domainWarning: Ref<string>
  sitesWarning: Ref<string>
  checkedUpdates: Ref<Record<string, 'available' | 'current'>>
  activeJob: Ref<AppInstallJob | undefined>
  jobDetailsOpen: Ref<boolean>
  confirmAction: Ref<'update' | 'uninstall' | undefined>
  cancelJobPending: Ref<boolean>
  load: (silent?: boolean) => Promise<void>
  openDetails: (item: AppMarketInventory['items'][number]) => void
  openInstall: (item: AppMarketInventory['items'][number]) => void
  install: () => Promise<void>
  lifecycle: (action: 'start' | 'stop' | 'restart') => Promise<void>
  checkUpdate: () => Promise<void>
  confirmMutation: () => Promise<void>
  refreshJob: (id: string) => Promise<void>
  startJobPolling: (job: AppInstallJob) => void
  syncJobPollingForWindow: (active: boolean) => void
  revealInstalledApp: (appID: string) => Promise<void>
  addDomain: () => Promise<void>
  removeDomain: (site: Site) => Promise<void>
  toggleAccess: () => Promise<void>
  openScriptManage: () => Promise<void>
  requestCancelJob: () => void
  confirmCancelJob: () => Promise<void>
  dismissJob: () => void
  consumeRouteIntent: () => Promise<void>
  refreshAfterSelfUpdate: (job: AppInstallJob) => boolean
}

describe('AppsView catalog filtering performance', () => {
  it('defers offscreen icon decoding and card rendering', () => {
    const source = readFileSync(new URL('./AppsView.vue', import.meta.url), 'utf8')

    expect(source).toContain('loading="lazy"')
    expect(source).toContain('decoding="async"')
    expect(source).toMatch(/\.app-card\s*\{[^}]*content-visibility:\s*auto;/)
    expect(source).toMatch(/\.app-card__body\s*\{[^}]*flex:\s*1 1 0;/)
    expect(source).toMatch(/\.app-card__title strong\s*\{[^}]*min-width:\s*0;[^}]*flex:\s*1 1 auto;/)
    expect(source).toMatch(/\.app-card__title \.status-badge\s*\{[^}]*flex:\s*0 0 auto;/)
    expect(source).toContain('v-if="isNewApp(item)" class="is-new">新品</em>')
  })

  it('keeps the compact inventory summary visually distinct', () => {
    const source = readFileSync(new URL('./AppsView.vue', import.meta.url), 'utf8')

    expect(source).toMatch(/\.app-market\s*\{[^}]*--success:\s*var\(--brand\);/)
    expect(source).toMatch(/\.app-card\.is-installed\s*\{[^}]*border-color:\s*color-mix\(in srgb, var\(--success\) 72%, var\(--border\)\);/)
    expect(source).toMatch(/\.market-hero\s*\{[^}]*overflow:\s*hidden;[^}]*radial-gradient\([^}]*var\(--market-accent\)/)
    expect(source).toMatch(/\.market-hero\s*\{[^}]*grid-template-columns:\s*max-content minmax\(0, 1fr\);/)
    expect(source).toMatch(/\.market-stats\s*\{[^}]*grid-template-columns:\s*repeat\(4, minmax\(116px, 132px\)\);/)
    expect(source).toMatch(/\.market-hero::before\s*\{[^}]*border:\s*30px solid color-mix\(in srgb, var\(--market-accent\) 7%, transparent\);/)
  })

  it('pauses the interactive terminal stream while its desktop window is inactive', () => {
    const source = readFileSync(new URL('./AppsView.vue', import.meta.url), 'utf8')
    expect(source).toContain('v-if="activeJob.interactive && windowActive"')
  })

  it('reuses the sorted search catalog while filters and queries change', () => {
    const view = setupView()
    const current = inventory('catalog-version')
    const first = current.items[0]
    if (!first) throw new Error('test inventory is incomplete')
    current.items.push({
      ...first,
      id: 'builtin-99',
      num: 99,
      token: 'notes',
      name_zh: 'Notes',
      name_en: 'Notes',
      desc_zh: '笔记服务',
      cat: 'productivity',
      runtime: { ...first.runtime, installed: false, state: 'unknown' },
    })
    view.inventory.value = current
    view.status.value = 'all'

    view.search.value = 'cloud'
    const catalog = view.appSearchCatalog.value
    expect(view.filteredApps.value.map((item) => item.id)).toEqual(['builtin-13'])
    expect(view.appSearchCatalog.value).toBe(catalog)

    view.search.value = ''
    view.category.value = 'productivity'
    expect(view.filteredApps.value.map((item) => item.id)).toEqual(['builtin-99'])
    expect(view.appSearchCatalog.value).toBe(catalog)
  })

  it('keeps installed apps first, then prioritizes valid additions for 60 UTC calendar days', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-16T12:00:00Z'))
    try {
      const view = setupView()
      const current = inventory('catalog-version')
      const installed = current.items[0]
      if (!installed) throw new Error('test inventory is incomplete')
      const uninstalled = { ...installed.runtime, installed: false, state: 'not_installed' }
      current.items.push(
        {
          ...installed,
          id: 'builtin-99',
          num: 99,
          token: 'newest',
          name_zh: 'Newest',
          name_en: 'Newest',
          addedAt: '2026-08-16',
          runtime: uninstalled,
        },
        {
          ...installed,
          id: 'builtin-98',
          num: 98,
          token: 'day-60',
          name_zh: 'Day 60',
          name_en: 'Day 60',
          addedAt: '2026-06-18',
          runtime: uninstalled,
        },
        {
          ...installed,
          id: 'builtin-1',
          num: 1,
          token: 'expired',
          name_zh: 'Expired',
          name_en: 'Expired',
          addedAt: '2026-06-17',
          runtime: uninstalled,
        },
        {
          ...installed,
          id: 'builtin-2',
          num: 2,
          token: 'future',
          name_zh: 'Future',
          name_en: 'Future',
          addedAt: '2026-08-17',
          runtime: uninstalled,
        },
        {
          ...installed,
          id: 'builtin-3',
          num: 3,
          token: 'invalid-date',
          name_zh: 'Invalid date',
          name_en: 'Invalid date',
          addedAt: '2026-02-29',
          runtime: uninstalled,
        },
      )
      view.inventory.value = current
      view.status.value = 'all'

      expect(view.filteredApps.value.map((item) => item.id)).toEqual([
        'builtin-13',
        'builtin-99',
        'builtin-98',
        'builtin-1',
        'builtin-2',
        'builtin-3',
      ])
    } finally {
      vi.useRealTimers()
    }
  })

})

function setupView(windowActive?: Ref<boolean>): AppsBindings {
  const component = AppsView as unknown as {
    setup: (props: Record<string, never>, context: { expose: () => void }) => AppsBindings
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

async function renderView(
  currentInventory: AppMarketInventory,
  selectedID: string,
): Promise<string> {
  const component = AppsView as unknown as {
    setup: (
      props: Record<string, never>,
      context: { expose: () => void },
    ) => AppsBindings
  }
  const renderable = {
    ...AppsView,
    setup(props: Record<string, never>, context: { expose: () => void }) {
      const bindings = component.setup(props, context)
      bindings.inventory.value = currentInventory
      bindings.selectedID.value = selectedID
      return bindings
    },
  }
  const context: SSRContext = {}
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  try {
    const html = await renderToString(
      createSSRApp(renderable as unknown as typeof AppsView),
      context,
    )
    return [html, ...Object.values(context.teleports || {})].join('')
  } finally {
    warn.mockRestore()
  }
}

function inventory(resourceVersion: string): AppMarketInventory {
  return {
    schemaVersion: 1,
    source: 'test',
    scriptSha256: 'a'.repeat(64),
    catalogMode: 'embedded',
    categories: [],
    installed: 1,
    running: 1,
    updateAvailable: 0,
    collectedAt: '2026-07-28T00:00:00Z',
    items: [
      {
        id: 'builtin-13',
        num: 13,
        source: 'builtin',
        token: 'cloudreve',
        name_zh: 'Cloudreve',
        name_en: 'Cloudreve',
        desc_zh: '云盘',
        desc_en: 'Cloud storage',
        cat: 'storage',
        icon: '',
        iconSha256: 'b'.repeat(64),
        slug: 'cloudreve',
        defaultPort: 5212,
        installPortConfigurable: true,
        installer: 'kejilion',
        runtime: {
          installed: true,
          state: 'running',
          ports: [{ privatePort: 5212, publicPort: 5212, ip: '0.0.0.0', type: 'tcp' }],
          accessMode: 'direct',
          updateStatus: 'check_required',
          resourceVersion,
          containerId: 'a'.repeat(64),
          detectedBy: ['container'],
        },
        capabilities: {
          add_domain: { enabled: true },
          check_update: { enabled: true },
          direct_access: { enabled: true },
          start: { enabled: false, reason: '当前状态不允许启动' },
          stop: { enabled: true },
          restart: { enabled: true },
          update: { enabled: true },
          uninstall: { enabled: true },
          manage: { enabled: false, reason: '已发现应用容器，请使用面板提供的生命周期操作' },
        },
      },
    ],
  }
}

function markerOnlyInventory(resourceVersion: string): AppMarketInventory {
  const result = inventory(resourceVersion)
  const current = result.items[0]
  if (!current) throw new Error('test inventory is incomplete')
  result.running = 0
  result.items[0] = {
    ...current,
    id: 'builtin-114',
    num: 114,
    token: 'openclaw',
    name_zh: 'OpenClaw',
    name_en: 'OpenClaw',
    runtime: {
      installed: true,
      state: 'unknown',
      ports: [],
      accessMode: 'not_applicable',
      updateStatus: 'unknown',
      resourceVersion,
      detectedBy: ['appno'],
      warning: 'kejilion.sh 安装标记存在，但 Docker Engine 中未发现运行产物',
    },
    capabilities: {
      add_domain: { enabled: false, reason: 'Docker Engine 中没有可执行生命周期操作的容器' },
      direct_access: { enabled: false, reason: 'Docker Engine 中没有可执行生命周期操作的容器' },
      manage: { enabled: true },
    },
  }
  return result
}

function kpanelInventory(resourceVersion: string): AppMarketInventory {
  const result = inventory(resourceVersion)
  const current = result.items[0]
  if (!current) throw new Error('test inventory is incomplete')
  result.items[0] = {
    ...current,
    id: 'thirdparty-kpanel',
    num: undefined,
    source: 'thirdparty',
    token: 'kpanel',
    name_zh: 'KPanel',
    name_en: 'KPanel',
    slug: 'kpanel',
  }
  return result
}

function proxySite(): Site {
  return {
    id: 'site-cloud',
    primaryDomain: 'cloud.example.com',
    domains: ['cloud.example.com'],
    type: 'proxy',
    enabled: true,
    health: 'healthy',
    consistency: 'synced',
    access: 'managed',
    source: 'panel',
    upstream: 'http://127.0.0.1:5212',
    resourceVersion: 'site-version',
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.route.name = 'apps'
  mocks.route.fullPath = '/apps'
  mocks.route.query = {}
  mocks.routerReplace.mockResolvedValue(undefined)
  const storage = new Map<string, string>()
  vi.stubGlobal('window', {
    localStorage: {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
      clear: () => storage.clear(),
    },
    setInterval: vi.fn(() => 1),
    clearInterval: vi.fn(),
    setTimeout: vi.fn(() => 1),
    clearTimeout: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    location: { hostname: 'localhost' },
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AppsView install port preflight', () => {
  it('returns to the installed application list and highlights the completed install', async () => {
    const view = setupView()
    const scrollIntoView = vi.fn()
    view.search.value = 'cloud'
    view.category.value = 'storage'
    view.source.value = 'thirdparty'
    view.status.value = 'all'
    view.appGrid.value = { scrollIntoView } as unknown as HTMLElement

    await view.revealInstalledApp('builtin-13')

    expect(view.search.value).toBe('')
    expect(view.category.value).toBe('all')
    expect(view.source.value).toBe('all')
    expect(view.status.value).toBe('installed')
    expect(view.recentInstalledID.value).toBe('builtin-13')
    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'smooth', block: 'start' })
  })

  it('passes an available script application port from the panel to the install job', async () => {
    const result = inventory('install-version')
    const item = result.items[0]
    if (!item) throw new Error('test inventory is incomplete')
    item.runtime = {
      installed: false,
      state: 'not_installed',
      ports: [],
      accessMode: 'not_applicable',
      updateStatus: 'not_installed',
      detectedBy: [],
    }
    item.capabilities.install = { enabled: true }
    const job: AppInstallJob = {
      id: 'a'.repeat(32),
      appId: item.id,
      appName: item.name_zh,
      action: 'install',
      interactive: true,
      status: 'queued',
      stage: 'queued',
      progress: 0,
      logs: [],
      createdAt: '2026-07-29T00:00:00Z',
    }
    mocks.installPort.mockResolvedValueOnce({
      port: 15212,
      available: true,
      conflicts: [],
      checkedAt: '2026-07-29T00:00:00Z',
    })
    mocks.install.mockResolvedValueOnce(job)
    mocks.job.mockResolvedValue(job)
    const view = setupView()
    view.inventory.value = result
    view.openInstall(item)
    view.installPort.value = 15212

    await view.install()

    expect(mocks.installPort).toHaveBeenCalledWith(item.id, 15212, expect.anything())
    expect(mocks.install).toHaveBeenCalledWith(item.id, {
      hostPort: 15212,
      accessMode: undefined,
    })
  })

  it('does not start installation when the host port is occupied', async () => {
    const result = inventory('install-version')
    const item = result.items[0]
    if (!item) throw new Error('test inventory is incomplete')
    item.runtime.installed = false
    item.runtime.state = 'not_installed'
    item.capabilities.install = { enabled: true }
    mocks.installPort.mockResolvedValueOnce({
      port: 5212,
      available: false,
      conflicts: [{ source: 'docker', protocol: 'tcp', container: 'existing-cloud' }],
      checkedAt: '2026-07-29T00:00:00Z',
    })
    const view = setupView()
    view.inventory.value = result
    view.openInstall(item)

    await view.install()

    expect(mocks.install).not.toHaveBeenCalled()
  })
})

describe('AppsView domain binding', () => {
  it('keeps a successful domain visible and uses the refreshed app version for access policy', async () => {
    const created = proxySite()
    mocks.createSite.mockResolvedValueOnce(created)
    mocks.inventory.mockResolvedValueOnce(inventory('fresh-version'))
    mocks.action.mockRejectedValueOnce(new Error('active task'))
    const view = setupView()
    view.inventory.value = inventory('stale-version')
    view.selectedID.value = 'builtin-13'
    view.domain.value = created.primaryDomain

    await view.addDomain()

    expect(mocks.action).toHaveBeenCalledWith('builtin-13', 'direct_access', {
      resourceVersion: 'fresh-version',
      accessMode: 'domain_only',
    })
    expect(view.sites.value).toEqual([created])
    expect(view.domain.value).toBe('')
    expect(view.domainError.value).toBe('')
    expect(view.domainWarning.value).toContain('域名已绑定，但 IP + 端口访问策略未调整')
    expect(mocks.toastSuccess).toHaveBeenCalledWith(
      '域名已绑定',
      'cloud.example.com 已生效；直接访问策略可稍后单独调整。',
    )
  })

  it('retries a transient site-list failure without hiding the bound domain', async () => {
    const created = proxySite()
    mocks.inventory.mockResolvedValueOnce(inventory('current-version'))
    mocks.listSites
      .mockRejectedValueOnce(new Error('temporary site read failure'))
      .mockResolvedValueOnce({ items: [created], total: 1 })
    mocks.publicNetwork.mockResolvedValueOnce(undefined)
    const view = setupView()

    await view.load(true)

    expect(mocks.listSites).toHaveBeenCalledTimes(2)
    expect(view.sites.value).toEqual([created])
    expect(view.sitesWarning.value).toBe('')
  })

  it('keeps the last successful domain snapshot when both site reads fail', async () => {
    const created = proxySite()
    mocks.inventory.mockResolvedValueOnce(inventory('current-version'))
    mocks.listSites.mockRejectedValue(new Error('site service unavailable'))
    mocks.publicNetwork.mockResolvedValueOnce(undefined)
    const view = setupView()
    view.sites.value = [created]

    await view.load(true)

    expect(mocks.listSites).toHaveBeenCalledTimes(2)
    expect(view.sites.value).toEqual([created])
    expect(view.sitesWarning.value).toContain('当前显示上次成功读取的结果')
  })

  it('refreshes a stale site version and retries domain removal once', async () => {
    const stale = proxySite()
    const fresh = { ...stale, resourceVersion: 'fresh-site-version' }
    mocks.removeSite
      .mockRejectedValueOnce(new ApiError('site resourceVersion changed', 409, 'resource_conflict'))
      .mockResolvedValueOnce({ primaryDomain: stale.primaryDomain })
    mocks.listSites
      .mockResolvedValueOnce({ items: [fresh], total: 1 })
      .mockResolvedValueOnce({ items: [], total: 0 })
    mocks.inventory.mockResolvedValueOnce(inventory('current-version'))
    mocks.publicNetwork.mockResolvedValueOnce(undefined)
    const view = setupView()
    view.inventory.value = inventory('current-version')
    view.selectedID.value = 'builtin-13'
    view.sites.value = [stale]

    await view.removeDomain(stale)

    expect(mocks.removeSite).toHaveBeenNthCalledWith(
      1,
      stale.id,
      'site-version',
      'configuration',
    )
    expect(mocks.removeSite).toHaveBeenNthCalledWith(
      2,
      fresh.id,
      'fresh-site-version',
      'configuration',
    )
    expect(view.domainError.value).toBe('')
  })

  it('refreshes a stale application version before changing direct access', async () => {
    const job: AppInstallJob = {
      id: 'access-job',
      appId: 'builtin-13',
      appName: 'Cloudreve',
      action: 'direct_access',
      status: 'queued',
      stage: 'queued',
      progress: 0,
      logs: [],
      createdAt: '2026-07-28T00:00:00Z',
    }
    mocks.action
      .mockRejectedValueOnce(new ApiError('container resourceVersion changed', 409, 'resource_conflict'))
      .mockResolvedValueOnce(job)
    mocks.inventory.mockResolvedValueOnce(inventory('fresh-version'))
    mocks.job.mockResolvedValue(job)
    const view = setupView()
    view.inventory.value = inventory('stale-version')
    view.selectedID.value = 'builtin-13'

    await view.toggleAccess()

    expect(mocks.action).toHaveBeenNthCalledWith(1, 'builtin-13', 'direct_access', {
      resourceVersion: 'stale-version',
      accessMode: 'domain_only',
    })
    expect(mocks.action).toHaveBeenNthCalledWith(2, 'builtin-13', 'direct_access', {
      resourceVersion: 'fresh-version',
      accessMode: 'domain_only',
    })
    expect(view.activeJob.value?.id).toBe('access-job')
  })
})

describe('AppsView update checks', () => {
  it('refreshes the inventory and retries a read-only resource conflict once', async () => {
    mocks.checkUpdate
      .mockRejectedValueOnce(new ApiError('container resourceVersion changed', 409, 'resource_conflict'))
      .mockResolvedValueOnce({
        containerId: 'a'.repeat(64),
        image: 'cloudreve/cloudreve:latest',
        status: 'current',
        updateAvailable: false,
        resourceVersion: 'fresh-version',
        checkedAt: '2026-07-28T00:00:00Z',
      })
    mocks.inventory.mockResolvedValueOnce(inventory('fresh-version'))
    const view = setupView()
    view.inventory.value = inventory('stale-version')
    view.selectedID.value = 'builtin-13'

    await view.checkUpdate()

    expect(mocks.checkUpdate).toHaveBeenNthCalledWith(1, 'builtin-13', 'stale-version')
    expect(mocks.checkUpdate).toHaveBeenNthCalledWith(2, 'builtin-13', 'fresh-version')
    expect(view.checkedUpdates.value['builtin-13']).toBe('current')
    expect(mocks.toastDanger).not.toHaveBeenCalled()
  })
})

describe('AppsView stopped applications', () => {
  it.each([
    ['exited', '已停止'],
    ['restarting', '重启中'],
    ['paused', '已暂停'],
  ])('renders %s application details when a legacy inventory returns null ports', async (state, label) => {
    const stopped = inventory('stopped-version')
    const item = stopped.items[0]
    if (!item) throw new Error('test inventory is incomplete')
    stopped.running = 0
    item.runtime.state = state
    item.runtime.status = state
    item.runtime.ports = null as unknown as AppMarketInventory['items'][number]['runtime']['ports']
    item.capabilities.start = { enabled: true }
    item.capabilities.stop = { enabled: false, reason: '当前状态不允许停止' }

    const html = await renderView(stopped, item.id)

    expect(html).toContain('role="dialog"')
    expect(html).toContain('aria-label="Cloudreve"')
    expect(html).toContain(label)
    expect(html).toContain('没有可用 HTTP 端口')
  })

  it('keeps a stopped application discoverable by leaving the running-only filter', async () => {
    const stopped = inventory('stopped-version')
    const item = stopped.items[0]
    if (!item) throw new Error('test inventory is incomplete')
    stopped.running = 0
    item.runtime.state = 'exited'
    item.runtime.status = 'exited'
    item.capabilities.start = { enabled: true }
    item.capabilities.stop = { enabled: false, reason: '当前状态不允许停止' }
    mocks.action.mockResolvedValueOnce({
      containerId: item.runtime.containerId,
      action: 'stop',
      status: 'completed',
      resourceVersion: 'stopped-version',
    })
    mocks.inventory.mockResolvedValueOnce(stopped)
    mocks.publicNetwork.mockResolvedValueOnce(undefined)
    const view = setupView()
    view.inventory.value = inventory('running-version')
    view.selectedID.value = 'builtin-13'
    view.status.value = 'running'

    await view.lifecycle('stop')

    expect(view.status.value).toBe('installed')
    expect(view.selectedID.value).toBe('builtin-13')
    expect(view.inventory.value?.items[0]?.runtime.state).toBe('exited')
    expect(view.inventory.value?.items[0]?.capabilities.update?.enabled).toBe(true)
    expect(view.inventory.value?.items[0]?.capabilities.uninstall?.enabled).toBe(true)

    view.selectedID.value = ''
    const stoppedItem = view.filteredApps.value[0]
    if (!stoppedItem) throw new Error('stopped application disappeared from the installed filter')
    view.openDetails(stoppedItem)

    expect(view.selected.value?.id).toBe('builtin-13')
    expect(view.selected.value?.runtime.state).toBe('exited')
    expect(view.selected.value?.capabilities.start?.enabled).toBe(true)
  })
})

describe('AppsView script management', () => {
  it('consumes a desktop manage intent and opens the interactive shell job', async () => {
    const job: AppInstallJob = {
      id: '0123456789abcdef0123456789abcdef',
      appId: 'builtin-114',
      appName: 'OpenClaw',
      action: 'manage',
      interactive: true,
      inputOpen: true,
      status: 'running',
      stage: 'interactive',
      progress: 5,
      logs: [],
      createdAt: '2026-07-28T00:00:00Z',
    }
    mocks.action.mockResolvedValueOnce(job)
    mocks.job.mockResolvedValue(job)
    const view = setupView()
    view.inventory.value = markerOnlyInventory('marker:sha256:fresh-version')
    mocks.route.query = { app: 'builtin-114', action: 'manage' }
    mocks.route.fullPath = '/apps?app=builtin-114&action=manage'

    await view.consumeRouteIntent()

    expect(view.selected.value?.id).toBe('builtin-114')
    expect(mocks.action).toHaveBeenCalledWith('builtin-114', 'manage', {
      resourceVersion: 'marker:sha256:fresh-version',
    })
    expect(view.activeJob.value).toEqual(job)
    expect(view.jobDetailsOpen.value).toBe(true)
    expect(mocks.routerReplace).toHaveBeenCalledWith({ query: {} })
  })

  it('opens the fixed-selector interactive management job with the current resource version', async () => {
    const job: AppInstallJob = {
      id: '0123456789abcdef0123456789abcdef',
      appId: 'builtin-114',
      appName: 'OpenClaw',
      action: 'manage',
      interactive: true,
      inputOpen: true,
      status: 'running',
      stage: 'interactive',
      progress: 5,
      logs: [],
      createdAt: '2026-07-28T00:00:00Z',
    }
    mocks.action.mockResolvedValueOnce(job)
    mocks.job.mockResolvedValue(job)
    const view = setupView()
    view.inventory.value = markerOnlyInventory('marker:sha256:fresh-version')
    view.selectedID.value = 'builtin-114'

    await view.openScriptManage()

    expect(mocks.action).toHaveBeenCalledWith('builtin-114', 'manage', {
      resourceVersion: 'marker:sha256:fresh-version',
    })
    expect(view.activeJob.value).toEqual(job)
    expect(view.jobDetailsOpen.value).toBe(true)
    expect(mocks.toastSuccess).toHaveBeenCalledWith(
      '脚本管理终端已打开',
      'OpenClaw 正在使用固定应用编号进入 kejilion.sh 原生菜单。',
    )
  })

  it('does not open recovery-only script management for an application with a container', async () => {
    const view = setupView()
    view.inventory.value = inventory('fresh-version')
    view.selectedID.value = 'builtin-13'

    await view.openScriptManage()

    expect(mocks.action).not.toHaveBeenCalled()
  })

  it('keeps the active update job across a transient Panel restart', async () => {
    const job: AppInstallJob = {
      id: 'fedcba9876543210fedcba9876543210',
      appId: 'builtin-13',
      appName: 'KPanel',
      action: 'update',
      status: 'running',
      stage: 'executing',
      progress: 25,
      logs: [],
      createdAt: '2026-07-28T00:00:00Z',
    }
    const view = setupView()
    view.activeJob.value = job
    window.localStorage.setItem('kpanel:active-app-job', job.id)
    mocks.job.mockRejectedValueOnce(new ApiError('Panel restarting', 503, 'request_failed'))

    await view.refreshJob(job.id)

    expect(view.activeJob.value).toEqual(job)
    expect(window.localStorage.getItem('kpanel:active-app-job')).toBe(job.id)
  })

  it('throttles background job polling and refreshes immediately when the window is focused', async () => {
    const job: AppInstallJob = {
      id: 'b'.repeat(32),
      appId: 'builtin-13',
      appName: 'Cloudreve',
      action: 'update',
      status: 'running',
      stage: 'executing',
      progress: 25,
      logs: [],
      createdAt: '2026-07-28T00:00:00Z',
    }
    const windowActive = ref(false)
    mocks.job.mockResolvedValue(job)
    const view = setupView(windowActive)

    view.startJobPolling(job)

    expect(mocks.job).not.toHaveBeenCalled()
    expect(window.setTimeout).toHaveBeenLastCalledWith(expect.any(Function), 15_000)

    view.syncJobPollingForWindow(true)
    await Promise.resolve()

    expect(mocks.job).toHaveBeenCalledOnce()
  })

  it('clears an update job only when the persisted job no longer exists', async () => {
    const job: AppInstallJob = {
      id: '0123456789abcdef0123456789abcdef',
      appId: 'builtin-13',
      appName: 'KPanel',
      action: 'update',
      status: 'running',
      stage: 'executing',
      progress: 25,
      logs: [],
      createdAt: '2026-07-28T00:00:00Z',
    }
    const view = setupView()
    view.activeJob.value = job
    window.localStorage.setItem('kpanel:active-app-job', job.id)
    mocks.job.mockRejectedValueOnce(new ApiError('job not found', 404, 'not_found'))

    await view.refreshJob(job.id)

    expect(view.activeJob.value).toBeUndefined()
    expect(window.localStorage.getItem('kpanel:active-app-job')).toBeNull()
  })

  it('ends only an active interactive job and keeps polling until systemd releases it', async () => {
    const job: AppInstallJob = {
      id: '0123456789abcdef0123456789abcdef',
      appId: 'builtin-114',
      appName: 'OpenClaw',
      action: 'manage',
      interactive: true,
      inputOpen: true,
      status: 'running',
      stage: 'interactive',
      progress: 5,
      logs: [],
      createdAt: '2026-07-28T00:00:00Z',
    }
    const cancelling: AppInstallJob = {
      ...job,
      inputOpen: false,
      stage: 'cancelling',
      message: '正在结束 kejilion.sh 交互任务，请等待后台进程安全退出',
    }
    mocks.cancelJob.mockResolvedValueOnce(cancelling)
    mocks.job.mockResolvedValue(cancelling)
    const view = setupView()
    view.activeJob.value = job

    view.requestCancelJob()
    expect(view.cancelJobPending.value).toBe(true)
    await view.confirmCancelJob()

    expect(mocks.cancelJob).toHaveBeenCalledWith(job.id)
    expect(view.cancelJobPending.value).toBe(false)
    expect(view.activeJob.value?.stage).toBe('cancelling')
    expect(window.localStorage.getItem('kpanel:active-app-job')).toBe(job.id)
  })

  it('dismisses a finished task record without changing a running task', () => {
    const view = setupView()
    const finished: AppInstallJob = {
      id: '0123456789abcdef0123456789abcdef',
      appId: 'builtin-114',
      appName: 'OpenClaw',
      action: 'manage',
      interactive: true,
      status: 'cancelled',
      stage: 'cancelled',
      progress: 100,
      logs: [],
      createdAt: '2026-07-28T00:00:00Z',
    }
    view.activeJob.value = finished
    window.localStorage.setItem('kpanel:active-app-job', finished.id)
    view.dismissJob()
    expect(view.activeJob.value).toBeUndefined()
    expect(window.localStorage.getItem('kpanel:active-app-job')).toBeNull()

    view.activeJob.value = { ...finished, status: 'running', stage: 'interactive' }
    view.dismissJob()
    expect(view.activeJob.value?.status).toBe('running')
  })
})

describe('AppsView application mutations', () => {
  it('opens the requested application detail from a desktop route intent', async () => {
    const view = setupView()
    view.inventory.value = inventory('detail-version')
    mocks.route.query = { app: 'builtin-13' }
    mocks.route.fullPath = '/apps?app=builtin-13'

    await view.consumeRouteIntent()

    expect(view.selected.value?.id).toBe('builtin-13')
    expect(view.confirmAction.value).toBeUndefined()
    expect(mocks.routerReplace).toHaveBeenCalledWith({ query: {} })
  })

  it('opens the existing KPanel update confirmation from the sidebar intent', async () => {
    const view = setupView()
    view.inventory.value = kpanelInventory('kpanel-version')
    mocks.route.query = { app: 'kpanel', action: 'update' }
    mocks.route.fullPath = '/apps?app=kpanel&action=update'

    await view.consumeRouteIntent()

    expect(view.selected.value?.id).toBe('thirdparty-kpanel')
    expect(view.confirmAction.value).toBe('update')
    expect(mocks.routerReplace).toHaveBeenCalledWith({ query: {} })
  })

  it('schedules an interface reload only after a successful KPanel self update', () => {
    const view = setupView()
    const job: AppInstallJob = {
      id: '89abcdef0123456789abcdef01234567',
      appId: 'thirdparty-kpanel',
      appName: 'KPanel',
      action: 'update',
      status: 'succeeded',
      stage: 'completed',
      progress: 100,
      logs: [],
      createdAt: '2026-07-31T00:00:00Z',
    }

    expect(view.refreshAfterSelfUpdate(job)).toBe(true)
    expect(window.setTimeout).toHaveBeenCalledWith(mocks.reloadPanelInterface, 600)
    expect(view.refreshAfterSelfUpdate({ ...job, appId: 'builtin-13' })).toBe(false)
  })

  it('refreshes inventory and retries an update once when the container version changed', async () => {
    const job: AppInstallJob = {
      id: '89abcdef0123456789abcdef01234567',
      appId: 'builtin-13',
      appName: 'Cloudreve',
      action: 'update',
      status: 'queued',
      stage: 'queued',
      progress: 0,
      logs: [],
      createdAt: '2026-07-28T00:00:00Z',
    }
    mocks.action
      .mockRejectedValueOnce(new ApiError('application state changed', 409, 'resource_conflict'))
      .mockResolvedValueOnce(job)
    mocks.inventory.mockResolvedValueOnce(inventory('fresh-version'))
    mocks.job.mockResolvedValue(job)
    const view = setupView()
    view.inventory.value = inventory('stale-version')
    view.selectedID.value = 'builtin-13'
    view.confirmAction.value = 'update'

    await view.confirmMutation()

    expect(mocks.action).toHaveBeenNthCalledWith(1, 'builtin-13', 'update', {
      resourceVersion: 'stale-version',
    })
    expect(mocks.action).toHaveBeenNthCalledWith(2, 'builtin-13', 'update', {
      resourceVersion: 'fresh-version',
    })
    expect(view.activeJob.value).toEqual(job)
    expect(view.confirmAction.value).toBeUndefined()
    expect(mocks.toastDanger).not.toHaveBeenCalled()
  })
})
