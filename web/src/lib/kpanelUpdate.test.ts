import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  detectKPanelUpdate,
  findKPanelApp,
  isKPanelSelfUpdate,
  kpanelUpdateHint,
} from './kpanelUpdate'
import type { AppMarketInventory } from '@/types/api'

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
})
