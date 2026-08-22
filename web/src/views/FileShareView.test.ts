import { readFileSync } from 'node:fs'
import { createSSRApp, ssrContextKey, type ComputedRef, type Ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FileShareView from './FileShareView.vue'
import type { PublicFileShareView } from '@/types/api'

const mocks = vi.hoisted(() => ({
  publicShare: vi.fn(),
  token: 'a'.repeat(43),
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
  api: { files: { publicShare: mocks.publicShare } },
}))

interface ShareBindings {
  snapshot: Ref<PublicFileShareView | undefined>
  loading: Ref<boolean>
  errorMessage: Ref<string>
  previewFailed: Ref<boolean>
  tokenIsValid: ComputedRef<boolean>
  canOpenDirect: ComputedRef<boolean>
  canPreviewImage: ComputedRef<boolean>
  load: () => Promise<void>
}

function setupView(): ShareBindings {
  const component = FileShareView as unknown as {
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

function publicFile(mime = 'image/png'): PublicFileShareView {
  const token = 'a'.repeat(43)
  return {
    name: 'site-logo.png',
    mime,
    sizeBytes: 2048,
    expiresAt: '2026-08-29T00:00:00Z',
    directPath: `/f/${token}`,
    downloadPath: `/f/${token}?download=1`,
  }
}

beforeEach(() => {
  mocks.token = 'a'.repeat(43)
  mocks.publicShare.mockReset()
})

describe('FileShareView anonymous file page', () => {
  it('loads exactly one public metadata endpoint without session state', async () => {
    const expected = publicFile()
    mocks.publicShare.mockResolvedValueOnce(expected)
    const view = setupView()

    await view.load()

    expect(mocks.publicShare).toHaveBeenCalledOnce()
    expect(mocks.publicShare).toHaveBeenCalledWith(mocks.token, expect.any(AbortSignal))
    expect(view.snapshot.value).toEqual(expected)
    expect(view.errorMessage.value).toBe('')
    expect(view.canOpenDirect.value).toBe(true)
    expect(view.canPreviewImage.value).toBe(true)
  })

  it('rejects malformed tokens before making a request', async () => {
    mocks.token = 'not-a-token'
    const view = setupView()

    await view.load()

    expect(view.tokenIsValid.value).toBe(false)
    expect(mocks.publicShare).not.toHaveBeenCalled()
    expect(view.errorMessage.value).toBe('分享链接格式无效。')
  })

  it('previews only an explicit safe raster MIME type', async () => {
    mocks.publicShare.mockResolvedValueOnce(publicFile('image/svg+xml'))
    const view = setupView()

    await view.load()

    expect(view.canOpenDirect.value).toBe(false)
    expect(view.canPreviewImage.value).toBe(false)
  })

  it('does not automatically fetch an oversized raster image', async () => {
    mocks.publicShare.mockResolvedValueOnce({
      ...publicFile('image/png'),
      sizeBytes: 12 * 1024 * 1024 + 1,
    })
    const view = setupView()

    await view.load()

    expect(view.canOpenDirect.value).toBe(true)
    expect(view.canPreviewImage.value).toBe(false)
  })

  it('keeps private file metadata out of the public template and bypasses session checks', () => {
    const source = readFileSync(new URL('./FileShareView.vue', import.meta.url), 'utf8')
    for (const privateField of ['path', 'mode', 'owner', 'group', 'resourceVersion']) {
      expect(source).not.toContain(`snapshot.${privateField}`)
    }
    expect(source).toContain("new Set(['image/jpeg', 'image/png', 'image/gif', 'image/webp', 'image/avif'])")
    expect(source).toContain(':href="snapshot.downloadPath"')
    expect(source).toContain(':src="snapshot.directPath"')
    expect(source).toContain('v-if="canOpenDirect" class="file-share-open"')

    const routerSource = readFileSync(new URL('../router.ts', import.meta.url), 'utf8')
    expect(routerSource).toContain("path: '/share/file/:token'")
    expect(routerSource).toContain("name: 'file-share'")
    expect(routerSource).toContain("titleKey: 'route.fileShare', public: true, skipSessionCheck: true")
    expect(routerSource.indexOf('if (to.meta.skipSessionCheck) return true')).toBeLessThan(
      routerSource.indexOf('const session = useSession()'),
    )
  })
})
