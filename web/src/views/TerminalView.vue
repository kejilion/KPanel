<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, onMounted, ref, type ComponentPublicInstance } from 'vue'
import { Circle, LoaderCircle, Menu, PanelLeftClose, PanelLeftOpen, Plus, RefreshCw, Search, SquareTerminal, X } from '@lucide/vue'
import HostTerminal from '@/components/terminal/HostTerminal.vue'
import TerminalToolbar from '@/components/terminal/TerminalToolbar.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import OperatingSystemIcon from '@/components/overview/OperatingSystemIcon.vue'
import { useTerminalFullscreen } from '@/composables/useTerminalFullscreen'
import { api, ApiError } from '@/lib/api'
import {
  readClusterHostOrder,
  sortClusterHosts,
  subscribeClusterHostOrder,
} from '@/lib/clusterHostOrder'
import { detectOperatingSystemIdentity } from '@/lib/operatingSystem'
import type { ClusterHost, ClusterHostList } from '@/types/api'
import { usePhraseCatalog } from '@/i18n/phrase'
import { useI18n } from '@/i18n'
import { desktopCloseGuardCoordinator, desktopWindowCloseGuardKey } from '@/lib/desktopRouteKeys'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/TerminalView/en-US').then((module) => module.default)
  : import('@/i18n/pages/TerminalView/zh-TW').then((module) => module.default))
const { locale, t } = useI18n()
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
const clusterHostOrderRevision = ref(0)
const terminalRefs = new Map<string, HostTerminalHandle>()
let controller: AbortController | undefined
let initialHostLoad = true
let unsubscribeClusterHostOrder: (() => void) | undefined
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
  clusterHostOrderRevision.value
  const needle = search.value.trim().toLowerCase()
  return sortClusterHosts(inventory.value?.items || [], readClusterHostOrder())
    .filter((host) => !needle || `${host.name} ${host.origin}`.toLowerCase().includes(needle))
})

const activeSession = computed(() => sessions.value.find((item) => item.id === activeSessionId.value))

const hostOperatingSystemIdentity = (host: ClusterHost) =>
  detectOperatingSystemIdentity(host.lastSnapshot?.telemetry)

async function loadHosts(): Promise<void> {
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  errorMessage.value = ''
  try {
    inventory.value = await api.cluster.hosts(controller.signal)
    if (initialHostLoad) {
      initialHostLoad = false
      const localHost = inventory.value.items.find((host) => host.isLocal && host.terminalAvailable)
      if (localHost) await openHost(localHost)
    }
  } catch {
    errorMessage.value = t('terminal.connectionsLoadFailed')
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
      ? t('terminal.sessionLimitReached')
      : t('terminal.connectionFailed')
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
  locale.value
  if (!host.terminalAvailable) {
    return host.kind === 'light_node'
      ? t('terminal.hostState.lightNode')
      : t('terminal.hostState.repairPairing')
  }
  if (host.isLocal) return t('terminal.hostState.local')
  return t('terminal.hostState.encrypted')
}

function sessionStateLabel(state: OpenTerminal['state']): string {
  locale.value
  if (state === 'connected') return t('terminal.connected')
  if (state === 'finished') return t('terminal.finished')
  if (state === 'reconnecting') return t('terminal.reconnecting')
  return t('terminal.connecting')
}

onMounted(() => {
  unsubscribeClusterHostOrder = subscribeClusterHostOrder(() => {
    clusterHostOrderRevision.value += 1
  })
  try {
    connectionsCollapsed.value = window.localStorage.getItem(connectionsCollapsedStorageKey) === '1'
  } catch {
    connectionsCollapsed.value = false
  }
  const guard = () => {
    const activeCount = sessions.value.filter((session) => session.state !== 'finished').length
    return !activeCount || window.confirm(t('terminal.closeSessionsConfirm', { count: activeCount }))
  }
  unregisterWindowCloseGuard = desktopWindowCloseGuards
    ? desktopWindowCloseGuards.register(guard)
    : desktopCloseGuardCoordinator.register('classic-terminal', guard)
  void loadHosts()
})
onBeforeUnmount(() => {
  unsubscribeClusterHostOrder?.()
  unregisterWindowCloseGuard?.()
  controller?.abort()
})
</script>

<template>
  <div class="page terminal-page" :data-locale="locale">
    <PageHeader title="多主机终端" description="通过集群加密通道连接本机、已授权 KPanel 节点和轻量节点，无需开放额外 SSH 或公网端口。" />

    <div v-if="errorMessage" class="terminal-alert" role="alert">{{ errorMessage }}</div>

    <section
      class="terminal-workspace terminal-theme-scope"
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
          <div class="terminal-connections__heading"><strong>连接列表</strong><small>{{ t('terminal.hostCount', { count: hosts.length }) }}</small></div>
          <div class="terminal-connections__actions">
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
              <X :size="17" />
            </button>
          </div>
        </header>
        <label v-show="!connectionsCollapsed || mobileConnectionsOpen" class="terminal-search">
          <Search :size="15" aria-hidden="true" />
          <input v-model="search" type="search" placeholder="搜索主机" />
        </label>
        <div id="terminal-connection-selector" v-show="!connectionsCollapsed || mobileConnectionsOpen" class="terminal-connections__list">
          <div v-if="!loading && !hosts.length" class="terminal-connections__empty">暂无可显示主机</div>
          <button v-for="host in hosts" :key="host.id" class="terminal-host" :class="{ 'is-active': activeSession?.hostId === host.id }" type="button" :disabled="openingHostId === host.id" @click="openHost(host)">
            <OperatingSystemIcon
              class="terminal-host__os"
              :distro="hostOperatingSystemIdentity(host).key"
              :label="hostOperatingSystemIdentity(host).label"
            />
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
            <OperatingSystemIcon
              class="terminal-host-rail__os"
              :distro="hostOperatingSystemIdentity(host).key"
              :label="hostOperatingSystemIdentity(host).label"
              :show-tooltip="false"
            />
            <i :class="{ 'is-ready': host.terminalAvailable }" aria-hidden="true" />
          </button>
        </div>
      </aside>

      <main class="terminal-stage" :class="{ 'is-fullscreen': workspaceFullscreen }">
        <button
          v-if="!sessions.length"
          class="terminal-stage__mobile-selector"
          type="button"
          aria-controls="terminal-connections-drawer"
          :aria-expanded="mobileConnectionsOpen"
          aria-label="打开主机选择"
          @click="mobileConnectionsOpen = true"
        >
          <Menu :size="18" />
          <span>{{ activeSession?.hostName || t('terminal.selectHost') }}</span>
          <small>{{ sessions.length ? t('terminal.sessionCount', { count: sessions.length }) : t('terminal.hostCount', { count: hosts.length }) }}</small>
        </button>
        <div v-if="sessions.length" class="terminal-tabs-bar">
          <button
            class="terminal-tabs-bar__connections"
            type="button"
            aria-controls="terminal-connections-drawer"
            :aria-expanded="mobileConnectionsOpen"
            :title="t('terminal.expandConnections')"
            :aria-label="t('terminal.expandConnections')"
            @click="mobileConnectionsOpen = true"
          >
            <Menu :size="18" />
          </button>
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
        <div v-if="!sessions.length" class="terminal-empty"><span><SquareTerminal :size="32" /></span><h2>{{ t('terminal.emptyTitle') }}</h2><p>{{ t('terminal.emptyDescription') }}</p></div>
        <HostTerminal v-for="item in sessions" v-show="item.id === activeSessionId" :key="item.id" :ref="(instance) => setTerminalRef(item.id, instance)" :session-id="item.id" :host-name="item.hostName" :initial-offset="item.offset" @state-change="item.state = $event" />
      </main>
    </section>
  </div>
</template>

<style scoped>
.terminal-page { min-height:calc(100vh - 100px); gap:18px; }
.terminal-alert { border:1px solid color-mix(in srgb,var(--danger) 34%,var(--border)); border-radius:10px; padding:11px 13px; color:var(--danger); background:color-mix(in srgb,var(--danger) 8%,var(--surface)); }
.terminal-workspace { position:relative; display:grid; height:var(--terminal-workspace-height); min-height:var(--terminal-workspace-min-height); grid-template-columns:256px minmax(0,1fr); overflow:hidden; border:1px solid var(--terminal-shell-border,#29383a); border-radius:var(--terminal-workspace-radius); background:var(--terminal-shell-background,#0b1214); box-shadow:var(--shadow-sm); transition:grid-template-columns 180ms ease; }
:global(:root:not([data-theme='dark'])) .terminal-workspace { --terminal-shell-border:rgb(255 255 255 / 18%); }
.terminal-workspace.is-connections-collapsed { grid-template-columns:52px minmax(0,1fr); }
.terminal-connections { display:grid; min-width:0; min-height:0; grid-template-rows:auto auto minmax(0,1fr); overflow:hidden; border-right:1px solid var(--terminal-shell-border,#29383a); color:var(--terminal-shell-text,#d8dddc); background:var(--terminal-shell-panel,#111a1d); }
.terminal-connections>header { display:flex; align-items:center; justify-content:space-between; padding:15px 13px 10px; color:var(--brand); }
.terminal-connections__heading { display:grid; min-width:0; gap:2px; color:var(--terminal-shell-text,#d8dddc); }
.terminal-connections__heading strong { font-size:15px; line-height:1.2; }
.terminal-connections>header small { color:var(--terminal-shell-muted,#8a9695); font-weight: 500; }
.terminal-connections__actions { display:flex; flex:0 0 auto; align-items:center; gap:6px; }
.terminal-connections__toggle { display:grid; width:30px; height:30px; flex:0 0 auto; place-items:center; border:1px solid var(--terminal-shell-border,#29383a); border-radius:8px; color:var(--terminal-shell-muted,#8a9695); background:var(--terminal-shell-background,#0b1214); cursor:pointer; transition:border-color .16s ease,color .16s ease,background-color .16s ease; }
.terminal-connections__toggle:hover,.terminal-connections__toggle:focus-visible { border-color:color-mix(in srgb,var(--brand) 62%,var(--terminal-shell-border,#29383a)); color:var(--brand); outline:none; }
.terminal-connections__mobile-close,.terminal-connections-overlay,.terminal-stage__mobile-selector { display:none; }
.terminal-stage__mobile-selector { width:100%; min-width:0; grid-row:1; grid-column:1; align-items:center; gap:8px; min-height:48px; border:0; border-bottom:1px solid var(--terminal-shell-border,#29383a); padding:7px 18px; color:var(--terminal-shell-text,#d8dddc); background:var(--terminal-shell-panel,#111a1d); font:inherit; font-weight: 600; text-align:left; cursor:pointer; }
.terminal-stage__mobile-selector:hover,.terminal-stage__mobile-selector:focus-visible { color:var(--brand); background:color-mix(in srgb,var(--brand) 7%,var(--terminal-shell-panel,#111a1d)); outline:none; }
.terminal-stage__mobile-selector span { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.terminal-stage__mobile-selector small { flex:0 0 auto; margin-left:auto; color:var(--terminal-shell-muted,#8a9695); font-weight: 500; }
.terminal-workspace.is-connections-collapsed .terminal-connections>header { justify-content:center; padding:12px 9px; }
.terminal-workspace.is-connections-collapsed .terminal-connections__heading,.terminal-workspace.is-connections-collapsed .terminal-connections__refresh { display:none; }
.terminal-search { position:relative; display:block; padding:0 10px 8px; color:var(--terminal-shell-muted,#8a9695); }
.terminal-search>svg { position:absolute; z-index:1; top:50%; left:21px; transform:translateY(calc(-50% - 4px)); pointer-events:none; }
.terminal-search input { width:100%; border:1px solid var(--terminal-shell-border,#29383a); border-radius:9px; padding:8px 10px 8px 34px; color:var(--terminal-shell-text,#d8dddc); background:var(--terminal-shell-background,#0b1214); transition:border-color .16s ease,box-shadow .16s ease; }
.terminal-search input::placeholder { color:var(--terminal-shell-muted,#8a9695); }
.terminal-search input:focus { border-color:color-mix(in srgb,var(--brand) 62%,var(--terminal-shell-border,#29383a)); outline:none; box-shadow:0 0 0 2px color-mix(in srgb,var(--brand) 14%,transparent); }
.terminal-connections__list { --scrollbar-size:8px; --scrollbar-track:var(--terminal-shell-panel,#111a1d); --scrollbar-thumb:var(--terminal-shell-scrollbar,#35474a); --scrollbar-thumb-hover:var(--terminal-shell-scrollbar-hover,#506367); --scrollbar-thumb-active:var(--brand); min-height:0; overflow-y:auto; overscroll-behavior:contain; padding-bottom:8px; scrollbar-color:var(--scrollbar-thumb) var(--scrollbar-track); scrollbar-width:thin; scrollbar-gutter:stable; }
.terminal-connections__rail { display:grid; grid-row:2 / -1; min-height:0; align-content:start; justify-items:center; gap:6px; overflow-y:auto; overscroll-behavior:contain; padding:7px 6px 10px; scrollbar-width:none; }
.terminal-connections__rail::-webkit-scrollbar { display:none; }
.terminal-host-rail { position:relative; display:grid; width:36px; height:36px; flex:0 0 auto; place-items:center; border:1px solid transparent; border-radius:9px; color:var(--terminal-shell-muted,#8a9695); background:transparent; cursor:pointer; }
.terminal-host-rail:hover,.terminal-host-rail:focus-visible,.terminal-host-rail.is-active { border-color:color-mix(in srgb,var(--brand) 58%,var(--terminal-shell-border,#29383a)); color:var(--brand); background:color-mix(in srgb,var(--brand) 12%,var(--terminal-shell-panel,#111a1d)); outline:none; }
.terminal-host-rail:disabled { cursor:wait; opacity:.55; }
.terminal-host-rail :deep(.terminal-host-rail__os) { width:26px; height:26px; border-radius:8px; box-shadow:none; }
.terminal-host-rail :deep(.terminal-host-rail__os svg) { width:16px; height:16px; }
.terminal-host-rail :deep(.terminal-host-rail__os img) { width:19px; height:19px; }
.terminal-host-rail i { position:absolute; right:3px; bottom:3px; width:7px; height:7px; border:1px solid var(--terminal-shell-panel,#111a1d); border-radius:50%; background:var(--terminal-shell-muted,#8a9695); }
.terminal-host-rail i.is-ready { background:var(--success); }
.terminal-host { position:relative; display:grid; width:calc(100% - 12px); grid-template-columns:auto minmax(0,1fr) auto; align-items:center; gap:9px; margin:2px 6px; border:1px solid transparent; border-radius:10px; padding:9px; text-align:left; color:var(--terminal-shell-text,#d8dddc); background:transparent; cursor:pointer; transition:border-color .16s ease,background-color .16s ease; }
.terminal-host:hover,.terminal-host:focus-visible,.terminal-host.is-active { border-color:color-mix(in srgb,var(--brand) 48%,var(--terminal-shell-border,#29383a)); background:color-mix(in srgb,var(--brand) 9%,var(--terminal-shell-panel,#111a1d)); outline:none; }
.terminal-host.is-active { box-shadow:inset 3px 0 0 var(--brand); }
.terminal-host:disabled { cursor:wait; opacity:.64; }
.terminal-host :deep(.terminal-host__os) { width:34px; height:34px; flex:0 0 auto; border-radius:9px; box-shadow:none; }
.terminal-host>span:nth-child(2) { display:grid; min-width:0; gap:2px; }
.terminal-host strong,.terminal-host small { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.terminal-host small { color:var(--terminal-shell-muted,#8a9695); font-size:13px; }
.terminal-host em { display:flex; align-items:center; gap:5px; color:var(--terminal-shell-muted,#8a9695); font-size:12px; font-style:normal; }
.terminal-host em.is-ready { color:var(--success); }
.terminal-connections__empty { display:flex; align-items:center; justify-content:center; gap:8px; min-height:180px; padding:20px; color:var(--terminal-shell-muted,#8a9695); text-align:center; }
.terminal-stage { display:grid; grid-template-columns:minmax(0,1fr); grid-template-rows:auto minmax(0,1fr); min-width:0; min-height:0; overflow:hidden; padding:0; background:var(--terminal-shell-background,#0b1214); }
.terminal-stage.is-fullscreen { position:fixed; z-index:6000; inset:0; width:100vw; height:100dvh; min-height:0; grid-template-rows:auto minmax(0,1fr); padding:0; border:0; }
.terminal-tabs-bar { display:flex; min-width:0; align-items:center; gap:12px; padding:8px 10px; border:0; border-bottom:1px solid var(--terminal-shell-border,#29383a); background:var(--terminal-shell-panel,#111a1d); }
.terminal-tabs-bar__connections { display:none; width:34px; height:34px; flex:0 0 auto; place-items:center; border:1px solid var(--terminal-shell-border,#29383a); border-radius:8px; color:var(--terminal-shell-muted,#8a9695); background:transparent; cursor:pointer; }
.terminal-tabs-bar__connections:hover,.terminal-tabs-bar__connections:focus-visible { border-color:var(--brand); color:var(--terminal-shell-text,#d8dddc); outline:none; }
.terminal-tabs { display:flex; min-width:0; flex:1; gap:5px; overflow-x:auto; scrollbar-width:thin; }
.terminal-stage :deep(.host-terminal) { border:0; border-radius:0; box-shadow:none; }
.terminal-tab { display:flex; flex:0 0 auto; align-items:center; gap:7px; max-width:220px; border:1px solid var(--terminal-shell-border,#29383a); border-radius:8px; padding:7px 9px; color:var(--terminal-shell-muted,#8a9695); background:var(--terminal-shell-panel,#111a1d); }
.terminal-tab.is-active { color:var(--terminal-shell-text,#d8dddc); border-color:var(--brand); }
.terminal-tab__name { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.terminal-tab__status { width:7px; height:7px; flex:0 0 auto; border-radius:999px; background:var(--terminal-shell-muted,#8a9695); box-shadow:0 0 0 2px color-mix(in srgb,currentColor 10%,transparent); }
.terminal-tab__status.is-connected { color:var(--success); background:var(--success); }
.terminal-tab__status.is-connecting { color:var(--warning); background:var(--warning); animation:terminal-pulse 1.4s ease-in-out infinite; }
.terminal-tab__status.is-reconnecting { color:var(--danger); background:var(--danger); animation:terminal-pulse 1s ease-in-out infinite; }
.terminal-tab__status.is-finished { color:var(--terminal-shell-muted,#8a9695); background:var(--terminal-shell-muted,#8a9695); }
.terminal-empty { display:grid; grid-row:2; grid-column:1; place-content:center; justify-items:center; padding:32px; color:var(--terminal-shell-muted,#8a9695); text-align:center; }
.terminal-empty span { display:grid; width:58px; height:58px; place-items:center; border-radius:18px; color:var(--brand); background:color-mix(in srgb,var(--brand) 10%,var(--terminal-shell-panel,#111a1d)); }
.terminal-empty h2 { margin:16px 0 6px; color:var(--terminal-shell-text,#d8dddc); font-size:22px; line-height:1.25; }
.terminal-empty p { max-width:420px; margin:0; line-height:1.65; }
.spin { animation:spin .8s linear infinite; }
@keyframes spin { to { transform:rotate(360deg); } }
@keyframes terminal-pulse { 50% { opacity:.38; } }
@media (prefers-reduced-motion: reduce) { .terminal-workspace,.terminal-connections { transition:none; } }
@media (max-width: 900px) {
  .terminal-workspace,.terminal-workspace.is-connections-collapsed { height:min(760px,calc(100dvh - 110px)); min-height:560px; grid-template-columns:minmax(0,1fr); grid-template-rows:minmax(0,1fr); }
  .terminal-connections { position:absolute; z-index:22; inset:0 auto 0 0; width:min(320px,calc(100% - 48px)); border-right:1px solid var(--terminal-shell-border,#29383a); border-bottom:0; box-shadow:var(--shadow-md); transform:translateX(-105%); transition:transform .2s ease; }
  .terminal-workspace.is-connections-drawer-open .terminal-connections { transform:translateX(0); }
  .terminal-connections-overlay { position:absolute; z-index:21; inset:0; display:block; border:0; background:rgb(5 16 13 / 42%); }
  .terminal-connections__collapse { display:none; }
  .terminal-connections__mobile-close { display:grid; }
  .terminal-workspace.is-connections-drawer-open .terminal-connections>header { min-height:42px; justify-content:flex-end; padding:6px 8px 0; }
  .terminal-workspace.is-connections-drawer-open .terminal-connections__heading,.terminal-workspace.is-connections-drawer-open .terminal-connections__refresh { display:none; }
  .terminal-stage { min-height:0; grid-template-rows:auto minmax(0,1fr); padding:0; }
  .terminal-stage__mobile-selector { display:flex; }
  .terminal-tabs-bar__connections { display:grid; }
  .terminal-stage.is-fullscreen .terminal-stage__mobile-selector { display:none; }
  .terminal-stage.is-fullscreen .terminal-tabs-bar__connections { display:none; }
  .terminal-tabs-bar { border-right:0; border-left:0; border-radius:0; }
  .terminal-stage :deep(.host-terminal) { border-right:0; border-bottom:0; border-left:0; border-radius:0; }
}
@media (max-width: 480px) { .terminal-workspace,.terminal-workspace.is-connections-collapsed { height:calc(100dvh - 94px); min-height:520px; grid-template-rows:minmax(0,1fr); border-radius:14px; } .terminal-empty { padding:20px 14px; } .terminal-empty span { width:58px; height:58px; border-radius:16px; } .terminal-empty h2 { margin-top:14px; font-size:19px; } }
</style>
