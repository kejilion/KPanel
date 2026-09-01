// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AppsView from './AppsView.vue'
import SitesView from './SitesView.vue'
import { resetLocaleForTest, setLocale } from '@/i18n'
import appsEnglishCatalog from '@/i18n/pages/AppsView/en-US'
import sharedEnglishCatalog from '@/i18n/pages/shared/en-US'
import sitesEnglishCatalog from '@/i18n/pages/SitesView/en-US'
import { registerPhraseCatalog, resetPhraseLocalizationForTest } from '@/i18n/phrase'
import type { AppMarketInventory, AppMarketItem } from '@/types/api'

const mocks = vi.hoisted(() => ({
  agentCapabilities: vi.fn(),
  publicNetwork: vi.fn(),
  portUsage: vi.fn(),
  siteList: vi.fn(),
  siteInstallations: vi.fn(),
  siteInstallation: vi.fn(),
  siteCreate: vi.fn(),
  siteUpdate: vi.fn(),
  siteRemove: vi.fn(),
  siteTerminal: vi.fn(),
  siteTerminalInput: vi.fn(),
  appInventory: vi.fn(),
  appJobs: vi.fn(),
  appJob: vi.fn(),
  appAction: vi.fn(),
  appCheckUpdate: vi.fn(),
  appInstall: vi.fn(),
  appInstallPort: vi.fn(),
  appCancelJob: vi.fn(),
  terminalOpen: vi.fn(),
  terminalInput: vi.fn(),
  terminalClose: vi.fn(),
  setAgent: vi.fn(),
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

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {},
  isTransientAgentError: () => false,
  api: {
    agent: { capabilities: mocks.agentCapabilities },
    system: {
      publicNetwork: mocks.publicNetwork,
      portUsage: mocks.portUsage,
    },
    sites: {
      list: mocks.siteList,
      installations: mocks.siteInstallations,
      installation: mocks.siteInstallation,
      create: mocks.siteCreate,
      update: mocks.siteUpdate,
      remove: mocks.siteRemove,
      terminal: mocks.siteTerminal,
      terminalInput: mocks.siteTerminalInput,
    },
    apps: {
      inventory: mocks.appInventory,
      jobs: mocks.appJobs,
      job: mocks.appJob,
      action: mocks.appAction,
      checkUpdate: mocks.appCheckUpdate,
      install: mocks.appInstall,
      installPort: mocks.appInstallPort,
      cancelJob: mocks.appCancelJob,
    },
    terminals: {
      open: mocks.terminalOpen,
      input: mocks.terminalInput,
      close: mocks.terminalClose,
    },
  },
}))

vi.mock('@/stores/panel', () => ({
  usePanelState: () => ({
    isReadOnly: { value: false },
    setAgent: mocks.setAgent,
  }),
}))

vi.mock('@/stores/toast', () => ({
  useToast: () => ({
    success: mocks.toastSuccess,
    danger: mocks.toastDanger,
    show: vi.fn(),
  }),
}))

vi.mock('@/lib/pageLifecycle', () => ({
  reloadPanelInterface: mocks.reloadPanelInterface,
}))

const viewStubs = {
  PageHeader: true,
  RouterLink: true,
  LoadingState: true,
  ErrorState: true,
  EmptyState: true,
  StatusBadge: true,
  MetricCard: true,
  CountryFlagIcon: true,
  SitesSectionTabs: true,
  SiteFavicon: true,
  SiteAppearanceName: true,
  SiteDeleteDialog: true,
  HostTerminal: true,
  AppInteractiveTerminal: true,
}

const mountedWrappers: Array<{ unmount: () => void }> = []

function registerEnglishCatalog(catalog: readonly (readonly [string, string])[]): void {
  registerPhraseCatalog(sharedEnglishCatalog)
  registerPhraseCatalog(catalog)
}

function appItem(): AppMarketItem {
  const enabled = { enabled: true }
  return {
    id: 'builtin-kpanel',
    num: 1,
    source: 'builtin',
    token: 'kpanel',
    name_zh: 'KPanel',
    name_en: 'KPanel',
    desc_zh: '面板管理',
    desc_en: 'Server control panel',
    cat: 'panel',
    icon: 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"%3E%3C/svg%3E',
    iconSha256: 'sha256:test',
    slug: 'kpanel',
    installer: 'declarative',
    runtime: {
      installed: true,
      state: 'running',
      status: 'Running',
      ports: [{ privatePort: 3000, publicPort: 3000, ip: '127.0.0.1', type: 'tcp' }],
      accessMode: 'direct',
      updateStatus: 'current',
      resourceVersion: 'resource-version',
      detectedBy: ['test'],
    },
    capabilities: {
      manage: enabled,
      start: enabled,
      stop: enabled,
      restart: enabled,
      check_update: enabled,
      update: enabled,
      uninstall: enabled,
      direct_access: enabled,
      add_domain: enabled,
      install: enabled,
    },
  }
}

function appInventory(item: AppMarketItem): AppMarketInventory {
  return {
    schemaVersion: 1,
    source: 'test',
    scriptSha256: 'sha256:test',
    catalogMode: 'embedded',
    categories: [{ key: 'panel', zh: '面板运维', zh_tw: '面板運維', en: 'Panel operations' }],
    items: [item],
    installed: 1,
    running: 1,
    updateAvailable: 0,
    collectedAt: '2026-09-01T08:00:00Z',
  }
}

beforeEach(async () => {
  vi.clearAllMocks()
  resetLocaleForTest()
  await setLocale('en-US', false)
  resetPhraseLocalizationForTest()
  mocks.route.query = {}
  mocks.agentCapabilities.mockResolvedValue([
    { id: 'sites.write', enabled: true },
    { id: 'sites.wordpress.install', enabled: true },
    { id: 'sites.proxy.install', enabled: true },
    { id: 'sites.recipes.install', enabled: true },
    { id: 'sites.templates.install', enabled: true },
    { id: 'system.port-usage.read', enabled: true },
  ])
  mocks.publicNetwork.mockResolvedValue({ ipv4: '198.51.100.10' })
  mocks.portUsage.mockResolvedValue({ available: true, total: 0, items: [] })
  mocks.siteList.mockResolvedValue({ items: [], total: 0 })
  mocks.siteInstallations.mockResolvedValue([])
  mocks.appInventory.mockResolvedValue(appInventory(appItem()))
  mocks.appJobs.mockResolvedValue({ items: [] })
})

afterEach(() => {
  for (const wrapper of mountedWrappers.splice(0)) wrapper.unmount()
  document.body.innerHTML = ''
  resetPhraseLocalizationForTest()
  resetLocaleForTest()
})

describe('portal English localization', () => {
  it('localizes the new website dialog and its DNS guide', async () => {
    registerEnglishCatalog(sitesEnglishCatalog)
    const wrapper = mount(SitesView, {
      attachTo: document.body,
      global: { stubs: viewStubs },
    })
    mountedWrappers.push(wrapper)
    await flushPromises()

    ;(wrapper.vm as unknown as { openCreate: () => void }).openCreate()
    await flushPromises()

    const panel = document.body.querySelector<HTMLElement>('.modal-panel')
    expect(panel).not.toBeNull()
    const text = panel?.textContent || ''
    expect(text).toContain('New Website')
    expect(text).toContain('Main Domain Name')
    expect(text).toContain('Popular builds')
    expect(text).toContain('Set up WordPress in one click')
    expect(text).toContain('Domain resolution first')
    expect(text).toContain('Open DNS Console')
    expect(text).toContain('Cancel')
    expect(text).not.toContain('新建网站')
    expect(text).not.toContain('主域名')
    expect(text).not.toContain('热门搭建')
    expect(text).not.toContain('一键搭建 WordPress')
  })

  it('localizes the app marketplace details dialog', async () => {
    registerEnglishCatalog(appsEnglishCatalog)
    const wrapper = mount(AppsView, {
      attachTo: document.body,
      global: { stubs: viewStubs },
    })
    mountedWrappers.push(wrapper)
    await flushPromises()

    const item = appItem()
    ;(wrapper.vm as unknown as { openDetails: (value: AppMarketItem) => void }).openDetails(item)
    await flushPromises()

    const panel = document.body.querySelector<HTMLElement>('.modal-panel')
    expect(panel).not.toBeNull()
    const text = panel?.textContent || ''
    expect(text).toContain('Runtime status')
    expect(text).toContain('Script Management')
    expect(text).toContain('Start')
    expect(text).toContain('Stop')
    expect(text).toContain('Restart')
    expect(text).toContain('Check for updates')
    expect(text).toContain('Open app')
    expect(text).toContain('Domain access')
    expect(text).toContain('Add Domain Name')
    expect(text).toContain('Bind')
    expect(text).toContain('IP + port access')
    expect(text).toContain('Direct access allowed')
    expect(text).toContain('Block')
    expect(text).toContain('Maintenance and uninstall')
    expect(text).toContain('Update')
    expect(text).toContain('Uninstall')
    expect(text).not.toContain('运行状态')
    expect(text).not.toContain('脚本管理')
    expect(text).not.toContain('检查更新')
    expect(text).not.toContain('打开应用')
    expect(text).not.toContain('维护与卸载')
  })
})
