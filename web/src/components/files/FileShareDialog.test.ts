// @vitest-environment jsdom
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import FileShareDialog from './FileShareDialog.vue'
import englishSharedCatalog from '@/i18n/pages/shared/en-US'
import { registerPhraseCatalog, resetPhraseLocalizationForTest } from '@/i18n/phrase'
import { ApiError } from '@/lib/api'
import type { FileEntry, FileShareAdminView } from '@/types/api'

const mocks = vi.hoisted(() => ({
  share: vi.fn(),
  createShare: vi.fn(),
  deleteShare: vi.fn(),
  writeText: vi.fn(),
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
    files: {
      share: mocks.share,
      createShare: mocks.createShare,
      deleteShare: mocks.deleteShare,
    },
  },
}))

const wrappers: VueWrapper[] = []

function entry(overrides: Partial<FileEntry> = {}): FileEntry {
  return {
    name: 'site logo.png',
    path: '/home/site logo.png',
    kind: 'file',
    mime: 'image/png',
    sizeBytes: 2048,
    mode: '-rw-r--r--',
    owner: 'root',
    group: 'root',
    modifiedAt: '2026-08-22T00:00:00Z',
    resourceVersion: 'sha256:file',
    editable: false,
    previewable: true,
    ...overrides,
  }
}

function activeShare(linksAvailable = false): FileShareAdminView {
  const token = 'a'.repeat(43)
  return {
    id: 'b'.repeat(22),
    createdAt: '2026-08-22T00:00:00Z',
    expiresAt: '2026-08-29T00:00:00Z',
    linksAvailable,
    sharePath: linksAvailable ? `/share/file/${token}` : undefined,
    directPath: linksAvailable ? `/f/${token}` : undefined,
  }
}

function mountDialog(fileEntry = entry()): VueWrapper {
  const wrapper = mount(FileShareDialog, {
    attachTo: document.body,
    props: { entry: fileEntry },
  })
  wrappers.push(wrapper)
  return wrapper
}

function button(label: string): HTMLButtonElement {
  const match = [...document.querySelectorAll<HTMLButtonElement>('button')]
    .find((candidate) => candidate.textContent?.trim().includes(label))
  if (!match) throw new Error(`Missing button: ${label}`)
  return match
}

beforeEach(() => {
  resetPhraseLocalizationForTest()
  vi.clearAllMocks()
  mocks.share.mockResolvedValue({ share: null })
  mocks.writeText.mockResolvedValue(undefined)
  vi.spyOn(window, 'confirm').mockReturnValue(true)
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: mocks.writeText },
  })
})

afterEach(() => {
  for (const wrapper of wrappers.splice(0).reverse()) wrapper.unmount()
  document.body.innerHTML = ''
  resetPhraseLocalizationForTest()
  vi.restoreAllMocks()
})

describe('FileShareDialog hash-only sharing', () => {
  const maxFileShareBytes = 512 * 1024 * 1024

  it('defaults to seven days and shows newly created links exactly once', async () => {
    const created = activeShare(true)
    mocks.createShare.mockResolvedValueOnce(created)
    mountDialog()
    await flushPromises()

    expect(mocks.share).toHaveBeenCalledWith('/home/site logo.png', 'sha256:file', expect.any(AbortSignal))
    expect((document.querySelector('input[value="7d"]') as HTMLInputElement).checked).toBe(true)

    button('创建分享').click()
    await flushPromises()

    expect(mocks.createShare).toHaveBeenCalledWith({
      path: '/home/site logo.png',
      expectedResourceVersion: 'sha256:file',
      expectedShareID: '',
      expiresIn: '7d',
    })
    expect((document.querySelector('#file-share-page-link') as HTMLInputElement).value)
      .toContain(created.sharePath)
    expect((document.querySelector('#file-share-direct-link') as HTMLInputElement).value)
      .toContain(created.directPath)
    expect(document.querySelector('#file-share-direct-link')?.parentElement?.querySelector('a')).not.toBeNull()
    expect(document.body.textContent).toContain('链接仅在本次显示')

    button('复制').click()
    await flushPromises()
    expect(mocks.writeText).toHaveBeenCalledWith(expect.stringContaining(created.sharePath || ''))
    expect(document.body.textContent).toContain('已复制')
  })

  it('does not recover an old token and rotates with the loaded share CAS id', async () => {
    const existing = activeShare(false)
    const rotated = activeShare(true)
    mocks.share.mockResolvedValueOnce({ share: existing })
    mocks.createShare.mockResolvedValueOnce(rotated)
    mountDialog()
    await flushPromises()

    expect(document.body.textContent).toContain('旧链接不会保存在 KPanel 中')
    expect(document.querySelector('#file-share-page-link')).toBeNull()
    expect(document.querySelector('#file-share-direct-link')).toBeNull()

    ;(document.querySelector('input[value="never"]') as HTMLInputElement).click()
    button('重新生成链接').click()
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith('重新生成会立即停用旧链接。确认继续吗？')
    expect(mocks.createShare).toHaveBeenCalledWith({
      path: '/home/site logo.png',
      expectedResourceVersion: 'sha256:file',
      expectedShareID: existing.id,
      expiresIn: 'never',
    })
  })

  it('does not offer inline opening for a non-image direct link', async () => {
    const created = activeShare(true)
    mocks.createShare.mockResolvedValueOnce(created)
    mountDialog(entry({ name: 'notes.txt', path: '/home/notes.txt', mime: 'text/plain' }))
    await flushPromises()

    button('创建分享').click()
    await flushPromises()

    const directRow = document.querySelector('#file-share-direct-link')?.parentElement
    expect(directRow?.querySelector('a')).toBeNull()
    expect(document.body.textContent).toContain('非图片文件的直链会直接下载文件。')
  })

  it('stops the active share only after confirmation', async () => {
    const existing = activeShare(false)
    mocks.share.mockResolvedValueOnce({ share: existing })
    mocks.deleteShare.mockResolvedValueOnce(undefined)
    mountDialog()
    await flushPromises()

    button('停止分享').click()
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith(
      '停止后，分享页、文件直链和正在使用这些链接的网站引用都会失效。确认停止吗？',
    )
    expect(mocks.deleteShare).toHaveBeenCalledWith(existing.id)
    expect(document.body.textContent).toContain('分享已停止。')
    expect(document.body.textContent).toContain('创建分享')
  })

  it('selects the visible link for manual copying when clipboard access is denied', async () => {
    const created = activeShare(true)
    mocks.createShare.mockResolvedValueOnce(created)
    mocks.writeText.mockRejectedValueOnce(new Error('denied'))
    Object.defineProperty(document, 'execCommand', {
      configurable: true,
      value: vi.fn(() => false),
    })
    mountDialog()
    await flushPromises()
    button('创建分享').click()
    await flushPromises()

    button('复制').click()
    await flushPromises()

    const input = document.querySelector('#file-share-page-link') as HTMLInputElement
    expect(document.activeElement).toBe(input)
    expect(input.selectionStart).toBe(0)
    expect(input.selectionEnd).toBe(input.value.length)
    expect(document.body.textContent).toContain('链接已选中，请手动复制')
  })

  it('distinguishes the share limit conflict from a file/share CAS conflict', async () => {
    mocks.createShare.mockRejectedValueOnce(new ApiError(
      'limit reached',
      409,
      'file_share_limit_reached',
    ))
    mountDialog()
    await flushPromises()

    button('创建分享').click()
    await flushPromises()

    expect(document.body.textContent).toContain('请在分享管理中停止不再使用的分享后重试')
    expect(document.body.textContent).not.toContain('分享状态或文件已发生变化')
  })

  it('rechecks share state after an ambiguous create failure', async () => {
    const recovered = activeShare(false)
    mocks.share
      .mockResolvedValueOnce({ share: null })
      .mockResolvedValueOnce({ share: recovered })
    mocks.createShare.mockRejectedValueOnce(new Error('response interrupted'))
    mountDialog()
    await flushPromises()

    button('创建分享').click()
    await flushPromises()

    expect(document.body.textContent).toContain('暂时无法生成分享链接，请稍后重试。')
    expect(button('重试').disabled).toBe(false)

    button('重试').click()
    await flushPromises()

    expect(mocks.share).toHaveBeenCalledTimes(2)
    expect(button('重新生成链接').disabled).toBe(false)
    expect(document.body.textContent).not.toContain('暂时无法生成分享链接')
  })

  it('rechecks the CAS id after an ambiguous share rotation', async () => {
    const existing = activeShare(false)
    const recovered = { ...activeShare(false), id: 'c'.repeat(22) }
    const rotated = { ...activeShare(true), id: 'd'.repeat(22) }
    mocks.share
      .mockResolvedValueOnce({ share: existing })
      .mockResolvedValueOnce({ share: recovered })
    mocks.createShare
      .mockRejectedValueOnce(new Error('response interrupted'))
      .mockResolvedValueOnce(rotated)
    mountDialog()
    await flushPromises()

    button('重新生成链接').click()
    await flushPromises()

    expect(button('重试').disabled).toBe(false)
    expect(button('重新生成链接').disabled).toBe(true)

    button('重试').click()
    await flushPromises()
    expect(button('重新生成链接').disabled).toBe(false)

    button('重新生成链接').click()
    await flushPromises()

    expect(mocks.createShare).toHaveBeenLastCalledWith(expect.objectContaining({
      expectedShareID: recovered.id,
    }))
  })

  it('explains and blocks an oversized file while still loading its share state', async () => {
    mountDialog(entry({ sizeBytes: maxFileShareBytes + 1 }))
    await flushPromises()

    expect(mocks.share).toHaveBeenCalledOnce()
    expect(document.body.textContent).toContain('文件分享仅支持不超过 512 MiB 的文件。')
    expect(document.body.textContent).toContain('请压缩或拆分文件后重试。')
    expect(document.querySelector('.file-share-dialog__expiry')).toBeNull()
    expect(button('创建分享').disabled).toBe(true)
    button('创建分享').click()
    expect(mocks.createShare).not.toHaveBeenCalled()
  })

  it('allows a file exactly at the sharing size limit', async () => {
    mountDialog(entry({ sizeBytes: maxFileShareBytes }))
    await flushPromises()

    expect(button('创建分享').disabled).toBe(false)
    expect(document.body.textContent).not.toContain('文件分享仅支持不超过 512 MiB 的文件。')
  })

  it('handles an authoritative too-large response without offering a state retry', async () => {
    mocks.createShare.mockRejectedValueOnce(new ApiError(
      'too large',
      413,
      'file_share_too_large',
    ))
    mountDialog()
    await flushPromises()

    button('创建分享').click()
    await flushPromises()

    expect(document.body.textContent).toContain('文件分享仅支持不超过 512 MiB 的文件。')
    expect(document.body.textContent).not.toContain('暂时无法生成分享链接')
    expect([...document.querySelectorAll('button')].some((candidate) => candidate.textContent?.includes('重试')))
      .toBe(false)
    expect(button('创建分享').disabled).toBe(true)
    button('创建分享').click()
    expect(mocks.createShare).toHaveBeenCalledOnce()
  })

  it('keeps stop sharing available for an oversized historical share', async () => {
    mocks.share.mockResolvedValueOnce({ share: activeShare(false) })
    mountDialog(entry({ sizeBytes: maxFileShareBytes + 1 }))
    await flushPromises()

    expect(button('停止分享').disabled).toBe(false)
    expect(button('重新生成链接').disabled).toBe(true)
    expect(document.body.textContent).toContain('当前链接已无法访问')
    expect(document.body.textContent).not.toContain('文件正在分享')
  })

  it('allows an oversized share-state lookup to be retried before stopping a historical share', async () => {
    mocks.share
      .mockRejectedValueOnce(new Error('network unavailable'))
      .mockResolvedValueOnce({ share: activeShare(false) })
    mountDialog(entry({ sizeBytes: maxFileShareBytes + 1 }))
    await flushPromises()

    expect(document.body.textContent).toContain('暂时无法读取分享状态，请稍后重试。')
    expect(button('重试').disabled).toBe(false)

    button('重试').click()
    await flushPromises()

    expect(mocks.share).toHaveBeenCalledTimes(2)
    expect(button('停止分享').disabled).toBe(false)
    expect(document.body.textContent).not.toContain('暂时无法读取分享状态')
  })

  it('fully localizes the oversized-file state', async () => {
    registerPhraseCatalog(englishSharedCatalog)
    mountDialog(entry({ sizeBytes: maxFileShareBytes + 1 }))
    await flushPromises()

    expect(document.body.textContent).toContain('File sharing supports files up to 512 MiB.')
    expect(document.body.textContent).toContain('Compress or split the file and try again.')
    expect(document.body.textContent).not.toContain('文件分享仅支持')
  })

  it('localizes content rendered through the teleported modal', async () => {
    registerPhraseCatalog(englishSharedCatalog)
    mountDialog()
    await flushPromises()

    expect(document.querySelector('[role="dialog"]')?.getAttribute('aria-label')).toBe('File sharing')
    expect(document.body.textContent).toContain('Anyone with the link can access this file.')
    expect(document.body.textContent).toContain('Create share')
    expect(document.body.textContent).not.toContain('任何持有链接的人都可以访问此文件。')
  })

  it('does not expose a Simplified Chinese API error in the English dialog', async () => {
    mocks.createShare.mockRejectedValueOnce(new ApiError(
      '文件分享存储不可用',
      503,
      'file_share_storage_unavailable',
    ))
    registerPhraseCatalog(englishSharedCatalog)
    mountDialog()
    await flushPromises()

    button('Create share').click()
    await flushPromises()

    expect(document.body.textContent).toContain('Unable to generate a share link. Try again later.')
    expect(document.body.textContent).not.toContain('文件分享存储不可用')
  })

  it('keeps auxiliary text readable and accurately describes changed files', () => {
    const dialogSource = readFileSync(resolve(process.cwd(), 'src/components/files/FileShareDialog.vue'), 'utf8')
    const publicSource = readFileSync(resolve(process.cwd(), 'src/views/FileShareView.vue'), 'utf8')

    expect(dialogSource).not.toContain('font-size: 12px')
    expect(publicSource).not.toContain('font-size: 12px')
    expect(dialogSource).toContain('链接将无法访问；分享记录仍可在“分享管理”中停止。')
    expect(publicSource).toContain('此链接将无法访问。')
    expect(dialogSource).not.toContain('链接会自动失效')
    expect(publicSource).not.toContain('此分享会自动失效')
  })
})
