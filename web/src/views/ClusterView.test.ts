import { readFileSync } from 'node:fs'
import { createSSRApp, ssrContextKey, type ComputedRef, type Ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ClusterView from './ClusterView.vue'
import { ApiError } from '@/lib/api'
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
  panelOrigin: ComputedRef<string>
  parsedAccessCredential: ComputedRef<{ origin: string; pairingCode: string } | undefined>
  accessCredentialText: ComputedRef<string>
  search: Ref<string>
  viewMode: Ref<'list' | 'card'>
  hostOrder: Ref<string[]>
  accessOpen: Ref<boolean>
  manageOpen: Ref<boolean>
  shareOpen: Ref<boolean>
  shareSettings: Ref<ClusterShareSettings | undefined>
  shareURL: ComputedRef<string>
  shareForm: { enabled: boolean; title: string; description: string }
  enablingMutualFiles: Ref<boolean>
  selected: Ref<ClusterHost | undefined>
  pairingCode: Ref<ClusterPairingCode | undefined>
  lightEnrollment: Ref<ClusterLightEnrollment | undefined>
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
  setViewMode: (mode: 'list' | 'card') => void
  moveHost: (hostID: string, offset: number) => void
  transportSecurityLabel: (host: ClusterHost) => string
  shortFingerprint: (value?: string) => string
  hostOperatingSystemIdentity: (host: ClusterHost) => { key: string; label: string }
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

describe('ClusterView compact summary layout', () => {
  it('keeps summary metrics and actions on one decorated row', () => {
    const source = readFileSync(new URL('./ClusterView.vue', import.meta.url), 'utf8')

    expect(source).toMatch(/\.cluster-hero\s*\{[^}]*display:\s*grid;[^}]*grid-template-columns:\s*max-content minmax\(0, 1fr\);/)
    expect(source).toMatch(/\.cluster-stats\s*\{[^}]*grid-template-columns:\s*repeat\(4, minmax\(116px, 132px\)\);/)
    expect(source).toMatch(/\.cluster-hero\s*\{[^}]*radial-gradient\([^}]*var\(--cluster-accent\)/)
    expect(source).toMatch(/\.cluster-hero__actions\s*\{[^}]*flex-wrap:\s*nowrap;/)
    expect(source).toContain('class="icon-button icon-button--small"')
    expect(source).toContain('aria-label="刷新集群状态"')
    expect(source.indexOf('aria-label="刷新集群状态"')).toBeLessThan(source.indexOf('@click="openAccess"'))
  })
})

describe('ClusterView inventory and navigation', () => {
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

    view.openPanel(light)

    expect(mocks.open).not.toHaveBeenCalled()
    expect(mocks.confirm).not.toHaveBeenCalled()
    expect(view.transportSecurityLabel(light)).toBe('轻量节点')
  })

  it('defaults to the row list and persists an explicit card preference', () => {
    const view = setupView()

    expect(view.viewMode.value).toBe('list')
    view.setViewMode('card')

    expect(view.viewMode.value).toBe('card')
    expect(mocks.localStorageSetItem).toHaveBeenCalledWith(
      'kpanel:cluster-host-view',
      'card',
    )
  })

  it('reorders the same host inventory and persists the preference locally', () => {
    const view = setupView()
    view.inventory.value = inventory()
    view.hostOrder.value = ['local', 'remote']

    view.moveHost('remote', -1)

    expect(view.filteredHosts.value.map((item) => item.id)).toEqual(['remote', 'local'])
    expect(mocks.localStorageSetItem).toHaveBeenCalledWith(
      'kpanel:cluster-host-order',
      JSON.stringify(['remote', 'local']),
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
      command:
        "bash <(curl -fsSL https://kejilion.sh) kpanel node join 'kpl1.example-token'",
      expiresAt: '2026-07-29T10:05:00Z',
    }
    mocks.createLightEnrollment.mockResolvedValueOnce(enrollment)

    await view.createLightEnrollment()
    await view.copyLightEnrollment()

    expect(view.lightEnrollment.value).toEqual(enrollment)
    expect(mocks.createLightEnrollment).toHaveBeenCalledOnce()
    expect(mocks.clipboardWriteText).toHaveBeenCalledWith(enrollment.command)
    expect(mocks.toastSuccess).toHaveBeenCalledWith('轻量节点接入命令已复制')
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
    await view.saveShare()

    expect(mocks.updateShare).toHaveBeenCalledWith({
      enabled: true,
      title: 'My fleet',
      description: 'Public status',
      expectedResourceVersion: 'share-v1',
    })
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
