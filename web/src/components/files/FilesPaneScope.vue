<script setup lang="ts">
import { computed, provide, ref } from 'vue'
import { useRouter } from 'vue-router'
import { createWindowRouter, reactiveRouteFor } from '@/lib/desktopWindowRoute'
import {
  desktopCloseGuardCoordinator,
  desktopWindowActiveKey,
  desktopWindowCloseGuardKey,
  windowRouteKey,
  windowRouterKey,
} from '@/lib/desktopRouteKeys'
import { filesSplitControlKey, type FilesPaneDensity, type FilesSplitRole } from '@/lib/filesSplit'

/**
 * One pane of the classic split file manager. The secondary pane provides a
 * pane-scoped router, active state and close guards exactly like a desktop
 * window, so the FilesView inside it stays a complete, independent instance.
 */

const props = defineProps<{
  role: FilesSplitRole
  active: boolean
  split: boolean
  splitAvailable: boolean
  density?: FilesPaneDensity
  toolbarStacked?: boolean
  label?: string
  /** Initial `/files` location of the secondary pane. */
  initialPath?: string
}>()

const emit = defineEmits<{
  activate: []
  navigate: [fullPath: string]
  openSplit: []
  closePane: []
}>()

const closeGuards = new Set<() => boolean | Promise<boolean>>()
const busyChecks = new Set<() => boolean>()

provide(desktopWindowActiveKey, computed(() => props.active))
provide(filesSplitControlKey, {
  role: props.role,
  available: computed(() => props.splitAvailable),
  open: computed(() => props.split),
  openSplit: () => emit('openSplit'),
  closePane: () => emit('closePane'),
  registerBusyCheck(check) {
    busyChecks.add(check)
    return () => {
      busyChecks.delete(check)
    }
  },
})

const ready = ref(props.role === 'primary')
const root = ref<HTMLElement>()

if (props.role === 'secondary') {
  const applicationRouter = useRouter()
  const handOff = (fullPath: string) => {
    void applicationRouter.push(fullPath)
  }
  const router = createWindowRouter(
    props.initialPath || '/files',
    handOff,
    // Anything outside the file manager belongs to the page, not this pane.
    (fullPath) => {
      handOff(fullPath)
      return true
    },
  )
  // The pane shares the page scroll with the primary pane; its navigation
  // must not jump the page to the top.
  router.options.scrollBehavior = undefined
  provide(windowRouterKey, router)
  provide(windowRouteKey, reactiveRouteFor(router))
  router.afterEach((to) => {
    if (to.path === '/files') emit('navigate', to.fullPath)
  })
  // Mount FilesView on its real location instead of the memory-history seed.
  void router.isReady().finally(() => {
    ready.value = true
  })

}

provide(desktopWindowCloseGuardKey, {
  register(guard) {
    closeGuards.add(guard)
    const unregisterGlobal = desktopCloseGuardCoordinator.register(`classic-files-${props.role}`, guard)
    return () => {
      closeGuards.delete(guard)
      unregisterGlobal()
    }
  },
})

/** Whether closing the pane now would interrupt uploads or transfers. */
function isBusy(): boolean {
  return [...busyChecks].some((check) => check())
}

/** Run the pane's unsaved-work checks before the workspace removes it. */
async function confirmClose(): Promise<boolean> {
  for (const guard of [...closeGuards]) {
    if (!(await guard())) return false
  }
  return true
}

/** Return keyboard focus to this pane after the other one closes. */
function focus(): void {
  root.value?.querySelector<HTMLElement>('.files-page')?.focus({ preventScroll: true })
}

defineExpose({ confirmClose, isBusy, focus })
</script>

<template>
  <div
    ref="root"
    class="files-pane"
    :class="[
      `files-pane--${role}`,
      split ? `files-pane--split files-pane--${density || 'regular'}` : undefined,
      {
        'files-pane--active': split && active,
        'files-pane--toolbar-stacked': split && toolbarStacked,
      },
    ]"
    :role="split ? 'region' : undefined"
    :aria-label="split ? label : undefined"
    @pointerdown.capture="emit('activate')"
    @focusin="emit('activate')"
  >
    <slot v-if="ready" />
  </div>
</template>
