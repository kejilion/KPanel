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
    loginPasskey: vi.fn(),
    loginBegin: vi.fn(),
    getPasskey: vi.fn(),
    refresh: vi.fn(),
    replace: vi.fn(),
    prefetch: vi.fn(),
    clipboardWriteText: vi.fn(),
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

vi.mock('@/lib/api', () => ({ ApiError: mocks.MockApiError, api: { auth: { passkeys: { loginBegin: mocks.loginBegin } } } }))
vi.mock('@/lib/passkeys', () => ({ passkeysSupported: () => true, getPasskey: mocks.getPasskey, passkeyError: (reason: Error) => reason.message }))

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
    loginPasskey: mocks.loginPasskey,
    refresh: mocks.refresh,
  }),
}))

interface LoginBindings {
  form: { username: string; password: string; totpCode: string }
  totpRequired: Ref<boolean>
  passkeyMode: Ref<boolean>
  passkeyAvailable: Ref<boolean>
  useRecoveryCode: Ref<boolean>
  loginPhase: Ref<'idle' | 'authenticating' | 'entering'>
  recoveryHelpVisible: Ref<boolean>
  recoveryCommandCopied: Ref<boolean>
  recoveryCommandCopyFailed: Ref<boolean>
  recoveryCommand: string
  busy: ComputedRef<boolean>
  submitLabel: ComputedRef<string>
  submit: () => Promise<void>
  retryConnection: () => Promise<void>
  toggleRecoveryHelp: () => void
  copyRecoveryCommand: () => Promise<void>
  togglePasskeyMode: () => void
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
  mocks.clipboardWriteText.mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText: mocks.clipboardWriteText },
    configurable: true,
  })
  mocks.login.mockImplementation(async () => {
    mocks.sessionState.authenticated = true
  })
  mocks.loginBegin.mockResolvedValue({ ceremonyId: 'ceremony', publicKey: { challenge: 'AA' } })
  mocks.getPasskey.mockResolvedValue({ id: 'credential' })
  mocks.loginPasskey.mockResolvedValue(undefined)
})

describe('LoginView console transition', () => {
  it('logs in with a Passkey without a password and creates a fresh ceremony after missing TOTP', async () => {
    const challenge = new mocks.MockApiError('second factor required')
    challenge.code = 'totp_required'
    mocks.loginPasskey.mockRejectedValueOnce(challenge).mockResolvedValueOnce(undefined)
    mocks.replace.mockResolvedValue(undefined)
    const view = setupView()
    view.passkeyAvailable.value = true
    view.togglePasskeyMode()
    view.form.username = 'admin'

    await view.submit()
    expect(view.totpRequired.value).toBe(true)
    expect(mocks.replace).not.toHaveBeenCalled()
    view.useRecoveryCode.value = true
    view.form.totpCode = 'ABCDE-FGHIJ-KLMNO'
    await view.submit()

    expect(mocks.loginBegin).toHaveBeenCalledTimes(2)
    expect(mocks.getPasskey).toHaveBeenCalledTimes(2)
    expect(mocks.loginPasskey).toHaveBeenLastCalledWith({ ceremonyId: 'ceremony', credential: { id: 'credential' }, totpCode: 'ABCDE-FGHIJ-KLMNO' }, expect.any(AbortSignal))
    expect(mocks.login).not.toHaveBeenCalled()
    expect(mocks.replace).toHaveBeenCalledWith('/overview')
  })

  it('keeps password fallback after browser cancellation and never submits an absent assertion', async () => {
    mocks.getPasskey.mockRejectedValueOnce(new DOMException('cancelled', 'NotAllowedError'))
    const view = setupView()
    view.passkeyAvailable.value = true
    view.togglePasskeyMode()
    view.form.username = 'admin'
    await view.submit()
    expect(mocks.loginPasskey).not.toHaveBeenCalled()
    expect(view.busy.value).toBe(false)
    view.togglePasskeyMode()
    expect(view.passkeyMode.value).toBe(false)
  })

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

  it('shows local-only recovery guidance and copies the restart-safe command', async () => {
    const view = setupView()

    view.toggleRecoveryHelp()
    expect(view.recoveryHelpVisible.value).toBe(true)
    expect(view.recoveryCommand).toContain('panel reset-password')
    expect(view.recoveryCommand).toContain('trap cleanup EXIT')
    expect(view.recoveryCommand).not.toContain('--disable-2fa')

    await view.copyRecoveryCommand()

    expect(mocks.clipboardWriteText).toHaveBeenCalledWith(view.recoveryCommand)
    expect(view.recoveryCommandCopied.value).toBe(true)
  })

  it('keeps the command selectable when clipboard permission is denied', async () => {
    mocks.clipboardWriteText.mockRejectedValue(new Error('clipboard denied'))
    const view = setupView()

    await view.copyRecoveryCommand()

    expect(view.recoveryCommandCopied.value).toBe(false)
    expect(view.recoveryCommandCopyFailed.value).toBe(true)
  })
})
