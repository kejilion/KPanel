<script setup lang="ts">
import { File, Folder, Grid2X2, Link2, Pencil, Plus, RotateCcw, Trash2 } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { useI18n } from '@/i18n'
import type { DesktopEntry } from '@/lib/desktopEntries'
import type { DesktopShortcut } from '@/types/api'

defineProps<{
  open: boolean
  hiddenEntries: DesktopEntry[]
  shortcuts: DesktopShortcut[]
  busy?: boolean
  canAutoArrange?: boolean
}>()

const emit = defineEmits<{
  close: []
  add: []
  edit: [shortcut: DesktopShortcut]
  remove: [shortcut: DesktopShortcut]
  restore: [entry: DesktopEntry]
  autoArrange: []
}>()

const i18n = useI18n()
</script>

<template>
  <ModalDialog
    :open="open"
    :title="i18n.t('desktop.iconManagerTitle')"
    size="small"
    @close="emit('close')"
  >
    <div class="desktop-icon-manager">
      <p class="desktop-icon-manager__hint">
        {{ i18n.t('desktop.iconManagerHint') }}
      </p>

      <section>
        <header>
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
              <Folder v-else-if="shortcut.targetType === 'directory'" :size="20" />
              <File v-else-if="shortcut.targetType === 'file'" :size="20" />
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

      <section>
        <header>
          <div>
            <strong>{{ i18n.t('desktop.hiddenEntriesTitle') }}</strong>
            <small>{{ i18n.t('desktop.hiddenEntriesHint') }}</small>
          </div>
          <span>{{ hiddenEntries.length }}</span>
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
