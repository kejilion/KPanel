<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, onMounted, ref, type ComponentPublicInstance } from 'vue'
import { Circle, Laptop, LoaderCircle, Menu, PanelLeftClose, PanelLeftOpen, Plus, RefreshCw, Server, ShieldCheck, SquareTerminal, X } from '@lucide/vue'
import HostTerminal from '@/components/terminal/HostTerminal.vue'
import TerminalToolbar from '@/components/terminal/TerminalToolbar.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { useTerminalFullscreen } from '@/composables/useTerminalFullscreen'
import { api, ApiError } from '@/lib/api'
import type { ClusterHost, ClusterHostList } from '@/types/api'
import { usePhraseCatalog } from '@/i18n/phrase'
import { useI18n } from '@/i18n'
import { desktopCloseGuardCoordinator, desktopWindowCloseGuardKey } from '@/lib/desktopRouteKeys'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/TerminalView/en-US').then((module) => module.default)
  : import('@/i18n/pages/TerminalView/zh-TW').then((module) => module.default))
const { t } = useI18n()
const desktopWindowCloseGuards = inject(desktopWindowCloseGuardKey, undefined)
let unregisterWindowCloseGuard: (() => void) | undefined

interface OpenTerminal {
  id: string
  hostId: string
  hostName: string
  offset: number
  state: 'connecting' | 'connected' | 'reconnecting' | 'finished'
}

interface HostTerminalHandle {
  focusTerminal: () => void
  scrollToTop: () => void
  scheduleResize: () => void
}

const inventory = ref<ClusterHostList>()
const sessions = ref<OpenTerminal[]>([])
const activeSessionId = ref('')
const loading = ref(true)
const openingHostId = ref('')
const errorMessage = ref('')
const search = ref('')
const connectionsCollapsed = ref(false)
const mobileConnectionsOpen = ref(false)
const terminalRefs = new Map<string, HostTerminalHandle>()
let controller: AbortController | undefined
const connectionsCollapsedStorageKey = 'kpanel:terminal:connections-collapsed'

function refreshActiveTerminal(): void {
  terminalRefs.get(activeSessionId.value)?.scheduleResize()
}

function focusActiveTerminal(): void {
  terminalRefs.get(activeSessionId.value)?.focusTerminal()
}

const {
  fullscreen: workspaceFullscreen,
  toggleFullscreen: toggleWorkspaceFullscreen,
  exitFullscreen: exitWorkspaceFullscreen,
} = useTerminalFullscreen(refreshActiveTerminal)

const hosts = computed(() => {
  const needle = search.value.trim().toLowerCase()
  return (inventory.value?.items || []).filter((host) => !needle || `${host.name} ${host.origin}`.toLowerCase().includes(needle))
})

const activeSession = computed(() => sessions.value.find((item) => item.id === activeSessionId.value))

async function loadHosts(): Promise<void> {
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  errorMessage.value = ''
  try {
    inventory.value = await api.cluster.hosts(controller.signal)
  } catch {
    errorMessage.value = '连接列表加载失败，请检查 Agent 与集群状态。'
  } finally {
    loading.value = false
  }
}

async function openHost(host: ClusterHost): Promise<void> {
  const existing = sessions.value.find((item) => item.hostId === host.id)
  if (existing) {
    selectSession(existing.id)
    mobileConnectionsOpen.value = false
    return
  }
  if (!host.terminalAvailable || openingHostId.value) return
  mobileConnectionsOpen.value = false
  openingHostId.value = host.id
  errorMessage.value = ''
  try {
    const opened = await api.terminals.open(host.id, 30, 120)
    const item: OpenTerminal = { id: opened.sessionId, hostId: host.id, hostName: host.name, offset: opened.offset, state: 'connecting' }
    sessions.value.push(item)
    activeSessionId.value = item.id
  } catch (reason) {
    errorMessage.value = reason instanceof ApiError && reason.code === 'terminal_limit'
      ? '已达到终端会话上限，请先关闭不用的连接。'
      : '终端连接失败，请确认目标 KPanel 在线且双方均已更新。'
  } finally {
    openingHostId.value = ''
  }
}

function removeSession(id: string): void {
  const index = sessions.value.findIndex((item) => item.id === id)
  if (index < 0) return
  sessions.value.splice(index, 1)
  terminalRefs.delete(id)
  if (activeSessionId.value === id) {
    activeSessionId.value = sessions.value[Math.max(0, index - 1)]?.id || ''
    if (activeSessionId.value) {
      void nextTick(() => {
        refreshActiveTerminal()
        focusActiveTerminal()
      })
    }
  }
  if (!sessions.value.length) exitWorkspaceFullscreen()
}

function setTerminalRef(
  id: string,
  instance: Element | ComponentPublicInstance | null,
): void {
  const handle = instance as unknown as Partial<HostTerminalHandle> | null
  if (
    typeof handle?.focusTerminal === 'function' &&
    typeof handle.scrollToTop === 'function' &&
    typeof handle.scheduleResize === 'function'
  ) {
    terminalRefs.set(id, handle as HostTerminalHandle)
  } else {
    terminalRefs.delete(id)
  }
}

function selectSession(id: string): void {
  activeSessionId.value = id
  void nextTick(() => {
    refreshActiveTerminal()
    focusActiveTerminal()
  })
}

function scrollActiveTerminalToTop(): void {
  terminalRefs.get(activeSessionId.value)?.scrollToTop()
}

function toggleConnections(): void {
  connectionsCollapsed.value = !connectionsCollapsed.value
  try {
    window.localStorage.setItem(
      connectionsCollapsedStorageKey,
      connectionsCollapsed.value ? '1' : '0',
    )
  } catch {
    // Storage can be unavailable in privacy modes; collapsing still works for this visit.
  }
  void nextTick(refreshActiveTerminal)
}

function hostStateLabel(host: ClusterHost): string {
  if (!host.terminalAvailable) return host.kind === 'light_node' ? '轻量监控节点' : '需要重新配对'
  if (host.isLocal) return '本机终端'
  return '加密直连'
}

function sessionStateLabel(state: OpenTerminal['state']): string {
  if (state === 'connected') return t('terminal.connected')
  if (state === 'finished') return t('terminal.finished')
  if (state === 'reconnecting') return t('terminal.reconnecting')
  return t('terminal.connecting')
}

onMounted(() => {
  try {
    connectionsCollapsed.value = window.localStorage.getItem(connectionsCollapsedStorageKey) === '1'
  } catch {
    connectionsCollapsed.value = false
  }
  const guard = () => {
    const activeCount = sessions.value.filter((session) => session.state !== 'finished').length
    return !activeCount || window.confirm(`关闭窗口将断开 ${activeCount} 个终端会话，是否继续？`)
  }
  unregisterWindowCloseGuard = desktopWindowCloseGuards
    ? desktopWindowCloseGuards.register(guard)
    : desktopCloseGuardCoordinator.register('classic-terminal', guard)
  void loadHosts()
})
onBeforeUnmount(() => {
  unregisterWindowCloseGuard?.()
  controller?.abort()
})
</script>

<template>
  <div class="page terminal-page">
    <PageHeader title="多主机终端" description="通过集群加密通道连接本机与已授权 KPanel 节点，无需开放额外 SSH 或公网端口。" />

    <div v-if="errorMessage" class="terminal-alert" role="alert">{{ errorMessage }}</div>

    <section
      class="terminal-workspace"
      :class="{
        'is-connections-collapsed': connectionsCollapsed,
        'is-connections-drawer-open': mobileConnectionsOpen,
      }"
    >
      <button
        v-if="mobileConnectionsOpen"
        class="terminal-connections-overlay"
        type="button"
        aria-label="关闭主机选择"
        @click="mobileConnectionsOpen = false"
      />
      <aside id="terminal-connections-drawer" class="terminal-connections">
        <header>
          <div class="terminal-connections__heading"><strong>连接列表</strong><small>{{ hosts.length }} 台主机</small></div>
          <div class="terminal-connections__actions">
            <ShieldCheck class="terminal-connections__shield" :size="18" />
            <button
              class="terminal-connections__toggle terminal-connections__refresh"
              type="button"
              :disabled="loading"
              :title="t('terminal.refreshConnections')"
              :aria-label="t('terminal.refreshConnections')"
              @click="loadHosts"
            >
              <RefreshCw :size="17" :class="{ spin: loading }" />
            </button>
            <button
              class="terminal-connections__toggle terminal-connections__collapse"
              type="button"
              aria-controls="terminal-connection-selector"
              :aria-expanded="!connectionsCollapsed"
              :title="t(connectionsCollapsed ? 'terminal.expandConnections' : 'terminal.collapseConnections')"
              :aria-label="t(connectionsCollapsed ? 'terminal.expandConnections' : 'terminal.collapseConnections')"
              @click="toggleConnections"
            >
              <PanelLeftOpen v-if="connectionsCollapsed" :size="18" />
              <PanelLeftClose v-else :size="18" />
            </button>
            <button
              class="terminal-connections__toggle terminal-connections__mobile-close"
              type="button"
              aria-label="关闭主机选择"
              @click="mobileConnectionsOpen = false"
            >
              <X :size="18" />
            </button>
          </div>
        </header>
        <label v-show="!connectionsCollapsed || mobileConnectionsOpen" class="terminal-search"><input v-model="search" type="search" placeholder="搜索主机" /></label>
        <div id="terminal-connection-selector" v-show="!connectionsCollapsed || mobileConnectionsOpen" class="terminal-connections__list">
          <div v-if="loading" class="terminal-connections__empty"><LoaderCircle class="spin" :size="22" /> {{ t('terminal.loadingHosts') }}</div>
          <div v-else-if="!hosts.length" class="terminal-connections__empty">暂无可显示主机</div>
          <button v-for="host in hosts" :key="host.id" class="terminal-host" :class="{ 'is-active': activeSession?.hostId === host.id }" type="button" :disabled="openingHostId === host.id" @click="openHost(host)">
            <span class="terminal-host__icon"><Laptop v-if="host.isLocal" :size="19" /><Server v-else :size="19" /></span>
            <span><strong>{{ host.name }}</strong><small>{{ host.origin || t('terminal.currentPanel') }}</small><em :class="{ 'is-ready': host.terminalAvailable }"><Circle :size="8" fill="currentColor" /> {{ hostStateLabel(host) }}</em></span>
            <LoaderCircle v-if="openingHostId === host.id" class="spin" :size="17" />
            <Plus v-else-if="host.terminalAvailable && !sessions.some((item) => item.hostId === host.id)" :size="17" />
          </button>
        </div>
        <div v-show="connectionsCollapsed && !mobileConnectionsOpen" class="terminal-connections__rail" aria-label="收起的主机列表">
          <button
            v-for="host in hosts"
            :key="host.id"
            class="terminal-host-rail"
            :class="{ 'is-active': activeSession?.hostId === host.id }"
            type="button"
            :disabled="openingHostId === host.id"
            :title="`${host.name} · ${hostStateLabel(host)}`"
            :aria-label="`${host.name} · ${hostStateLabel(host)}`"
            @click="openHost(host)"
          >
            <Laptop v-if="host.isLocal" :size="18" />
            <Server v-else :size="18" />
            <i :class="{ 'is-ready': host.terminalAvailable }" aria-hidden="true" />
          </button>
        </div>
      </aside>

      <main class="terminal-stage" :class="{ 'is-fullscreen': workspaceFullscreen }">
        <div class="terminal-stage__mobile-selector">
          <button
            type="button"
            aria-controls="terminal-connections-drawer"
            :aria-expanded="mobileConnectionsOpen"
            aria-label="打开主机选择"
            @click="mobileConnectionsOpen = true"
          >
            <Menu :size="18" />
            <span>{{ activeSession?.hostName || '选择主机' }}</span>
          </button>
          <small>{{ sessions.length ? `${sessions.length} 个终端会话` : `${hosts.length} 台主机` }}</small>
        </div>
        <div v-if="sessions.length" class="terminal-tabs-bar">
          <nav v-if="sessions.length" class="terminal-tabs" aria-label="已打开终端">
            <button v-for="item in sessions" :key="item.id" type="button" class="terminal-tab" :class="{ 'is-active': item.id === activeSessionId }" :title="`${item.hostName} · ${sessionStateLabel(item.state)}`" @click="selectSession(item.id)">
              <span class="terminal-tab__status" :class="`is-${item.state}`" aria-hidden="true" />
              <SquareTerminal :size="14" /><span class="terminal-tab__name">{{ item.hostName }}</span>
              <span class="sr-only">{{ sessionStateLabel(item.state) }}</span>
              <X :size="14" @click.stop="removeSession(item.id)" />
            </button>
          </nav>
          <TerminalToolbar
            :fullscreen="workspaceFullscreen"
            @scroll-top="scrollActiveTerminalToTop"
            @toggle-fullscreen="toggleWorkspaceFullscreen"
          />
        </div>
        <div v-if="!sessions.length" class="terminal-empty"><span><SquareTerminal :size="34" /></span><h2>选择一台主机开始</h2><p>左侧会明确标记本机、可加密直连的 KPanel，以及仅提供监控的轻量节点。</p></div>
        <HostTerminal v-for="item in sessions" v-show="item.id === activeSessionId" :key="item.id" :ref="(instance) => setTerminalRef(item.id, instance)" :session-id="item.id" :host-name="item.hostName" :initial-offset="item.offset" @state-change="item.state = $event" />
      </main>
    </section>
  </div>
</template>

<style scoped>
.terminal-page { min-height:calc(100vh - 100px); gap:18px; }
.terminal-alert { border:1px solid color-mix(in srgb,var(--danger) 34%,var(--border)); border-radius:10px; padding:11px 13px; color:var(--danger); background:color-mix(in srgb,var(--danger) 8%,var(--surface)); }
.terminal-workspace { position:relative; display:grid; height:var(--terminal-workspace-height); min-height:var(--terminal-workspace-min-height); grid-template-columns:280px minmax(0,1fr); overflow:hidden; border:1px solid var(--border); border-radius:var(--terminal-workspace-radius); background:var(--surface); box-shadow:var(--shadow-sm); transition:grid-template-columns 180ms ease; }
.terminal-workspace.is-connections-collapsed { grid-template-columns:52px minmax(0,1fr); }
.terminal-connections { display:grid; min-width:0; min-height:0; grid-template-rows:auto auto minmax(0,1fr); overflow:hidden; border-right:1px solid var(--border); background:color-mix(in srgb,var(--surface) 92%,var(--brand) 8%); }
.terminal-connections>header { display:flex; align-items:center; justify-content:space-between; padding:17px 16px 12px; color:var(--brand); }
.terminal-connections__heading { display:grid; min-width:0; gap:2px; color:var(--text); }
.terminal-connections>header small { color:var(--text-muted); font-weight:500; }
.terminal-connections__actions { display:flex; flex:0 0 auto; align-items:center; gap:8px; }
.terminal-connections__toggle { display:grid; width:32px; height:32px; flex:0 0 auto; place-items:center; border:1px solid var(--border); border-radius:8px; color:var(--text-muted); background:var(--surface); cursor:pointer; }
.terminal-connections__toggle:hover,.terminal-connections__toggle:focus-visible { border-color:color-mix(in srgb,var(--brand) 55%,var(--border)); color:var(--brand); outline:none; }
.terminal-connections__mobile-close,.terminal-connections-overlay,.terminal-stage__mobile-selector { display:none; }
.terminal-workspace.is-connections-collapsed .terminal-connections>header { justify-content:center; padding:12px 9px; }
.terminal-workspace.is-connections-collapsed .terminal-connections__heading,.terminal-workspace.is-connections-collapsed .terminal-connections__shield,.terminal-workspace.is-connections-collapsed .terminal-connections__refresh { display:none; }
.terminal-search { display:block; padding:0 12px 10px; }
.terminal-search input { width:100%; border:1px solid var(--border); border-radius:9px; padding:9px 11px; color:var(--text); background:var(--surface); }
.terminal-connections__list { min-height:0; overflow-y:auto; overscroll-behavior:contain; padding-bottom:8px; scrollbar-gutter:stable; }
.terminal-connections__rail { display:grid; grid-row:2 / -1; min-height:0; align-content:start; justify-items:center; gap:6px; overflow-y:auto; overscroll-behavior:contain; padding:7px 6px 10px; scrollbar-width:none; }
.terminal-connections__rail::-webkit-scrollbar { display:none; }
.terminal-host-rail { position:relative; display:grid; width:36px; height:36px; flex:0 0 auto; place-items:center; border:1px solid transparent; border-radius:9px; color:var(--text-muted); background:transparent; cursor:pointer; }
.terminal-host-rail:hover,.terminal-host-rail:focus-visible,.terminal-host-rail.is-active { border-color:color-mix(in srgb,var(--brand) 48%,var(--border)); color:var(--brand); background:color-mix(in srgb,var(--brand) 11%,var(--surface)); outline:none; }
.terminal-host-rail:disabled { cursor:wait; opacity:.55; }
.terminal-host-rail i { position:absolute; right:3px; bottom:3px; width:7px; height:7px; border:1px solid var(--surface); border-radius:50%; background:var(--text-muted); }
.terminal-host-rail i.is-ready { background:var(--success); }
.terminal-host { display:grid; width:calc(100% - 16px); grid-template-columns:auto minmax(0,1fr) auto; align-items:center; gap:10px; margin:4px 8px; border:1px solid transparent; border-radius:11px; padding:11px; text-align:left; color:var(--text); background:transparent; }
.terminal-host:hover,.terminal-host.is-active { border-color:color-mix(in srgb,var(--brand) 45%,var(--border)); background:color-mix(in srgb,var(--brand) 10%,var(--surface)); }
.terminal-host__icon { display:grid; width:36px; height:36px; place-items:center; border-radius:10px; color:var(--brand); background:color-mix(in srgb,var(--brand) 12%,var(--surface)); }
.terminal-host>span:nth-child(2) { display:grid; min-width:0; gap:2px; }
.terminal-host strong,.terminal-host small { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.terminal-host small { color:var(--text-muted); font-size:11px; }
.terminal-host em { display:flex; align-items:center; gap:5px; color:var(--text-muted); font-size:10px; font-style:normal; }
.terminal-host em.is-ready { color:var(--success); }
.terminal-connections__empty { display:flex; align-items:center; justify-content:center; gap:8px; min-height:180px; padding:20px; color:var(--text-muted); text-align:center; }
.terminal-stage { display:grid; grid-template-rows:auto auto minmax(0,1fr); min-width:0; min-height:0; overflow:hidden; padding:12px; background:var(--terminal-shell-background,#0b1214); }
.terminal-stage.is-fullscreen { position:fixed; z-index:6000; inset:0; width:100vw; height:100dvh; min-height:0; padding:0; border:0; }
.terminal-tabs-bar { display:flex; min-width:0; align-items:center; gap:12px; padding:8px 10px; border:1px solid var(--terminal-shell-border,#29383a); border-bottom:0; border-radius:var(--terminal-shell-radius,12px) var(--terminal-shell-radius,12px) 0 0; background:var(--terminal-shell-panel,#111a1d); }
.terminal-tabs { display:flex; min-width:0; flex:1; gap:5px; overflow-x:auto; scrollbar-width:thin; }
.terminal-stage.is-fullscreen .terminal-tabs-bar { border-width:0 0 1px; border-radius:0; }
.terminal-stage :deep(.host-terminal) { border-top:0; border-radius:0 0 var(--terminal-shell-radius,12px) var(--terminal-shell-radius,12px); }
.terminal-stage.is-fullscreen :deep(.host-terminal) { border-width:0; border-radius:0; }
.terminal-tab { display:flex; flex:0 0 auto; align-items:center; gap:7px; max-width:220px; border:1px solid var(--terminal-shell-border,#29383a); border-radius:8px; padding:7px 9px; color:var(--terminal-shell-muted,#8a9695); background:var(--terminal-shell-panel,#111a1d); }
.terminal-tab.is-active { color:var(--terminal-shell-text,#d8dddc); border-color:var(--brand); }
.terminal-tab__name { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.terminal-tab__status { width:7px; height:7px; flex:0 0 auto; border-radius:999px; background:var(--terminal-shell-muted,#8a9695); box-shadow:0 0 0 2px color-mix(in srgb,currentColor 10%,transparent); }
.terminal-tab__status.is-connected { color:var(--success); background:var(--success); }
.terminal-tab__status.is-connecting { color:var(--warning); background:var(--warning); animation:terminal-pulse 1.4s ease-in-out infinite; }
.terminal-tab__status.is-reconnecting { color:var(--danger); background:var(--danger); animation:terminal-pulse 1s ease-in-out infinite; }
.terminal-tab__status.is-finished { color:var(--terminal-shell-muted,#8a9695); background:var(--terminal-shell-muted,#8a9695); }
.terminal-empty { display:grid; place-content:center; justify-items:center; padding:36px; color:var(--terminal-shell-muted,#8a9695); text-align:center; }
.terminal-empty span { display:grid; width:72px; height:72px; place-items:center; border:1px solid var(--terminal-shell-border,#29383a); border-radius:20px; color:var(--brand); background:var(--terminal-shell-panel,#111a1d); }
.terminal-empty h2 { margin:18px 0 5px; color:var(--terminal-shell-text,#d8dddc); }
.terminal-empty p { max-width:480px; margin:0; }
.spin { animation:spin .8s linear infinite; }
@keyframes spin { to { transform:rotate(360deg); } }
@keyframes terminal-pulse { 50% { opacity:.38; } }
@media (prefers-reduced-motion: reduce) { .terminal-workspace,.terminal-connections { transition:none; } }
@media (max-width: 900px) {
  .terminal-workspace,.terminal-workspace.is-connections-collapsed { height:min(760px,calc(100dvh - 110px)); min-height:560px; grid-template-columns:minmax(0,1fr); grid-template-rows:minmax(0,1fr); }
  .terminal-connections { position:absolute; z-index:22; inset:0 auto 0 0; width:min(320px,calc(100% - 48px)); border-right:1px solid var(--border); border-bottom:0; box-shadow:var(--shadow-md); transform:translateX(-105%); transition:transform .2s ease; }
  .terminal-workspace.is-connections-drawer-open .terminal-connections { transform:translateX(0); }
  .terminal-connections-overlay { position:absolute; z-index:21; inset:0; display:block; border:0; background:rgb(5 16 13 / 42%); }
  .terminal-connections__collapse { display:none; }
  .terminal-connections__mobile-close { display:grid; }
  .terminal-workspace.is-connections-drawer-open .terminal-connections>header { justify-content:space-between; padding:17px 16px 12px; }
  .terminal-workspace.is-connections-drawer-open .terminal-connections__heading { display:grid; }
  .terminal-workspace.is-connections-drawer-open .terminal-connections__shield,.terminal-workspace.is-connections-drawer-open .terminal-connections__refresh { display:grid; }
  .terminal-stage { min-height:0; padding:0; }
  .terminal-stage__mobile-selector { display:flex; min-width:0; align-items:center; justify-content:space-between; gap:10px; min-height:50px; padding:8px 12px; border-bottom:1px solid var(--terminal-shell-border,#29383a); background:var(--terminal-shell-panel,#111a1d); }
  .terminal-stage__mobile-selector button { display:flex; min-width:0; align-items:center; gap:8px; border:0; padding:7px 8px; color:var(--terminal-shell-text,#d8dddc); background:transparent; font-weight:700; cursor:pointer; }
  .terminal-stage__mobile-selector button span { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
  .terminal-stage__mobile-selector small { flex:0 0 auto; color:var(--terminal-shell-muted,#8a9695); }
  .terminal-stage.is-fullscreen .terminal-stage__mobile-selector { display:none; }
  .terminal-tabs-bar { border-right:0; border-left:0; border-radius:0; }
  .terminal-stage :deep(.host-terminal) { border-right:0; border-bottom:0; border-left:0; border-radius:0; }
}
@media (max-width: 480px) { .terminal-workspace,.terminal-workspace.is-connections-collapsed { height:calc(100dvh - 94px); min-height:520px; grid-template-rows:minmax(0,1fr); border-radius:14px; } .terminal-empty { padding:20px 14px; } .terminal-empty span { width:58px; height:58px; border-radius:16px; } .terminal-empty h2 { margin-top:14px; font-size:19px; } }
</style>
