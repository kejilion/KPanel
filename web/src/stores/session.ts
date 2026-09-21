import { computed, reactive } from 'vue'
import { api, resetApiSecurityState } from '@/lib/api'
import type { PasskeyCredentialJSON } from '@/lib/passkeys'
import type { AgentStatus, AuthStatus, LoginRequest, SetupRequest, User } from '@/types/api'

interface SessionState {
  checked: boolean
  loading: boolean
  setupRequired: boolean
  authenticated: boolean
  user?: User
  expiresAt?: string
  agent?: AgentStatus
  error?: unknown
}

const state = reactive<SessionState>({
  checked: false,
  loading: false,
  setupRequired: false,
  authenticated: false,
})

let statusPromise: Promise<void> | undefined

function applyStatus(status: AuthStatus): void {
  state.setupRequired = status.setupRequired
  state.authenticated = status.authenticated
  state.user = status.user
  state.expiresAt = status.expiresAt
  state.agent = status.agent
  state.error = undefined
}

async function refresh(force = false): Promise<void> {
  if (statusPromise && !force) return statusPromise

  statusPromise = (async () => {
    state.loading = true
    try {
      applyStatus(await api.auth.status())
    } catch (error) {
      state.authenticated = false
      state.setupRequired = false
      state.user = undefined
      state.error = error
    } finally {
      state.checked = true
      state.loading = false
      statusPromise = undefined
    }
  })()

  return statusPromise
}

async function login(input: LoginRequest): Promise<void> {
  state.loading = true
  try {
    applyStatus(await api.auth.login(input))
  } finally {
    state.loading = false
  }
}

async function setup(input: SetupRequest): Promise<void> {
  state.loading = true
  try {
    applyStatus(await api.auth.setup(input))
  } finally {
    state.loading = false
  }
}

async function loginPasskey(input: { ceremonyId: string; credential: PasskeyCredentialJSON; totpCode?: string }, signal?: AbortSignal): Promise<void> {
  state.loading = true
  try {
    applyStatus(await api.auth.passkeys.loginFinish(input, signal))
  } finally {
    state.loading = false
  }
}

async function logout(): Promise<void> {
  try {
    await api.auth.logout()
  } finally {
    resetApiSecurityState()
    state.authenticated = false
    state.user = undefined
    state.agent = undefined
  }
}

export function useSession() {
  return {
    state,
    isAgentWritable: computed(
      () => Boolean(state.agent?.connected && state.agent.compatible && !state.agent.readOnly),
    ),
    refresh,
    login,
    loginPasskey,
    setup,
    logout,
  }
}
