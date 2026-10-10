<script setup lang="ts">
import FileEntryIcon from '@/components/files/FileEntryIcon.vue'

const DIRECTORY_ARTWORK_URL = '/desktop-icons/folder-open-shortcut-kpanel-flat-v1.webp'

defineProps<{
  kind: 'file' | 'directory'
  name: string
}>()
</script>

<template>
  <span
    class="desktop__shortcut-artwork"
    :class="`desktop__shortcut-artwork--${kind}`"
    aria-hidden="true"
  >
    <img
      v-if="kind === 'directory'"
      class="desktop__shortcut-directory-image"
      :src="DIRECTORY_ARTWORK_URL"
      alt=""
      draggable="false"
      width="58"
      height="58"
    />
    <FileEntryIcon
      v-else
      :entry="{ name, kind: 'file' }"
      :size="54"
    />
  </span>
</template>
