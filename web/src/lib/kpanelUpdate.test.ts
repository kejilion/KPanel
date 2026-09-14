import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  detectKPanelUpdate,
  findKPanelApp,
  isKPanelSelfUpdate,
  isKPanelUpdateSettingsIntent,
  kpanelAppUpdatePath,
  kpanelReleasesURL,
  kpanelUpdateHint,
  kpanelUpdateSettingsPath,
  kpanelUpdateSettingsSection,
  officialKPanelReleaseURL,
  releaseFromAutomaticUpdate,
  releaseMatchesTarget,
} from './kpanelUpdate'
import type { AppMarketInventory, KPanelReleaseInfo } from '@/types/api'

const mocks = vi.hoisted(() => ({
  inventory: vi.fn(),
  checkUpdate: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    apps: {
      inventory: mocks.inventory,
      checkUpdate: mocks.checkUpdate,
    },
  },
}))

function inventory(installed = true, canCheck = true): AppMarketInventory {
  return {
    schemaVersion: 1,
    source: 'test',
    scriptSha256: 'a'.repeat(64),
    catalogMode: 'embedded',
    categories: [],
    installed: installed ? 1 : 0,
    running: installed ? 1 : 0,
    updateAvailable: 0,
    collectedAt: '2026-07-31T00:00:00Z',
    items: [
      {
        id: 'thirdparty-kpanel',
        source: 'thirdparty',
        token: 'kpanel',
        name_zh: 'KPanel',
        name_en: 'KPanel',
        desc_zh: 'KPanel',
        desc_en: 'KPanel',
        cat: 'ops',
        icon: '/app-icons/kpanel.webp',
        iconSha256: 'b'.repeat(64),
        slug: 'kpanel',
        installer: 'kejilion',
        runtime: {
          installed,
          state: installed ? 'running' : 'not_installed',
          ports: [],
          accessMode: installed ? 'direct' : 'not_applicable',
          updateStatus: installed ? 'check_required' : 'not_installed',
          resourceVersion: installed ? 'resource-version' : undefined,
          detectedBy: installed ? ['docker'] : [],
        },
        capabilities: {
          check_update: canCheck ? { enabled: true } : { enabled: false, reason: 'unavailable' },
        },
      },
    ],
  }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('KPanel update detection', () => {
  it('keeps the settings target and app-market fallback as canonical shared paths', () => {
    expect(kpanelUpdateSettingsSection).toBe('version-updates')
    expect(kpanelUpdateSettingsPath).toBe('/settings?section=version-updates')
    expect(kpanelAppUpdatePath).toBe('/apps?app=kpanel&action=update')
    expect(isKPanelUpdateSettingsIntent('version-updates')).toBe(true)
    expect(isKPanelUpdateSettingsIntent(['version-updates'])).toBe(false)
  })

  it('does not report fixed image references as current or available', async () => {
    mocks.inventory.mockResolvedValue(inventory())
    mocks.checkUpdate.mockResolvedValue({ status: 'fixed', updateAvailable: false })
    await expect(detectKPanelUpdate()).resolves.toBe('unavailable')
  })
  it('checks only the installed KPanel application through the existing image API', async () => {
    const current = inventory()
    mocks.inventory.mockResolvedValue(current)
    mocks.checkUpdate.mockResolvedValue({ status: 'available', updateAvailable: true })

    await expect(detectKPanelUpdate()).resolves.toBe('available')
    expect(findKPanelApp(current)?.token).toBe('kpanel')
    expect(mocks.checkUpdate).toHaveBeenCalledWith('thirdparty-kpanel', 'resource-version')
  })

  it('does not query the registry when KPanel is not manageable', async () => {
    mocks.inventory.mockResolvedValue(inventory(false, false))

    await expect(detectKPanelUpdate()).resolves.toBe('unavailable')
    expect(mocks.checkUpdate).not.toHaveBeenCalled()
  })

  it('recognizes only the KPanel update job as a self update', () => {
    expect(isKPanelSelfUpdate({ appId: 'thirdparty-kpanel', action: 'update' })).toBe(true)
    expect(isKPanelSelfUpdate({ appId: 'thirdparty-kpanel', action: 'uninstall' })).toBe(false)
    expect(isKPanelSelfUpdate({ appId: 'builtin-13', action: 'update' })).toBe(false)
  })

  it('describes the installed version without presenting it as the update target', () => {
    expect(kpanelUpdateHint('0.34.1')).toBe(
      '当前版本 v0.34.1，发现可用更新，点击更新到最新版本',
    )
    expect(kpanelUpdateHint('v0.34.1')).toBe(
      '当前版本 v0.34.1，发现可用更新，点击更新到最新版本',
    )
    expect(kpanelUpdateHint()).toBe('发现 KPanel 新版本，点击更新到最新版本')
  })

  it('builds only canonical official release links and pins notes to the target identity', () => {
    const digest = `sha256:${'a'.repeat(64)}`
    const release: KPanelReleaseInfo = {
      channel: 'stable', version: '1.18.0', imageDigest: digest,
      notes: [{ kind: 'added', text: '显示更新内容' }], cached: true, stale: false,
    }

    expect(officialKPanelReleaseURL('V1.18.0')).toBe(`${kpanelReleasesURL}/tag/v1.18.0`)
    expect(officialKPanelReleaseURL('1.18')).toBe(kpanelReleasesURL)
    expect(releaseMatchesTarget(release, 'v1.18.0', digest)).toBe(true)
    expect(releaseMatchesTarget(release, '1.18.1', digest)).toBe(false)
    expect(releaseMatchesTarget(release, '1.18.0', `sha256:${'b'.repeat(64)}`)).toBe(false)

    expect(releaseFromAutomaticUpdate({
      available: true,
      enabled: false,
      state: 'waiting',
      channel: 'stable',
      canInstall: true,
      installRequested: false,
      schedule: 'daily-04:00-local',
      observationHours: 24,
      candidateVersion: '1.18.0',
      candidateImageDigest: digest,
      candidateNotes: release.notes,
      resourceVersion: 'sha256:state',
    })).toMatchObject(release)
  })
})
