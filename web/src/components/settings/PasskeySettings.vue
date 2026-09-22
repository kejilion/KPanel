<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { KeyRound, LoaderCircle } from '@lucide/vue'
import { api, resetApiSecurityState } from '@/lib/api'
import { createPasskey, passkeyError, passkeysSupported } from '@/lib/passkeys'
import { formatDateTime } from '@/lib/format'
import { useI18n } from '@/i18n'
import { useSession } from '@/stores/session'
import type { PasskeyList, PasskeySummary } from '@/types/api'

const i18n = useI18n()
const session = useSession()
const router = useRouter()
const status = ref<PasskeyList>()
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const action = ref<'add' | 'revoke' | 'configure' | 'disable'>()
const selected = ref<PasskeySummary>()
const form = reactive({ name: '', password: '', totpCode: '' })
const formElement = ref<HTMLFormElement>()
const addButton = ref<HTMLButtonElement>()
let actionOpener: HTMLElement | undefined
const supported = passkeysSupported()
const controller = new AbortController()
const needsSecondFactor = computed(() => Boolean(session.state.user?.totpEnabled))
const canSubmit = computed(() => {
  if (busy.value || !form.password || (needsSecondFactor.value && !form.totpCode)) return false
  if (action.value === 'revoke') return Boolean(selected.value)
  if (action.value === 'configure') return Boolean(status.value?.configurable && status.value.detectedOrigin)
  if (action.value === 'disable') return Boolean(status.value?.origin && !status.value.originManaged)
  return Boolean(status.value?.available && supported && form.name.trim())
})

async function load(): Promise<void> {
  if (busy.value) return
  loading.value = true
  error.value = ''
  try {
    status.value = await api.auth.passkeys.list(controller.signal)
  } catch (reason) {
    if (!controller.signal.aborted) error.value = passkeyError(reason)
  } finally {
    loading.value = false
  }
}

function resetForm(): void {
  action.value = undefined
  selected.value = undefined
  form.name = ''
  form.password = ''
  form.totpCode = ''
  error.value = ''
}

async function startAction(next: 'add' | 'revoke' | 'configure' | 'disable', credential?: PasskeySummary, event?: Event): Promise<void> {
  if (busy.value) return
  actionOpener = event?.currentTarget as HTMLElement | undefined
  resetForm()
  action.value = next
  selected.value = credential
  await nextTick()
  formElement.value?.querySelector('input')?.focus()
}

async function cancelForm(): Promise<void> {
  resetForm()
  await nextTick()
  if (actionOpener?.isConnected) actionOpener.focus()
  else addButton.value?.focus()
}

async function submit(): Promise<void> {
  if (!canSubmit.value) return
  busy.value = true
  error.value = ''
  try {
    const authentication = { password: form.password, totpCode: form.totpCode || undefined }
    if (action.value === 'configure') {
      await api.auth.passkeys.configureOrigin(authentication, controller.signal)
      status.value = await api.auth.passkeys.list(controller.signal)
      resetForm()
      return
    } else if (action.value === 'disable') {
      await api.auth.passkeys.disableOrigin(authentication, controller.signal)
    } else if (action.value === 'revoke' && selected.value) {
      await api.auth.passkeys.delete({ ...authentication, id: selected.value.id }, controller.signal)
    } else {
      const ceremony = await api.auth.passkeys.registerBegin({ ...authentication, name: form.name.trim() }, controller.signal)
      form.password = ''
      form.totpCode = ''
      controller.signal.throwIfAborted()
      const credential = await createPasskey(ceremony.publicKey, controller.signal)
      await api.auth.passkeys.registerFinish({ ceremonyId: ceremony.ceremonyId, credential }, controller.signal)
    }
    resetForm()
    resetApiSecurityState()
    session.state.authenticated = false
    session.state.user = undefined
    session.state.expiresAt = undefined
    session.state.agent = undefined
    await router.replace({ name: 'login', query: { passkeyChanged: '1' } })
  } catch (reason) {
    if (!controller.signal.aborted) error.value = passkeyError(reason)
  } finally {
    form.password = ''
    form.totpCode = ''
    busy.value = false
  }
}

onMounted(() => { void load() })
onBeforeUnmount(() => {
  controller.abort()
  form.password = ''
  form.totpCode = ''
})
</script>

<template>
  <section class="passkey-settings settings-section panel-card" aria-labelledby="passkey-settings-title">
    <header class="passkey-heading settings-section__header">
      <KeyRound :size="21" aria-hidden="true" />
      <div><h2 id="passkey-settings-title">{{ i18n.t('passkey.title') }}</h2><p>{{ i18n.t('passkey.intro') }}</p></div>
    </header>
    <p v-if="loading" role="status">{{ i18n.t('passkey.loading') }}</p>
    <div v-if="error" class="inline-alert inline-alert--danger" role="alert">
      {{ error }}
      <button v-if="!status" class="button-link" type="button" @click="load">{{ i18n.t('passkey.retry') }}</button>
    </div>
    <template v-if="status && !loading">
      <p v-if="!status.available" class="inline-alert inline-alert--info">{{ i18n.t('passkey.unavailable') }}</p>
      <p v-if="!status.available && status.origin" class="passkey-note">{{ i18n.t('passkey.domain', { domain: status.origin }) }}</p>
      <div v-if="!status.available && status.configurable && status.detectedOrigin && !action" class="passkey-setup">
        <p class="passkey-note">{{ i18n.t('passkey.detectedOrigin', { origin: status.detectedOrigin }) }}</p>
        <p class="passkey-note">{{ i18n.t('passkey.configureNotice') }}</p>
        <button class="button button--secondary" type="button" :disabled="busy" @click="startAction('configure', undefined, $event)">{{ i18n.t('passkey.configure') }}</button>
      </div>
      <p v-if="status.available && !supported" class="inline-alert inline-alert--info">{{ i18n.t('passkey.unsupported') }}</p>
      <p v-if="status.rpId" class="passkey-note">{{ i18n.t('passkey.domain', { domain: status.rpId }) }}</p>
      <ul v-if="status.credentials.length" class="passkey-list">
        <li v-for="credential in status.credentials" :key="credential.id">
          <div class="passkey-details">
            <strong>{{ credential.name }}</strong>
            <span>{{ i18n.t('passkey.created', { time: formatDateTime(credential.createdAt) }) }}</span>
            <span>{{ i18n.t('passkey.lastUsed', { time: credential.lastUsedAt ? formatDateTime(credential.lastUsedAt) : i18n.t('passkey.neverUsed') }) }}</span>
          </div>
          <button class="button button--ghost" type="button" :disabled="busy" :aria-label="`${i18n.t('passkey.revoke')} ${credential.name}`" @click="startAction('revoke', credential, $event)">{{ i18n.t('passkey.revoke') }}</button>
        </li>
      </ul>
      <p v-else role="status">{{ i18n.t('passkey.empty') }}</p>
      <div v-if="status.origin && !action" class="passkey-actions">
        <button class="button button--danger" type="button" :disabled="busy || status.originManaged" @click="startAction('disable', undefined, $event)">{{ i18n.t('passkey.disable') }}</button>
      </div>
      <p v-if="status.originManaged" class="passkey-note">{{ i18n.t('passkey.originManaged') }}</p>
      <button v-if="!action" ref="addButton" class="button button--secondary" type="button" :disabled="!status.available || !supported" @click="startAction('add', undefined, $event)">{{ i18n.t('passkey.add') }}</button>
      <form v-else ref="formElement" class="form-stack passkey-form" @submit.prevent="submit">
        <p v-if="action === 'revoke' && selected" class="inline-alert inline-alert--warning">{{ i18n.t('passkey.revokeNotice', { name: selected.name }) }}</p>
        <p v-if="action === 'configure' && status.detectedOrigin" class="inline-alert inline-alert--warning">{{ i18n.t('passkey.configureConfirm', { origin: status.detectedOrigin }) }}</p>
        <p v-if="action === 'disable'" class="inline-alert inline-alert--warning">{{ i18n.t('passkey.disableConfirm') }}</p>
        <p v-if="action !== 'configure' && action !== 'disable'" class="passkey-note">{{ i18n.t('passkey.changeNotice') }}</p>
        <label v-if="action === 'add'" class="field"><span>{{ i18n.t('passkey.name') }}</span><input v-model="form.name" maxlength="64" :placeholder="i18n.t('passkey.nameHint')" :disabled="busy" required /></label>
        <label class="field"><span>{{ i18n.t('passkey.currentPassword') }}</span><input v-model="form.password" type="password" autocomplete="current-password" :disabled="busy" required /></label>
        <label v-if="needsSecondFactor" class="field"><span>{{ i18n.t('passkey.currentFactor') }}</span><input v-model.trim="form.totpCode" autocomplete="one-time-code" maxlength="17" :disabled="busy" required /></label>
        <p v-if="busy" role="status">{{ i18n.t('passkey.waiting') }}</p>
        <div class="passkey-actions">
          <button class="button button--ghost" type="button" :disabled="busy" @click="cancelForm">{{ i18n.t('passkey.cancel') }}</button>
          <button class="button" :class="action === 'revoke' || action === 'disable' ? 'button--danger' : 'button--primary'" type="submit" :disabled="!canSubmit"><LoaderCircle v-if="busy" class="spin" :size="16" />{{ i18n.t(action === 'revoke' ? 'passkey.confirmRevoke' : action === 'disable' ? 'passkey.disable' : action === 'configure' ? 'passkey.configure' : 'passkey.add') }}</button>
        </div>
      </form>
      <p class="passkey-note">{{ i18n.t('passkey.keepRecovery') }}</p>
    </template>
  </section>
</template>

<style scoped>
.passkey-settings { padding: 24px; font-size: 14px; line-height: 1.6; }
.passkey-heading { display: flex; align-items: flex-start; gap: 12px; }
.passkey-heading > svg { flex-shrink: 0; margin-top: 3px; color: var(--brand); }
.passkey-heading h2 { margin: 0; font-size: 17px; }
.passkey-heading p { margin: 4px 0 16px; color: var(--text-soft); }
.passkey-note { color: var(--text-soft); font-size: 13px; overflow-wrap: anywhere; }
.passkey-list { list-style: none; padding: 0; margin: 16px 0; }
.passkey-list li { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; padding: 16px 0; border-bottom: 1px solid var(--border); }
.passkey-details { display: grid; gap: 4px; min-width: 0; overflow-wrap: anywhere; }
.passkey-details span { color: var(--text-soft); font-size: 13px; }
.passkey-form { max-width: 560px; margin-top: 16px; }
.passkey-setup { display: grid; gap: 4px; margin: 16px 0; }
.passkey-actions { display: flex; flex-wrap: wrap; gap: 12px; }
.passkey-settings .button, .passkey-settings input, .passkey-settings .button-link { font-size: 14px; }
@media (max-width: 600px) { .passkey-settings { padding: 16px; } }
</style>
