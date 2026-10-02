<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { isNavigationFailure, NavigationFailureType, useRoute, useRouter } from 'vue-router'
import FilesPaneScope from '@/components/files/FilesPaneScope.vue'
import FilesView from '@/views/FilesView.vue'
import {
  FILES_PANE_TOOLBAR_STACK_WIDTH,
  FILES_SPLIT_GAP,
  FILES_SPLIT_MIN_WIDTH,
  FILES_SPLIT_STACK_WIDTH,
  filesPaneDensity,
  normalizeFilesSplitPath,
  readFilesSplitPreference,
  writeFilesSplitPreference,
  type FilesSplitRole,
} from '@/lib/filesSplit'
import { useToast } from '@/stores/toast'

/**
 * Classic-mode file manager route. A single pane is the unchanged FilesView;
 * on a wide workspace the user can open a second, fully independent pane and
 * drag or copy between them like two desktop file windows.
 */

const route = useRoute()
const router = useRouter()
const toast = useToast()
const root = ref<HTMLElement>()
const width = ref(0)
const preference = readFilesSplitPreference()
const requested = ref(preference.open)
const secondaryPath = ref(preference.secondaryPath)
const secondaryMounted = ref(false)
const activePane = ref<FilesSplitRole>('primary')
const primaryScope = ref<InstanceType<typeof FilesPaneScope>>()
const secondaryScope = ref<InstanceType<typeof FilesPaneScope>>()
let closing = false
let observer: ResizeObserver | undefined

const available = computed(() => width.value >= FILES_SPLIT_MIN_WIDTH)
const stacked = computed(() => (
  secondaryMounted.value && width.value > 0 && width.value < FILES_SPLIT_STACK_WIDTH
))
const paneWidth = computed(() => (
  stacked.value ? width.value : (width.value - FILES_SPLIT_GAP) / 2
))
const paneDensity = computed(() => filesPaneDensity(paneWidth.value))
const paneToolbarStacked = computed(() => (
  paneWidth.value > 0 && paneWidth.value < FILES_PANE_TOOLBAR_STACK_WIDTH
))

watch([requested, available], ([wanted, wide]) => {
  if (!wanted) secondaryMounted.value = false
  // Narrowing keeps an open pane mounted (stacked) so its uploads and
  // transfers continue; only a wide workspace opens it.
  else if (wide) secondaryMounted.value = true
}, { immediate: true })

watch(secondaryMounted, (mounted) => {
  if (!mounted) activePane.value = 'primary'
})

function persist(): void {
  writeFilesSplitPreference({ open: requested.value, secondaryPath: secondaryPath.value })
}

function primaryLocation(): string {
  const query = new URLSearchParams()
  if (typeof route.query.path === 'string') query.set('path', route.query.path)
  if (typeof route.query.hostId === 'string') query.set('hostId', route.query.hostId)
  return normalizeFilesSplitPath(`/files?${query.toString()}`) || '/files'
}

function rememberSecondaryPath(fullPath: string): void {
  secondaryPath.value = normalizeFilesSplitPath(fullPath)
  persist()
}

/**
 * Close one pane and keep the other, like closing one of two windows. The
 * main pane owns the page URL, so closing it moves it to the second pane's
 * directory and host; either way the second FilesView is the one removed.
 */
async function closePane(role: FilesSplitRole): Promise<void> {
  const secondary = secondaryScope.value
  if (closing || !secondary) return
  const affected = role === 'primary' ? [primaryScope.value, secondary] : [secondary]
  if (affected.some((scope) => scope?.isBusy())) {
    toast.show('有文件操作进行中', { message: '等上传、复制或移动完成后再关闭此栏。' })
    return
  }
  closing = true
  try {
    if (role === 'primary' && !(await primaryScope.value?.confirmClose())) return
    if (!(await secondary.confirmClose())) return
    if (role === 'primary') {
      // Push a location object so the URL reads like the pane's own navigation.
      const target = new URL(secondaryPath.value || '/files', 'http://kpanel.invalid')
      try {
        const failure = await router.push({ name: 'files', query: Object.fromEntries(target.searchParams) })
        if (failure && !isNavigationFailure(failure, NavigationFailureType.duplicated)) return
        // FilesView may reject the host switch and restore the previous route.
        await nextTick()
        if (primaryLocation() !== normalizeFilesSplitPath(target.pathname + target.search)) return
      } catch {
        return
      }
    }
    requested.value = false
    persist()
    // The clicked close button is gone; keep keyboard focus in the remaining pane.
    await nextTick()
    primaryScope.value?.focus()
  } finally {
    closing = false
  }
}

function openSplit(): void {
  if (secondaryMounted.value || !available.value) return
  // A new second pane starts beside the current directory and host.
  secondaryPath.value ??= primaryLocation()
  requested.value = true
  persist()
}

onMounted(() => {
  const element = root.value
  if (!element) return
  width.value = element.clientWidth
  if (typeof ResizeObserver === 'undefined') return
  observer = new ResizeObserver((entries) => {
    width.value = entries[0]?.contentRect.width ?? element.clientWidth
  })
  observer.observe(element)
})

onBeforeUnmount(() => {
  observer?.disconnect()
})
</script>

<template>
  <div
    ref="root"
    class="files-workspace"
    :class="{
      'files-workspace--split': secondaryMounted,
      'files-workspace--stacked': stacked,
    }"
  >
    <FilesPaneScope
      ref="primaryScope"
      role="primary"
      label="主文件栏"
      :active="!secondaryMounted || activePane === 'primary'"
      :split="secondaryMounted"
      :split-available="available"
      :density="paneDensity"
      :toolbar-stacked="paneToolbarStacked"
      @activate="activePane = 'primary'"
      @open-split="openSplit"
      @close-pane="closePane('primary')"
    >
      <FilesView />
    </FilesPaneScope>
    <FilesPaneScope
      v-if="secondaryMounted"
      ref="secondaryScope"
      role="secondary"
      label="第二文件栏"
      :active="activePane === 'secondary'"
      :split="true"
      :split-available="available"
      :density="paneDensity"
      :toolbar-stacked="paneToolbarStacked"
      :initial-path="secondaryPath"
      @activate="activePane = 'secondary'"
      @navigate="rememberSecondaryPath"
      @close-pane="closePane('secondary')"
    >
      <FilesView />
    </FilesPaneScope>
  </div>
</template>

<style>
/*
 * Unscoped on purpose: pane rules adapt FilesView's own layout to pane width,
 * mirroring the desktop-window container rules in desktop.css. The extra
 * `.files-workspace--split` class keeps them above FilesView's scoped rules.
 */
.files-workspace--split {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  align-items: start;
}

.files-workspace--stacked {
  grid-template-columns: minmax(0, 1fr);
}

.files-workspace--split > .files-pane {
  box-sizing: border-box;
  min-width: 0;
  padding: 14px;
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  background: color-mix(in srgb, var(--surface-subtle, var(--surface)) 55%, transparent);
  transition: border-color 160ms ease, box-shadow 160ms ease;
}

.files-workspace--split > .files-pane--active {
  border-color: color-mix(in srgb, var(--brand) 55%, var(--border));
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--brand) 18%, transparent);
}

.files-workspace--split .files-pane--split .file-command-bar {
  align-items: stretch;
  flex-direction: column;
  flex-wrap: nowrap;
  gap: 10px;
}

.files-workspace--split .files-pane--split .file-shortcuts {
  width: 100%;
  min-width: 0;
  flex-wrap: nowrap;
  gap: 6px;
  overflow-x: auto;
  overscroll-behavior-x: contain;
  scrollbar-width: none;
}

.files-workspace--split .files-pane--split .file-shortcuts::-webkit-scrollbar {
  display: none;
}

.files-workspace--split .files-pane--split .file-shortcuts button {
  flex: 0 0 auto;
}

.files-workspace--split .files-pane--split .file-command-bar__actions {
  width: 100%;
  min-width: 0;
  margin-left: 0;
  flex-wrap: wrap;
  justify-content: flex-end;
}

/*
 * Two panes cannot fit nine labelled buttons: they wrapped into three rows.
 * Beside each other the secondary actions shrink to icon squares that keep
 * their tooltip and accessible name; only the primary upload keeps its text.
 */
.files-workspace--split .files-pane--split .file-command-bar__actions {
  flex-wrap: nowrap;
}

.files-workspace--split .files-pane--split .file-command-bar__actions .button:not(.button--primary) {
  width: 40px;
  min-width: 40px;
  flex: 0 0 40px;
  padding: 0;
  justify-content: center;
}

.files-workspace--split .files-pane--split .file-command-bar__actions .button:not(.button--primary) .file-command-bar__label {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip-path: inset(50%);
  white-space: nowrap;
}

/* Each pane owns its batch bar instead of two bars stacking at page bottom. */
.files-workspace--split .files-pane--split .batch-bar {
  position: sticky;
  z-index: 45;
  bottom: max(10px, env(safe-area-inset-bottom));
  left: auto;
  width: min(760px, calc(100% - 24px));
  margin: 12px auto 0;
  transform: none;
}

.files-workspace--split .files-pane--compact .file-row,
.files-workspace--split .files-pane--narrow .file-row {
  grid-template-columns: 38px minmax(180px, 1fr) 90px 108px 46px;
}

.files-workspace--split .files-pane--compact .file-row > :nth-child(4),
.files-workspace--split .files-pane--compact .file-row > :nth-child(5),
.files-workspace--split .files-pane--narrow .file-row > :nth-child(4),
.files-workspace--split .files-pane--narrow .file-row > :nth-child(5) {
  display: none;
}

/* Host, path, search and view controls cannot share one row below ~760px. */
.files-workspace--split .files-pane--toolbar-stacked .file-toolbar {
  align-items: stretch;
  flex-direction: column;
  gap: 9px;
  padding: 10px;
}

.files-workspace--split .files-pane--toolbar-stacked .file-toolbar__controls {
  width: 100%;
}

.files-workspace--split .files-pane--toolbar-stacked .file-toolbar__controls .file-search {
  width: auto;
  min-width: 0;
  flex: 1 1 auto;
}

.files-workspace--split .files-pane--narrow .file-row--header {
  display: none;
}

.files-workspace--split .files-pane--narrow .file-row {
  grid-template-columns: 34px minmax(0, 1fr) 42px;
}

.files-workspace--split .files-pane--narrow .file-row:not(.file-row--header) > :nth-child(3),
.files-workspace--split .files-pane--narrow .file-row:not(.file-row--header) > :nth-child(6) {
  display: none;
}

@media (prefers-reduced-motion: reduce) {
  .files-workspace--split > .files-pane {
    transition: none;
  }
}
</style>
