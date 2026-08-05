import { createSSRApp, ssrContextKey, type ComputedRef, type Ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LoginView from './LoginView.vue'

const mocks = vi.hoisted(() => {
  class MockApiError extends Error {
    code = ''
  }

  return {
    MockApiError,
    login: vi.fn(),
    refresh: vi.fn(),
    replace: vi.fn(),
    prefetch: vi.fn(),
    route: { query: {} as Record<string, unknown> },
    sessionState: {
      loading: false,
      setupRequired: false,
      authenticated: false,
      error: '',
    },
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ replace: mocks.replace }),
}))

vi.mock('@/lib/api', () => ({ ApiError: mocks.MockApiError }))

vi.mock('@/components/layout/AuthLayout.vue', () => ({
  default: { template: '<main><slot /></main>' },
}))

vi.mock('@/lib/navigation', () => ({
  prefetchNavigationRoute: mocks.prefetch,
}))

vi.mock('@/stores/session', () => ({
  useSession: () => ({
    state: mocks.sessionState,
    login: mocks.login,
    refresh: mocks.refresh,
  }),
}))

interface LoginBindings {
  form: { username: string; password: string; totpCode: string }
  totpRequired: Ref<boolean>
  useRecoveryCode: Ref<boolean>
  loginPhase: Ref<'idle' | 'authenticating' | 'entering'>
  busy: ComputedRef<boolean>
  submitLabel: ComputedRef<string>
  submit: () => Promise<void>
  retryConnection: () => Promise<void>
}

function setupView(): LoginBindings {
  const component = LoginView as unknown as {
    setup: (props: Record<string, never>, context: { expose: () => void }) => LoginBindings
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

beforeEach(() => {
  vi.clearAllMocks()
  mocks.route.query = {}
  mocks.sessionState.loading = false
  mocks.sessionState.setupRequired = false
  mocks.sessionState.authenticated = false
  mocks.sessionState.error = ''
  mocks.refresh.mockResolvedValue(undefined)
  mocks.login.mockImplementation(async () => {
    mocks.sessionState.authenticated = true
  })
})

describe('LoginView console transition', () => {
  it('keeps visible progress until the destination route finishes loading', async () => {
    let finishNavigation: (() => void) | undefined
    mocks.replace.mockImplementation(() => new Promise<void>((resolve) => {
      finishNavigation = resolve
    }))
    const view = setupView()
    view.form.username = 'admin'
    view.form.password = 'StrongPassword123'

    const submission = view.submit()
    await vi.waitFor(() => expect(mocks.replace).toHaveBeenCalledWith('/overview'))

    expect(view.loginPhase.value).toBe('entering')
    expect(view.busy.value).toBe(true)
    expect(view.submitLabel.value).toContain('正在进入控制台')

    finishNavigation?.()
    await submission
    expect(view.loginPhase.value).toBe('idle')
    expect(view.busy.value).toBe(false)
  })

  it('prefers the requested local route after authentication', async () => {
    mocks.route.query = { redirect: '/files?path=%2Fhome' }
    mocks.replace.mockResolvedValue(undefined)
    const view = setupView()
    view.form.username = 'admin'
    view.form.password = 'StrongPassword123'

    await view.submit()

    expect(mocks.replace).toHaveBeenCalledWith('/files?path=%2Fhome')
  })

  it('requires a second factor and supports a one-time recovery code', async () => {
    const challenge = new mocks.MockApiError('second factor required')
    challenge.code = 'totp_required'
    mocks.login.mockRejectedValueOnce(challenge).mockResolvedValueOnce(undefined)
    mocks.replace.mockResolvedValue(undefined)
    const view = setupView()
    view.form.username = 'admin'
    view.form.password = 'StrongPassword123'

    await view.submit()
    expect(view.totpRequired.value).toBe(true)

    view.useRecoveryCode.value = true
    view.form.totpCode = 'ABCDE-FGHIJ-KLMNO'
    await view.submit()

    expect(mocks.login).toHaveBeenLastCalledWith({
      username: 'admin',
      password: 'StrongPassword123',
      totpCode: 'ABCDE-FGHIJ-KLMNO',
    })
  })

  it('navigates to setup when a connection retry finds an uninitialized panel', async () => {
    mocks.refresh.mockImplementation(async () => {
      mocks.sessionState.setupRequired = true
    })
    const view = setupView()

    await view.retryConnection()

    expect(mocks.refresh).toHaveBeenCalledWith(true)
    expect(mocks.replace).toHaveBeenCalledWith('/setup')
  })
})
