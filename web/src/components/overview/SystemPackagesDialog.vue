<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch, type Component } from 'vue'
import { Archive, Check, Download, FilePenLine, Film, Gauge, GitBranch, LoaderCircle, Network, Package, Play, ShieldCheck, SquareTerminal, RefreshCw, Search, Trash2, TriangleAlert } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import HostTerminal from '@/components/terminal/HostTerminal.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { systemPackageLaunchCommand } from '@/lib/systemPackageLaunch'
import { useToast } from '@/stores/toast'
import type { SystemPackageID, SystemPackageItem, SystemPackagesSnapshot, TerminalSession } from '@/types/api'

const props = withDefaults(defineProps<{ open: boolean; readable: boolean; writable: boolean; unavailableReason?: string }>(), {
  unavailableReason: '',
})
const emit = defineEmits<{ close: [] }>()
const toast = useToast()
const snapshot = ref<SystemPackagesSnapshot>()
const selected = ref<SystemPackageID[]>([])
const search = ref('')
const statusFilter = ref<'all' | 'installed' | 'missing'>('all')
const loading = ref(false)
const refreshing = ref(false)
const submitting = ref(false)
const error = ref('')
const terminalOpen = ref(false)
const terminalOpening = ref(false)
const terminalError = ref('')
const terminalPackage = ref<SystemPackageItem>()
const terminalSession = ref<TerminalSession>()
let controller: AbortController | undefined
let timer: number | undefined
let terminalGeneration = 0

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const metadata: Record<SystemPackageID, { title: string; detail: string }> = {
  curl: { title: 'curl', detail: '命令行 HTTP 下载与接口调试' },
  wget: { title: 'wget', detail: '稳定的文件与镜像下载工具' },
  sudo: { title: 'sudo', detail: '按策略临时提升命令权限' },
  socat: { title: 'socat', detail: '端口转发与双向数据通道' },
  htop: { title: 'htop', detail: '交互式进程与资源监控' },
  iftop: { title: 'iftop', detail: '实时查看网卡流量连接' },
  unzip: { title: 'unzip', detail: '解压 ZIP 文件' },
  tar: { title: 'tar', detail: '归档与压缩文件处理' },
  tmux: { title: 'tmux', detail: '保持运行的多路终端会话' },
  ffmpeg: { title: 'ffmpeg', detail: '音视频转码与推流工具' },
  btop: { title: 'btop', detail: '现代化系统资源监控' },
  ranger: { title: 'ranger', detail: '终端文件管理器' },
  ncdu: { title: 'ncdu', detail: '交互式磁盘占用分析' },
  fzf: { title: 'fzf', detail: '终端模糊查找器' },
  vim: { title: 'vim', detail: '高效的终端文本编辑器' },
  nano: { title: 'nano', detail: '易上手的终端文本编辑器' },
  git: { title: 'git', detail: '分布式版本控制工具' },
}

const categoryIcons: Record<SystemPackageItem['category'], Component> = {
  network: Network,
  system: ShieldCheck,
  monitor: Gauge,
  archive: Archive,
  terminal: SquareTerminal,
  media: Film,
  editor: FilePenLine,
  developer: GitBranch,
}

const running = computed(() => snapshot.value?.maintenance.state === 'running' && snapshot.value.maintenance.action === 'packages')
const maintenanceBusy = computed(() => snapshot.value?.maintenance.state === 'running' && snapshot.value.maintenance.action !== 'packages')
const selectedSet = computed(() => new Set(selected.value))
const installedCount = computed(() => snapshot.value?.items.filter((item) => item.installed).length || 0)
const terminalTitle = computed(() => terminalPackage.value ? `${terminalPackage.value.id} · 独立终端` : '软件包独立终端')
const filteredItems = computed(() => {
  const needle = search.value.trim().toLowerCase()
  return (snapshot.value?.items || []).filter((item) => {
    if (statusFilter.value === 'installed' && !item.installed) return false
    if (statusFilter.value === 'missing' && item.installed) return false
    const copy = metadata[item.id]
    return !needle || `${copy.title} ${copy.detail}`.toLowerCase().includes(needle)
  })
})
const installable = computed(() => selected.value.filter((id) => !snapshot.value?.items.find((item) => item.id === id)?.installed))
const removable = computed(() => selected.value.filter((id) => snapshot.value?.items.find((item) => item.id === id)?.installed))

function clearTimer(): void {
  if (timer !== undefined) window.clearTimeout(timer)
  timer = undefined
}

async function load(silent = false): Promise<void> {
  if (!props.open || !props.readable) return
  controller?.abort()
  controller = new AbortController()
  if (silent) refreshing.value = true
  else loading.value = true
  error.value = ''
  try {
    snapshot.value = await api.system.packages(controller.signal)
    selected.value = selected.value.filter((id) => snapshot.value?.items.some((item) => item.id === id))
    clearTimer()
    if (running.value) timer = window.setTimeout(() => void load(true), 1_800)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reason instanceof ApiError ? reason.message : '无法读取软件包状态。'
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

function toggle(item: SystemPackageItem): void {
  if (running.value || submitting.value) return
  selected.value = selectedSet.value.has(item.id)
    ? selected.value.filter((id) => id !== item.id)
    : [...selected.value, item.id]
}

function selectMissing(): void {
  selected.value = (snapshot.value?.items || []).filter((item) => !item.installed).map((item) => item.id)
}

function statusFilterLabel(filter: 'all' | 'installed' | 'missing'): string {
  if (filter === 'installed') return '已安装'
  if (filter === 'missing') return '未安装'
  return '全部'
}

async function submit(action: 'install' | 'remove'): Promise<void> {
  const items = action === 'install' ? installable.value : removable.value
  if (!props.writable || !snapshot.value?.resourceVersion || !items.length || running.value || maintenanceBusy.value || submitting.value) return
  const message = action === 'install'
    ? `将使用 ${snapshot.value.manager} 按顺序安装 ${items.length} 个软件包。首个失败项会停止任务，已完成的安装会保留。确认开始吗？`
    : `将卸载 ${items.length} 个软件包及包管理器判定的关联配置。KPanel 不会自动重装它们，确认继续吗？`
  if (!window.confirm(translatePhrase(message))) return
  submitting.value = true
  try {
    const result = await api.system.packagesAction({ action, items, expectedResourceVersion: snapshot.value.resourceVersion })
    toast.success(action === 'install' ? '软件包安装已开始' : '软件包卸载已开始', result.message)
    await load(true)
  } catch (reason) {
    toast.danger('软件包任务启动失败', reason instanceof ApiError ? reason.message : 'Agent 未能启动后台任务。')
    await load(true)
  } finally {
    submitting.value = false
  }
}

function encodeTerminalInput(value: string): string {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return window.btoa(binary).replace(/=+$/, '')
}

function closePackageTerminal(): void {
  terminalGeneration += 1
  terminalOpen.value = false
  terminalOpening.value = false
  terminalError.value = ''
  terminalSession.value = undefined
  terminalPackage.value = undefined
}

async function launch(item: SystemPackageItem): Promise<void> {
  const command = systemPackageLaunchCommand(item.id)
  if (!item.installed || !item.launchable || !command || terminalOpening.value) return
  terminalOpen.value = true
  terminalOpening.value = true
  terminalError.value = ''
  terminalPackage.value = item
  terminalSession.value = undefined
  const generation = ++terminalGeneration
  let opened: TerminalSession | undefined
  try {
    opened = await api.terminals.open('local', 30, 120)
    if (generation !== terminalGeneration || !terminalOpen.value) {
      await api.terminals.close(opened.sessionId).catch(() => undefined)
      return
    }
    await api.terminals.input(opened.sessionId, encodeTerminalInput(`${command}\r`))
    if (generation !== terminalGeneration || !terminalOpen.value) {
      await api.terminals.close(opened.sessionId).catch(() => undefined)
      return
    }
    terminalSession.value = opened
  } catch (reason) {
    if (opened) await api.terminals.close(opened.sessionId).catch(() => undefined)
    if (generation === terminalGeneration && terminalOpen.value) {
      terminalError.value = reason instanceof ApiError && reason.code === 'terminal_limit'
        ? '已达到终端会话上限，请先关闭不用的终端。'
        : '软件包独立终端启动失败，请检查 Agent 与终端服务状态。'
    }
  } finally {
    if (generation === terminalGeneration) terminalOpening.value = false
  }
}

watch(() => [props.open, props.readable] as const, ([open, readable]) => {
  if (open && readable) void load()
  else {
    controller?.abort()
    clearTimer()
    closePackageTerminal()
  }
}, { immediate: true })

onBeforeUnmount(() => { controller?.abort(); clearTimer(); closePackageTerminal() })
</script>

<template>
  <ModalDialog
    :open="open"
    :title="phrase('常用软件包')"
    :description="phrase('精选 kejilion.sh 基础工具中的常用实用项；查看真实状态、批量安装或卸载，并快速启动交互式工具。')"
    size="wide"
    @close="emit('close')"
  >
    <div class="packages-dialog">
      <div v-if="!readable" class="inline-alert inline-alert--warning">{{ phrase(unavailableReason || '当前 Agent 的软件包管理适配器尚未就绪。') }}</div>
      <LoadingState v-else-if="loading && !snapshot" :rows="6" />
      <ErrorState v-else-if="error && !snapshot" :message="phrase(error)" @retry="load()" />
      <template v-else-if="snapshot">
        <header class="packages-summary">
          <span class="packages-summary__icon"><Package :size="22" /></span>
          <span><strong>{{ snapshot.manager }}</strong><small>{{ phrase(`已安装 ${installedCount}/${snapshot.items.length} 个常用工具`) }}</small></span>
          <span v-if="running" class="packages-summary__progress"><LoaderCircle :size="16" class="spin" /> {{ phrase(`${snapshot.maintenance.message} · ${snapshot.maintenance.progress}%`) }}</span>
          <button class="icon-button" type="button" :disabled="refreshing || submitting" :title="phrase('刷新软件包状态')" @click="load(true)"><RefreshCw :size="17" :class="{ spin: refreshing }" /></button>
        </header>

        <div v-if="maintenanceBusy" class="inline-alert inline-alert--warning"><TriangleAlert :size="17" /> {{ phrase('另一项系统维护任务正在运行，完成后才能管理软件包。') }}</div>
        <div v-else-if="snapshot.maintenance.action === 'packages' && snapshot.maintenance.state === 'failed'" class="inline-alert inline-alert--danger"><TriangleAlert :size="17" /> {{ phrase(snapshot.maintenance.message || '软件包任务执行失败。') }}</div>

        <div class="packages-toolbar">
          <label class="packages-search"><Search :size="17" /><input v-model="search" type="search" :placeholder="phrase('搜索名称或用途')" /></label>
          <div class="packages-filters" :aria-label="phrase('安装状态')">
            <button v-for="filter in (['all', 'installed', 'missing'] as const)" :key="filter" type="button" :class="{ 'is-active': statusFilter === filter }" @click="statusFilter = filter">
              {{ phrase(statusFilterLabel(filter)) }}
            </button>
          </div>
        </div>

        <div class="packages-selection">
          <button type="button" :disabled="running || submitting" @click="selectMissing">{{ phrase('选择全部未安装工具') }}</button>
          <button type="button" :disabled="running || submitting" @click="selected = []">{{ phrase('清空选择') }}</button>
          <small>{{ phrase(`已选择 ${selected.length} 项`) }}</small>
        </div>

        <div v-if="filteredItems.length" class="packages-grid">
          <article v-for="item in filteredItems" :key="item.id" class="package-card" :class="{ 'is-selected': selectedSet.has(item.id) }">
            <button class="package-card__select" type="button" :disabled="running || submitting" @click="toggle(item)">
              <span class="package-card__check"><Check v-if="selectedSet.has(item.id)" :size="16" /></span>
              <span class="package-card__icon" :class="`is-${item.category}`" :data-package-category="item.category" aria-hidden="true"><component :is="categoryIcons[item.category]" :size="18" /></span>
              <span class="package-card__body"><strong>{{ metadata[item.id].title }}</strong><small>{{ phrase(metadata[item.id].detail) }}</small></span>
              <span class="package-card__status" :class="{ 'is-installed': item.installed }">{{ phrase(item.installed ? '已安装' : '未安装') }}</span>
            </button>
            <button v-if="item.installed && item.launchable" class="package-card__launch" type="button" :title="phrase(`在独立终端启动 ${item.id}`)" @click="launch(item)"><Play :size="15" />{{ phrase('启动') }}</button>
          </article>
        </div>
        <div v-else class="packages-empty">{{ phrase('没有符合当前条件的软件包。') }}</div>

        <footer class="packages-footer">
          <span>{{ phrase(running ? '任务在后台运行，关闭窗口不会中断；完成后将重新读取真实状态。' : '仅管理固定目录中的软件包；自定义包名请继续使用终端。') }}</span>
          <div>
            <button class="button" type="button" :disabled="!writable || !removable.length || running || maintenanceBusy || submitting" @click="submit('remove')"><Trash2 :size="16" />{{ phrase(`卸载所选（${removable.length}）`) }}</button>
            <button class="button button--primary" type="button" :disabled="!writable || !installable.length || running || maintenanceBusy || submitting" @click="submit('install')"><LoaderCircle v-if="submitting" :size="16" class="spin" /><Download v-else :size="16" />{{ phrase(`安装所选（${installable.length}）`) }}</button>
          </div>
        </footer>
      </template>
    </div>
  </ModalDialog>

  <ModalDialog
    :open="terminalOpen"
    :title="phrase(terminalTitle)"
    :description="phrase('独立会话，不会占用或改动主终端页面中的会话。')"
    size="wide"
    allow-fullscreen
    @close="closePackageTerminal"
  >
    <div class="package-terminal">
      <LoadingState v-if="terminalOpening" :rows="4" />
      <ErrorState v-else-if="terminalError" :message="phrase(terminalError)" />
      <HostTerminal
        v-else-if="terminalSession"
        :session-id="terminalSession.sessionId"
        :host-name="phrase('本机')"
        :initial-offset="terminalSession.offset"
      />
    </div>
  </ModalDialog>
</template>

<style scoped>
.packages-dialog { display:grid; gap:14px; }
.packages-summary { display:flex; min-height:58px; align-items:center; gap:10px; padding:10px 12px; border:1px solid var(--border); border-radius:var(--radius); background:var(--surface-subtle); }
.packages-summary__icon { display:grid; width:38px; height:38px; place-items:center; border-radius:var(--radius); color:var(--brand); background:color-mix(in srgb,var(--brand) 10%,transparent); }
.packages-summary>span:nth-child(2) { display:grid; gap:3px; }
.packages-summary small { color:var(--muted); font-size:13px; }
.packages-summary__progress { display:inline-flex; align-items:center; gap:6px; margin-left:auto; color:var(--brand); font-size:13px; }
.packages-summary .icon-button { margin-left:auto; }
.packages-summary__progress+.icon-button { margin-left:0; }
.packages-toolbar { display:grid; grid-template-columns:minmax(190px,1fr) auto; align-items:center; gap:10px; }
.packages-search { position:relative; }
.packages-search svg { position:absolute; top:50%; left:11px; transform:translateY(-50%); color:var(--muted); }
.packages-search input { width:100%; min-height:38px; padding:8px 10px 8px 35px; border:1px solid var(--border); border-radius:var(--radius-sm); color:inherit; background:var(--surface); font:inherit; font-size:14px; }
.packages-filters { display:flex; gap:4px; padding:3px; border:1px solid var(--border); border-radius:var(--radius-sm); background:var(--surface-subtle); }
.packages-filters button { display:inline-flex; min-height:30px; align-items:center; gap:4px; padding:4px 9px; border:0; border-radius:var(--radius-sm); color:var(--muted); background:transparent; font:inherit; font-size:13px; cursor:pointer; }
.packages-filters button.is-active { color:var(--text); background:var(--surface); box-shadow:var(--shadow-sm); }
.packages-selection { display:flex; align-items:center; gap:8px; }
.packages-selection button { min-height:32px; padding:5px 9px; border:1px solid var(--border); border-radius:var(--radius-sm); color:var(--muted); background:transparent; font:inherit; font-size:13px; cursor:pointer; }
.packages-selection small { margin-left:auto; color:var(--muted); font-size:13px; }
.packages-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:8px; padding-bottom:76px; }
.package-card { position:relative; display:flex; min-height:70px; border:1px solid var(--border); border-radius:var(--radius); background:transparent; overflow:hidden; }
.package-card.is-selected { border-color:color-mix(in srgb,var(--brand) 50%,var(--border)); background:color-mix(in srgb,var(--brand) 6%,transparent); }
.package-card__select { display:grid; min-width:0; flex:1; grid-template-columns:20px 34px minmax(0,1fr) auto; align-items:center; gap:9px; padding:11px 10px; border:0; color:inherit; background:transparent; text-align:left; cursor:pointer; }
.package-card__check { display:grid; width:18px; height:18px; place-items:center; border:1px solid var(--border); border-radius:var(--radius-sm); color:#fff; }
.is-selected .package-card__check { border-color:var(--brand); background:var(--brand); }
.package-card__icon { display:grid; width:34px; height:34px; place-items:center; border-radius:var(--radius-sm); color:var(--brand-strong); background:var(--brand-soft); }
.package-card__icon.is-network,.package-card__icon.is-developer { color:var(--blue); background:var(--blue-soft); }
.package-card__icon.is-monitor { color:var(--success); background:var(--success-soft); }
.package-card__icon.is-archive { color:var(--amber); background:var(--amber-soft); }
.package-card__icon.is-media,.package-card__icon.is-editor { color:var(--violet); background:var(--violet-soft); }
.package-card__icon.is-system { color:var(--text); background:var(--neutral-soft); }
.package-card__body { display:grid; min-width:0; gap:4px; }
.package-card__body strong { font-size:14px; }
.package-card__body small { overflow:hidden; color:var(--muted); font-size:13px; line-height:1.35; text-overflow:ellipsis; white-space:nowrap; }
.package-card__status { padding:3px 7px; border-radius:999px; color:var(--muted); background:var(--surface-subtle); font-size:12px; white-space:nowrap; }
.package-card__status.is-installed { color:var(--brand); background:color-mix(in srgb,var(--brand) 11%,transparent); }
.package-card__launch { display:inline-flex; min-width:66px; align-items:center; justify-content:center; gap:4px; border:0; border-left:1px solid var(--border); color:var(--brand); background:color-mix(in srgb,var(--brand) 4%,transparent); font:inherit; font-size:13px; cursor:pointer; }
.package-card__launch:hover,.package-card__launch:focus-visible { background:color-mix(in srgb,var(--brand) 10%,transparent); outline:none; }
.packages-empty { display:grid; min-height:160px; place-items:center; color:var(--muted); }
.packages-footer { position:sticky; z-index:2; bottom:0; display:flex; align-items:center; justify-content:space-between; gap:14px; padding:12px 0 10px; background:var(--surface); box-shadow:0 -12px 18px var(--surface); }
.packages-footer>span { max-width:52%; color:var(--muted); font-size:12px; line-height:1.45; }
.packages-footer>div { display:flex; gap:8px; }
.packages-footer .button { display:inline-flex; align-items:center; gap:6px; }
.package-terminal { height:min(62vh,560px); min-height:360px; }
@media (max-width:800px) { .packages-toolbar { grid-template-columns:1fr; } .packages-filters { width:max-content; max-width:100%; overflow-x:auto; } .packages-grid { grid-template-columns:1fr; } .packages-footer { align-items:stretch; flex-direction:column; } .packages-footer>span { max-width:none; } .packages-footer>div { display:grid; grid-template-columns:1fr 1fr; } .packages-footer .button { justify-content:center; } }
</style>
