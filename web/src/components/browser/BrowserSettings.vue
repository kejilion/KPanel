<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { LoaderCircle, RefreshCw } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { api, ApiError } from '@/lib/api'
import type { UserPreferences } from '@/types/api'
import type { BrowserComponent, BrowserComponentAction, BrowserComponentKind } from '@/lib/webBrowser'
import { useBrowserText } from '@/i18n/webBrowser'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: []; saved: [startURL: string]; changed: [action: BrowserComponentAction] }>()
const t = useBrowserText()
const component = ref<BrowserComponent>()
const selectedKind = ref<BrowserComponentKind>('chromium')
const loading = ref(false), submitting = ref(false), saving = ref(false)
const startURL = ref(''), version = ref(''), error = ref(''), saved = ref(false)
let storedStartURL = ''
const confirmAction = ref<BrowserComponentAction>()
const busy = computed(() => submitting.value || ['queued', 'running'].includes(component.value?.job?.state || ''))
const state = computed(() => !component.value?.installed ? 'notInstalled' : component.value.running ? component.value.healthy ? 'serviceReady' : 'serviceStarting' : 'serviceStopped')
const jobMessage = computed(() => {
  const message = component.value?.job?.message || ''
  if (message === 'browser_task_interrupted') return t('taskInterrupted')
  if (message.startsWith('browser_network_') || message.startsWith('browser_panel_')) return t('networkSetupFailed')
  return message
})
let timer: ReturnType<typeof setTimeout> | undefined
let controller: AbortController | undefined
let epoch = 0, completedJob = ''

async function refresh(initial = false): Promise<void> {
  clearTimeout(timer)
  const current = epoch
  if (initial) loading.value = true
  try {
    const value = await api.browser.component(controller?.signal)
    if (current !== epoch) return
    component.value = value
    if (value.installed) selectedKind.value = value.kind || 'chromium'
    const job = value.job
    if (initial && job && !['queued', 'running'].includes(job.state)) completedJob = job.id
    if (job?.state === 'succeeded' && completedJob !== job.id) {
      completedJob = job.id
      emit('changed', job.action)
    }
    if (['queued', 'running'].includes(job?.state || '')) timer = setTimeout(() => void refresh(), 2000)
  } catch { if (current === epoch) { error.value = t('settingsLoadFailed'); component.value = undefined } }
  finally { if (current === epoch) loading.value = false }
}
async function load(): Promise<void> {
  epoch++; controller?.abort(); controller = new AbortController()
  error.value = ''; saved.value = false; confirmAction.value = undefined; version.value = ''
  const current = epoch
  const preferences = async () => {
    try {
      const value = await api.preferences.get(controller?.signal)
      if (current === epoch) { storedStartURL = value.browser?.startURL || ''; startURL.value = storedStartURL; version.value = value.resourceVersion }
    } catch { if (current === epoch) error.value = t('settingsLoadFailed') }
  }
  await Promise.all([refresh(true), preferences()])
}
async function save(): Promise<void> {
  const value = startURL.value.trim()
  if (value) {
    try { const url = new URL(value); if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || value.length > 4096 || /[\x00-\x20\x7f]/.test(value)) throw Error() }
    catch { error.value = t('startURLInvalid'); return }
  }
  saving.value = true; saved.value = false; error.value = ''
  const current = epoch
  try {
    let expected = version.value, result: UserPreferences | undefined
    for (let attempt = 0; attempt < 3; attempt++) {
      try { result = await api.preferences.update({ browser: { startURL: value }, expectedResourceVersion: expected }, controller?.signal); break }
      catch (cause) {
        if (current !== epoch) return
        if (!(cause instanceof ApiError) || cause.status !== 409 || attempt === 2) throw cause
        const latest = await api.preferences.get(controller?.signal)
        if (current !== epoch) return
        // Desktop layout and locale share the version, but this form owns only
        // the browser section. Never overwrite a concurrent start-page edit.
        if ((latest.browser?.startURL || '') !== storedStartURL) throw cause
        expected = latest.resourceVersion
      }
    }
    if (!result) return
    if (current !== epoch) return
    version.value = result.resourceVersion; storedStartURL = result.browser?.startURL || ''; startURL.value = storedStartURL; saved.value = true; emit('saved', startURL.value)
  } catch { if (current === epoch) error.value = t('settingsSaveFailed') }
  finally { if (current === epoch) saving.value = false }
}
async function run(action: BrowserComponentAction): Promise<void> {
  if (!component.value || busy.value) return
  if (['stop', 'restart', 'uninstall'].includes(action) && confirmAction.value !== action) { confirmAction.value = action; return }
  confirmAction.value = undefined; submitting.value = true; error.value = ''
  const current = epoch
  try {
    const job = action === 'install' ? await api.browser.componentAction(action, component.value.resourceVersion, selectedKind.value) : await api.browser.componentAction(action, component.value.resourceVersion)
    if (current === epoch && component.value) { component.value.job = job; await refresh() }
  } catch { if (current === epoch) { error.value = t('serviceActionFailed'); await refresh() } }
  finally { if (current === epoch) submitting.value = false }
}
function close(): void { if (saving.value || submitting.value) return; emit('close') }
watch(() => props.open, open => {
  if (open) void load()
  else { epoch++; controller?.abort(); clearTimeout(timer); saving.value = false; submitting.value = false }
}, { immediate: true })
onBeforeUnmount(() => { epoch++; controller?.abort(); clearTimeout(timer) })
</script>

<template>
  <ModalDialog :open="open" :title="t('settings')" size="medium" @close="close">
    <div class="browser-settings">
      <section :aria-label="t('component')">
        <div class="browser-settings__heading"><h3>{{ t('component') }}</h3><button type="button" class="button button--secondary" :disabled="loading || submitting" :aria-label="t('refreshStatus')" @click="load"><RefreshCw :size="16" /></button></div>
        <p>{{ t('installHint') }}</p>
        <p v-if="loading" role="status"><LoaderCircle :size="16" />{{ t('checking') }}</p>
        <template v-else-if="component">
          <strong>{{ t(state) }}</strong>
          <label for="browser-component-kind">{{ t('browserKind') }}</label>
          <select id="browser-component-kind" v-model="selectedKind" :disabled="component.installed || busy || !component.supported">
            <option value="chromium">Chromium</option>
            <option value="tor" :disabled="!component.torSupported">Tor Browser</option>
          </select>
          <p>{{ t(component.installed ? 'switchKindHint' : selectedKind === 'tor' ? 'torInstallHint' : 'chooseKindHint') }}</p>
          <p v-if="!component.torSupported">{{ t('torArchitecture') }}</p>
          <p v-if="!component.supported" role="status">{{ t(component.reason === 'panel_compose_upgrade_required' ? 'composeUpgradeRequired' : 'runtimeRequired') }}</p>
          <div v-else class="browser-settings__actions">
            <button v-if="!component.installed" class="button button--primary" :disabled="busy" @click="run('install')">{{ t('installEnable') }}</button>
            <template v-else>
              <button class="button button--secondary" :disabled="busy" @click="run('install')">{{ t('repairService') }}</button>
              <button v-if="!component.running" class="button button--primary" :disabled="busy" @click="run('start')">{{ t('enableService') }}</button>
              <button v-if="component.running" class="button button--secondary" :disabled="busy" @click="run('stop')">{{ t('stopService') }}</button>
              <button class="button button--secondary" :disabled="busy" @click="run('restart')">{{ t('restartService') }}</button>
              <button class="button button--danger" :disabled="busy" @click="run('uninstall')">{{ t('uninstallService') }}</button>
            </template>
          </div>
          <div v-if="confirmAction" class="browser-settings__confirm" role="alert">
            <p>{{ t(confirmAction === 'uninstall' ? 'uninstallConfirm' : 'stopConfirm') }}</p>
            <div class="browser-settings__actions"><button class="button button--secondary" @click="confirmAction = undefined">{{ t('cancel') }}</button><button class="button button--danger" @click="run(confirmAction!)">{{ t('accept') }}</button></div>
          </div>
          <div v-if="component.job" role="status" class="browser-settings__job">
            <strong>{{ t(busy ? 'serviceWorking' : component.job.state === 'succeeded' ? 'serviceSucceeded' : 'serviceFailed') }}</strong>
            <p v-if="busy">{{ t('backgroundHint') }}</p>
            <p v-if="jobMessage">{{ jobMessage }}</p>
          </div>
        </template>
      </section>
      <form v-if="selectedKind !== 'tor'" @submit.prevent="save">
        <h3>{{ t('home') }}</h3>
        <label for="browser-start-url">{{ t('startURLLabel') }}</label>
        <input id="browser-start-url" v-model="startURL" type="url" placeholder="https://example.com" maxlength="4096" :disabled="!version || saving" />
        <p>{{ t('startURLHint') }}</p>
        <button class="button button--primary" type="submit" :disabled="!version || saving">{{ t(saving ? 'saving' : 'saveSettings') }}</button>
        <p v-if="saved" role="status">{{ t('settingsSaved') }}</p>
      </form>
      <p v-if="error" role="alert" class="browser-settings__error">{{ error }}</p>
    </div>
    <template #footer><button class="button button--secondary" :disabled="saving || submitting" @click="close">{{ t('done') }}</button></template>
  </ModalDialog>
</template>

<style scoped>
.browser-settings{display:grid;gap:1.25rem;font-size:14px;line-height:1.6;color:var(--text)}
.browser-settings h3{margin:0;font-size:16px}.browser-settings p{margin:.5rem 0;overflow-wrap:anywhere;color:var(--text-soft)}
.browser-settings__heading,.browser-settings__actions{display:flex;align-items:center;justify-content:space-between;gap:.6rem;flex-wrap:wrap}.browser-settings__actions{justify-content:flex-start;margin:.75rem 0}
.browser-settings .button{min-height:40px;font-size:14px}.browser-settings__heading .button{min-width:40px}
.browser-settings form{border-top:1px solid var(--border);padding-top:1rem}.browser-settings label{display:block;margin:.6rem 0 .3rem}.browser-settings input{box-sizing:border-box;width:100%;min-width:0;min-height:44px;padding:.5rem .75rem;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--surface);color:var(--text);font:inherit}
.browser-settings__confirm,.browser-settings__job{padding:.75rem;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--surface-subtle)}.browser-settings__error{color:var(--danger)!important}
.browser-settings select{width:100%;min-height:44px;padding:.5rem .75rem;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--surface);color:var(--text);font:inherit}
.browser-settings input:focus-visible,.browser-settings select:focus-visible{outline:2px solid var(--brand);outline-offset:2px}
</style>
