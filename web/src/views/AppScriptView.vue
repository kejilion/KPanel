<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { LoaderCircle, Play, RefreshCw, TriangleAlert } from '@lucide/vue'
import AppInteractiveTerminal from '@/components/apps/AppInteractiveTerminal.vue'
import { useI18n } from '@/i18n'
import { localizeError } from '@/i18n/errors'
import { ApiError, api } from '@/lib/api'
import { requestQuietAppJobStop, retryPendingAppJobStops } from '@/lib/appJobQuietStop'
import { useSession } from '@/stores/session'
import { desktopWindowCloseGuardKey, desktopWindowLaunchKey } from '@/lib/desktopRouteKeys'
import { usePhraseCatalog } from '@/i18n/phrase'
import type { AppInstallJob, AppMarketItem } from '@/types/api'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/AppScriptView/en-US').then((module) => module.default)
  : import('@/i18n/pages/AppScriptView/zh-TW').then((module) => module.default))

const route = useRoute()
const i18n = useI18n()
const session = useSession()
const currentOwner = () => session.state.authenticated ? session.state.user?.id || '' : ''
const owner = currentOwner()
const windowCloseGuards = inject(desktopWindowCloseGuardKey, undefined)
const consumeWindowLaunch = inject(desktopWindowLaunchKey, () => false)
const loading = ref(true)
const error = ref('')
const item = ref<AppMarketItem>()
const job = ref<AppInstallJob>()
let controller: AbortController | undefined
let loadRequest: Promise<void> | undefined
let closeRequest: Promise<boolean> | undefined
let unregisterWindowCloseGuard: (() => void) | undefined
let userClosed = false
let closingOwner = ''

const activeJobStorageKey = 'kpanel:active-app-job'
const appID = computed(() => String(route.params.appId || ''))
function isActiveJob(value?: AppInstallJob): boolean {
  return value?.status === 'queued' || value?.status === 'running'
}

function isInteractiveJob(value: unknown): value is AppInstallJob {
  return Boolean(
    value &&
      typeof value === 'object' &&
      'id' in value &&
      'appId' in value &&
      'interactive' in value,
  )
}

function rememberJob(id: string): void {
  try {
    window.localStorage.setItem(activeJobStorageKey, id)
  } catch {
    // The terminal remains usable when browser storage is unavailable.
  }
}

async function existingInteractiveJob(appId: string, signal: AbortSignal): Promise<AppInstallJob | undefined> {
  const jobs = await api.apps.jobs(signal)
  const active = jobs.items.find((candidate) => candidate.appId === appId && isActiveJob(candidate))
  if (!active) return undefined
  if (active.appId === appId && active.action === 'manage' && active.interactive) return active
  throw new Error(i18n.t('appScript.activeJob', { name: active.appName }))
}

async function launchManage(target: AppMarketItem): Promise<AppInstallJob> {
  const start = async (candidate: AppMarketItem): Promise<AppInstallJob> => {
    const resourceVersion = candidate.runtime.resourceVersion
    if (!resourceVersion) throw new Error(i18n.t('appScript.resourceVersionMissing'))
    const result = await api.apps.action(candidate.id, 'manage', { resourceVersion })
    if (!isInteractiveJob(result) || !result.interactive) {
      throw new Error(i18n.t('appScript.interactiveTaskMissing'))
    }
    return result
  }

  try {
    return await start(target)
  } catch (reason) {
    if (!(reason instanceof ApiError) || reason.code !== 'resource_conflict') throw reason
    const refreshed = (await api.apps.inventory()).items.find((candidate) => candidate.id === target.id)
    if (!refreshed) throw reason
    item.value = refreshed
    return start(refreshed)
  }
}

async function load(allowLaunch = false): Promise<void> {
  controller?.abort()
  const requestController = new AbortController()
  controller = requestController
  loading.value = true
  error.value = ''
  job.value = undefined
  try {
    if (!/^[A-Za-z0-9_-]{1,128}$/.test(appID.value)) throw new Error(i18n.t('appScript.invalidAppId'))
    const inventory = await api.apps.inventory(requestController.signal)
    if (controller !== requestController) return
    const target = inventory.items.find((candidate) => candidate.id === appID.value)
    if (!target) throw new Error(i18n.t('appScript.appNotFound'))
    item.value = target
    if (
      !target.runtime.installed ||
      !target.runtime.resourceVersion ||
      !target.capabilities.manage?.enabled
    ) {
      throw new Error(target.capabilities.manage?.reason || i18n.t('appScript.manageUnavailable'))
    }
    const existing = await existingInteractiveJob(target.id, requestController.signal)
    if (controller !== requestController) return
    job.value = existing || (allowLaunch && !userClosed && currentOwner() === owner ? await launchManage(target) : undefined)
    if (job.value) rememberJob(job.value.id)
    if (userClosed && isActiveJob(job.value)) requestQuietAppJobStop(job.value!.id, closingOwner, currentOwner)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = localizeError(reason, 'appScript.openFailed')
  } finally {
    if (controller === requestController) loading.value = false
  }
}

function startLoad(allowLaunch = false): void {
  const request = load(allowLaunch)
  loadRequest = request
  void request.finally(() => {
    if (loadRequest === request) loadRequest = undefined
  })
}

async function stopActiveJobBeforeClose(): Promise<boolean> {
  userClosed = true
  closingOwner = owner
  const active = job.value
  if (!active || !isActiveJob(active)) return true
  requestQuietAppJobStop(active.id, closingOwner, currentOwner)
  return true
}

function guardWindowClose(): Promise<boolean> {
  if (closeRequest) return closeRequest
  const request = stopActiveJobBeforeClose()
  closeRequest = request
  void request.finally(() => {
    if (closeRequest === request) closeRequest = undefined
  })
  return request
}

onMounted(() => {
  retryPendingAppJobStops(currentOwner(), currentOwner)
  unregisterWindowCloseGuard = windowCloseGuards?.register(guardWindowClose)
  startLoad(consumeWindowLaunch())
})
onBeforeUnmount(() => {
  unregisterWindowCloseGuard?.()
  if (!userClosed) controller?.abort()
})
</script>

<template>
  <section class="app-script-page terminal-theme-scope">

    <div v-if="loading" class="app-script-page__state" role="status">
      <LoaderCircle class="spin" :size="24" />
      <strong>{{ i18n.t('appScript.openingTitle') }}</strong>
      <small>正在校验安装状态、管理能力和资源版本。</small>
    </div>

    <div v-else-if="error" class="app-script-page__state is-error" role="alert">
      <TriangleAlert :size="26" />
      <strong>脚本终端无法启动</strong>
      <small>{{ error }}</small>
      <button class="button button--small" type="button" @click="startLoad()">
        <RefreshCw :size="14" /><span>重新尝试</span>
      </button>
    </div>

    <!-- The window title bar already names the app and owns maximize, so the
         terminal drops its own header. It stays mounted in unfocused windows so
         side-by-side scripts keep updating; it pauses itself while minimized. -->
    <AppInteractiveTerminal
      v-else-if="job"
      class="app-script-page__terminal"
      :job-id="job.id"
      :input-open="job.inputOpen"
      kind="app"
      headless
    />
    <div v-else class="app-script-page__state">
      <strong>{{ i18n.t('appScript.idleTitle') }}</strong>
      <small>{{ i18n.t('appScript.idleDescription') }}</small>
      <button class="button button--primary" type="button" data-app-script-start @click="startLoad(true)">
        <Play :size="16" />{{ i18n.t('appScript.start') }}
      </button>
    </div>
  </section>
</template>

<style scoped>
.app-script-page {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
  background: var(--terminal-wallpaper-background, var(--terminal-shell-background));
}

.app-script-page__terminal {
  height: 100%;
  min-height: 0;
  flex: 1 1 0;
  border: 0;
  border-radius: 0;
}

.app-script-page__terminal :deep(.interactive-terminal__screen) {
  height: auto;
  min-height: 0;
}

.app-script-page__state {
  display: grid;
  min-height: 0;
  flex: 1;
  place-items: center;
  align-content: center;
  gap: 9px;
  padding: 28px;
  color: var(--terminal-shell-muted);
  text-align: center;
}

.app-script-page__state strong {
  color: var(--terminal-shell-text);
  font-size: 14px;
}

.app-script-page__state small {
  max-width: 460px;
  line-height: 1.6;
}

.app-script-page__state.is-error > svg {
  color: var(--danger);
}
</style>
