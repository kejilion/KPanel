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
  label?: string
  /** Initial `/files` location of the secondary pane. */
  initialPath?: string
}>()

const emit = defineEmits<{
  activate: []
  navigate: [fullPath: string]
  toggleSplit: []
}>()

provide(desktopWindowActiveKey, computed(() => props.active))
provide(filesSplitControlKey, {
  role: props.role,
  available: computed(() => props.splitAvailable),
  open: computed(() => props.split),
  toggle: () => emit('toggleSplit'),
})

const ready = ref(props.role === 'primary')
const closeGuards = new Set<() => boolean | Promise<boolean>>()

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

  provide(desktopWindowCloseGuardKey, {
    register(guard) {
      closeGuards.add(guard)
      const unregisterGlobal = desktopCloseGuardCoordinator.register('classic-files-secondary', guard)
      return () => {
        closeGuards.delete(guard)
        unregisterGlobal()
      }
    },
  })
}

/** Run the pane's unsaved-work checks before the workspace removes it. */
async function confirmClose(): Promise<boolean> {
  for (const guard of [...closeGuards]) {
    if (!(await guard())) return false
  }
  return true
}

defineExpose({ confirmClose })
</script>

<template>
  <div
    class="files-pane"
    :class="[
      `files-pane--${role}`,
      split ? `files-pane--split files-pane--${density || 'regular'}` : undefined,
      { 'files-pane--active': split && active },
    ]"
    :role="split ? 'region' : undefined"
    :aria-label="split ? label : undefined"
    @pointerdown.capture="emit('activate')"
    @focusin="emit('activate')"
  >
    <slot v-if="ready" />
  </div>
</template>
