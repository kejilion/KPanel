import { readFileSync } from 'node:fs'
import { createSSRApp, ssrContextKey, type ComputedRef, type Ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ClusterView from './ClusterView.vue'
import { ApiError } from '@/lib/api'
import { registerPhraseCatalog, translatePhrase } from '@/i18n/phrase'
import english from '@/i18n/pages/ClusterView/en-US'
import traditionalChinese from '@/i18n/pages/ClusterView/zh-TW'
import sharedEnglish from '@/i18n/pages/shared/en-US'
import sharedTraditionalChinese from '@/i18n/pages/shared/zh-TW'
import type {
  ClusterHostTemporarySortDirection,
  ClusterHostTemporarySortKey,
} from '@/lib/clusterHostTemporarySort'
import type {
  ClusterHost,
  ClusterHostList,
  ClusterLightEnrollment,
  ClusterPairingCode,
  ClusterShareSettings,
} from '@/types/api'

const mocks = vi.hoisted(() => ({
  hosts: vi.fn(),
  host: vi.fn(),
  add: vi.fn(),
  rename: vi.fn(),
  remove: vi.fn(),
  refresh: vi.fn(),
  enableMutualFiles: vi.fn(),
  createPairingCode: vi.fn(),
  createLightEnrollment: vi.fn(),
  controllers: vi.fn(),
  revokeController: vi.fn(),
  shareSettings: vi.fn(),
  updateHostOrder: vi.fn(),
  updateShare: vi.fn(),
  resetShareToken: vi.fn(),
  open: vi.fn(),
  confirm: vi.fn(),
  toastSuccess: vi.fn(),
  toastDanger: vi.fn(),
  clipboardWriteText: vi.fn(),
  execCommand: vi.fn(),
  localStorageGetItem: vi.fn(),
  localStorageSetItem: vi.fn(),
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
    cluster: {
      hosts: mocks.hosts,
      host: mocks.host,
      add: mocks.add,
      rename: mocks.rename,
      remove: mocks.remove,
      refresh: mocks.refresh,
      enableMutualFiles: mocks.enableMutualFiles,
      createPairingCode: mocks.createPairingCode,
      createLightEnrollment: mocks.createLightEnrollment,
      controllers: mocks.controllers,
      revokeController: mocks.revokeController,
      shareSettings: mocks.shareSettings,
      updateHostOrder: mocks.updateHostOrder,
      updateShare: mocks.updateShare,
      resetShareToken: mocks.resetShareToken,
    },
  },
}))

vi.mock('@/stores/toast', () => ({
  useToast: () => ({
    success: mocks.toastSuccess,
    danger: mocks.toastDanger,
  }),
}))

interface ClusterBindings {
  inventory: Ref<ClusterHostList | undefined>
  filteredHosts: ComputedRef<ClusterHost[]>
  originAssessment: ComputedRef<{ mode: string; message: string }>
  canSubmitAdd: ComputedRef<boolean>
  panelOrigin: ComputedRef<string>
  parsedAccessCredential: ComputedRef<{ origin: string; pairingCode: string } | undefined>
  accessCredentialText: ComputedRef<string>
  search: Ref<string>
  viewMode: Ref<'list' | 'card' | 'globe'>
  hostOrder: Ref<string[]>
  hostOrderResourceVersion: Ref<string>
  temporarySortKey: Ref<ClusterHostTemporarySortKey>
  temporarySortDirection: Ref<ClusterHostTemporarySortDirection>
  temporarySortActive: ComputedRef<boolean>
  hostOrderControlTitle: ComputedRef<string>
  accessOpen: Ref<boolean>
  manageOpen: Ref<boolean>
  shareOpen: Ref<boolean>
  shareSettings: Ref<ClusterShareSettings | undefined>
  shareURL: ComputedRef<string>
  shareForm: { enabled: boolean; title: string; description: string; applyPanelOrder: boolean }
  enablingMutualFiles: Ref<boolean>
  selected: Ref<ClusterHost | undefined>
  pairingCode: Ref<ClusterPairingCode | undefined>
  lightEnrollment: Ref<ClusterLightEnrollment | undefined>
  lightEnrollmentConnected: Ref<boolean>
  lightEnrollmentState: Ref<'waiting' | 'registered' | 'connected' | 'expired'>
  editName: Ref<string>
  addForm: { name: string; accessCredential: string }
  load: (silent?: boolean) => Promise<void>
  addHost: () => Promise<void>
  openManage: (host: ClusterHost) => void
  saveName: () => Promise<void>
  removeHost: () => Promise<void>
  mutualFilesHostEligible: (host: ClusterHost) => boolean
  enableMutualFiles: () => Promise<void>
  openPanel: (host: ClusterHost) => void
  panelURL: (host: ClusterHost) => string
  displayHostAddress: (host: ClusterHost) => string
  copyAccessCredential: () => Promise<void>
  createLightEnrollment: () => Promise<void>
  copyLightEnrollment: () => Promise<void>
  openShare: () => Promise<void>
  saveShare: () => Promise<void>
  resetShareLink: () => Promise<void>
  copyShareLink: () => Promise<void>
  formatClusterAccessCredential: (origin: string, pairingCode: string) => string
  parseClusterAccessCredential: (
    raw: string,
  ) => { origin: string; pairingCode: string } | undefined
  setViewMode: (mode: 'list' | 'card' | 'globe') => void
  moveHost: (hostID: string, offset: number) => Promise<void>
  transportSecurityLabel: (host: ClusterHost) => string
  shortFingerprint: (value?: string) => string
  hostOperatingSystemIdentity: (host: ClusterHost) => { key: string; label: string }
  formatHostLatency: (host: ClusterHost) => string
  lightNodeCapabilitySummary: (host: ClusterHost) => string
  controllerCapabilitySummary: (scope: string) => string
}

function setupView(): ClusterBindings {
  const component = ClusterView as unknown as {
    setup: (props: Record<string, never>, context: { expose: () => void }) => ClusterBindings
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

function host(id: string, isLocal: boolean, origin: string): ClusterHost {
  return {
    id,
    isLocal,
    kind: 'panel',
    name: isLocal ? '当前 KPanel' : '香港节点',
    origin,
    transportSecurity: origin.startsWith('http://') ? 'e2e_http' : 'tls',
    peerFingerprint: isLocal ? undefined : `sha256:${'a'.repeat(64)}`,
    remoteNodeId: isLocal ? 'local-node' : 'remote-node',
    federationProtocol: 'v1',
		scope: isLocal ? 'cluster.summary.read cluster.terminal.open' : 'cluster.summary.read',
		terminalAvailable: isLocal,
    fileTransferAvailable: false,
    mutualFileTransferAvailable: false,
    panelVersion: '0.27.0',
    state: 'online',
    lastSnapshot: {
      telemetry: {
        agentVersion: '0.27.0',
        agentProtocolVersion: 'v1',
        hostname: isLocal ? 'center' : 'hk-01',
        os: 'Debian GNU/Linux 13',
        osId: 'debian',
        osLike: ['linux'],
        architecture: 'amd64',
        uptimeSeconds: 3600,
        load: { one: 0.1, five: 0.2, fifteen: 0.3 },
        cpu: { cores: 2, usagePercent: 12.5 },
        memory: {
          totalBytes: 8 * 1024 ** 3,
          availableBytes: 6 * 1024 ** 3,
          usedBytes: 2 * 1024 ** 3,
          usagePercent: 25,
        },
        disk: {
          totalBytes: 100 * 1024 ** 3,
          usedBytes: 20 * 1024 ** 3,
          usagePercent: 20,
        },
        network: {
          receivedBytes: 1000,
          sentBytes: 2000,
          tcpConnections: 10,
          udpConnections: 2,
        },
        publicNetwork: {
          ipv4: isLocal ? '203.0.113.10' : '198.51.100.20',
          country: isLocal ? 'CN' : 'HK',
          countryCode: isLocal ? 'CN' : 'HK',
          city: isLocal ? 'Shanghai' : 'Hong Kong',
          isp: 'Example Network',
        },
        collectedAt: '2026-07-29T10:00:00Z',
      },
      receivedAt: '2026-07-29T10:00:01Z',
      latencyMilliseconds: isLocal ? 0 : 42,
      receiveBytesPerSecond: 1024,
      transmitBytesPerSecond: 2048,
    },
    lastAttemptAt: '2026-07-29T10:00:01Z',
    lastSuccessAt: '2026-07-29T10:00:01Z',
    consecutiveFailures: 0,
    polling: false,
    resourceVersion: `${id}-version`,
    createdAt: '2026-07-29T09:00:00Z',
    updatedAt: '2026-07-29T10:00:01Z',
  }
}

function inventory(): ClusterHostList {
  return {
    items: [
      host('local', true, 'https://stored-local.invalid'),
      host('remote', false, 'https://hk.example.com'),
    ],
    total: 2,
    remoteTotal: 1,
    maxHosts: 100,
    pollIntervalSeconds: 30,
    nodeId: 'local-node',
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.controllers.mockResolvedValue({ items: [] })
  mocks.updateHostOrder.mockImplementation(async ({ ids }: { ids: string[] }) => ({
    ids,
    configured: true,
    resourceVersion: 'sha256:order-v2',
  }))
  vi.stubGlobal('window', {
    location: { origin: 'https://center.example.com' },
    open: mocks.open,
    confirm: mocks.confirm.mockReturnValue(true),
    localStorage: {
      getItem: mocks.localStorageGetItem.mockReturnValue(null),
      setItem: mocks.localStorageSetItem,
    },
  })
  vi.stubGlobal('navigator', {
    clipboard: {
      writeText: mocks.clipboardWriteText.mockResolvedValue(undefined),
    },
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ClusterView live host details', () => {
  it('refreshes an open light node version and telemetry without overwriting a name edit', async () => {
    const view = setupView()
    const initial = { ...host('light', false, ''), kind: 'light_node' as const }
    view.openManage(initial)
    view.editName.value = '正在编辑的名称'
    const updated = { ...initial, panelVersion: '1.3.1', state: 'stale' as const, resourceVersion: 'changed-elsewhere',
      lastSnapshot: { ...initial.lastSnapshot!, receivedAt: '2026-09-05T03:00:00Z',
        telemetry: { ...initial.lastSnapshot!.telemetry, agentVersion: '1.3.1' } } }
    mocks.hosts.mockResolvedValue({ ...inventory(), items: [updated] })
    await view.load(true)
    expect(view.selected.value).toEqual(updated)
    mocks.rename.mockResolvedValueOnce(updated)
    await view.saveName()
    expect(mocks.rename).toHaveBeenCalledWith(initial.id, {
      name: '正在编辑的名称', expectedResourceVersion: initial.resourceVersion,
    })
    expect(view.editName.value).toBe('正在编辑的名称')
    expect(view.manageOpen.value).toBe(true)

    mocks.hosts.mockRejectedValueOnce(new Error('offline'))
    await view.load(true)
    expect(view.selected.value).toEqual(updated)
  })

  it('closes details when a successful refresh confirms the node was removed', async () => {
    const view = setupView()
    view.openManage(host('removed', false, ''))
    mocks.hosts.mockResolvedValue({ ...inventory(), items: [] })
    await view.load(true)
    expect(view.selected.value).toBeUndefined()
    expect(view.manageOpen.value).toBe(false)
  })
})

describe('ClusterView capability disclosures', () => {
  it('describes each live light-node capability instead of labelling every node read-only', () => {
    const view = setupView()
    const lightNode = {
      ...host('light-capabilities', false, ''),
      kind: 'light_node' as const,
      terminalAvailable: false,
      fileManagementAvailable: false,
    }

    expect(view.lightNodeCapabilitySummary(lightNode)).toBe('只读摘要 · 主动 HTTPS 上报')
    expect(view.lightNodeCapabilitySummary({ ...lightNode, terminalAvailable: true })).toBe(
      '摘要监控 · 远程终端',
    )
    expect(view.lightNodeCapabilitySummary({ ...lightNode, fileManagementAvailable: true })).toBe(
      '摘要监控 · 文件管理',
    )
    expect(view.lightNodeCapabilitySummary({
      ...lightNode,
      terminalAvailable: true,
      fileManagementAvailable: true,
    })).toBe('摘要监控 · 远程终端 · 文件管理')
  })

  it('maps controller scopes to their effective management permissions', () => {
    const view = setupView()

    expect(view.controllerCapabilitySummary('cluster.summary.read')).toBe('权限：摘要读取')
    expect(view.controllerCapabilitySummary(
      'cluster.summary.read cluster.terminal.open',
    )).toBe('权限：摘要读取 · 远程终端')
    expect(view.controllerCapabilitySummary(
      'cluster.summary.read cluster.terminal.open cluster.files.read',
    )).toBe('权限：摘要读取 · 远程终端 · 文件管理（含写入与删除）')
  })

  it('states the full grant in the access and enrollment copy for every locale', () => {
    const source = readFileSync(new URL('./ClusterView.vue', import.meta.url), 'utf8')
    const fullGrant = '权限包含摘要读取、远程终端和文件管理（含写入与删除）。'
    const enrollment = '将加入摘要监控，并启用远程终端和文件管理（含写入与删除）。'

    expect(source).toContain(fullGrant)
    expect(source).toContain(enrollment)
    expect(source).not.toContain('文件只读访问')
    expect(source).not.toContain('不包含任何远程管理权限')
    expect(new Map(english).get(
      '权限：摘要读取 · 远程终端 · 文件管理（含写入与删除）',
    )).toContain('writes and deletion')
    expect(new Map(traditionalChinese).get(
      '权限：摘要读取 · 远程终端 · 文件管理（含写入与删除）',
    )).toContain('寫入與刪除')
  })
})

describe('ClusterView compact summary layout', () => {
  it('keeps summary metrics and actions on one decorated row', () => {
    const source = readFileSync(new URL('./ClusterView.vue', import.meta.url), 'utf8')

    // The hero is a wrapping flex row rather than a two-track grid: a desktop
    // window can be narrower than the stats + actions need, and the old
    // `max-content` track plus `nowrap` pushed the refresh button off the left
    // edge instead of letting the actions drop to their own row.
    expect(source).toMatch(/\.cluster-hero\s*\{[^}]*display:\s*flex;[^}]*flex-wrap:\s*wrap;/)
    expect(source).toMatch(/\.cluster-stats\s*\{[^}]*grid-template-columns:\s*repeat\(4, minmax\(96px, 132px\)\);/)
    expect(source).toMatch(/\.cluster-hero\s*\{[^}]*radial-gradient\([^}]*var\(--cluster-accent\)/)
    expect(source).toMatch(/\.cluster-hero__actions\s*\{[^}]*flex-wrap:\s*wrap;/)
    expect(source).toContain('class="button button--primary button--small cluster-hero__add"')
    expect(source).toMatch(/\.cluster-hero__actions\s*>\s*\.cluster-hero__add\s*\{[^}]*flex-basis:\s*100%;/)
    expect(source).toMatch(/class="icon-button"[\s\S]{0,200}aria-label="刷新集群状态"/)
    expect(source).not.toMatch(/class="icon-button icon-button--small"[\s\S]{0,200}aria-label="刷新集群状态"/)
    expect(source.indexOf('aria-label="刷新集群状态"')).toBeLessThan(source.indexOf('@click="openAccess"'))
    expect(source).toMatch(/\.cluster-grid\.is-list \.cluster-card__details\s*\{[^}]*align-content:\s*stretch;[^}]*gap:\s*0;[^}]*padding:\s*0;/)
    expect(source).toMatch(/\.cluster-grid\.is-list \.cluster-card__details > :is\(div, a\)\s*\{[^}]*align-content:\s*center;[^}]*padding:\s*15px 8px;/)
    expect(source).toMatch(/\.cluster-grid\.is-list \.cluster-card__details > \.cluster-metric-link\s*\{[^}]*border-radius:\s*0;/)
    expect(source).toMatch(/\.cluster-metric-link:hover,\s*\.cluster-metric-link:focus-visible\s*\{[^}]*background:\s*var\(--brand-soft\);/)
    expect(source).toMatch(/\.cluster-metric-link:focus-visible\s*\{[^}]*outline:\s*2px solid var\(--brand\);/)
    expect(source).toContain("formatNetworkTrafficCounter(host.lastSnapshot.telemetry.network, 'received')")
    expect(source).toContain("formatNetworkTrafficCounter(host.lastSnapshot.telemetry.network, 'sent')")
    expect(source).not.toContain('formatTotalNetworkTraffic')
  })

  it('lets narrow hero actions wrap without squeezing translated labels', () => {
    const source = readFileSync(new URL('./ClusterView.vue', import.meta.url), 'utf8')
    const narrowStyles = source.slice(source.indexOf('@media (max-width: 680px)'))
    expect(narrowStyles).not.toContain('42px repeat(3, minmax(0, 1fr))')
    expect(narrowStyles).toMatch(/\.cluster-hero__actions\s*>\s*\*\s*\{[^}]*flex:\s*1 0 auto;[^}]*max-width:\s*100%;[^}]*min-height:\s*40px;[^}]*white-space:\s*normal;/)
    expect(narrowStyles).toMatch(/\.cluster-hero__actions\s*>\s*\.icon-button\s*\{[^}]*flex:\s*0 0 40px;/)
    expect(source).toMatch(/\.cluster-hero__actions\s*>\s*\.button\s*\{[^}]*min-height:\s*40px;[^}]*font-size:\s*max\(14px, \.875rem\);/)
  })

  it.each([
    { locale: 'en-US', shared: sharedEnglish, page: english, expected: ['Notifications', 'List', 'Cards', 'cores'] },
    { locale: 'zh-TW', shared: sharedTraditionalChinese, page: traditionalChinese, expected: ['通知', '列表', '卡片', '核'] },
  ])('translates short controls and CPU cores with the $locale page catalog loaded after shared', ({ shared, page, expected }) => {
    const unregisterShared = registerPhraseCatalog(shared)
    const unregisterPage = registerPhraseCatalog(page)
    try {
      const labels = ['通知', '列表', '卡片', '核'] as const
      expect(labels.map(label => new Map(page).get(label))).toEqual(expected)
      expect(labels.map(translatePhrase)).toEqual(expected)
    } finally {
      unregisterPage()
      unregisterShared()
    }
  })

  it('routes native confirmations through core i18n', () => {
    const source = readFileSync(new URL('./ClusterView.vue', import.meta.url), 'utf8')
    expect(source.match(/window\.confirm\(t\('cluster\.confirm\./g)).toHaveLength(5)
    expect(source).not.toContain('重置公开链接？旧链接会立即失效。')
  })
})

describe('ClusterView inventory and navigation', () => {
  it('does not present an unmeasured host latency as zero', () => {
    const view = setupView()

    expect(view.formatHostLatency(host('local', true, 'https://stored-local.invalid'))).toBe('--')
    expect(view.formatHostLatency(host('remote', false, 'https://hk.example.com'))).toBe('42 ms')
  })

  it('uses the overview operating-system identity mapping for host icons', () => {
    const view = setupView()
    const known = host('known', false, 'https://known.example.com')
    known.lastSnapshot!.telemetry.os = 'AlmaLinux 9.6 (Sage Margay)'
    known.lastSnapshot!.telemetry.osId = 'almalinux'
    known.lastSnapshot!.telemetry.osLike = ['rhel', 'centos', 'fedora']
    expect(view.hostOperatingSystemIdentity(known)).toEqual({
      key: 'alma',
      label: 'AlmaLinux',
    })

    const unknown = host('unknown', false, 'https://unknown.example.com')
    unknown.lastSnapshot!.telemetry.os = 'Vendor Linux 1'
    unknown.lastSnapshot!.telemetry.osId = 'vendorlinux'
    unknown.lastSnapshot!.telemetry.osLike = ['ubuntu', 'debian']
    expect(view.hostOperatingSystemIdentity(unknown)).toEqual({
      key: 'linux',
      label: 'Vendor Linux 1',
    })
  })

  it('uses one cached host-list request and does not fan out into per-host requests', async () => {
    let resolveHosts: ((value: ClusterHostList) => void) | undefined
    mocks.hosts.mockReturnValueOnce(
      new Promise<ClusterHostList>((resolve) => {
        resolveHosts = resolve
      }),
    )
    const view = setupView()

    const first = view.load()
    const overlapping = view.load(true)

    expect(mocks.hosts).toHaveBeenCalledOnce()
    expect(mocks.host).not.toHaveBeenCalled()
    resolveHosts?.(inventory())
    await Promise.all([first, overlapping])
    expect(view.inventory.value?.total).toBe(2)
  })

  it('opens the exact panel origin in a new isolated browser context', () => {
    const view = setupView()
    const current = host('local', true, 'https://stored-local.invalid')
    const remote = host('remote', false, 'https://hk.example.com')

    view.openPanel(remote)
    view.openPanel(current)

    expect(mocks.open).toHaveBeenNthCalledWith(
      1,
      'https://hk.example.com',
      '_blank',
      'noopener,noreferrer',
    )
    expect(mocks.open).toHaveBeenNthCalledWith(
      2,
      'https://center.example.com',
      '_blank',
      'noopener,noreferrer',
    )
    expect(mocks.confirm).not.toHaveBeenCalled()
  })

  it('uses a synchronized security entrance only for the remote panel jump', () => {
    const view = setupView()
    const remote = host('remote', false, 'https://hk.example.com:8443')
    remote.securityEntrancePath = 'panel-secure1'
    const current = host('local', true, 'https://stored-local.invalid')
    current.securityEntrancePath = 'panel-ignored1'

    view.openPanel(remote)
    view.openPanel(current)

    expect(mocks.open).toHaveBeenNthCalledWith(
      1,
      'https://hk.example.com:8443/panel-secure1',
      '_blank',
      'noopener,noreferrer',
    )
    expect(mocks.open).toHaveBeenNthCalledWith(
      2,
      'https://center.example.com',
      '_blank',
      'noopener,noreferrer',
    )
  })

  it('falls back to the root origin for an invalid entrance path', () => {
    const view = setupView()
    const remote = host('remote', false, 'https://hk.example.com')
    remote.securityEntrancePath = '//attacker.example'

    expect(view.panelURL(remote)).toBe('https://hk.example.com')
    view.openPanel(remote)

    expect(mocks.open).toHaveBeenCalledWith(
      'https://hk.example.com',
      '_blank',
      'noopener,noreferrer',
    )
  })

  it('never exposes a management-page jump for a telemetry-only light node', () => {
    const view = setupView()
    const light = host('light', false, '')
    light.kind = 'light_node'
    light.origin = ''
    light.transportSecurity = 'tls'
    light.federationProtocol = 'light-v1'
    light.lastSnapshot!.telemetry.publicNetwork = {
      ipv4: '198.51.100.20',
      ipv6: '2001:db8::20',
    }

    view.openPanel(light)

    expect(mocks.open).not.toHaveBeenCalled()
    expect(mocks.confirm).not.toHaveBeenCalled()
    expect(view.transportSecurityLabel(light)).toBe('轻量节点')
    expect(view.displayHostAddress(light)).toBe('198.51.100.20 · 2001:db8::20')
  })

  it('finds a lightweight node by either reported public IP', () => {
    const view = setupView()
    const light = host('light', false, '')
    light.kind = 'light_node'
    light.lastSnapshot!.telemetry.publicNetwork = {
      ipv4: '198.51.100.20',
      ipv6: '2001:db8::20',
    }
    view.inventory.value = { ...inventory(), items: [light], total: 1 }

    view.search.value = '2001:db8'

    expect(view.filteredHosts.value).toEqual([light])
  })

  it.each(['card', 'globe'] as const)('defaults to the row list and persists the %s preference', (mode) => {
    const view = setupView()

    expect(view.viewMode.value).toBe('list')
    view.setViewMode(mode)

    expect(view.viewMode.value).toBe(mode)
    expect(mocks.localStorageSetItem).toHaveBeenCalledWith(
      'kpanel:cluster-host-view',
      mode,
    )
  })

  it('reorders the same host inventory and persists the preference through the panel', async () => {
    const view = setupView()
    view.inventory.value = inventory()
    view.hostOrder.value = ['local', 'remote']
    view.hostOrderResourceVersion.value = 'sha256:order-v1'

    await view.moveHost('remote', -1)

    expect(view.filteredHosts.value.map((item) => item.id)).toEqual(['remote', 'local'])
    expect(mocks.updateHostOrder).toHaveBeenCalledWith({
      ids: ['remote', 'local'],
      expectedResourceVersion: 'sha256:order-v1',
    })
    expect(mocks.localStorageSetItem).toHaveBeenCalledWith(
      'kpanel:cluster-host-order',
      JSON.stringify(['remote', 'local']),
    )
  })

  it('temporarily sorts live metrics without changing or persisting the custom order', async () => {
    const view = setupView()
    const values = inventory()
    values.items[0]!.lastSnapshot!.telemetry.cpu.usagePercent = 10
    values.items[1]!.lastSnapshot!.telemetry.cpu.usagePercent = 80
    view.inventory.value = values
    view.hostOrder.value = ['local', 'remote']
    view.hostOrderResourceVersion.value = 'sha256:order-v1'

    view.temporarySortKey.value = 'cpu'

    expect(view.filteredHosts.value.map((item) => item.id)).toEqual(['remote', 'local'])
    expect(view.hostOrder.value).toEqual(['local', 'remote'])
    expect(view.temporarySortActive.value).toBe(true)
    expect(view.hostOrderControlTitle.value).toBe('临时排序中，切回自定义顺序后可调整')
    await view.moveHost('remote', -1)
    expect(mocks.updateHostOrder).not.toHaveBeenCalled()
    expect(mocks.localStorageSetItem).not.toHaveBeenCalled()

    view.temporarySortDirection.value = 'asc'
    expect(view.filteredHosts.value.map((item) => item.id)).toEqual(['local', 'remote'])

    view.temporarySortKey.value = 'custom'
    expect(view.filteredHosts.value.map((item) => item.id)).toEqual(['local', 'remote'])
    expect(view.temporarySortActive.value).toBe(false)
  })

  it('resets temporary sorting when a new cluster page instance opens', () => {
    const current = setupView()
    current.temporarySortKey.value = 'traffic'
    current.temporarySortDirection.value = 'asc'

    const reopened = setupView()

    expect(reopened.temporarySortKey.value).toBe('custom')
    expect(reopened.temporarySortDirection.value).toBe('desc')
  })

  it('lets the panel order override a conflicting browser cache', async () => {
    mocks.localStorageGetItem.mockReturnValue(JSON.stringify(['local', 'remote']))
    mocks.hosts.mockResolvedValueOnce({
      ...inventory(),
      hostOrder: {
        ids: ['remote', 'local'],
        configured: true,
        resourceVersion: 'sha256:server-order',
      },
    })
    const view = setupView()

    await view.load()

    expect(view.filteredHosts.value.map((item) => item.id)).toEqual(['remote', 'local'])
    expect(view.hostOrderResourceVersion.value).toBe('sha256:server-order')
    expect(mocks.updateHostOrder).not.toHaveBeenCalled()
    expect(mocks.localStorageSetItem).toHaveBeenCalledWith(
      'kpanel:cluster-host-order',
      JSON.stringify(['remote', 'local']),
    )
  })

  it('migrates a legacy browser order once the panel reports no configured order', async () => {
    mocks.localStorageGetItem.mockReturnValue(JSON.stringify(['remote', 'local']))
    mocks.hosts.mockResolvedValueOnce({
      ...inventory(),
      hostOrder: {
        ids: [],
        configured: false,
        resourceVersion: 'sha256:unconfigured',
      },
    })
    const view = setupView()

    await view.load()

    expect(mocks.updateHostOrder).toHaveBeenCalledWith({
      ids: ['remote', 'local'],
      expectedResourceVersion: 'sha256:unconfigured',
    })
    expect(view.filteredHosts.value.map((item) => item.id)).toEqual(['remote', 'local'])
  })

  it('rolls an explicit reorder back when the panel rejects a stale version', async () => {
    mocks.updateHostOrder.mockRejectedValueOnce(
      new ApiError('changed', 409, 'cluster_host_order_changed'),
    )
    mocks.hosts.mockResolvedValueOnce({
      ...inventory(),
      hostOrder: {
        ids: ['local', 'remote'],
        configured: true,
        resourceVersion: 'sha256:fresh',
      },
    })
    const view = setupView()
    view.inventory.value = inventory()
    view.hostOrder.value = ['local', 'remote']
    view.hostOrderResourceVersion.value = 'sha256:stale'

    await view.moveHost('remote', -1)

    expect(view.filteredHosts.value.map((item) => item.id)).toEqual(['local', 'remote'])
    expect(mocks.localStorageSetItem).not.toHaveBeenCalledWith(
      'kpanel:cluster-host-order',
      JSON.stringify(['remote', 'local']),
    )
    expect(mocks.toastDanger).toHaveBeenCalledWith(
      '保存排序失败',
      '主机顺序已在其他页面变化，请刷新后重试。',
    )
  })

  it('combines the browser-visible URL and one-time code into one copyable credential', async () => {
    const view = setupView()
    view.pairingCode.value = {
      code: 'kp2.one-time-code',
      scope: 'cluster.summary.read',
      expiresAt: '2026-07-29T10:05:00Z',
    }

    expect(view.panelOrigin.value).toBe('https://center.example.com')
    expect(view.accessCredentialText.value).toBe(
      'KPANEL_CLUSTER_ACCESS_V1\nhttps://center.example.com\nkp2.one-time-code',
    )
    await view.copyAccessCredential()

    expect(mocks.clipboardWriteText).toHaveBeenCalledWith(
      'KPANEL_CLUSTER_ACCESS_V1\nhttps://center.example.com\nkp2.one-time-code',
    )
    expect(mocks.toastSuccess).toHaveBeenCalledWith('接入凭据已复制')
  })

  it('generates and copies the one-use non-panel Linux enrollment command', async () => {
    const view = setupView()
    const enrollment: ClusterLightEnrollment = {
      id: 'light-enrollment',
      command:
        "bash <(curl -fsSL https://kejilion.sh) kpanel node join 'kpl1.example-token'",
      expiresAt: '2026-07-29T10:05:00Z',
    }
    mocks.createLightEnrollment.mockResolvedValueOnce(enrollment)
    view.addForm.name = '英国AMR'

    await view.createLightEnrollment()
    await view.copyLightEnrollment()

    expect(view.lightEnrollment.value).toEqual(enrollment)
    expect(mocks.createLightEnrollment).toHaveBeenCalledWith('英国AMR')
    expect(mocks.clipboardWriteText).toHaveBeenCalledWith(enrollment.command)
    expect(mocks.toastSuccess).toHaveBeenCalledWith('轻量节点接入命令已复制')
  })

  it('enables the primary action only after the exact enrolled node sends its first report', async () => {
    const view = setupView()
    const initial = inventory()
    view.inventory.value = initial
    view.addForm.name = '英国AMR'
    const enrollment: ClusterLightEnrollment = {
      id: 'light-enrollment',
      command:
        "bash <(curl -fsSL https://kejilion.sh) kpanel node join 'kpl1.example-token'",
      expiresAt: '2026-07-29T10:05:00Z',
    }
    mocks.createLightEnrollment.mockResolvedValueOnce(enrollment)

    await view.createLightEnrollment()

    expect(view.lightEnrollmentConnected.value).toBe(false)
    expect(view.lightEnrollmentState.value).toBe('waiting')
    expect(view.canSubmitAdd.value).toBe(false)

    const unrelated = host('light-unrelated', false, '')
    unrelated.kind = 'light_node'
    unrelated.name = '英国AMR'
    const registered = host(enrollment.id, false, '')
    registered.kind = 'light_node'
    registered.name = '英国AMR'
    registered.state = 'unknown'
    registered.lastSnapshot = undefined
    registered.lastSuccessAt = undefined
    mocks.hosts.mockResolvedValueOnce({
      ...initial,
      items: [...initial.items, unrelated, registered],
      total: 4,
      remoteTotal: 3,
    })

    await view.load(true)

    expect(view.lightEnrollmentConnected.value).toBe(false)
    expect(view.lightEnrollmentState.value).toBe('registered')
    expect(view.canSubmitAdd.value).toBe(false)

    const connected = {
      ...registered,
      ...host(enrollment.id, false, ''),
      kind: 'light_node' as const,
      name: '英国AMR',
    }
    mocks.hosts.mockResolvedValueOnce({
      ...initial,
      items: [...initial.items, unrelated, connected],
      total: 4,
      remoteTotal: 3,
    })
    await view.load(true)

    expect(view.lightEnrollmentConnected.value).toBe(true)
    expect(view.lightEnrollmentState.value).toBe('connected')
    expect(view.canSubmitAdd.value).toBe(true)
    await view.addHost()

    expect(mocks.add).not.toHaveBeenCalled()
    expect(mocks.toastSuccess).toHaveBeenCalledWith(
      '轻量节点已连接',
      '英国AMR 已完成首次状态上报。',
    )
    expect(mocks.toastSuccess).toHaveBeenCalledWith(
      '轻量节点已添加',
      '节点已出现在当前主机列表。',
    )
    expect(view.lightEnrollment.value).toBeUndefined()
  })

  it('explains the only missing prerequisite when no authenticated HTTPS origin exists', async () => {
    const view = setupView()
    mocks.createLightEnrollment.mockRejectedValueOnce(
      new ApiError('Light node HTTPS origin is required', 422, 'cluster_light_https_required'),
    )

    await view.createLightEnrollment()

    expect(view.lightEnrollment.value).toBeUndefined()
    expect(mocks.toastDanger).toHaveBeenCalledWith(
      '轻量节点命令生成失败',
      '轻量节点需要可从被控机访问的 HTTPS 根地址。请先通过 k fd 为 KPanel 绑定域名后重试。',
    )
  })

  it('falls back to a temporary selection when the Clipboard API is blocked', async () => {
    const view = setupView()
    const input = {
      value: '',
      readOnly: false,
      style: {} as Record<string, string>,
      setAttribute: vi.fn(),
      focus: vi.fn(),
      select: vi.fn(),
      setSelectionRange: vi.fn(),
      remove: vi.fn(),
    }
    const appendChild = vi.fn()
    const createElement = vi.fn(() => input)
    mocks.clipboardWriteText.mockRejectedValue(new Error('clipboard permission denied'))
    mocks.execCommand.mockReturnValue(true)
    vi.stubGlobal('document', {
      body: { appendChild },
      createElement,
      execCommand: mocks.execCommand,
    })

    view.pairingCode.value = {
      code: 'kp2.one-time-code',
      scope: 'cluster.summary.read',
      expiresAt: '2026-07-29T10:05:00Z',
    }
    await view.copyAccessCredential()

    expect(mocks.execCommand).toHaveBeenCalledTimes(1)
    expect(mocks.toastSuccess).toHaveBeenCalledWith('接入凭据已复制')
    expect(input.remove).toHaveBeenCalledTimes(1)
    expect(mocks.toastDanger).not.toHaveBeenCalled()
  })

  it('warns before opening an HTTP management page while preserving the exact IP and port', () => {
    const view = setupView()
    const remote = host('direct', false, 'http://198.51.100.20:8080')

    mocks.confirm.mockReturnValueOnce(false)
    view.openPanel(remote)
    expect(mocks.open).not.toHaveBeenCalled()
    expect(mocks.confirm).toHaveBeenCalledWith(
      expect.stringContaining('管理页面仍通过普通 HTTP 打开'),
    )

    mocks.confirm.mockReturnValueOnce(true)
    view.openPanel(remote)
    expect(mocks.open).toHaveBeenCalledWith(
      'http://198.51.100.20:8080',
      '_blank',
      'noopener,noreferrer',
    )
  })

  it('classifies HTTPS and encrypted HTTP IP origins before submission', () => {
    const view = setupView()
    const cases = [
      ['https://panel.example.com:8443', 'tls'],
      ['http://198.51.100.20:8080', 'e2e_http'],
      ['http://[2606:4700:4700::1111]:8080', 'e2e_http'],
      ['http://panel.example.com:8080', 'invalid'],
      ['http://198.51.100.20', 'invalid'],
      ['http://198.51.100.20:80', 'invalid'],
      ['http://198.51.100.20:8080/admin', 'invalid'],
      ['https://panel.example.com/admin', 'invalid'],
    ] as const

    for (const [origin, expected] of cases) {
      view.addForm.accessCredential = view.formatClusterAccessCredential(
        origin,
        'kp2.one-time-code',
      )
      expect(view.originAssessment.value.mode, origin).toBe(expected)
    }
  })

  it('parses one combined paste and rejects incomplete or malformed credentials', () => {
    const view = setupView()
    const bundled = view.formatClusterAccessCredential(
      'https://panel.example.com',
      'kp2.one-time-code',
    )

    view.addForm.accessCredential = bundled
    expect(view.parsedAccessCredential.value).toEqual({
      origin: 'https://panel.example.com',
      pairingCode: 'kp2.one-time-code',
    })
    expect(
      view.parseClusterAccessCredential(
        'https://legacy.example.com\nkp2.legacy-one-time-code',
      ),
    ).toEqual({
      origin: 'https://legacy.example.com',
      pairingCode: 'kp2.legacy-one-time-code',
    })
    expect(view.parseClusterAccessCredential('https://panel.example.com')).toBeUndefined()
    expect(
      view.parseClusterAccessCredential(
        'https://panel.example.com\nkp2.invalid code',
      ),
    ).toBeUndefined()
  })

  it('describes the negotiated transport and shortens long peer fingerprints', () => {
    const view = setupView()
    const httpsHost = host('tls', false, 'https://hk.example.com')
    const directHost = host('direct', false, 'http://198.51.100.20:8080')

    expect(view.transportSecurityLabel(httpsHost)).toBe('HTTPS')
    expect(view.transportSecurityLabel(directHost)).toBe('加密直连')
    expect(view.shortFingerprint(directHost.peerFingerprint)).toMatch(/^sha256:a+…a{8}$/)
  })

  it('names the private-network allowlist settings when an origin is blocked', async () => {
    const view = setupView()
    mocks.add.mockRejectedValueOnce(
      new ApiError('Cluster origin is blocked', 422, 'cluster_origin_blocked'),
    )
    view.addForm.accessCredential = view.formatClusterAccessCredential(
      'http://10.120.6.54:8080',
      `kp2.${'a'.repeat(180)}`,
    )

    await view.addHost()

    const [title, detail] = mocks.toastDanger.mock.calls.at(-1) ?? []
    expect(title).toBe('添加主机失败')
    expect(detail).toContain('KPANEL_CLUSTER_PRIVATE_CIDRS')
    expect(detail).toContain('KEJILION_PANEL_CLUSTER_PRIVATE_CIDRS')
    expect(detail).toContain('可信代理 CIDR 不控制此项')
    for (const locale of [english, traditionalChinese]) {
      expect(new Map(locale).get(detail)).toContain('KEJILION_PANEL_CLUSTER_PRIVATE_CIDRS')
    }
  })

  it('does not report an unfinished two-phase pairing as complete', async () => {
    const view = setupView()
    const pending = host('pending', false, 'http://198.51.100.20:8080')
    pending.state = 'pairing'
    mocks.add.mockResolvedValueOnce(pending)
    mocks.hosts.mockResolvedValueOnce(inventory())
    view.addForm.accessCredential = view.formatClusterAccessCredential(
      pending.origin,
      `kp2.${'a'.repeat(180)}`,
    )

    await view.addHost()

    expect(mocks.add).toHaveBeenCalledWith({
      name: undefined,
      origin: pending.origin,
      pairingCode: `kp2.${'a'.repeat(180)}`,
    })
    expect(mocks.toastSuccess).toHaveBeenCalledWith(
      '主机已加入集群',
      expect.stringContaining('安全配对正在后台继续'),
    )
    expect(mocks.toastSuccess).not.toHaveBeenCalledWith(
      '主机已加入集群',
      expect.stringContaining('双向文件互传已自动启用'),
    )
  })

  it('reports whether a completed pairing enabled two-way files automatically', async () => {
    const enabledView = setupView()
    const enabled = host('paired-mutual', false, 'https://files.example.com')
    enabled.federationProtocol = 'v2'
    enabled.fileTransferAvailable = true
    enabled.mutualFileTransferAvailable = true
    mocks.add.mockResolvedValueOnce(enabled)
    mocks.hosts.mockResolvedValueOnce(inventory())
    enabledView.addForm.accessCredential = enabledView.formatClusterAccessCredential(
      enabled.origin,
      `kp2.${'b'.repeat(180)}`,
    )

    await enabledView.addHost()

    expect(mocks.toastSuccess).toHaveBeenLastCalledWith(
      '主机已加入集群',
      '香港节点 已完成配对，双向文件互传已自动启用。',
    )

    mocks.toastSuccess.mockClear()
    const oneWayView = setupView()
    const oneWay = host('paired-one-way', false, 'https://old-files.example.com')
    oneWay.federationProtocol = 'v2'
    oneWay.fileTransferAvailable = true
    mocks.add.mockResolvedValueOnce(oneWay)
    mocks.hosts.mockResolvedValueOnce(inventory())
    oneWayView.addForm.accessCredential = oneWayView.formatClusterAccessCredential(
      oneWay.origin,
      `kp2.${'c'.repeat(180)}`,
    )

    await oneWayView.addHost()

    expect(mocks.toastSuccess).toHaveBeenLastCalledWith(
      '主机已加入集群',
      '香港节点 已完成配对，当前保持单向文件读取；可在主机管理中启用，旧版 KPanel 需先升级。',
    )
  })

  it('enables mutual file transfer only for an active remote v2 host with file scope', async () => {
    const view = setupView()
    const list = inventory()
    const remote = host('mutual-files', false, 'https://files.example.com')
    remote.federationProtocol = 'v2'
    remote.scope = 'cluster.summary.read cluster.files.read'
    remote.fileTransferAvailable = true
    list.items = [remote]
    list.total = 1
    view.inventory.value = list
    view.openManage(remote)

    expect(view.mutualFilesHostEligible(remote)).toBe(true)
    expect(view.mutualFilesHostEligible({ ...remote, isLocal: true })).toBe(false)
    expect(view.mutualFilesHostEligible({ ...remote, federationProtocol: 'v1' })).toBe(false)
    expect(view.mutualFilesHostEligible({ ...remote, state: 'pairing' })).toBe(false)
    expect(view.mutualFilesHostEligible({
      ...remote,
      scope: 'cluster.summary.read',
      fileTransferAvailable: false,
    })).toBe(false)
    expect(view.mutualFilesHostEligible({ ...remote, kind: 'light_node' })).toBe(false)

    const enabled = {
      ...remote,
      mutualFileTransferAvailable: true,
      resourceVersion: 'mutual-files-enabled-version',
    }
    mocks.enableMutualFiles.mockResolvedValueOnce(enabled)

    await view.enableMutualFiles()

    expect(mocks.enableMutualFiles).toHaveBeenCalledOnce()
    expect(mocks.enableMutualFiles).toHaveBeenCalledWith(remote.id)
    expect(view.enablingMutualFiles.value).toBe(false)
    expect(view.selected.value).toEqual(enabled)
    expect(view.inventory.value?.items[0]).toEqual(enabled)
    expect(mocks.toastSuccess).toHaveBeenCalledWith(
      '双向文件互传已启用',
      '香港节点 现在可以与当前 KPanel 互相复制文件。',
    )

    const source = readFileSync(new URL('./ClusterView.vue', import.meta.url), 'utf8')
    expect(source).toContain('v-if="mutualFilesHostEligible(selected)"')
    expect(source).toContain('v-if="selected.mutualFileTransferAvailable"')
    expect(source).toContain("enablingMutualFiles ? '正在启用…' : '启用双向文件互传'")
    expect(source).toContain("enablingMutualFiles ? '正在刷新…' : '刷新连接'")
  })

  it('keeps the old pairing unchanged when mutual file transfer cannot be enabled', async () => {
    const view = setupView()
    const remote = host('mutual-files-failure', false, 'https://files.example.com')
    remote.federationProtocol = 'v2'
    remote.scope = 'cluster.summary.read cluster.files.read'
    remote.fileTransferAvailable = true
    view.inventory.value = { ...inventory(), items: [remote], total: 1 }
    view.openManage(remote)
    mocks.enableMutualFiles.mockRejectedValueOnce(
      new ApiError('protocol incompatible', 426, 'cluster_mutual_files_unsupported'),
    )

    await view.enableMutualFiles()

    expect(view.enablingMutualFiles.value).toBe(false)
    expect(view.selected.value?.mutualFileTransferAvailable).toBe(false)
    expect(mocks.toastDanger).toHaveBeenCalledWith(
      '启用双向文件互传失败',
      '目标 KPanel 版本不支持双向文件互传，请先升级目标面板后重试。',
    )
  })

  it('refreshes an active mutual file connection and preserves its state on failure', async () => {
    const view = setupView()
    const active = host('mutual-files-active', false, 'https://files.example.com')
    active.federationProtocol = 'v2'
    active.scope = 'cluster.summary.read cluster.files.read'
    active.fileTransferAvailable = true
    active.mutualFileTransferAvailable = true
    view.inventory.value = { ...inventory(), items: [active], total: 1 }
    view.openManage(active)
    const refreshed = { ...active, resourceVersion: 'mutual-files-refreshed-version' }
    mocks.enableMutualFiles.mockResolvedValueOnce(refreshed)

    await view.enableMutualFiles()

    expect(mocks.enableMutualFiles).toHaveBeenCalledOnce()
    expect(mocks.enableMutualFiles).toHaveBeenCalledWith(active.id)
    expect(view.selected.value).toEqual(refreshed)
    expect(view.inventory.value?.items[0]).toEqual(refreshed)
    expect(mocks.toastSuccess).toHaveBeenCalledWith(
      '双向文件互传连接已刷新',
      '香港节点 已使用当前 KPanel 地址刷新互传连接。',
    )

    mocks.enableMutualFiles.mockRejectedValueOnce(
      new ApiError('remote unavailable', 503, 'cluster_remote_unreachable'),
    )
    await view.enableMutualFiles()

    expect(mocks.enableMutualFiles).toHaveBeenCalledTimes(2)
    expect(view.enablingMutualFiles.value).toBe(false)
    expect(view.selected.value?.mutualFileTransferAvailable).toBe(true)
    expect(view.inventory.value?.items[0]?.mutualFileTransferAvailable).toBe(true)
    expect(mocks.toastDanger).toHaveBeenCalledWith(
      '刷新双向文件互传连接失败',
      '暂时无法连接目标 KPanel，请检查域名、证书和网络。',
    )
  })

  it('enables, copies, and rotates the anonymous public share link', async () => {
    const view = setupView()
    const initial: ClusterShareSettings = {
      enabled: false,
      title: 'My fleet',
      description: 'Public status',
      resourceVersion: 'share-v1',
    }
    const enabled: ClusterShareSettings = {
      ...initial,
      enabled: true,
      sharePath: `/share/${'a'.repeat(64)}`,
      resourceVersion: 'share-v2',
    }
    const rotated: ClusterShareSettings = {
      ...enabled,
      sharePath: `/share/${'b'.repeat(64)}`,
      resourceVersion: 'share-v3',
    }
    mocks.shareSettings.mockResolvedValueOnce(initial)
    mocks.updateShare.mockResolvedValueOnce(enabled)
    mocks.resetShareToken.mockResolvedValueOnce(rotated)

    await view.openShare()
    expect(view.shareOpen.value).toBe(true)
    view.shareForm.enabled = true
    view.hostOrder.value = ['remote', 'local']
    await view.saveShare()

    // Saving never mirrors the panel order implicitly.
    expect(mocks.updateShare).toHaveBeenCalledWith({
      enabled: true,
      title: 'My fleet',
      description: 'Public status',
      expectedResourceVersion: 'share-v1',
    })
    expect(view.shareForm.applyPanelOrder).toBe(false)

    // An explicit request copies the current panel order once.
    mocks.updateShare.mockResolvedValueOnce(enabled)
    view.shareForm.applyPanelOrder = true
    await view.saveShare()
    expect(mocks.updateShare).toHaveBeenLastCalledWith(expect.objectContaining({ hostOrder: ['remote', 'local'] }))
    expect(view.shareForm.applyPanelOrder).toBe(false)
    expect(view.shareURL.value).toBe(`https://center.example.com/share/${'a'.repeat(64)}`)
    await view.copyShareLink()
    expect(mocks.clipboardWriteText).toHaveBeenCalledWith(view.shareURL.value)

    await view.resetShareLink()
    expect(mocks.resetShareToken).toHaveBeenCalledWith('share-v2')
    expect(view.shareURL.value).toBe(`https://center.example.com/share/${'b'.repeat(64)}`)
  })

  it('lets the local node update its display name while keeping removal unavailable', async () => {
    const view = setupView()
    const list = inventory()
    const local = list.items[0]
    if (!local) throw new Error('local fixture is missing')
    view.inventory.value = list

    view.search.value = '当前面板'
    expect(view.filteredHosts.value).toHaveLength(1)
    expect(view.filteredHosts.value[0]).toMatchObject({ id: 'local', isLocal: true })

    const renamed = { ...local, name: '控制中心', resourceVersion: 'local-renamed-version' }
    mocks.rename.mockResolvedValueOnce(renamed)
    view.openManage(local)
    view.editName.value = renamed.name
    await view.saveName()
    await view.removeHost()

    expect(view.manageOpen.value).toBe(true)
    expect(view.selected.value).toMatchObject({ id: 'local', name: '控制中心' })
    expect(mocks.rename).toHaveBeenCalledWith('local', {
      name: '控制中心',
      expectedResourceVersion: local.resourceVersion,
    })
    expect(mocks.remove).not.toHaveBeenCalled()
    expect(view.inventory.value?.items.some((item) => item.isLocal)).toBe(true)
  })
})
