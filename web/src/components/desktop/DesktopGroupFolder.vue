<script setup lang="ts">
import type { Component } from 'vue'
import { useI18n } from '@/i18n'
import DesktopGroupPreviewIcon from './DesktopGroupPreviewIcon.vue'

/**
 * Phone-screen tile for a desktop group. Like a phone folder it shows up to
 * four member previews and opens the group sheet on one tap or Enter/Space.
 */
defineProps<{
  name: string
  count: number
  expanded: boolean
  previews: Array<{ key: string; label: string; iconURL?: string; icon?: Component }>
}>()
const emit = defineEmits<{ open: [trigger: HTMLButtonElement] }>()
const i18n = useI18n()
</script>

<template>
  <div class="desktop__icon-slot desktop-folder-slot">
    <button
      type="button"
      class="desktop__icon desktop-folder-tile"
      :aria-label="i18n.t('desktop.groupFolderOpen', { name, count })"
      :title="name"
      aria-haspopup="dialog"
      :aria-expanded="expanded"
      @click="emit('open', $event.currentTarget as HTMLButtonElement)"
      @contextmenu.prevent.stop
    >
      <span class="desktop__icon-glyph desktop-folder-tile__glyph">
        <DesktopGroupPreviewIcon
          v-for="preview in previews"
          :key="preview.key"
          :label="preview.label"
          :icon-u-r-l="preview.iconURL"
          :icon="preview.icon"
        />
      </span>
      <span class="desktop__icon-label">{{ name }}</span>
    </button>
  </div>
</template>
