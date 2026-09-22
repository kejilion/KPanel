<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Check, Copy, Eye, EyeOff, LoaderCircle, LockKeyhole } from '@lucide/vue'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import { ApiError, api } from '@/lib/api'
import { getPasskey, passkeyError, passkeysSupported } from '@/lib/passkeys'
import { useI18n } from '@/i18n'
import { localizeError } from '@/i18n/errors'
import { prefetchNavigationRoute } from '@/lib/navigation'
import { useSession } from '@/stores/session'

const route = useRoute()
const router = useRouter()
const session = useSession()
const i18n = useI18n()
const form = reactive({
  username: '',
  password: '',
  totpCode: '',
})
const showPassword = ref(false)
const totpRequired = ref(false)
const useRecoveryCode = ref(false)
const recoveryHelpVisible = ref(false)
const recoveryCommandCopied = ref(false)
const recoveryCommandCopyFailed = ref(false)
const error = ref('')
const loginPhase = ref<'idle' | 'authenticating' | 'entering'>('idle')
const passkeyMode = ref(false)
const passkeyAvailable = ref(false)
const passkeyStatusError = ref(false)
const passkeyStatusLoaded = ref(false)
const passkeySupported = passkeysSupported()
const passkeyController = new AbortController()

const destination = computed(() => (
  typeof route.query.redirect === 'string' && route.query.redirect.startsWith('/')
    ? route.query.redirect
    : '/overview'
))
const destinationPath = computed(() => destination.value.split(/[?#]/, 1)[0] || '/overview')
const busy = computed(() => loginPhase.value !== 'idle' || session.state.loading)
const submitLabel = computed(() => {
  if (loginPhase.value === 'entering') return i18n.t('auth.entering')
  if (loginPhase.value === 'authenticating' || session.state.loading) return i18n.t('auth.verifying')
  return i18n.t(passkeyMode.value ? 'passkey.login' : 'auth.secureLogin')
})
const recoveryCommand = `cd /home/docker/kpanel && sh -c 'set -e; cleanup() { status=$?; trap - EXIT; docker compose --env-file .env up -d panel; exit "$status"; }; trap cleanup EXIT; docker compose --env-file .env stop panel; docker compose --env-file .env run --rm --no-deps panel reset-password'`

const canSubmit = computed(() =>
  form.username.trim().length > 0 && (passkeyMode.value ? passkeyAvailable.value && passkeySupported : form.password.length > 0) && (
    (!totpRequired.value && (!passkeyMode.value || !form.totpCode)) || (useRecoveryCode.value
      ? /^[A-Za-z2-7-]{15,17}$/.test(form.totpCode)
      : /^\d{6}$/.test(form.totpCode))
  ),
)

async function submit(): Promise<void> {
  if (!canSubmit.value || busy.value) return
  error.value = ''
  loginPhase.value = 'authenticating'

  try {
    if (passkeyMode.value) {
      const ceremony = await api.auth.passkeys.loginBegin({ username: form.username.trim() }, passkeyController.signal)
      passkeyController.signal.throwIfAborted()
      const credential = await getPasskey(ceremony.publicKey, passkeyController.signal)
      await session.loginPasskey({ ceremonyId: ceremony.ceremonyId, credential, totpCode: form.totpCode || undefined }, passkeyController.signal)
    } else {
      await session.login({
        username: form.username.trim(),
        password: form.password,
        totpCode: totpRequired.value ? form.totpCode : undefined,
      })
    }
    loginPhase.value = 'entering'
    await router.replace(destination.value)
  } catch (reason) {
    if (reason instanceof ApiError && reason.code === 'totp_required') {
      totpRequired.value = true
      error.value = i18n.t(passkeyMode.value ? 'passkey.retryFactor' : 'auth.totpRequired')
      return
    }
    if (reason instanceof ApiError && reason.code === 'invalid_second_factor') {
      totpRequired.value = true
      error.value = i18n.t(useRecoveryCode.value ? 'auth.invalidRecovery' : 'auth.invalidTotp')
      form.totpCode = ''
      return
    }
    error.value = session.state.authenticated
      ? i18n.t('auth.loginResourceFailed')
      : passkeyMode.value ? passkeyError(reason) : localizeError(reason, 'auth.loginFailed')
  } finally {
    loginPhase.value = 'idle'
  }
}

function togglePasskeyMode(): void {
  if (busy.value) return
  passkeyMode.value = !passkeyMode.value
  form.password = ''
  form.totpCode = ''
  error.value = ''
}

async function loadPasskeyStatus(): Promise<void> {
  passkeyStatusError.value = false
  try {
    const status = await api.auth.passkeys.status(passkeyController.signal)
    passkeyAvailable.value = status.available
  } catch {
    if (!passkeyController.signal.aborted) passkeyStatusError.value = true
  } finally {
    passkeyStatusLoaded.value = true
  }
}

async function retryConnection(): Promise<void> {
  await session.refresh(true)
  if (session.state.error) return
  if (session.state.setupRequired) {
    await router.replace('/setup')
    return
  }
  if (session.state.authenticated) await router.replace(destination.value)
}

function toggleRecoveryHelp(): void {
  recoveryHelpVisible.value = !recoveryHelpVisible.value
  recoveryCommandCopied.value = false
  recoveryCommandCopyFailed.value = false
}

async function copyRecoveryCommand(): Promise<void> {
  let copied = false
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(recoveryCommand)
      copied = true
    } catch {
      // HTTP IP access and restricted browsing contexts may deny the modern clipboard API.
    }
  }
  if (!copied && typeof document !== 'undefined' && typeof document.execCommand === 'function') {
    const textarea = document.createElement('textarea')
    textarea.value = recoveryCommand
    textarea.setAttribute('readonly', '')
    textarea.style.position = 'fixed'
    textarea.style.opacity = '0'
    document.body.appendChild(textarea)
    textarea.select()
    copied = document.execCommand('copy')
    textarea.remove()
  }
  recoveryCommandCopied.value = copied
  recoveryCommandCopyFailed.value = !copied
}

onMounted(() => {
  void loadPasskeyStatus()
  // Warm only the destination view. This overlaps the small chunk download
  // with credential entry without loading protected data or the full console.
  void prefetchNavigationRoute(destinationPath.value)
})
onBeforeUnmount(() => passkeyController.abort())
</script>

<template>
  <AuthLayout>
    <div class="auth-card__heading">
      <span class="auth-card__icon"><LockKeyhole :size="21" /></span>
      <div>
        <span class="eyebrow">{{ i18n.t('auth.welcome') }}</span>
        <h2>{{ i18n.t('auth.loginTitle') }}</h2>
      </div>
    </div>
    <p class="auth-card__intro">{{ i18n.t('auth.loginIntro') }}</p>

    <div v-if="session.state.error" class="inline-alert inline-alert--danger" role="alert">
      {{ localizeError(session.state.error, 'error.authenticationRequired') }}
      <button type="button" @click="retryConnection">{{ i18n.t('common.retryConnection') }}</button>
    </div>
    <div v-if="error" class="inline-alert inline-alert--danger" role="alert">{{ error }}</div>
    <div v-if="route.query.passkeyChanged === '1'" class="inline-alert inline-alert--info" role="status">{{ i18n.t('passkey.changed') }}</div>

    <form class="form-stack" @submit.prevent="submit">
      <label class="field">
        <span>{{ i18n.t('auth.username') }}</span>
        <input v-model.trim="form.username" autocomplete="username" autofocus required />
      </label>

      <label v-if="!passkeyMode" class="field">
        <span>{{ i18n.t('auth.password') }}</span>
        <span class="input-wrap input-wrap--action">
          <input
            v-model="form.password"
            :type="showPassword ? 'text' : 'password'"
            autocomplete="current-password"
            required
          />
          <button
            class="input-action"
            type="button"
            :aria-label="i18n.t(showPassword ? 'auth.hidePassword' : 'auth.showPassword')"
            @click="showPassword = !showPassword"
          >
            <EyeOff v-if="showPassword" :size="17" />
            <Eye v-else :size="17" />
          </button>
        </span>
      </label>

      <label v-if="totpRequired" class="field">
        <span>{{ i18n.t(useRecoveryCode ? 'auth.recoveryCode' : 'auth.totpCode') }}</span>
        <input
          v-model.trim="form.totpCode"
          :inputmode="useRecoveryCode ? 'text' : 'numeric'"
          autocomplete="one-time-code"
          :maxlength="useRecoveryCode ? 17 : 6"
          :placeholder="i18n.t(useRecoveryCode ? 'auth.recoveryPlaceholder' : 'auth.totpPlaceholder')"
          autofocus
          :required="totpRequired"
        />
        <button
          class="button-link auth-recovery-toggle"
          type="button"
          @click="useRecoveryCode = !useRecoveryCode; form.totpCode = ''; error = ''"
        >
          {{ i18n.t(useRecoveryCode ? 'auth.useAuthenticator' : 'auth.useRecovery') }}
        </button>
      </label>

      <button class="button button--primary button--block" type="submit" :disabled="!canSubmit || busy">
        <LoaderCircle v-if="busy" class="spin" :size="17" />
        {{ submitLabel }}
      </button>

      <button v-if="passkeyAvailable && passkeySupported" class="button button--secondary button--block" type="button" :disabled="busy" @click="togglePasskeyMode">
        {{ i18n.t(passkeyMode ? 'passkey.usePassword' : 'passkey.login') }}
      </button>
      <div v-if="passkeyStatusError" class="inline-alert inline-alert--warning" role="status">
        {{ i18n.t('passkey.loadFailed') }}
        <button class="button-link" type="button" @click="loadPasskeyStatus">{{ i18n.t('passkey.retry') }}</button>
      </div>
      <p v-else-if="passkeyStatusLoaded && (!passkeyAvailable || !passkeySupported)" class="passkey-help">{{ i18n.t(!passkeyAvailable ? 'passkey.unavailable' : 'passkey.unsupported') }}</p>
      <p v-if="passkeyMode" class="passkey-help">{{ i18n.t('passkey.intro') }}</p>

      <button
        class="button-link auth-forgot-password"
        type="button"
        :aria-expanded="recoveryHelpVisible"
        @click="toggleRecoveryHelp"
      >
        {{ i18n.t('auth.forgotPassword') }}
      </button>
    </form>

    <div v-if="recoveryHelpVisible" class="inline-alert inline-alert--info recovery-help" role="note">
      <strong>{{ i18n.t('auth.recoveryHelpTitle') }}</strong>
      <p>{{ i18n.t('auth.recoveryHelpIntro') }}</p>
      <span>{{ i18n.t('auth.recoveryCommandIntro') }}</span>
      <pre><code>{{ recoveryCommand }}</code></pre>
      <small>{{ i18n.t('auth.recoveryDataSafe') }}</small>
      <div class="recovery-help__actions">
        <button class="button button--ghost button--small" type="button" @click="copyRecoveryCommand">
          <Check v-if="recoveryCommandCopied" :size="14" />
          <Copy v-else :size="14" />
          {{ i18n.t(recoveryCommandCopied ? 'auth.recoveryCommandCopied' : 'auth.copyRecoveryCommand') }}
        </button>
        <button class="button-link" type="button" @click="toggleRecoveryHelp">
          {{ i18n.t('auth.hideRecoveryHelp') }}
        </button>
      </div>
      <small v-if="recoveryCommandCopyFailed" role="status">{{ i18n.t('auth.recoveryCommandCopyFailed') }}</small>
    </div>

    <p class="auth-card__security">{{ i18n.t('auth.sessionSecurity') }}</p>

    <Transition name="fade">
      <div v-if="loginPhase === 'entering'" class="login-transition" role="status" aria-live="polite">
        <span class="login-transition__icon"><LoaderCircle class="spin" :size="24" /></span>
        <strong>{{ i18n.t('auth.loginSuccess') }}</strong>
        <small>{{ i18n.t('auth.loadingConsole') }}</small>
      </div>
    </Transition>
  </AuthLayout>
</template>

<style scoped>
.passkey-help {
  margin: 0;
  color: var(--text-soft);
  font-size: 13px;
  line-height: 1.6;
}
.auth-forgot-password {
  justify-self: end;
  margin-top: -8px;
  font-size: 12px;
}

.recovery-help {
  display: grid;
  gap: 10px;
  margin-top: 16px;
}

.recovery-help p,
.recovery-help small {
  margin: 0;
  line-height: 1.6;
}

.recovery-help pre {
  max-width: 100%;
  margin: 0;
  padding: 11px 12px;
  overflow-x: auto;
  color: var(--text);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  font-size: 11px;
  line-height: 1.55;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  user-select: all;
}

.recovery-help__actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
</style>
