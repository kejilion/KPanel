// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import FileShareManagerDialog from './FileShareManagerDialog.vue'
import englishSharedCatalog from '@/i18n/pages/shared/en-US'
import { registerPhraseCatalog, resetPhraseLocalizationForTest } from '@/i18n/phrase'
import { ApiError } from '@/lib/api'
import type { FileShareListItem } from '@/types/api'

const mocks = vi.hoisted(() => ({
  shares: vi.fn(),
  deleteShare: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {
    readonly status: number
    constructor(message: string, status = 0) {
      super(message)
      this.status = status
    }
  },
  api: { files: { shares: mocks.shares, deleteShare: mocks.deleteShare } },
}))

const wrappers: VueWrapper[] = []

function share(id: string, path: string, expiresAt?: string): FileShareListItem {
  return {
    id,
    path,
    createdAt: '2026-08-22T00:00:00Z',
    expiresAt,
    linksAvailable: false,
  }
}

function mountDialog(): VueWrapper {
  const wrapper = mount(FileShareManagerDialog, { attachTo: document.body })
  wrappers.push(wrapper)
  return wrapper
}

beforeEach(() => {
  resetPhraseLocalizationForTest()
  vi.clearAllMocks()
  vi.spyOn(window, 'confirm').mockReturnValue(true)
})

afterEach(() => {
  for (const wrapper of wrappers.splice(0).reverse()) wrapper.unmount()
  document.body.innerHTML = ''
  resetPhraseLocalizationForTest()
  vi.restoreAllMocks()
})

describe('FileShareManagerDialog', () => {
  it('lists permanent and expired shares even when their source files are no longer reachable', async () => {
    const values = [
      share('share-permanent', '/deleted/site-logo.png'),
      share('share-expired', '/moved/archive.zip', '2000-01-01T00:00:00Z'),
    ]
    mocks.shares.mockResolvedValueOnce({ shares: values })
    mountDialog()
    await flushPromises()

    expect(mocks.shares).toHaveBeenCalledWith(expect.any(AbortSignal))
    expect(document.body.textContent).toContain('/deleted/site-logo.png')
    expect(document.body.textContent).toContain('/moved/archive.zip')
    expect(document.body.textContent).toContain('永久有效')
    expect(document.body.textContent).toContain('已过期')
  })

  it('stops a stored share by id and removes it from the list', async () => {
    const value = share('share-one', '/missing/old-image.png')
    mocks.shares.mockResolvedValueOnce({ shares: [value] })
    mocks.deleteShare.mockResolvedValueOnce(undefined)
    mountDialog()
    await flushPromises()

    const stop = document.querySelector<HTMLButtonElement>(
      '[aria-label="停止分享 /missing/old-image.png"]',
    )!
    stop.click()
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith('停止这个分享后，现有链接会立即失效。确认继续吗？')
    expect(mocks.deleteShare).toHaveBeenCalledWith(value.id)
    expect(document.body.textContent).not.toContain('/missing/old-image.png')
    expect(document.body.textContent).toContain('分享已停止。')
    expect(document.body.textContent).toContain('还没有文件分享')
  })

  it('keeps an actionable retry state when listing fails', async () => {
    mocks.shares
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({ shares: [] })
    mountDialog()
    await flushPromises()

    expect(document.body.textContent).toContain('暂时无法读取分享列表，请稍后重试。')
    const retry = [...document.querySelectorAll<HTMLButtonElement>('button')]
      .find((candidate) => candidate.textContent?.includes('重试'))!
    retry.click()
    await flushPromises()

    expect(mocks.shares).toHaveBeenCalledTimes(2)
    expect(document.body.textContent).toContain('还没有文件分享')
  })

  it('localizes content rendered through the teleported modal', async () => {
    mocks.shares.mockResolvedValueOnce({
      shares: [share('share-one', '/missing/old-image.png')],
    })
    registerPhraseCatalog(englishSharedCatalog)
    mountDialog()
    await flushPromises()

    expect(document.querySelector('[role="dialog"]')?.getAttribute('aria-label')).toBe('Share management')
    expect(document.body.textContent).toContain('Manage file shares in one place.')
    expect(document.body.textContent).toContain('Never expires')
    expect(document.body.textContent).not.toContain('集中管理文件分享。')
  })

  it('does not expose a Simplified Chinese API error in the English dialog', async () => {
    mocks.shares.mockRejectedValueOnce(new ApiError('文件分享存储不可用', 503))
    registerPhraseCatalog(englishSharedCatalog)
    mountDialog()
    await flushPromises()

    expect(document.body.textContent).toContain('Unable to load the share list. Try again later.')
    expect(document.body.textContent).not.toContain('文件分享存储不可用')
  })
})
