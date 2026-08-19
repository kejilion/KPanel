<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { LoaderCircle, RefreshCw, SquareTerminal, TriangleAlert } from '@lucide/vue'
import AppInteractiveTerminal from '@/components/apps/AppInteractiveTerminal.vue'
import { ApiError, api } from '@/lib/api'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import { usePhraseCatalog } from '@/i18n/phrase'
import type { AppInstallJob, AppMarketItem } from '@/types/api'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/AppScriptView/en-US').then((module) => module.default)
  : import('@/i18n/pages/AppScriptView/zh-TW').then((module) => module.default))

const route = useRoute()
const windowActive = inject(desktopWindowActiveKey, computed(() => true))
const loading = ref(true)
const error = ref('')
const item = ref<AppMarketItem>()
const job = ref<AppInstallJob>()
let controller: AbortController | undefined

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
  const active = jobs.items.find(isActiveJob)
  if (!active) return undefined
  if (active.appId === appId && active.action === 'manage' && active.interactive) return active
  throw new Error(`已有应用任务正在运行：${active.appName}`)
}

async function launchManage(target: AppMarketItem): Promise<AppInstallJob> {
  const start = async (candidate: AppMarketItem): Promise<AppInstallJob> => {
    const resourceVersion = candidate.runtime.resourceVersion
    if (!resourceVersion) throw new Error('应用状态缺少安全版本标识。')
    const result = await api.apps.action(candidate.id, 'manage', { resourceVersion })
    if (!isInteractiveJob(result) || !result.interactive) {
      throw new Error('Agent 未返回可交互的脚本任务。')
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

async function load(): Promise<void> {
  controller?.abort()
  const requestController = new AbortController()
  controller = requestController
  loading.value = true
  error.value = ''
  job.value = undefined
  try {
    if (!/^[A-Za-z0-9_-]{1,128}$/.test(appID.value)) throw new Error('应用标识无效。')
    const inventory = await api.apps.inventory(requestController.signal)
    if (controller !== requestController) return
    const target = inventory.items.find((candidate) => candidate.id === appID.value)
    if (!target) throw new Error('应用目录中没有找到对应应用。')
    item.value = target
    if (
      !target.runtime.installed ||
      !target.runtime.resourceVersion ||
      !target.capabilities.manage?.enabled
    ) {
      throw new Error(target.capabilities.manage?.reason || '此应用没有可用的脚本管理入口。')
    }
    const existing = await existingInteractiveJob(target.id, requestController.signal)
    if (controller !== requestController) return
    job.value = existing || await launchManage(target)
    rememberJob(job.value.id)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reason instanceof Error ? reason.message : '无法打开应用脚本终端。'
  } finally {
    if (controller === requestController) loading.value = false
  }
}

onMounted(() => void load())
onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <section class="app-script-page">
    <div v-if="loading" class="app-script-page__state" role="status">
      <LoaderCircle class="spin" :size="24" />
      <strong>正在启动脚本终端…</strong>
      <small>正在校验安装状态、管理能力和资源版本。</small>
    </div>

    <div v-else-if="error" class="app-script-page__state is-error" role="alert">
      <TriangleAlert :size="26" />
      <strong>脚本终端无法启动</strong>
      <small>{{ error }}</small>
      <button class="button button--small" type="button" @click="load">
        <RefreshCw :size="14" /><span>重新尝试</span>
      </button>
    </div>

    <template v-else-if="job">
      <AppInteractiveTerminal
        v-if="windowActive"
        class="app-script-page__terminal"
        :job-id="job.id"
        :input-open="job.inputOpen"
        kind="app"
      />
      <div v-else class="app-script-page__state">
        <SquareTerminal :size="24" />
        <strong>终端已在后台保持</strong>
        <small>重新聚焦此窗口后继续显示脚本交互。</small>
      </div>
    </template>
  </section>
</template>

<style scoped>
.app-script-page {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
  background: var(--terminal-shell-background);
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
