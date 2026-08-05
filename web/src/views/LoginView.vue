<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Eye, EyeOff, LoaderCircle, LockKeyhole } from '@lucide/vue'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import { ApiError } from '@/lib/api'
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
const error = ref('')
const loginPhase = ref<'idle' | 'authenticating' | 'entering'>('idle')

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
  return i18n.t('auth.secureLogin')
})

const canSubmit = computed(() =>
  form.username.trim().length > 0 && form.password.length > 0 && (
    !totpRequired.value || (useRecoveryCode.value
      ? /^[A-Za-z2-7-]{15,17}$/.test(form.totpCode)
      : /^\d{6}$/.test(form.totpCode))
  ),
)

async function submit(): Promise<void> {
  if (!canSubmit.value || busy.value) return
  error.value = ''
  loginPhase.value = 'authenticating'

  try {
    await session.login({
      username: form.username.trim(),
      password: form.password,
      totpCode: totpRequired.value ? form.totpCode : undefined,
    })
    loginPhase.value = 'entering'
    await router.replace(destination.value)
  } catch (reason) {
    if (reason instanceof ApiError && reason.code === 'totp_required') {
      totpRequired.value = true
      error.value = i18n.t('auth.totpRequired')
      return
    }
    if (reason instanceof ApiError && reason.code === 'invalid_second_factor') {
      error.value = i18n.t(useRecoveryCode.value ? 'auth.invalidRecovery' : 'auth.invalidTotp')
      form.totpCode = ''
      return
    }
    error.value = session.state.authenticated
      ? i18n.t('auth.loginResourceFailed')
      : localizeError(reason, 'auth.loginFailed')
  } finally {
    loginPhase.value = 'idle'
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

onMounted(() => {
  // Warm only the destination view. This overlaps the small chunk download
  // with credential entry without loading protected data or the full console.
  void prefetchNavigationRoute(destinationPath.value)
})
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

    <form class="form-stack" @submit.prevent="submit">
      <label class="field">
        <span>{{ i18n.t('auth.username') }}</span>
        <input v-model.trim="form.username" autocomplete="username" autofocus required />
      </label>

      <label class="field">
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
          required
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
    </form>

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
