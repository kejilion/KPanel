import { readFileSync } from 'node:fs'
import { createSSRApp, ssrContextKey, type ComputedRef, type Ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ClusterShareView from './ClusterShareView.vue'
import { ApiError } from '@/lib/api'
import type { ClusterHostTemporarySortKey } from '@/lib/clusterHostTemporarySort'
import type { ClusterHostDetails, PublicClusterShareHost, PublicClusterShareSnapshot } from '@/types/api'

const mocks = vi.hoisted(() => ({
  publicShare: vi.fn(),
  token: 'a'.repeat(64),
  getItem: vi.fn(),
  setItem: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { token: mocks.token } }),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {
    readonly status: number
    constructor(message: string, status = 0) {
      super(message)
      this.status = status
    }
  },
  api: { cluster: { publicShare: mocks.publicShare } },
}))

interface ShareBindings {
  publicDetails: ComputedRef<Record<string, ClusterHostDetails>>
  sortKey: Ref<ClusterHostTemporarySortKey>
  changeSort: (key: ClusterHostTemporarySortKey) => void
  sortDirection: Ref<'asc' | 'desc'>
  search: Ref<string>
  filteredHosts: ComputedRef<PublicClusterShareHost[]>
  snapshot: Ref<PublicClusterShareSnapshot | undefined>
  loading: Ref<boolean>
  refreshing: Ref<boolean>
  errorMessage: Ref<string>
  tokenIsValid: ComputedRef<boolean>
  viewMode: Ref<'list' | 'card' | 'globe'>
  load: (silent?: boolean) => Promise<void>
  setViewMode: (mode: 'list' | 'card' | 'globe') => void
  restoreViewMode: () => void
}

function setupView(): ShareBindings {
  const component = ClusterShareView as unknown as {
    setup: (props: Record<string, never>, context: { expose: () => void }) => ShareBindings
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

function publicSnapshot(): PublicClusterShareSnapshot {
  return {
    title: 'My fleet',
    description: 'Public status',
    generatedAt: '2026-08-15T12:00:00Z',
    total: 1,
    online: 1,
    attention: 0,
    items: [{
      id: 'host-public-id',
      name: 'Singapore node',
      state: 'online',
      os: 'Debian GNU/Linux 13',
      architecture: 'x86_64',
      uptimeSeconds: 3600,
      load: { one: 0.1, five: 0.2, fifteen: 0.3 },
      cpu: { cores: 4, usagePercent: 12.5 },
      memory: { totalBytes: 8 * 1024 ** 3, usedBytes: 3 * 1024 ** 3, usagePercent: 37.5 },
      disk: { totalBytes: 100 * 1024 ** 3, usedBytes: 20 * 1024 ** 3, usagePercent: 20 },
      network: { receivedBytes: 1024, sentBytes: 2048, receiveBytesPerSecond: 1024, transmitBytesPerSecond: 2048 },
      location: { country: 'Singapore', countryCode: 'SG', city: 'Singapore', isp: 'Example ISP' },
      collectedAt: '2026-08-15T12:00:00Z',
    }],
  }
}

beforeEach(() => {
  mocks.token = 'a'.repeat(64)
  mocks.publicShare.mockReset()
  mocks.getItem.mockReset().mockReturnValue(null)
  mocks.setItem.mockReset()
  vi.stubGlobal('window', { localStorage: { getItem: mocks.getItem, setItem: mocks.setItem } })
})

afterEach(() => vi.unstubAllGlobals())

describe('ClusterShareView anonymous snapshot', () => {
  it('derives value details from the whole public snapshot and clears them on revocation', async () => {
    const view = setupView()
    const data = publicSnapshot()
    data.items[0] = { ...data.items[0]!, price: '$120/year', expiresOn: '2027-01-01', trafficMonthlyQuotaGiB: 100, trafficCalculation: 'sent' }
    view.snapshot.value = data
    view.search.value = 'no matches'
    expect(view.filteredHosts.value).toEqual([])
    expect(view.publicDetails.value).toEqual({ 'host-public-id': { price: '$120/year', expiresOn: '2027-01-01' } })
    mocks.publicShare.mockRejectedValueOnce(new ApiError('revoked', 404))
    await view.load(true)
    expect(view.publicDetails.value).toEqual({})
  })

  it.each(['cpu', 'memory', 'disk', 'traffic'] as const)('sorts public %s in both directions with pending hosts last and refreshes the order', async key => {
    const view = setupView()
    const data = publicSnapshot()
    const host = data.items[0]!
    const high: PublicClusterShareHost = {
      ...host, id: 'high', name: 'node high',
      cpu: { ...host.cpu, usagePercent: 90 },
      memory: { ...host.memory, usagePercent: 90 },
      disk: { ...host.disk, usagePercent: 90 },
      // The sum is larger even though received traffic is lower.
      network: { ...host.network, receivedBytes: 512, sentBytes: 4096 },
    }
    data.items = [
      { ...host, id: 'pending', name: 'pending', state: 'pending', collectedAt: undefined },
      { ...host, id: 'low', name: 'node low' }, high, { ...high, id: 'equal' },
    ]
    view.snapshot.value = data
    view.changeSort(key)
    expect(view.sortDirection.value).toBe('desc')
    expect(view.filteredHosts.value.map(h => h.id)).toEqual(['high', 'equal', 'low', 'pending'])
    view.sortDirection.value = 'asc'
    expect(view.filteredHosts.value.map(h => h.id)).toEqual(['low', 'high', 'equal', 'pending'])
    view.search.value = 'node'
    expect(view.filteredHosts.value.map(h => h.id)).toEqual(['low', 'high', 'equal'])
    view.sortDirection.value = 'desc'
    expect(view.filteredHosts.value.map(h => h.id)).toEqual(['high', 'equal', 'low'])

    const refreshed = { ...data, items: data.items.map(h => h.id === 'low' ? { ...high, id: 'low' } : h) }
    mocks.publicShare.mockResolvedValueOnce(refreshed)
    await view.load(true)
    expect(view.filteredHosts.value.map(h => h.id)).toEqual(['low', 'high', 'equal'])
    view.search.value = ''
    view.changeSort('custom')
    expect(view.filteredHosts.value.map(h => h.id)).toEqual(['pending', 'low', 'high', 'equal'])
    expect(data.items.map(h => h.id)).toEqual(['pending', 'low', 'high', 'equal'])
    expect(mocks.setItem).not.toHaveBeenCalled()
  })

  it('sorts public details and keeps filtering and the original public order intact', () => {
    const view = setupView()
    const data = publicSnapshot()
    const host = data.items[0]!
    data.items = [
      { ...host, id: 'late', name: 'node late', expiresOn: '2028-01-01', price: '$120/year' },
      { ...host, id: 'early', name: 'node early', expiresOn: '2027-01-01', price: '$12/month' },
      { ...host, id: 'empty', name: 'empty' },
    ]
    view.snapshot.value = data
    view.sortKey.value = 'expiresOn'
    expect(view.filteredHosts.value.map(h => h.id)).toEqual(['early', 'late', 'empty'])
    view.sortKey.value = 'price'
    expect(view.filteredHosts.value.map(h => h.id)).toEqual(['late', 'early', 'empty'])
    view.sortDirection.value = 'desc'
    view.search.value = 'node'
    expect(view.filteredHosts.value.map(h => h.id)).toEqual(['early', 'late'])
    view.sortKey.value = 'custom'
    expect(view.filteredHosts.value.map(h => h.id)).toEqual(['late', 'early'])
    expect(data.items.map(h => h.id)).toEqual(['late', 'early', 'empty'])
    expect(mocks.setItem).not.toHaveBeenCalled()
  })

  it('loads exactly one allowlisted public endpoint without session state', async () => {
    const expected = publicSnapshot()
    mocks.publicShare.mockResolvedValueOnce(expected)
    const view = setupView()

    await view.load()

    expect(mocks.publicShare).toHaveBeenCalledOnce()
    expect(mocks.publicShare).toHaveBeenCalledWith(mocks.token, expect.any(AbortSignal))
    expect(view.snapshot.value).toEqual(expected)
    expect(view.errorMessage.value).toBe('')
  })

  it('rejects malformed links before making a network request', async () => {
    mocks.token = 'not-a-token'
    const view = setupView()

    await view.load()

    expect(view.tokenIsValid.value).toBe(false)
    expect(mocks.publicShare).not.toHaveBeenCalled()
    expect(view.errorMessage.value).toBe('分享链接格式无效。')
  })

  it('silently updates an existing snapshot without entering either loading state', async () => {
    const view = setupView()
    mocks.publicShare.mockResolvedValueOnce(publicSnapshot())
    await view.load()
    const previous = view.snapshot.value
    let complete!: (snapshot: PublicClusterShareSnapshot) => void
    mocks.publicShare.mockReturnValueOnce(new Promise(resolve => { complete = resolve }))

    const polling = view.load(true)
    expect(view.loading.value).toBe(false)
    expect(view.refreshing.value).toBe(false)
    expect(view.snapshot.value).toBe(previous)

    const updated = publicSnapshot()
    updated.items[0]!.cpu.usagePercent = 80
    complete(updated)
    await polling
    expect(view.snapshot.value?.items[0]?.cpu.usagePercent).toBe(80)
  })

  it('keeps a manual refresh in progress when a background poll is due', async () => {
    const view = setupView()
    mocks.publicShare.mockResolvedValueOnce(publicSnapshot())
    await view.load()
    let complete!: (snapshot: PublicClusterShareSnapshot) => void
    mocks.publicShare.mockReturnValueOnce(new Promise(resolve => { complete = resolve }))

    const manualRefresh = view.load()
    const signal = mocks.publicShare.mock.calls[1]![1] as AbortSignal
    expect(view.loading.value).toBe(false)
    expect(view.refreshing.value).toBe(true)
    await view.load(true)
    expect(mocks.publicShare).toHaveBeenCalledTimes(2)
    expect(signal.aborted).toBe(false)
    expect(view.refreshing.value).toBe(true)

    complete(publicSnapshot())
    await manualRefresh
    expect(view.refreshing.value).toBe(false)
  })

  it('ignores a superseded request without clearing the current loading state', async () => {
    const view = setupView()
    let finishOld!: (snapshot: PublicClusterShareSnapshot) => void
    let finishNew!: (snapshot: PublicClusterShareSnapshot) => void
    mocks.publicShare.mockReturnValueOnce(new Promise(resolve => { finishOld = resolve }))
    mocks.publicShare.mockReturnValueOnce(new Promise(resolve => { finishNew = resolve }))

    const oldRequest = view.load()
    const newRequest = view.load()
    finishOld(publicSnapshot())
    await oldRequest
    expect(view.snapshot.value).toBeUndefined()
    expect(view.loading.value).toBe(true)

    const current = publicSnapshot()
    current.title = 'Current share'
    finishNew(current)
    await newRequest
    expect(view.snapshot.value?.title).toBe('Current share')
    expect(view.loading.value).toBe(false)
  })

  it.each(['list', 'card', 'globe'] as const)('remembers the public %s view independently of the management page', (mode) => {
    const view = setupView()
    expect(view.viewMode.value).toBe('list')
    view.setViewMode(mode)
    expect(mocks.setItem).toHaveBeenCalledWith('kpanel:cluster-share-view', mode)
    mocks.getItem.mockReturnValue(mode)
    const reopened = setupView()
    reopened.restoreViewMode()
    expect(reopened.viewMode.value).toBe(mode)
  })

  it('keeps the default list for unknown preferences and works when storage is blocked', () => {
    const view = setupView()
    mocks.getItem.mockReturnValue('unsupported')
    view.restoreViewMode()
    expect(view.viewMode.value).toBe('list')
    mocks.getItem.mockImplementation(() => { throw new Error('blocked') })
    mocks.setItem.mockImplementation(() => { throw new Error('blocked') })
    expect(() => view.restoreViewMode()).not.toThrow()
    view.setViewMode('globe')
    expect(view.viewMode.value).toBe('globe')
  })

  it('clears the old snapshot when the public link is revoked but retains it for a temporary error', async () => {
    const view = setupView()
    const previous = publicSnapshot()
    view.snapshot.value = previous
    mocks.publicShare.mockRejectedValueOnce(new ApiError('temporary', 500))
    await view.load(true)
    expect(view.snapshot.value).toEqual(previous)
    mocks.publicShare.mockRejectedValueOnce(new ApiError('revoked', 404))
    await view.load(true)
    expect(view.snapshot.value).toBeUndefined()
    expect(view.errorMessage.value).toBe('分享链接无效、已关闭或已经重置。')
  })

  it('keeps management and identity fields out of the public page template', () => {
    const source = readFileSync(new URL('./ClusterShareView.vue', import.meta.url), 'utf8')
    expect(source).toContain("import LogoMark from '@/components/common/LogoMark.vue'")
    expect(source).toContain('<LogoMark compact class="share-brand__logo" />')
    expect(source).toContain('公开页不包含 IP、管理入口或访问凭据')
    for (const privateField of ['origin', 'peerFingerprint', 'remoteNodeId', 'resourceVersion']) {
      expect(source).not.toContain(`host.${privateField}`)
    }
    // System and region details sit behind the same hover cards as the cluster page;
    // they only read fields the public snapshot already whitelists.
    expect(source).toContain('<ClusterHostSystemInfo class="share-card__system" :system="host" />')
    expect(source).toContain('<ClusterHostRegionInfo class="share-card__region" :location="host.location" shared />')
    expect(source.indexOf('<ClusterHostRegionInfo')).toBeLessThan(source.indexOf('<h2>{{ host.name }}</h2>'))
    expect(source).not.toContain('<OperatingSystemIcon')
    expect(source).not.toContain('<CountryFlagIcon')
    expect(source).toContain("formatNetworkTrafficCounter(clusterTrafficCounters(host), 'received')")
    expect(source).toContain("formatNetworkTrafficCounter(clusterTrafficCounters(host), 'sent')")
    expect(source).not.toContain('formatTotalNetworkTraffic')
    expect(source).toContain("const viewMode = ref<ShareViewMode>('list')")
    expect(source).toContain(':hosts="filteredHosts"')
    expect(source).toContain('const { resolved: resolvedTheme, setTheme } = useTheme()')
    expect(source).toContain(':class="`is-${viewMode}`"')
    expect(source.indexOf('实时网速</dt>')).toBeLessThan(source.indexOf('<ClusterTrafficHeading'))
    expect(source.indexOf('<ClusterTrafficHeading')).toBeLessThan(source.indexOf('运行时间</dt>'))
    expect(source).toContain('<div class="share-details__uptime">')
    expect(source).toContain('formatCapacityPair(host.memory.usedBytes, host.memory.totalBytes)')
    expect(source).toContain(':class="`is-${usageTone(host.disk.usagePercent)}`"')
    expect(source).toContain('@click="load()"')
    expect(source).toContain('<span>刷新</span>')

    // Same responsive contract as the cluster list: no sideways scrolling at any width.
    const styles = source.slice(source.indexOf('<style scoped>'))
    expect(styles).toMatch(/\.share-grid\s*\{[^}]*container:\s*share-layout \/ inline-size;[^}]*grid-template-columns:\s*repeat\(auto-fill, minmax\(min\(100%, 22\.5rem\), 1fr\)\);/)
    const medium = styles.slice(styles.indexOf('@container share-layout (min-width: 42.5rem)'), styles.indexOf('@container share-layout (min-width: 62rem)'))
    expect(medium).toMatch(/grid-template-areas:\s*"header header"\s*"metrics details";/)
    expect(styles.slice(styles.indexOf('@container share-layout (min-width: 62rem)'))).toMatch(/grid-template-areas:\s*"header metrics details";/)
    expect(styles).not.toMatch(/minmax\(300px, 1fr\) minmax\(300px, 0\.95fr\) 24rem/)

    const routerSource = readFileSync(new URL('../router.ts', import.meta.url), 'utf8')
    expect(routerSource).toContain("path: '/share/:token'")
    expect(routerSource).toContain("public: true, skipSessionCheck: true")
    expect(routerSource.indexOf('if (to.meta.skipSessionCheck) return true')).toBeLessThan(
      routerSource.indexOf('const session = useSession()'),
    )
  })
})
