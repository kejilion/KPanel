<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { LayoutPanelLeft, LogOut, Moon, RotateCw, Search, Sun } from '@lucide/vue'
import LanguageSelector from '@/components/common/LanguageSelector.vue'
import { useI18n } from '@/i18n'
import {
  DESKTOP_START_MENU_SECTIONS,
  moveStartMenuIndex,
  searchDesktopStartMenu,
  type DesktopStartMenuItem,
  type DesktopStartMenuSection,
} from '@/lib/desktopStartMenu'

/**
 * Search-first start menu opened from the taskbar K button. It only presents
 * items and reports choices; DesktopView owns every launch and command.
 */

const props = defineProps<{
  open: boolean
  items: readonly DesktopStartMenuItem[]
  entriesState: 'loading' | 'ready' | 'unavailable'
  userName: string
  signingOut?: boolean
  dark?: boolean
  /** The toggle button; moving focus to it does not dismiss the menu. */
  opener?: HTMLElement
}>()

const emit = defineEmits<{
  close: [restoreFocus: boolean]
  select: [item: DesktopStartMenuItem]
  retry: []
  theme: []
  classic: []
  'sign-out': []
  'after-leave': []
}>()

const MENU_ID = 'desktop-start-menu'
const LIST_ID = `${MENU_ID}-results`

const i18n = useI18n()
const root = ref<HTMLElement>()
const input = ref<HTMLInputElement>()
const gridElement = ref<HTMLElement>()
const body = ref<HTMLElement>()
const query = ref('')
const active = ref(0)
const failedIcons = ref(new Set<string>())
const loadedIcons = ref(new Set<string>())

const searching = computed(() => Boolean(query.value.trim()))
const results = computed(() => searchDesktopStartMenu(props.items, query.value))
const gridCount = computed(() =>
  searching.value ? 0 : results.value.filter((item) => item.section === 'system').length)
const groups = computed(() => DESKTOP_START_MENU_SECTIONS
  .map((section) => ({
    section,
    grid: section === 'system' && !searching.value,
    options: results.value
      .map((item, index) => ({ item, index }))
      .filter((option) => option.item.section === section),
  }))
  .filter((group) => group.options.length))
const entryCount = computed(() => props.items.filter((item) => item.section === 'entries').length)
const activeId = computed(() => results.value[active.value] ? optionId(active.value) : undefined)
const userInitial = computed(() => props.userName.trim().slice(0, 1).toLocaleUpperCase() || 'K')

function optionId(index: number): string {
  return `${MENU_ID}-option-${index}`
}

function sectionLabel(section: DesktopStartMenuSection): string {
  if (section === 'system') return i18n.t('desktop.startMenuSystem')
  if (section === 'entries') return i18n.t('desktop.startMenuEntries')
  return i18n.t('desktop.startMenuActions')
}

function monogram(label: string): string {
  return label.trim().slice(0, 1).toLocaleUpperCase() || 'K'
}

function showImage(item: DesktopStartMenuItem): boolean {
  return Boolean(item.iconURL && !failedIcons.value.has(item.iconURL))
}

function imageLoaded(item: DesktopStartMenuItem): boolean {
  return Boolean(item.iconURL && loadedIcons.value.has(item.iconURL))
}

function addIcon(target: Set<string>, url?: string): Set<string> {
  return url && !target.has(url) ? new Set([...target, url]) : target
}

function onIconLoad(url?: string): void {
  loadedIcons.value = addIcon(loadedIcons.value, url)
}

function onIconError(url?: string): void {
  failedIcons.value = addIcon(failedIcons.value, url)
}

function gridColumns(): number {
  const options = gridElement.value
    ? [...gridElement.value.querySelectorAll<HTMLElement>('[role="option"]')]
    : []
  if (!options.length) return 1
  const top = options[0]!.offsetTop
  const columns = options.findIndex((option) => option.offsetTop !== top)
  return columns === -1 ? options.length : columns
}

function setGridElement(element: unknown): void {
  gridElement.value = element instanceof HTMLElement ? element : undefined
}

const NAVIGATION_KEYS = ['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Home', 'End'] as const
type NavigationKey = (typeof NAVIGATION_KEYS)[number]

function isNavigationKey(key: string): key is NavigationKey {
  return (NAVIGATION_KEYS as readonly string[]).includes(key)
}

/** Keyboard moves keep the highlight visible; pointer hover never scrolls the list under the pointer. */
async function revealActive(): Promise<void> {
  await nextTick()
  root.value?.querySelector<HTMLElement>(`#${optionId(active.value)}`)?.scrollIntoView?.({ block: 'nearest' })
}

function onSearchKeyDown(event: KeyboardEvent): void {
  // Safari reports the Enter that commits an IME composition as keyCode 229.
  if (event.isComposing || event.keyCode === 229) return
  if (isNavigationKey(event.key)) {
    // While typing, horizontal keys edit the query instead of moving the highlight.
    if (searching.value && event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return
    event.preventDefault()
    active.value = moveStartMenuIndex(active.value, event.key, results.value.length, {
      count: gridCount.value,
      columns: gridColumns(),
    })
    void revealActive()
    return
  }
  if (event.key !== 'Enter') return
  const item = results.value[active.value]
  if (!item) return
  event.preventDefault()
  emit('select', item)
}

function onKeyDown(event: KeyboardEvent): void {
  if (event.key !== 'Escape' || event.isComposing || event.defaultPrevented) return
  event.preventDefault()
  // An open language list closes first through its own document listener; the
  // handled flag keeps the desktop's window listener from closing the menu too.
  if (event.target instanceof Element && event.target.closest('.language-selector')
    && root.value?.querySelector('.language-selector__menu')) return
  event.stopPropagation()
  if (query.value && event.target === input.value) {
    query.value = ''
    return
  }
  emit('close', true)
}

function onFocusOut(event: FocusEvent): void {
  const next = event.relatedTarget
  // A null target is a window blur or a click on non-focusable chrome; the
  // desktop pointer handler decides those cases.
  if (!(next instanceof Node)) return
  if (root.value?.contains(next) || props.opener?.contains(next)) return
  emit('close', false)
}

watch(() => props.open, async (open) => {
  if (!open) return
  query.value = ''
  active.value = 0
  await nextTick()
  input.value?.focus({ preventScroll: true })
}, { immediate: true })

watch(query, () => {
  active.value = 0
  if (body.value) body.value.scrollTop = 0
})

watch(() => results.value.length, (length) => {
  if (active.value >= length) active.value = length ? 0 : -1
  else if (active.value < 0 && length) active.value = 0
})

</script>

<template>
  <Transition name="desktop-start-menu" @after-leave="emit('after-leave')">
    <section
      v-if="open"
      :id="MENU_ID"
      ref="root"
      class="desktop-start-menu"
      role="dialog"
      aria-modal="false"
      :aria-label="i18n.t('desktop.startMenuLabel')"
      @keydown="onKeyDown"
      @focusout="onFocusOut"
      @contextmenu.prevent.stop
    >
      <label class="desktop-start-menu__search">
        <Search :size="18" aria-hidden="true" />
        <input
          ref="input"
          v-model="query"
          type="text"
          role="combobox"
          aria-autocomplete="list"
          aria-expanded="true"
          :aria-controls="LIST_ID"
          :aria-activedescendant="activeId"
          :aria-label="i18n.t('desktop.startMenuSearch')"
          :placeholder="i18n.t('desktop.startMenuSearch')"
          autocomplete="off"
          spellcheck="false"
          enterkeyhint="go"
          maxlength="120"
          @keydown="onSearchKeyDown"
        />
      </label>

      <!-- Pointer presses keep focus in the search box so the highlight and typing stay in sync. -->
      <div ref="body" class="desktop-start-menu__body" @mousedown.prevent>
        <div :id="LIST_ID" class="desktop-start-menu__results" role="listbox" :aria-label="i18n.t('desktop.startMenuLabel')">
          <div
            v-for="group in groups"
            :key="group.section"
            class="desktop-start-menu__group"
            :class="{ 'desktop-start-menu__group--grid': group.grid }"
            role="group"
            :aria-labelledby="`${MENU_ID}-section-${group.section}`"
          >
            <div :id="`${MENU_ID}-section-${group.section}`" class="desktop-start-menu__heading" role="presentation">
              {{ sectionLabel(group.section) }}
            </div>
            <div :ref="group.grid ? setGridElement : undefined" class="desktop-start-menu__options">
              <div
                v-for="option in group.options"
                :id="optionId(option.index)"
                :key="option.item.key"
                class="desktop-start-menu__option"
                :class="{
                  'desktop-start-menu__option--active': option.index === active,
                  'desktop-start-menu__option--tile': group.grid,
                }"
                role="option"
                :aria-selected="option.index === active"
                :title="option.item.detail ? `${option.item.label} · ${option.item.detail}` : option.item.label"
                :data-start-menu-key="option.item.key"
                @pointermove="active = option.index"
                @click="emit('select', option.item)"
              >
                <span
                  class="desktop-start-menu__icon"
                  :class="{
                    'desktop-start-menu__icon--plain': !option.item.gradient,
                    'desktop-start-menu__icon--native': option.item.section === 'system',
                  }"
                  :style="option.item.gradient ? { background: option.item.gradient } : undefined"
                  aria-hidden="true"
                >
                  <img
                    v-if="showImage(option.item)"
                    :class="{ 'desktop-start-menu__icon-image--loading': !imageLoaded(option.item) }"
                    :src="option.item.iconURL"
                    alt=""
                    draggable="false"
                    decoding="async"
                    referrerpolicy="no-referrer"
                    @load="onIconLoad(option.item.iconURL)"
                    @error="onIconError(option.item.iconURL)"
                  />
                  <template v-if="!showImage(option.item) || !imageLoaded(option.item)">
                    <component :is="option.item.icon" v-if="option.item.icon" :size="group.grid ? 22 : 18" :stroke-width="1.9" />
                    <span v-else>{{ monogram(option.item.label) }}</span>
                  </template>
                </span>
                <span class="desktop-start-menu__text">
                  <span class="desktop-start-menu__label">{{ option.item.label }}</span>
                  <span v-if="!group.grid && option.item.detail" class="desktop-start-menu__detail">{{ option.item.detail }}</span>
                </span>
                <span v-if="option.item.hidden && !group.grid" class="desktop-start-menu__badge">
                  {{ i18n.t('desktop.startMenuHidden') }}
                </span>
              </div>
            </div>
          </div>
        </div>

        <p v-if="searching && !results.length" class="desktop-start-menu__note" role="status">
          {{ i18n.t('desktop.startMenuNoResults', { query: query.trim() }) }}
        </p>
        <template v-else-if="!searching">
          <p v-if="entriesState === 'loading'" class="desktop-start-menu__note" role="status">
            {{ i18n.t('desktop.startMenuEntriesLoading') }}
          </p>
          <div v-else-if="entriesState === 'unavailable'" class="desktop-start-menu__note desktop-start-menu__note--action" role="status">
            <span>{{ i18n.t('desktop.startMenuEntriesUnavailable') }}</span>
            <button type="button" class="desktop-start-menu__retry" @click="emit('retry')">
              <RotateCw :size="15" aria-hidden="true" />
              {{ i18n.t('desktop.startMenuRetry') }}
            </button>
          </div>
          <p v-else-if="!entryCount" class="desktop-start-menu__note" role="status">
            {{ i18n.t('desktop.startMenuEntriesEmpty') }}
          </p>
        </template>
      </div>

      <footer class="desktop-start-menu__footer">
        <button
          class="desktop-start-menu__user"
          type="button"
          :disabled="signingOut"
          @click="emit('sign-out')"
        >
          <span class="desktop-start-menu__avatar" aria-hidden="true">{{ userInitial }}</span>
          <span class="desktop-start-menu__user-text">
            <strong>{{ userName }}</strong>
            <small>{{ i18n.t('nav.signOut') }}</small>
          </span>
          <LogOut :size="16" aria-hidden="true" />
        </button>
        <div class="desktop-start-menu__footer-actions">
          <LanguageSelector compact />
          <button
            class="desktop-start-menu__footer-button"
            type="button"
            :aria-label="dark ? i18n.t('desktop.menuLight') : i18n.t('desktop.menuDark')"
            :title="dark ? i18n.t('desktop.menuLight') : i18n.t('desktop.menuDark')"
            @click="emit('theme')"
          >
            <Sun v-if="dark" :size="17" aria-hidden="true" />
            <Moon v-else :size="17" aria-hidden="true" />
          </button>
          <button
            class="desktop-start-menu__footer-button"
            type="button"
            :aria-label="i18n.t('desktop.switchClassic')"
            :title="i18n.t('desktop.switchClassic')"
            @click="emit('classic')"
          >
            <LayoutPanelLeft :size="17" aria-hidden="true" />
          </button>
        </div>
      </footer>
    </section>
  </Transition>
</template>
