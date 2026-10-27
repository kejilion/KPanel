import { computed, reactive } from 'vue'
import { applySiteBranding } from '@/stores/branding'
import { api, resetApiSecurityState } from '@/lib/api'
import { stopAppearanceSync } from '@/lib/appearanceSync'
import { startPreferencesSync, stopPreferencesSync } from '@/lib/preferencesSync'
import { applyLoginAppearance, cancelLoginWallpaper } from '@/lib/loginAppearance'
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
  appearance?: AuthStatus['appearance']
  error?: unknown
}

const state = reactive<SessionState>({
  checked: false,
  loading: false,
  setupRequired: false,
  authenticated: false,
})

let statusPromise: Promise<void> | undefined
let generation = 0

function applyStatus(status: AuthStatus): void {
  if (!status.authenticated || state.user?.id !== status.user?.id) stopPreferencesSync()
  applySiteBranding(status.authenticated ? status.appearance?.branding : status.loginAppearance?.branding)
  if (!status.authenticated && !status.setupRequired && status.loginAppearance) applyLoginAppearance(status.loginAppearance)
  else cancelLoginWallpaper()
  state.appearance = status.appearance
  state.setupRequired = status.setupRequired
  state.authenticated = status.authenticated
  state.user = status.user
  state.expiresAt = status.expiresAt
  state.agent = status.agent
  state.error = undefined
}

async function refresh(force = false): Promise<void> {
  if (statusPromise && !force) return statusPromise

  const run = ++generation
  statusPromise = (async () => {
    state.loading = true
    try {
      const status = await api.auth.status(AbortSignal.timeout(10_000))
      if (run === generation) applyStatus(status)
    } catch (error) {
      if (run !== generation) return
      cancelLoginWallpaper()
      state.appearance = undefined
      state.authenticated = false
      stopPreferencesSync()
      state.setupRequired = false
      state.user = undefined
      state.error = error
    } finally {
      if (run !== generation) return
      state.checked = true
      state.loading = false
      statusPromise = undefined
    }
  })()

  return statusPromise
}

async function login(input: LoginRequest): Promise<void> {
  const run = ++generation
  statusPromise = undefined
  state.loading = true
  try {
    const status = await api.auth.login(input)
    if (run === generation) applyStatus(status)
  } finally {
    if (run === generation) state.loading = false
  }
}

async function setup(input: SetupRequest): Promise<void> {
  const run = ++generation
  statusPromise = undefined
  state.loading = true
  try {
    const status = await api.auth.setup(input)
    if (run === generation) applyStatus(status)
  } finally {
    if (run === generation) state.loading = false
  }
}

async function loginPasskey(input: { ceremonyId: string; credential: PasskeyCredentialJSON; totpCode?: string }, signal?: AbortSignal): Promise<void> {
  const run = ++generation
  statusPromise = undefined
  state.loading = true
  try {
    const status = await api.auth.passkeys.loginFinish(input, signal)
    if (run === generation) applyStatus(status)
  } finally {
    if (run === generation) state.loading = false
  }
}

async function logout(): Promise<void> {
  const run = ++generation
  statusPromise = undefined
  try {
    await api.auth.logout()
  } finally {
    if (run === generation) state.loading = false
  }
  if (run !== generation) return
  stopAppearanceSync()
  stopPreferencesSync()
  resetApiSecurityState()
  cancelLoginWallpaper()
  state.authenticated = false
  state.user = undefined
  state.agent = undefined
  state.appearance = undefined
  state.checked = false
  // Refresh public branding before routing back to login.
  await refresh(true)
}

function takeAppearanceSnapshot(): AuthStatus['appearance'] {
  const snapshot = state.appearance
  // A later shell mount must revalidate instead of replaying an old login snapshot.
  state.appearance = undefined
  return snapshot
}

/** A logout/account change may happen while the router awaits initial hydration. */
async function restorePreferences(): Promise<boolean> {
  const owner = state.user?.id
  if (!state.authenticated || !owner) return false
  await startPreferencesSync(owner)
  return state.authenticated && state.user?.id === owner
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
    takeAppearanceSnapshot,
    restorePreferences,
  }
}
