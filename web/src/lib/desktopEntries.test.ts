// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AppMarketItem, PublicNetworkSummary, Site } from '@/types/api'
import { clearDesktopEntriesCacheForTest, loadDesktopEntries } from './desktopEntries'
import { api } from '@/lib/api'

vi.mock('@/lib/api', () => ({
  api: {
    apps: {
      inventory: vi.fn(),
    },
    sites: {
      list: vi.fn(),
      iconURL: (id: string) => `/api/v1/sites/${id}/icon`,
    },
    system: {
      publicNetwork: vi.fn(),
    },
  },
}))

function makeSite(overrides: Partial<Site> & { id: string; primaryDomain: string }): Site {
  return {
    domains: [overrides.primaryDomain],
    type: 'proxy',
    enabled: true,
    health: 'healthy',
    consistency: 'synced',
    access: 'managed',
    source: 'panel',
    resourceVersion: 'v1',
    ...overrides,
  }
}

function inventory(items: AppMarketItem[] = []) {
  return {
    schemaVersion: 1,
    source: 'catalog',
    scriptSha256: '',
    catalogMode: 'cached' as const,
    collectedAt: '2026-01-01T00:00:00.000Z',
    items,
    installed: items.filter((i) => i.runtime.installed).length,
    running: 0,
    updateAvailable: 0,
    categories: [],
  }
}

function makeApp(overrides: Partial<AppMarketItem> & { id: string }): AppMarketItem {
  return {
    num: 1,
    source: 'builtin',
    token: 't',
    name_zh: overrides.id,
    name_en: overrides.id,
    desc_zh: '',
    desc_en: '',
    cat: 'dev',
    url: 'http://example.com',
    icon: `/api/v1/apps/${overrides.id}/icon`,
    iconSha256: 'sha',
    slug: overrides.id,
    defaultPort: 80,
    installPortConfigurable: false,
    installer: 'declarative',
    runtime: {
      installed: true,
      state: 'running',
      ports: [{ privatePort: 80, publicPort: 8080, type: 'tcp' }],
      accessMode: 'direct',
      updateStatus: 'current',
      detectedBy: [],
    },
    capabilities: {},
    ...overrides,
  }
}

describe('desktop entries', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearDesktopEntriesCacheForTest()
  })

  it('loads installed apps as entries with entry URLs', async () => {
    const app = makeApp({ id: 'nginx', runtime: { installed: true, state: 'running', ports: [{ privatePort: 80, publicPort: 8080, type: 'tcp' }], accessMode: 'direct', updateStatus: 'current', detectedBy: [] } })
    vi.mocked(api.apps.inventory).mockResolvedValue(inventory([app]))
    vi.mocked(api.sites.list).mockResolvedValue({ items: [], total: 0 })

    const entries = await loadDesktopEntries(undefined, '192.168.1.5')
    expect(entries.apps).toHaveLength(1)
    expect(entries.apps[0]!.kind).toBe('app')
    expect(entries.apps[0]!.url).toContain('8080')
    expect(entries.visible).toHaveLength(1)
  })

  it('starts inventory and sites while public network discovery is still pending', async () => {
    let release!: (value: PublicNetworkSummary) => void
    vi.mocked(api.system.publicNetwork).mockReturnValueOnce(new Promise(resolve => { release = resolve }))
    vi.mocked(api.apps.inventory).mockResolvedValue(inventory([makeApp({ id: 'nginx' })]))
    vi.mocked(api.sites.list).mockResolvedValue({ items: [], total: 0 })
    const pending = loadDesktopEntries()
    expect(api.apps.inventory).toHaveBeenCalledOnce()
    expect(api.sites.list).toHaveBeenCalledOnce()
    release({ ipv4: '192.168.1.5' })
    expect((await pending).apps[0]?.url).toBe('http://192.168.1.5:8080')
  })

  it('keeps KPanel manageable but hides its redundant desktop self-entry', async () => {
    const kpanel = makeApp({
      id: 'thirdparty-kpanel',
      token: 'kpanel',
      name_zh: 'KPanel',
      runtime: {
        installed: true,
        state: 'running',
        ports: [{ privatePort: 8080, publicPort: 8080, type: 'tcp' }],
        accessMode: 'direct',
        updateStatus: 'current',
        detectedBy: [],
      },
    })
    vi.mocked(api.apps.inventory).mockResolvedValue(inventory([kpanel]))
    vi.mocked(api.sites.list).mockResolvedValue({ items: [], total: 0 })

    const entries = await loadDesktopEntries(undefined, '192.168.1.5')

    expect(entries.apps).toHaveLength(1)
    expect(entries.apps[0]?.id).toBe('thirdparty-kpanel')
    expect(entries.visible).toHaveLength(0)
  })

  it('does not replace hidden KPanel with its matching proxy site', async () => {
    const kpanel = makeApp({
      id: 'thirdparty-kpanel',
      token: 'kpanel',
      runtime: {
        installed: true,
        state: 'running',
        ports: [{ privatePort: 8080, publicPort: 8080, type: 'tcp' }],
        accessMode: 'direct',
        updateStatus: 'current',
        detectedBy: [],
      },
    })
    const panelSite = makeSite({
      id: 'panel-site',
      primaryDomain: 'panel.example.com',
      upstream: 'http://127.0.0.1:8080',
      certificate: { status: 'valid' },
    })
    const blog = makeSite({
      id: 'blog',
      primaryDomain: 'blog.example.com',
      type: 'static',
    })
    vi.mocked(api.apps.inventory).mockResolvedValue(inventory([kpanel]))
    vi.mocked(api.sites.list).mockResolvedValue({ items: [panelSite, blog], total: 2 })

    const entries = await loadDesktopEntries(undefined, '192.168.1.5')

    expect(entries.visible.map((entry) => entry.id)).toEqual(['blog'])
  })

  it('skips installed apps without a reachable URL', async () => {
    const app = makeApp({ id: 'noaccess', runtime: { installed: true, state: 'stopped', ports: [], accessMode: 'domain_only', updateStatus: 'current', detectedBy: [] } })
    vi.mocked(api.apps.inventory).mockResolvedValue(inventory([app]))
    vi.mocked(api.sites.list).mockResolvedValue({ items: [], total: 0 })

    const entries = await loadDesktopEntries(undefined, '192.168.1.5')
    expect(entries.apps).toHaveLength(0)
    expect(entries.visible).toHaveLength(0)
  })

  it('loads an installed script-managed app even when it has no web URL', async () => {
    const app = makeApp({
      id: 'openclaw',
      runtime: {
        installed: true,
        state: 'unknown',
        ports: [],
        accessMode: 'not_applicable',
        updateStatus: 'unknown',
        resourceVersion: 'marker:sha256:version',
        detectedBy: ['appno'],
      },
      capabilities: { manage: { enabled: true } },
    })
    vi.mocked(api.apps.inventory).mockResolvedValue(inventory([app]))
    vi.mocked(api.sites.list).mockResolvedValue({ items: [], total: 0 })

    const entries = await loadDesktopEntries(undefined, '192.168.1.5')
    expect(entries.apps).toHaveLength(1)
    expect(entries.apps[0]).toMatchObject({ id: 'openclaw', launch: 'script', url: undefined })
    expect(entries.visible.map((entry) => entry.id)).toEqual(['openclaw'])
  })

  it('loads configured sites as site entries', async () => {
    const site = makeSite({ id: 's1', primaryDomain: 'example.com', type: 'proxy', upstream: 'http://127.0.0.1:8081', enabled: true, health: 'healthy' })
    vi.mocked(api.apps.inventory).mockResolvedValue(inventory([]))
    vi.mocked(api.sites.list).mockResolvedValue({ items: [site], total: 1 })

    const entries = await loadDesktopEntries(undefined, '192.168.1.5')
    expect(entries.sites).toHaveLength(1)
    expect(entries.sites[0]!.kind).toBe('site')
    expect(entries.sites[0]!.name).toBe('example.com')
    expect(entries.sites[0]!.url).toBe('http://example.com')
    expect(entries.sites[0]!.iconURL).toContain('s1/icon')
  })

  it('keeps enabled warning and unknown sites visible while excluding disabled sites', async () => {
    const disabled = makeSite({ id: 'd', primaryDomain: 'disabled.com', enabled: false, health: 'healthy', upstream: 'http://127.0.0.1:1' })
    const unhealthy = makeSite({ id: 'u', primaryDomain: 'unhealthy.com', enabled: true, health: 'warning', upstream: 'http://127.0.0.1:2' })
    const noUpstreamProxy = makeSite({ id: 'n', primaryDomain: 'noup.com', enabled: true, health: 'healthy', upstream: undefined })
    // Static sites are reachable without an upstream.
    const staticSite = makeSite({ id: 'st', primaryDomain: 'static.com', type: 'static', enabled: true, health: 'healthy', upstream: undefined })
    vi.mocked(api.apps.inventory).mockResolvedValue(inventory([]))
    vi.mocked(api.sites.list).mockResolvedValue({ items: [disabled, unhealthy, noUpstreamProxy, staticSite], total: 4 })

    const entries = await loadDesktopEntries(undefined, '192.168.1.5')
    expect(entries.sites).toHaveLength(3)
    expect(entries.sites.map((entry) => entry.id)).toEqual(['u', 'n', 'st'])
  })

  it('dedupes an app and a site pointing at the same URL, keeping the app', async () => {
    // The app listens on public port 8080 and a proxy site forwards to it, so
    // appAccessURL resolves to https://apps.example.com. The site entry for the
    // same domain resolves to the same URL, so only the app entry survives.
    const app = makeApp({
      id: 'panel',
      runtime: { installed: true, state: 'running', ports: [{ privatePort: 80, publicPort: 8080, type: 'tcp' }], accessMode: 'direct', updateStatus: 'current', detectedBy: [] },
    })
    const proxySite = makeSite({
      id: 'p',
      primaryDomain: 'apps.example.com',
      type: 'proxy',
      upstream: 'http://127.0.0.1:8080',
      enabled: true,
      health: 'healthy',
      certificate: { status: 'valid' },
    })
    // The same domain as a site entry, matching the app's resolved URL.
    const site = makeSite({
      id: 's',
      primaryDomain: 'apps.example.com',
      type: 'proxy',
      upstream: 'http://127.0.0.1:9999',
      enabled: true,
      health: 'healthy',
      certificate: { status: 'valid' },
    })
    vi.mocked(api.apps.inventory).mockResolvedValue(inventory([app]))
    vi.mocked(api.sites.list).mockResolvedValue({ items: [proxySite, site], total: 2 })

    const entries = await loadDesktopEntries(undefined, '192.168.1.5')
    // App resolves through its proxy site to https://apps.example.com.
    expect(entries.apps[0]!.url).toBe('https://apps.example.com')
    // Two configured sites on the same domain.
    expect(entries.sites).toHaveLength(2)
    expect(entries.sites[0]!.url).toBe('https://apps.example.com')
    // The app and both sites share one URL → only the app entry is visible.
    expect(entries.visible).toHaveLength(1)
    expect(entries.visible[0]!.kind).toBe('app')
    expect(entries.visible[0]!.id).toBe('panel')
  })

  it('returns empty visible set when both sources fail', async () => {
    vi.mocked(api.apps.inventory).mockRejectedValue(new Error('offline'))
    vi.mocked(api.sites.list).mockRejectedValue(new Error('offline'))

    const entries = await loadDesktopEntries(undefined, '192.168.1.5')
    expect(entries.apps).toHaveLength(0)
    expect(entries.sites).toHaveLength(0)
    expect(entries.visible).toHaveLength(0)
  })

  it('keeps unrelated sites when one site shares an app URL', async () => {
    const app = makeApp({
      id: 'panel',
      runtime: { installed: true, state: 'running', ports: [{ privatePort: 80, publicPort: 8080, type: 'tcp' }], accessMode: 'direct', updateStatus: 'current', detectedBy: [] },
    })
    const appSite = makeSite({ id: 'app-site', primaryDomain: 'app.example.com', upstream: 'http://127.0.0.1:8080', certificate: { status: 'valid' } })
    const blog = makeSite({ id: 'blog', primaryDomain: 'blog.example.com', upstream: undefined, health: 'warning' })
    const docs = makeSite({ id: 'docs', primaryDomain: 'docs.example.com', type: 'unknown', upstream: undefined, health: 'unknown' })
    vi.mocked(api.apps.inventory).mockResolvedValue(inventory([app]))
    vi.mocked(api.sites.list).mockResolvedValue({ items: [appSite, blog, docs], total: 3 })

    const entries = await loadDesktopEntries(undefined, '192.168.1.5')
    expect(entries.visible.map((entry) => entry.id)).toEqual(['panel', 'blog', 'docs'])
  })

  it('hides every proxy site backed by an installed app when domains differ', async () => {
    const app = makeApp({
      id: 'panel',
      runtime: { installed: true, state: 'running', ports: [{ privatePort: 80, publicPort: 8080, type: 'tcp' }], accessMode: 'direct', updateStatus: 'current', detectedBy: [] },
    })
    const primary = makeSite({
      id: 'primary',
      primaryDomain: 'panel.example.com',
      upstream: 'http://127.0.0.1:8080',
      certificate: { status: 'valid' },
    })
    const alias = makeSite({
      id: 'alias',
      primaryDomain: 'panel-alias.example.com',
      upstream: 'http://localhost:8080',
    })
    const unrelated = makeSite({
      id: 'blog',
      primaryDomain: 'blog.example.com',
      upstream: 'http://127.0.0.1:9090',
    })
    vi.mocked(api.apps.inventory).mockResolvedValue(inventory([app]))
    vi.mocked(api.sites.list).mockResolvedValue({ items: [primary, alias, unrelated], total: 3 })

    const entries = await loadDesktopEntries(undefined, '192.168.1.5')
    expect(entries.apps[0]!.url).toBe('https://panel.example.com')
    expect(entries.sites).toHaveLength(3)
    expect(entries.visible.map((entry) => entry.id)).toEqual(['panel', 'blog'])
  })

  it('reuses a fresh cache and lets an explicit refresh bypass it', async () => {
    const site = makeSite({ id: 's1', primaryDomain: 'blog.example.com', health: 'warning' })
    vi.mocked(api.apps.inventory).mockResolvedValue(inventory([]))
    vi.mocked(api.sites.list).mockResolvedValue({ items: [site], total: 1 })

    const first = await loadDesktopEntries(undefined, '192.168.1.5')
    const cached = await loadDesktopEntries(undefined, '192.168.1.5')
    expect(cached).toBe(first)
    expect(api.apps.inventory).toHaveBeenCalledTimes(1)
    expect(api.sites.list).toHaveBeenCalledTimes(1)

    await loadDesktopEntries(undefined, '192.168.1.5', true)
    expect(api.apps.inventory).toHaveBeenCalledTimes(2)
    expect(api.sites.list).toHaveBeenCalledTimes(2)
  })

  it('keeps the last successful desktop entries when a refresh temporarily fails', async () => {
    const site = makeSite({ id: 's1', primaryDomain: 'blog.example.com', health: 'warning' })
    vi.mocked(api.apps.inventory).mockResolvedValueOnce(inventory([])).mockRejectedValueOnce(new Error('offline'))
    vi.mocked(api.sites.list).mockResolvedValueOnce({ items: [site], total: 1 }).mockRejectedValueOnce(new Error('offline'))

    const first = await loadDesktopEntries(undefined, '192.168.1.5')
    const fallback = await loadDesktopEntries(undefined, '192.168.1.5', true)
    expect(fallback).toBe(first)
    expect(fallback.visible.map((entry) => entry.id)).toEqual(['s1'])
  })
})
