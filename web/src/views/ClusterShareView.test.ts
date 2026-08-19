import { readFileSync } from 'node:fs'
import { createSSRApp, ssrContextKey, type ComputedRef, type Ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ClusterShareView from './ClusterShareView.vue'
import type { PublicClusterShareSnapshot } from '@/types/api'

const mocks = vi.hoisted(() => ({
  publicShare: vi.fn(),
  token: 'a'.repeat(64),
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
  snapshot: Ref<PublicClusterShareSnapshot | undefined>
  errorMessage: Ref<string>
  tokenIsValid: ComputedRef<boolean>
  viewMode: Ref<'list' | 'card'>
  load: (silent?: boolean) => Promise<void>
  setViewMode: (mode: 'list' | 'card') => void
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
      network: { receiveBytesPerSecond: 1024, transmitBytesPerSecond: 2048 },
      location: { country: 'Singapore', countryCode: 'SG', city: 'Singapore', isp: 'Example ISP' },
      collectedAt: '2026-08-15T12:00:00Z',
    }],
  }
}

beforeEach(() => {
  mocks.token = 'a'.repeat(64)
  mocks.publicShare.mockReset()
})

afterEach(() => vi.unstubAllGlobals())

describe('ClusterShareView anonymous snapshot', () => {
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

  it('defaults to the compact list and can switch to cards', () => {
    const view = setupView()

    expect(view.viewMode.value).toBe('list')
    view.setViewMode('card')
    expect(view.viewMode.value).toBe('card')
  })

  it('keeps management and identity fields out of the public page template', () => {
    const source = readFileSync(new URL('./ClusterShareView.vue', import.meta.url), 'utf8')
    expect(source).toContain('公开页不包含 IP、管理入口或访问凭据')
    for (const privateField of ['origin', 'peerFingerprint', 'remoteNodeId', 'resourceVersion']) {
      expect(source).not.toContain(`host.${privateField}`)
    }
    expect(source).toContain('<OperatingSystemIcon')
    expect(source).toContain('<CountryFlagIcon')
    expect(source).toContain("const viewMode = ref<'list' | 'card'>('list')")
    expect(source).toContain('const { resolved: resolvedTheme, setTheme } = useTheme()')
    expect(source).toContain(':class="`is-${viewMode}`"')
    expect(source).toMatch(/@media \(max-width: 650px\)[\s\S]*?\.share-grid\s*\{[^}]*grid-template-columns:\s*minmax\(0, 1fr\);/)

    const routerSource = readFileSync(new URL('../router.ts', import.meta.url), 'utf8')
    expect(routerSource).toContain("path: '/share/:token'")
    expect(routerSource).toContain("public: true, skipSessionCheck: true")
    expect(routerSource.indexOf('if (to.meta.skipSessionCheck) return true')).toBeLessThan(
      routerSource.indexOf('const session = useSession()'),
    )
  })
})
