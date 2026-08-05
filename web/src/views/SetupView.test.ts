import { createSSRApp, ssrContextKey, type ComputedRef, type Ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SetupView from './SetupView.vue'

const mocks = vi.hoisted(() => ({
  setup: vi.fn(),
  refresh: vi.fn(),
  replace: vi.fn(),
  sessionState: {
    loading: false,
    setupRequired: true,
    authenticated: false,
    error: '',
  },
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ replace: mocks.replace }),
}))

vi.mock('@/components/layout/AuthLayout.vue', () => ({
  default: { template: '<main><slot /></main>' },
}))

vi.mock('@/stores/session', () => ({
  useSession: () => ({
    state: mocks.sessionState,
    setup: mocks.setup,
    refresh: mocks.refresh,
  }),
}))

interface SetupBindings {
  form: { token: string; username: string; password: string; confirmPassword: string }
  submitted: Ref<boolean>
  canSubmit: ComputedRef<boolean>
  submit: () => Promise<void>
  retryConnection: () => Promise<void>
}

function setupView(): SetupBindings {
  const component = SetupView as unknown as {
    setup: (props: Record<string, never>, context: { expose: () => void }) => SetupBindings
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
  mocks.sessionState.loading = false
  mocks.sessionState.setupRequired = true
  mocks.sessionState.authenticated = false
  mocks.sessionState.error = ''
  mocks.setup.mockResolvedValue(undefined)
  mocks.refresh.mockResolvedValue(undefined)
  mocks.replace.mockResolvedValue(undefined)
})

describe('SetupView validation and recovery', () => {
  it('marks an invalid password as submitted without calling the setup API', async () => {
    const view = setupView()
    view.form.token = 'bootstrap-token'
    view.form.username = 'admin'
    view.form.password = 'short'
    view.form.confirmPassword = 'short'

    await view.submit()

    expect(view.submitted.value).toBe(true)
    expect(view.canSubmit.value).toBe(false)
    expect(mocks.setup).not.toHaveBeenCalled()
  })

  it('matches the server rule that the username must start with a letter or number', () => {
    const view = setupView()
    view.form.token = 'bootstrap-token'
    view.form.password = 'StrongPassword123'
    view.form.confirmPassword = 'StrongPassword123'

    view.form.username = '.admin'
    expect(view.canSubmit.value).toBe(false)

    view.form.username = 'admin'
    expect(view.canSubmit.value).toBe(true)
  })

  it('navigates to login when a connection retry finds setup already completed', async () => {
    mocks.refresh.mockImplementation(async () => {
      mocks.sessionState.setupRequired = false
    })
    const view = setupView()

    await view.retryConnection()

    expect(mocks.refresh).toHaveBeenCalledWith(true)
    expect(mocks.replace).toHaveBeenCalledWith('/login')
  })
})
