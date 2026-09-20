<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, provide, ref, shallowRef, watch } from 'vue'
import type { Component } from 'vue'
import { Copy, LoaderCircle, Maximize2, Minus, RotateCw, TriangleAlert, X } from '@lucide/vue'
import {
  createWindowRouter,
  reactiveRouteFor,
  resolveWindowComponent,
  synchronizeWindowRoute,
} from '@/lib/desktopWindowRoute'
import {
  desktopBrowserHistoryKey,
  desktopWindowActiveKey,
  desktopCloseGuardCoordinatorKey,
  desktopWindowCloseGuardKey,
  windowRouteKey,
  windowRouterKey,
} from '@/lib/desktopRouteKeys'
import { DEFAULT_WINDOW_GRADIENT, desktopRoutePath, findDesktopApp } from '@/lib/desktopApps'
import { useDesktopMode } from '@/stores/desktopMode'
import { useToast } from '@/stores/toast'
import { useI18n } from '@/i18n'
import { useWindowGesture } from '@/composables/useWindowGesture'
import {
  detectWindowSnapTarget,
  geometryForWindowSnap,
  type WindowGeometry,
  type WindowSnapTarget,
} from '@/lib/desktopWindowGeometry'
import type { WindowGestureEnd, WindowGestureKind } from '@/composables/useWindowGesture'
import type { DesktopWindowState } from '@/stores/desktopMode'
import type { DesktopBrowserHistoryPoint } from '@/lib/desktopBrowserHistory'

/**
 * One desktop window. The window renders the page component for its route
 * directly (no nested RouterView) and provides a window-scoped router + route,
 * so views that call `useRoute()` / `useRouter()` / `RouterLink` resolve against
 * the window. `AiView` can navigate `/ai/s/:id` inside the window without
 * leaving the desktop, and multiple windows of the same app stay independent.
 */

const props = defineProps<{
  windowState: DesktopWindowState
  icon: Component
  iconUrl?: string
  title?: string
}>()

const desktop = useDesktopMode()
const i18n = useI18n()
const toast = useToast()
const browserHistory = inject(desktopBrowserHistoryKey, undefined)
const router = createWindowRouter(
  props.windowState.path,
  undefined,
  handoffDesktopRoute,
  browserHistory?.go,
)
// Vue component definitions must stay raw. A deep ref proxies object-based
// SFCs and triggers a runtime warning every time a desktop window opens.
const component = shallowRef<Component>()
const open = ref(false)
const closing = ref(false)
const loading = ref(true)
const loadError = ref(false)
const checkingClose = ref(false)
const iconFailed = ref(false)
const snapTarget = ref<WindowSnapTarget | null>(null)
let closeTimer: number | undefined
let openFrame: number | undefined
let loadSequence = 0
let loadedComponentPath = ''

function motionDuration(duration: number): number {
  return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ? 0 : duration
}

// Provide the window router + route to this window's subtree so views resolve
// navigation against the window instead of the app.
provide(windowRouterKey, router)
provide(windowRouteKey, reactiveRouteFor(router))

const isFocused = computed(() => desktop.focusedId.value === props.windowState.id)
const isActive = computed(() => isFocused.value && !props.windowState.minimized && !closing.value)
provide(desktopWindowActiveKey, isActive)
const closeGuards = new Set<() => boolean | Promise<boolean>>()
const windowElement = ref<HTMLElement>()
const focusReturnTarget = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
const closeGuardCoordinator = inject(desktopCloseGuardCoordinatorKey, undefined)
provide(desktopWindowCloseGuardKey, {
  register(guard) {
    closeGuards.add(guard)
    const unregisterGlobal = closeGuardCoordinator?.register(props.windowState.id, guard)
    return () => {
      closeGuards.delete(guard)
      unregisterGlobal?.()
    }
  },
})

const title = computed(() => props.title || i18n.t(props.windowState.titleKey as Parameters<typeof i18n.t>[0]))
watch(
  () => props.iconUrl,
  () => { iconFailed.value = false },
)

function componentPath(path: string): string {
  return path.startsWith('/ai/s/') ? '/ai' : path
}

function titleKeyForPath(path: string): string {
  path = desktopRoutePath(path)
  if (path === '/monitoring') return 'route.monitoring'
  if (path === '/processes') return 'route.processes'
  if (path === '/system') return 'route.systemCenter'
  if (path === '/sites/environment') return 'route.environment'
  return findDesktopApp(path)?.labelKey ?? props.windowState.titleKey
}

function browserHistoryPoint(fullPath = router.currentRoute.value.fullPath): DesktopBrowserHistoryPoint {
  const depth = router.options.history.state.monitoringZoomDepth
  return {
    windowId: props.windowState.id,
    fullPath,
    ...(typeof depth === 'number' && Number.isSafeInteger(depth) && depth >= 0 && depth <= 32
      ? { monitoringZoomDepth: depth }
      : {}),
  }
}

let lastBrowserHistoryPoint = browserHistoryPoint(props.windowState.path)

function handoffDesktopRoute(fullPath: string): boolean {
  // System Center links inside Overview stay in the current window. Other
  // registered desktop applications receive their own window.
  if (desktopRoutePath(fullPath) === '/system') return false
  const app = findDesktopApp(fullPath)
  if (!app) return false
  const from = browserHistoryPoint()
  const windowId = desktop.openWindow(fullPath, titleKeyForPath(fullPath), app.allowMultiple, true)
  if (windowId === 0) {
    toast.show(i18n.t('desktop.windowLimitTitle'), {
      message: i18n.t('desktop.windowLimitMessage'),
    })
  }
  if (windowId !== 0) {
    void browserHistory?.navigate(from, { windowId, fullPath })
  }
  return true
}

const windowGradient = computed(() => {
  const gradient = findDesktopApp(props.windowState.path)?.gradient ?? DEFAULT_WINDOW_GRADIENT
  return `linear-gradient(145deg, ${gradient[0]} 0%, ${gradient[1]} 100%)`
})

const windowStyle = computed(() => {
  if (props.windowState.maximized) {
    return {
      left: '10px',
      top: '10px',
      width: 'calc(100vw - 20px)',
      height: 'calc(100dvh - 82px)',
      zIndex: props.windowState.z,
    }
  }
  if (props.windowState.snap) {
    const isLeft = props.windowState.snap === 'left'
    return {
      left: isLeft ? '10px' : 'var(--desktop-side-split-right-left, calc(50vw + 5px))',
      top: '10px',
      width: isLeft
        ? 'var(--desktop-side-split-left-width, calc(50vw - 15px))'
        : 'var(--desktop-side-split-right-width, calc(50vw - 15px))',
      height: 'calc(100dvh - 82px)',
      zIndex: props.windowState.z,
    }
  }
  return {
    left: `${props.windowState.geometry.left}px`,
    top: `${props.windowState.geometry.top}px`,
    width: `${props.windowState.geometry.width}px`,
    height: `${props.windowState.geometry.height}px`,
    zIndex: props.windowState.z,
  }
})

const snapPreviewStyle = computed(() => {
  if (!snapTarget.value) return undefined
  const geometry = geometryForWindowSnap(snapTarget.value, {
    width: window.innerWidth,
    height: window.innerHeight,
  }, desktop.sideSplitRatio.value)
  return {
    left: `${geometry.left}px`,
    top: `${geometry.top}px`,
    width: `${geometry.width}px`,
    height: `${geometry.height}px`,
  }
})

function isMaximized(): boolean {
  return props.windowState.maximized
}

function onMinimize(): void {
  desktop.minimizeWindow(props.windowState.id)
  window.requestAnimationFrame(() => {
    document.querySelector<HTMLElement>(`[data-window-id="${props.windowState.id}"]`)?.focus({ preventScroll: true })
  })
}

function onToggleMaximize(): void {
  if (isCompactLayout()) return
  desktop.toggleMaximize(props.windowState.id)
}

function isCompactLayout(): boolean {
  return window.innerWidth <= 760
    || (window.matchMedia?.('(hover: none) and (pointer: coarse)').matches ?? false)
}

async function onClose(): Promise<void> {
  if (closing.value || checkingClose.value) return
  checkingClose.value = true
  try {
    for (const guard of closeGuards) {
      if (!(await guard())) return
    }
    closing.value = true
    closeTimer = window.setTimeout(() => {
      const isLastWindow = desktop.windows.value.length === 1
      desktop.closeWindow(props.windowState.id)
      if (isLastWindow) {
        window.requestAnimationFrame(() => {
          if (focusReturnTarget?.isConnected) focusReturnTarget.focus({ preventScroll: true })
        })
      }
    }, motionDuration(220))
  } finally {
    checkingClose.value = false
  }
}

defineExpose({
  requestClose: onClose,
})

function onPointerDownWindow(): void {
  desktop.focusWindow(props.windowState.id)
}

function onWindowKeyDown(event: KeyboardEvent): void {
  if (event.altKey && event.key === 'F4') {
    event.preventDefault()
    void onClose()
  }
}

// Load the active in-window route lazily. A sequence guard prevents a slower
// earlier import from replacing the component after quick internal navigation.
async function loadComponent(path: string): Promise<void> {
  const nextComponentPath = componentPath(path)
  if (component.value && loadedComponentPath === nextComponentPath) return
  const sequence = ++loadSequence
  component.value = undefined
  loading.value = true
  loadError.value = false
  try {
    const resolved = await resolveWindowComponent(path)
    if (sequence !== loadSequence) return
    component.value = resolved
    loadedComponentPath = nextComponentPath
  } catch {
    if (sequence !== loadSequence) return
    loadError.value = true
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

const stopRouteWatch = watch(
  () => router.currentRoute.value.fullPath,
  (fullPath, previous) => {
    const path = router.currentRoute.value.path
    if (path === '/' || fullPath === previous) return
    const nextPoint = browserHistoryPoint(fullPath)
    desktop.updateWindowRoute(props.windowState.id, fullPath, titleKeyForPath(path))
    void loadComponent(path)
    if (previous && previous !== '/' && !synchronizingFromBrowserHistory) {
      void browserHistory?.navigate(
        { ...lastBrowserHistoryPoint, fullPath: previous },
        nextPoint,
      )
    }
    lastBrowserHistoryPoint = nextPoint
  },
)

let synchronizingFromBrowserHistory = false
let browserHistorySyncSequence = 0
const stopWindowPathWatch = watch(
  () => props.windowState.path,
  async (fullPath) => {
    if (fullPath === router.currentRoute.value.fullPath) return
    synchronizingFromBrowserHistory = true
    try {
      await synchronizeWindowRoute(router, fullPath)
      lastBrowserHistoryPoint = browserHistoryPoint(fullPath)
    } finally {
      synchronizingFromBrowserHistory = false
    }
  },
)

const stopBrowserHistory = browserHistory?.subscribe((point) => {
  if (closing.value || point.windowId !== props.windowState.id) return
  desktop.restoreWindow(props.windowState.id)
  const sequence = ++browserHistorySyncSequence
  synchronizingFromBrowserHistory = true
  void synchronizeWindowRoute(
    router,
    point.fullPath,
    typeof point.monitoringZoomDepth === 'number'
      ? { monitoringZoomDepth: point.monitoringZoomDepth }
      : {},
  )
    .catch(() => undefined)
    .finally(() => {
      if (sequence !== browserHistorySyncSequence) return
      lastBrowserHistoryPoint = point
      synchronizingFromBrowserHistory = false
    })
})

const stopActiveWatch = watch(
  isActive,
  (active) => {
    if (!active) return
    void nextTick(() => {
      const element = windowElement.value
      if (element && !element.contains(document.activeElement)) element.focus({ preventScroll: true })
    })
  },
  { immediate: true, flush: 'post' },
)

// Drag + resize gesture updates geometry through the store.
const gesture = useWindowGesture(
  () => props.windowState.geometry,
  (geometry) => desktop.updateGeometry(props.windowState.id, geometry, false),
  {
    onStart: () => {
      snapTarget.value = null
      desktop.focusWindow(props.windowState.id)
    },
    onMoveActivate: restoreLayoutForDrag,
    onMove: updateSnapPreview,
    onEnd: finishWindowGesture,
  },
)

function restoreLayoutForDrag(event: PointerEvent): WindowGeometry | undefined {
  if (!props.windowState.maximized && !props.windowState.snap) return undefined
  const rect = windowElement.value?.getBoundingClientRect()
  const geometry = props.windowState.geometry
  const anchorX = rect?.width
    ? Math.min(Math.max((event.clientX - rect.left) / rect.width, .14), .86)
    : .5
  const titlebarOffset = rect
    ? Math.min(Math.max(event.clientY - rect.top, 8), 34)
    : 21
  return desktop.restoreWindowForDrag(props.windowState.id, {
    ...geometry,
    left: event.clientX - geometry.width * anchorX,
    top: event.clientY - titlebarOffset,
  })
}

function updateSnapPreview(kind: WindowGestureKind, event: PointerEvent): void {
  snapTarget.value = kind === 'move'
    ? detectWindowSnapTarget(
        { x: event.clientX, y: event.clientY },
        { width: window.innerWidth, height: window.innerHeight },
      )
    : null
}

function finishWindowGesture(result: WindowGestureEnd): void {
  const target = result.kind === 'move' && result.event
    ? detectWindowSnapTarget(
        { x: result.event.clientX, y: result.event.clientY },
        { width: window.innerWidth, height: window.innerHeight },
      )
    : snapTarget.value
  snapTarget.value = null
  if (result.cancelled) {
    if (result.moved) desktop.commitGeometry(props.windowState.id)
    return
  }
  if (result.kind === 'move' && result.moved && target) {
    desktop.snapWindow(props.windowState.id, target)
    return
  }
  if (result.moved) desktop.commitGeometry(props.windowState.id)
}

function onTitlebarPointerDown(event: PointerEvent): void {
  if (isCompactLayout()) return
  const target = event.target as HTMLElement | null
  if (target?.closest('button,[data-no-drag]')) return
  gesture.onPointerDown(event, null)
}

function retryLoad(): void {
  void loadComponent(router.currentRoute.value.path)
}

onBeforeUnmount(() => {
  loadSequence += 1
  browserHistorySyncSequence += 1
  stopRouteWatch()
  stopWindowPathWatch()
  stopBrowserHistory?.()
  stopActiveWatch()
  if (closeTimer !== undefined) window.clearTimeout(closeTimer)
  if (openFrame !== undefined) window.cancelAnimationFrame(openFrame)
})

// Open animation: first frame registers the open state.
openFrame = requestAnimationFrame(() => {
  openFrame = undefined
  open.value = true
})

const handleEdges = ['n', 's', 'e', 'w', 'ne', 'nw', 'se', 'sw'] as const
</script>

<template>
  <section
    ref="windowElement"
    :id="`desktop-window-${windowState.id}`"
    class="desktop-window"
    :class="{
      'desktop-window--focused': isFocused,
      'desktop-window--maximized': isMaximized(),
      'desktop-window--snapped': Boolean(windowState.snap),
      'desktop-window--minimized': windowState.minimized,
      'desktop-window--open': open,
      'desktop-window--closing': closing,
      'desktop-window--gesturing': gesture.active.value,
    }"
    :style="windowStyle"
    role="dialog"
    tabindex="-1"
    :aria-label="title"
    :aria-hidden="windowState.minimized || closing"
    :aria-busy="loading"
    :inert="windowState.minimized || closing || undefined"
    @pointerdown="onPointerDownWindow"
    @keydown="onWindowKeyDown"
    @contextmenu.stop
  >
    <header
      class="desktop-window__titlebar"
      @pointerdown="onTitlebarPointerDown"
      @dblclick="onToggleMaximize"
    >
      <div class="desktop-window__actions" @pointerdown.stop @dblclick.stop>
        <button
          class="desktop-window__action desktop-window__action--minimize"
          type="button"
          :aria-label="i18n.t('desktop.minimize')"
          @click="onMinimize"
        >
          <Minus :size="15" aria-hidden="true" />
        </button>
        <button
          class="desktop-window__action desktop-window__action--maximize"
          type="button"
          :aria-label="i18n.t(windowState.maximized ? 'desktop.restore' : 'desktop.maximize')"
          @click="onToggleMaximize"
        >
          <Copy v-if="isMaximized()" :size="12" aria-hidden="true" />
          <Maximize2 v-else :size="12" aria-hidden="true" />
        </button>
        <button
          class="desktop-window__action desktop-window__action--close"
          type="button"
          :disabled="checkingClose"
          :aria-label="i18n.t('desktop.close')"
          @click="onClose"
        >
          <X :size="15" aria-hidden="true" />
        </button>
      </div>
      <div class="desktop-window__title">
        <span
          class="desktop-window__app-glyph"
          :class="{ 'desktop-window__app-glyph--image': iconUrl && !iconFailed }"
          :style="iconUrl && !iconFailed ? undefined : { background: windowGradient }"
        >
          <img
            v-if="iconUrl && !iconFailed"
            :src="iconUrl"
            alt=""
            draggable="false"
            @error="iconFailed = true"
          >
          <component v-else :is="icon" :size="13" :stroke-width="2.2" aria-hidden="true" />
        </span>
        <span class="desktop-window__title-label">{{ title }}</span>
      </div>
    </header>

    <div class="desktop-window__body" :class="{ 'desktop-window__body--loading': loading || loadError }">
      <div v-if="loading" class="desktop-window__loading" role="status">
        <LoaderCircle :size="23" aria-hidden="true" />
        <span>{{ i18n.t('common.loading') }}</span>
      </div>
      <div v-else-if="loadError" class="desktop-window__load-error" role="alert">
        <span><TriangleAlert :size="22" aria-hidden="true" /></span>
        <strong>{{ i18n.t('desktop.windowLoadFailed') }}</strong>
        <button class="button button--small" type="button" @click="retryLoad">
          <RotateCw :size="14" aria-hidden="true" />
          {{ i18n.t('common.retry') }}
        </button>
      </div>
      <component :is="component" v-else-if="component" />
    </div>

    <div
      v-for="edge in windowState.maximized || windowState.snap ? [] : handleEdges"
      :key="edge"
      class="desktop-window__resize"
      :class="`desktop-window__resize--${edge}`"
      :data-resize-edge="edge"
      :aria-hidden="true"
      @pointerdown="(event) => gesture.onPointerDown(event, gesture.edgeForTarget(event.target as HTMLElement))"
    />

    <Teleport v-if="snapTarget" to=".desktop">
      <div class="desktop-window-snap-preview" :style="snapPreviewStyle" aria-hidden="true" />
    </Teleport>
  </section>
</template>
