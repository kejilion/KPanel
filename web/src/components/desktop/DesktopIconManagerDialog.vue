<script setup lang="ts">
import { computed } from 'vue'
import {
  Eye,
  EyeOff,
  Grid2X2,
  LayoutGrid,
  Link2,
  Pencil,
  Plus,
  RotateCcw,
  Trash2,
} from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { useI18n } from '@/i18n'
import type { DesktopEntry } from '@/lib/desktopEntries'
import type { DesktopWidgetDefinition } from '@/lib/desktopWidgets'
import type { DesktopShortcut } from '@/types/api'
import DesktopShortcutArtwork from './DesktopShortcutArtwork.vue'

type ManagedWidget = Pick<
  DesktopWidgetDefinition,
  'key' | 'icon' | 'titleKey' | 'descriptionKey' | 'tone'
>

const props = defineProps<{
  open: boolean
  hiddenEntries: DesktopEntry[]
  shortcuts: DesktopShortcut[]
  widgets: readonly ManagedWidget[]
  hiddenWidgetKeys: readonly string[]
  busy?: boolean
  canAutoArrange?: boolean
}>()

const emit = defineEmits<{
  close: []
  add: []
  edit: [shortcut: DesktopShortcut]
  remove: [shortcut: DesktopShortcut]
  restore: [entry: DesktopEntry]
  toggleWidget: [key: string, visible: boolean]
  autoArrange: []
}>()

const i18n = useI18n()
const visibleWidgetCount = computed(() => props.widgets.filter((widget) => !props.hiddenWidgetKeys.includes(widget.key)).length)

function isWidgetVisible(key: string): boolean {
  return !props.hiddenWidgetKeys.includes(key)
}
</script>

<template>
  <ModalDialog
    :open="open"
    :title="i18n.t('desktop.iconManagerTitle')"
    :description="i18n.t('desktop.iconManagerHint')"
    size="large"
    @close="emit('close')"
  >
    <div class="desktop-icon-manager">
      <section class="desktop-icon-manager__section desktop-icon-manager__section--widgets">
        <header class="desktop-icon-manager__section-header">
          <div class="desktop-icon-manager__section-heading">
            <span class="desktop-icon-manager__section-icon" aria-hidden="true">
              <LayoutGrid :size="18" />
            </span>
            <div>
              <strong>{{ i18n.t('desktop.widgetManagerTitle') }}</strong>
              <small>{{ i18n.t('desktop.widgetManagerHint') }}</small>
            </div>
          </div>
          <span class="desktop-icon-manager__count">
            {{ visibleWidgetCount }} / {{ widgets.length }}
          </span>
        </header>

        <div class="desktop-icon-manager__widget-grid">
          <article
            v-for="widget in widgets"
            :key="widget.key"
            class="desktop-icon-manager__widget"
            :class="[
              `desktop-icon-manager__widget--${widget.tone}`,
              { 'desktop-icon-manager__widget--hidden': !isWidgetVisible(widget.key) },
            ]"
            :data-widget-key="widget.key"
          >
            <div class="desktop-icon-manager__widget-main">
              <span class="desktop-icon-manager__widget-icon" aria-hidden="true">
                <component :is="widget.icon" :size="18" />
              </span>
              <div>
                <strong>{{ i18n.t(widget.titleKey) }}</strong>
                <small>{{ i18n.t(widget.descriptionKey) }}</small>
              </div>
            </div>
            <div class="desktop-icon-manager__widget-footer">
              <span class="desktop-icon-manager__widget-status">
                <i :class="{ 'desktop-icon-manager__status-dot--hidden': !isWidgetVisible(widget.key) }" />
                {{ i18n.t(isWidgetVisible(widget.key) ? 'desktop.widgetVisible' : 'desktop.widgetHidden') }}
              </span>
              <button
                class="button button--ghost desktop-icon-manager__widget-toggle"
                type="button"
                :disabled="busy"
                :aria-pressed="isWidgetVisible(widget.key)"
                @click="emit('toggleWidget', widget.key, !isWidgetVisible(widget.key))"
              >
                <EyeOff v-if="isWidgetVisible(widget.key)" :size="14" aria-hidden="true" />
                <Eye v-else :size="14" aria-hidden="true" />
                {{ i18n.t(isWidgetVisible(widget.key) ? 'desktop.widgetHide' : 'desktop.widgetShow') }}
              </button>
            </div>
          </article>
        </div>
      </section>

      <div class="desktop-icon-manager__collections">
        <section class="desktop-icon-manager__section">
          <header class="desktop-icon-manager__section-header">
            <div>
              <strong>{{ i18n.t('desktop.customShortcutsTitle') }}</strong>
              <small>{{ i18n.t('desktop.customShortcutsHint') }}</small>
            </div>
            <button class="button button--ghost" type="button" :disabled="busy" @click="emit('add')">
              <Plus :size="14" aria-hidden="true" />
              {{ i18n.t('desktop.shortcutAdd') }}
            </button>
          </header>
          <div v-if="shortcuts.length" class="desktop-icon-manager__list">
            <article v-for="shortcut in shortcuts" :key="shortcut.id">
              <span class="desktop-icon-manager__glyph" aria-hidden="true">
                <img v-if="shortcut.iconURL" :src="shortcut.iconURL" alt="" />
                <DesktopShortcutArtwork
                  v-else-if="shortcut.targetType === 'directory' || shortcut.targetType === 'file'"
                  :kind="shortcut.targetType"
                  :name="shortcut.name"
                />
                <Link2 v-else :size="20" />
              </span>
              <div>
                <strong>{{ shortcut.name }}</strong>
                <small>{{ shortcut.description || shortcut.path || shortcut.url }}</small>
              </div>
              <span class="desktop-icon-manager__actions">
                <button
                  v-if="shortcut.targetType === 'url'"
                  class="button button--ghost button--icon"
                  type="button"
                  :title="i18n.t('desktop.shortcutEdit')"
                  :aria-label="i18n.t('desktop.shortcutEdit')"
                  :disabled="busy"
                  @click="emit('edit', shortcut)"
                >
                  <Pencil :size="14" aria-hidden="true" />
                </button>
                <button
                  class="button button--ghost button--icon"
                  type="button"
                  :title="i18n.t(shortcut.targetType === 'url' ? 'desktop.shortcutDelete' : 'desktop.removeFromDesktop')"
                  :aria-label="i18n.t(shortcut.targetType === 'url' ? 'desktop.shortcutDelete' : 'desktop.removeFromDesktop')"
                  :disabled="busy"
                  @click="emit('remove', shortcut)"
                >
                  <Trash2 :size="14" aria-hidden="true" />
                </button>
              </span>
            </article>
          </div>
          <p v-else class="desktop-icon-manager__empty">{{ i18n.t('desktop.customShortcutsEmpty') }}</p>
        </section>

        <section class="desktop-icon-manager__section">
          <header class="desktop-icon-manager__section-header">
            <div>
              <strong>{{ i18n.t('desktop.hiddenEntriesTitle') }}</strong>
              <small>{{ i18n.t('desktop.hiddenEntriesHint') }}</small>
            </div>
            <span class="desktop-icon-manager__count">{{ hiddenEntries.length }}</span>
          </header>
          <div v-if="hiddenEntries.length" class="desktop-icon-manager__list">
            <article v-for="entry in hiddenEntries" :key="entry.key">
              <span class="desktop-icon-manager__glyph" aria-hidden="true">
                <img v-if="entry.iconURL" :src="entry.iconURL" alt="" />
                <span v-else>{{ entry.name.trim().slice(0, 1).toLocaleUpperCase() }}</span>
              </span>
              <div>
                <strong>{{ entry.name }}</strong>
                <small>{{ entry.kind === 'app'
                  ? i18n.t('desktop.detailApp')
                  : i18n.t('desktop.detailSite') }}</small>
              </div>
              <button
                class="button button--ghost"
                type="button"
                :disabled="busy"
                @click="emit('restore', entry)"
              >
                <RotateCcw :size="14" aria-hidden="true" />
                {{ i18n.t('desktop.restoreToDesktop') }}
              </button>
            </article>
          </div>
          <p v-else class="desktop-icon-manager__empty">{{ i18n.t('desktop.hiddenEntriesEmpty') }}</p>
        </section>
      </div>

      <button
        class="desktop-icon-manager__layout-action"
        type="button"
        :disabled="busy || !canAutoArrange"
        @click="emit('autoArrange')"
      >
        <span aria-hidden="true"><Grid2X2 :size="18" /></span>
        <span>
          <strong>{{ i18n.t('desktop.autoArrange') }}</strong>
          <small>{{ i18n.t(canAutoArrange
            ? 'desktop.autoArrangeHint'
            : 'desktop.autoArrangeDesktopOnly') }}</small>
        </span>
      </button>
    </div>
    <template #footer>
      <button class="button button--primary" type="button" @click="emit('close')">
        {{ i18n.t('common.closeDialog') }}
      </button>
    </template>
  </ModalDialog>
</template>
