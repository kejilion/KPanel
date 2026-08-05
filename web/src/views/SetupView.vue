<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Eye, EyeOff, KeyRound, LoaderCircle, ShieldCheck } from '@lucide/vue'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import { useI18n } from '@/i18n'
import { localizeError } from '@/i18n/errors'
import { useSession } from '@/stores/session'

const router = useRouter()
const session = useSession()
const i18n = useI18n()
const form = reactive({
  token: '',
  username: 'admin',
  password: '',
  confirmPassword: '',
})
const showPassword = ref(false)
const error = ref('')
const submitted = ref(false)
const usernamePattern = /^[A-Za-z0-9][A-Za-z0-9._-]{2,31}$/

const passwordChecks = computed(() => [
  { label: i18n.t('auth.passwordLength'), valid: form.password.length >= 12 },
  { label: i18n.t('auth.passwordComposition'), valid: /[A-Za-z]/.test(form.password) && /\d/.test(form.password) },
])

const canSubmit = computed(
  () =>
    form.token.trim().length > 0 &&
    usernamePattern.test(form.username) &&
    passwordChecks.value.every((item) => item.valid) &&
    form.password === form.confirmPassword,
)

async function submit(): Promise<void> {
  submitted.value = true
  error.value = ''
  if (!canSubmit.value) return

  try {
    await session.setup({
      token: form.token.trim(),
      username: form.username.trim(),
      password: form.password,
    })
    await router.replace('/overview')
  } catch (reason) {
    error.value = localizeError(reason, 'auth.setupFailed')
  }
}

async function retryConnection(): Promise<void> {
  await session.refresh(true)
  if (session.state.error || session.state.setupRequired) return
  await router.replace(session.state.authenticated ? '/overview' : '/login')
}
</script>

<template>
  <AuthLayout>
    <div class="auth-card__heading">
      <span class="auth-card__icon"><ShieldCheck :size="22" /></span>
      <div>
        <span class="eyebrow">{{ i18n.t('auth.firstUse') }}</span>
        <h2>{{ i18n.t('auth.setupTitle') }}</h2>
      </div>
    </div>
    <p class="auth-card__intro">
      {{ i18n.t('auth.setupIntro') }}
    </p>

    <div v-if="session.state.error" class="inline-alert inline-alert--danger" role="alert">
      {{ localizeError(session.state.error, 'error.authenticationRequired') }}
      <button type="button" @click="retryConnection">{{ i18n.t('common.retryConnection') }}</button>
    </div>
    <div v-if="error" class="inline-alert inline-alert--danger" role="alert">{{ error }}</div>

    <form class="form-stack" novalidate @submit.prevent="submit">
      <label class="field">
        <span>{{ i18n.t('auth.bootstrapToken') }}</span>
        <span class="input-wrap">
          <KeyRound :size="17" aria-hidden="true" />
          <input
            v-model.trim="form.token"
            autocomplete="one-time-code"
            :placeholder="i18n.t('auth.bootstrapPlaceholder')"
            :aria-invalid="submitted && !form.token"
            aria-describedby="setup-token-error"
            required
          />
        </span>
        <small v-if="submitted && !form.token" id="setup-token-error">{{ i18n.t('auth.bootstrapRequired') }}</small>
      </label>

      <label class="field">
        <span>{{ i18n.t('auth.adminUsername') }}</span>
        <input
          v-model.trim="form.username"
          autocomplete="username"
          maxlength="32"
          :aria-invalid="submitted && !usernamePattern.test(form.username)"
          aria-describedby="setup-username-error"
          required
        />
        <small v-if="submitted && !usernamePattern.test(form.username)" id="setup-username-error">
          {{ i18n.t('auth.usernameRule') }}
        </small>
      </label>

      <label class="field">
        <span>{{ i18n.t('auth.adminPassword') }}</span>
        <span class="input-wrap input-wrap--action">
          <input
            v-model="form.password"
            :type="showPassword ? 'text' : 'password'"
            autocomplete="new-password"
            :placeholder="i18n.t('auth.strongPasswordPlaceholder')"
            :aria-invalid="submitted && !passwordChecks.every((item) => item.valid)"
            aria-describedby="setup-password-requirements setup-password-error"
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
        <small v-if="submitted && !passwordChecks.every((item) => item.valid)" id="setup-password-error">
          {{ i18n.t('auth.passwordRule') }}
        </small>
      </label>

      <div id="setup-password-requirements" class="password-checks" :aria-label="i18n.t('auth.passwordRequirements')" aria-live="polite">
        <span
          v-for="check in passwordChecks"
          :key="check.label"
          :class="{ 'is-valid': check.valid }"
          :aria-label="`${check.label}：${i18n.t(check.valid ? 'auth.requirementMet' : 'auth.requirementNotMet')}`"
        >
          <i aria-hidden="true" /> {{ check.label }}
        </span>
      </div>

      <label class="field">
        <span>{{ i18n.t('auth.confirmPassword') }}</span>
        <input
          v-model="form.confirmPassword"
          type="password"
          autocomplete="new-password"
          :aria-invalid="submitted && form.password !== form.confirmPassword"
          aria-describedby="setup-confirm-password-error"
          required
        />
        <small v-if="submitted && form.password !== form.confirmPassword" id="setup-confirm-password-error">
          {{ i18n.t('auth.passwordMismatch') }}
        </small>
      </label>

      <button class="button button--primary button--block" type="submit" :disabled="session.state.loading">
        <LoaderCircle v-if="session.state.loading" class="spin" :size="17" />
        {{ i18n.t(session.state.loading ? 'auth.initializing' : 'auth.finishSetup') }}
      </button>
    </form>
  </AuthLayout>
</template>
