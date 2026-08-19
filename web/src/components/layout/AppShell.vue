<script setup lang="ts">
import type { Component } from 'vue'
import {
  computed,
  defineAsyncComponent,
  defineComponent,
  h,
  onBeforeUnmount,
  onMounted,
  provide,
  ref,
  watch,
} from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Boxes,
  Bot,
  HeartPulse,
  ClipboardList,
  CircleArrowUp,
  Container,
  Folder,
  LayoutDashboard,
  LogOut,
  LoaderCircle,
  Menu,
  Monitor,
  Moon,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  Settings,
  Store,
  Sun,
  SquareTerminal,
  X,
} from '@lucide/vue'
import AgentBanner from '@/components/layout/AgentBanner.vue'
import LanguageSelector from '@/components/common/LanguageSelector.vue'
import LogoMark from '@/components/common/LogoMark.vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import { useSession } from '@/stores/session'
import { usePanelState } from '@/stores/panel'
import { useTheme } from '@/stores/theme'
import { useToast } from '@/stores/toast'
import { useDesktopMode } from '@/stores/desktopMode'
import { api } from '@/lib/api'
import {
  prefetchNavigationRoute,
  routeNavigationState,
} from '@/lib/navigation'
import { readSidebarCollapsed, writeSidebarCollapsed } from '@/lib/sidebarPreference'
import { detectKPanelUpdate, kpanelUpdateHint } from '@/lib/kpanelUpdate'
import { useI18n } from '@/i18n'
import type { MessageKey } from '@/i18n/messages/zh-CN'
import {
  desktopBrowserHistoryKey,
  desktopCloseGuardCoordinator,
} from '@/lib/desktopRouteKeys'
import { createDesktopBrowserHistory } from '@/lib/desktopBrowserHistory'

interface NavigationItem {
  labelKey: MessageKey
  to: string
  icon: Component
}

const navigation: NavigationItem[] = [
  { labelKey: 'route.overview', to: '/overview', icon: LayoutDashboard },
  { labelKey: 'route.ai', to: '/ai', icon: Bot },
  { labelKey: 'route.sites', to: '/sites', icon: Boxes },
  { labelKey: 'route.apps', to: '/apps', icon: Store },
  { labelKey: 'route.docker', to: '/docker', icon: Container },
  { labelKey: 'route.files', to: '/files', icon: Folder },
  { labelKey: 'route.terminal', to: '/terminal', icon: SquareTerminal },
  { labelKey: 'route.diagnostics', to: '/diagnostics', icon: HeartPulse },
  { labelKey: 'route.cluster', to: '/cluster', icon: Network },
  { labelKey: 'route.activity', to: '/activity', icon: ClipboardList },
  { labelKey: 'route.settings', to: '/settings', icon: Settings },
]

const route = useRoute()
const router = useRouter()
const desktopBrowserHistory = createDesktopBrowserHistory(router)
provide(desktopBrowserHistoryKey, desktopBrowserHistory)
const session = useSession()
const panel = usePanelState()
const theme = useTheme()
const toast = useToast()
const desktop = useDesktopMode()
const i18n = useI18n()
// Desktop mode is an opt-in overlay; load it lazily so the main bundle stays
// lean for the default classic experience. If the split chunk cannot load,
// atomically restore the classic shell instead of leaving an inert blank app.
const DesktopLoadingView = defineComponent({
  name: 'DesktopLoadingView',
  setup() {
    return () => h(
      'div',
      {
        class: 'desktop',
        role: 'status',
        'aria-label': i18n.t('common.loading'),
      },
      [h('div', { class: 'desktop__wallpaper', 'aria-hidden': 'true' })],
    )
  },
})
const DesktopView = defineAsyncComponent({
  loader: () => import('@/components/desktop/DesktopView.vue'),
  loadingComponent: DesktopLoadingView,
  delay: 0,
  onError(_error, retry, fail, attempts) {
    if (attempts < 2) {
      retry()
      return
    }
    desktop.enterClassic()
    toast.danger(i18n.t('nav.loadFailedTitle'), i18n.t('nav.loadFailedMessage'))
    fail()
  },
})
const desktopActive = computed(() => desktop.mode.value === 'desktop')
const DESKTOP_ENTRY_NOTICE_KEY = 'kpanel:desktop-entry-notice:v2'

function readDesktopEntrySeen(): boolean {
  try {
    return typeof window !== 'undefined' && window.localStorage.getItem(DESKTOP_ENTRY_NOTICE_KEY) === 'seen'
  } catch {
    return false
  }
}

function markDesktopEntrySeen(): void {
  desktopEntrySeen.value = true
  try {
    window.localStorage.setItem(DESKTOP_ENTRY_NOTICE_KEY, 'seen')
  } catch {
    // Storage can be unavailable without blocking mode switching.
  }
}

const desktopEntrySeen = ref(readDesktopEntrySeen())
const menuOpen = ref(false)
const signingOut = ref(false)
const sidebarCollapsed = ref(readSidebarCollapsed())
const kpanelUpdateAvailable = ref(false)
const checkingKPanelUpdate = ref(false)
const kpanelUpdateDescription = computed(() => kpanelUpdateHint(panel.state.agent?.version))

const pageTitle = computed(() => route.meta.titleKey ? i18n.t(route.meta.titleKey) : 'KPanel')
const isAIWorkspace = computed(() => route.path.startsWith('/ai'))
const agentStatus = computed(() => {
  const agent = panel.state.agent
  if (!agent?.connected) return { status: 'offline', label: i18n.t('agent.offline') }
  if (!agent.compatible) return { status: 'incompatible', label: i18n.t('agent.incompatible') }
  if (agent.readOnly) return { status: 'read_only', label: i18n.t('agent.readOnly') }
  return { status: 'connected', label: i18n.t('agent.online') }
})
let agentTimer: number | undefined
let navigationWarmupTimer: number | undefined
let navigationWarmupCancelled = false
let lastKPanelUpdateCheckAt = 0
const kpanelUpdateCheckInterval = 60 * 60 * 1000

function closeMenu(): void {
  menuOpen.value = false
}

function toggleSidebar(): void {
  sidebarCollapsed.value = !sidebarCollapsed.value
  writeSidebarCollapsed(sidebarCollapsed.value)
}

function navigationItemPending(path: string): boolean {
  if (!routeNavigationState.pending) return false
  return routeNavigationState.targetPath === path || routeNavigationState.targetPath.startsWith(`${path}/`)
}

function prefetchNavigation(path: string): void {
  void prefetchNavigationRoute(path)
}

async function warmNavigation(): Promise<void> {
  const connection = (navigator as Navigator & { connection?: { saveData?: boolean } }).connection
  if (connection?.saveData) return

  for (const item of navigation) {
    if (navigationWarmupCancelled) return
    if (route.path === item.to || route.path.startsWith(`${item.to}/`)) continue
    await prefetchNavigationRoute(item.to)
    await new Promise<void>((resolve) => window.setTimeout(resolve, 120))
  }
}

function toggleTheme(): void {
  theme.setTheme(theme.resolved.value === 'dark' ? 'light' : 'dark')
}

async function enterDesktopSafely(): Promise<void> {
  if (!(await desktopCloseGuardCoordinator.checkAll())) return
  markDesktopEntrySeen()
  desktop.enterDesktop()
}

function openKPanelUpdate(): void {
  if (!kpanelUpdateAvailable.value) return
  closeMenu()
  void router.push({
    name: 'apps',
    query: { app: 'kpanel', action: 'update' },
  })
}

async function refreshKPanelUpdate(): Promise<void> {
  const agent = panel.state.agent
  if (
    checkingKPanelUpdate.value ||
    !agent?.connected ||
    !agent.compatible ||
    agent.readOnly ||
    Date.now() - lastKPanelUpdateCheckAt < kpanelUpdateCheckInterval
  ) {
    return
  }
  checkingKPanelUpdate.value = true
  lastKPanelUpdateCheckAt = Date.now()
  try {
    kpanelUpdateAvailable.value = (await detectKPanelUpdate()) === 'available'
  } catch {
    // Registry or Agent failures must not interrupt normal navigation.
  } finally {
    checkingKPanelUpdate.value = false
  }
}

async function signOut(): Promise<void> {
  signingOut.value = true
  try {
    await session.logout()
    await router.replace('/login')
  } catch {
    toast.danger(i18n.t('nav.signOutFailed'), i18n.t('nav.signOutRetry'))
  } finally {
    signingOut.value = false
  }
}

async function refreshAgent(): Promise<void> {
  try {
    const status = await api.agent.health()
    panel.setAgent(status)
    session.state.agent = status
    void refreshKPanelUpdate()
  } catch (error) {
    const previous = panel.state.agent
    panel.setAgent({
      connected: false,
      compatible: previous?.compatible ?? true,
      readOnly: true,
      version: previous?.version,
      protocolVersion: previous?.protocolVersion,
      lastSeenAt: previous?.lastSeenAt,
      reason: i18n.t('agent.unreachable'),
    })
  }
}

onMounted(() => {
  void refreshAgent()
  agentTimer = window.setInterval(refreshAgent, 30_000)
  navigationWarmupTimer = window.setTimeout(() => {
    void warmNavigation()
  }, 2_000)
})

onBeforeUnmount(() => {
  desktopBrowserHistory.dispose()
  if (agentTimer) window.clearInterval(agentTimer)
  navigationWarmupCancelled = true
  if (navigationWarmupTimer) window.clearTimeout(navigationWarmupTimer)
})

watch(
  () => routeNavigationState.failureSequence,
  (current, previous) => {
    if (current === previous) return
    toast.danger(i18n.t('nav.loadFailedTitle'), i18n.t('nav.loadFailedMessage'))
  },
)
</script>

<template>
  <div class="app-shell">
    <Transition name="fade">
      <button v-if="menuOpen && !desktopActive" class="mobile-overlay" type="button" :aria-label="i18n.t('nav.close')" @click="closeMenu" />
    </Transition>
    <aside
      v-show="!desktopActive"
      class="sidebar"
      :class="{
        'sidebar--open': menuOpen,
        'sidebar--collapsed': sidebarCollapsed,
      }"
      :inert="desktopActive ? true : undefined"
      :aria-hidden="desktopActive ? 'true' : undefined"
    >
      <div class="sidebar__brand">
        <LogoMark />
        <button
          class="icon-button sidebar__collapse"
          type="button"
          :aria-label="sidebarCollapsed ? i18n.t('nav.expand') : i18n.t('nav.collapse')"
          :aria-expanded="!sidebarCollapsed"
          :title="sidebarCollapsed ? i18n.t('nav.expand') : i18n.t('nav.collapse')"
          @click="toggleSidebar"
        >
          <PanelLeftOpen v-if="sidebarCollapsed" :size="17" />
          <PanelLeftClose v-else :size="17" />
        </button>
        <button class="icon-button sidebar__close" type="button" :aria-label="i18n.t('nav.close')" @click="closeMenu">
          <X :size="19" />
        </button>
      </div>

      <nav class="sidebar__nav" :aria-label="i18n.t('nav.main')">
        <RouterLink
          v-for="item in navigation"
          :key="item.to"
          :to="item.to"
          class="sidebar__link"
          :class="{
            'sidebar__link--pending': navigationItemPending(item.to),
            'router-link-active': route.path === item.to || route.path.startsWith(`${item.to}/`),
          }"
          :aria-label="i18n.t(item.labelKey)"
          :title="sidebarCollapsed ? i18n.t(item.labelKey) : undefined"
          @pointerenter="prefetchNavigation(item.to)"
          @focus="prefetchNavigation(item.to)"
          @touchstart.passive="prefetchNavigation(item.to)"
          @click="closeMenu"
        >
          <component :is="item.icon" :size="18" :stroke-width="1.9" aria-hidden="true" />
          <span>{{ i18n.t(item.labelKey) }}</span>
          <LoaderCircle
            v-if="navigationItemPending(item.to)"
            class="sidebar__link-loader"
            :size="15"
            :aria-label="i18n.t('common.loading')"
          />
        </RouterLink>
      </nav>

      <div class="sidebar__footer">
        <div class="sidebar__agent" :title="sidebarCollapsed ? agentStatus.label : undefined">
          <StatusBadge :status="agentStatus.status" :label="agentStatus.label" subtle />
          <button
            v-if="kpanelUpdateAvailable"
            class="sidebar__version sidebar__version--update"
            type="button"
            :aria-label="kpanelUpdateDescription"
            :title="kpanelUpdateDescription"
            @click="openKPanelUpdate"
          >
            <CircleArrowUp :size="16" aria-hidden="true" />
            <span>{{ i18n.t('nav.updateAvailable') }}</span>
          </button>
          <small v-else-if="panel.state.agent?.version">v{{ panel.state.agent.version }}</small>
        </div>
        <button
          class="sidebar__user"
          type="button"
          :disabled="signingOut"
          :title="sidebarCollapsed ? i18n.t('nav.signOut') : undefined"
          @click="signOut"
        >
          <span class="avatar">{{ session.state.user?.username?.slice(0, 1).toUpperCase() || 'A' }}</span>
          <span>
            <strong>{{ session.state.user?.displayName || session.state.user?.username || i18n.t('common.admin') }}</strong>
            <small>{{ i18n.t('nav.signOut') }}</small>
          </span>
          <LogOut :size="16" aria-hidden="true" />
        </button>
      </div>
    </aside>

    <div
      v-show="!desktopActive"
      class="app-shell__main"
      :class="{ 'app-shell__main--sidebar-collapsed': sidebarCollapsed }"
      :inert="desktopActive ? true : undefined"
      :aria-hidden="desktopActive ? 'true' : undefined"
    >
      <Transition name="fade">
        <div
          v-if="routeNavigationState.pending"
          class="route-loading-progress"
          role="progressbar"
          :aria-label="i18n.t('nav.pageLoading')"
        />
      </Transition>
      <header class="topbar">
        <div class="topbar__title">
          <button class="icon-button topbar__menu" type="button" :aria-label="i18n.t('nav.open')" @click="menuOpen = true">
            <Menu :size="20" />
          </button>
          <div>
            <span>{{ i18n.t('common.console') }}</span>
            <strong>{{ pageTitle }}</strong>
          </div>
        </div>
        <div class="topbar__actions">
          <StatusBadge :status="agentStatus.status" :label="agentStatus.label" subtle />
          <LanguageSelector compact />
          <button
            class="icon-button desktop-entry-button"
            :class="{ 'desktop-entry-button--unseen': !desktopEntrySeen }"
            type="button"
            :aria-label="i18n.t('desktop.enterDesktop')"
            :title="i18n.t('desktop.enterDesktop')"
            @click="enterDesktopSafely"
          >
            <span class="desktop-entry-button__icon">
              <Monitor :size="18" />
              <i v-if="!desktopEntrySeen" class="desktop-entry-button__notice" aria-hidden="true" />
            </span>
            <span class="desktop-entry-button__label">{{ i18n.t('desktop.enterDesktop') }}</span>
          </button>
          <button class="icon-button" type="button" :aria-label="i18n.t('nav.themeToggle')" @click="toggleTheme">
            <Sun v-if="theme.resolved.value === 'dark'" :size="18" />
            <Moon v-else :size="18" />
          </button>
        </div>
      </header>

      <AgentBanner v-if="!isAIWorkspace" />

      <main class="page-content" :class="{ 'page-content--ai': isAIWorkspace }">
        <RouterView v-if="!desktopActive" />
      </main>
    </div>

    <DesktopView
      v-if="desktop.mode.value === 'desktop'"
      :agent="panel.state.agent"
      :kpanel-update-available="kpanelUpdateAvailable"
      :kpanel-update-description="kpanelUpdateDescription"
    />
  </div>
</template>
