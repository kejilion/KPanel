<script setup lang="ts">
import { computed } from 'vue'
import { fileEntryIconKind, type FileIconKind } from '@/lib/fileEntryPresentation'
import type { FileEntry } from '@/types/api'

const props = withDefaults(defineProps<{
  entry: Pick<FileEntry, 'name' | 'kind'> & Partial<Pick<FileEntry, 'mime' | 'editable' | 'previewable'>>
  size?: number
}>(), { size: 32 })

const kind = computed(() => fileEntryIconKind({ editable: false, previewable: false, ...props.entry }))
const colors: Record<FileIconKind, readonly [string, string]> = {
  folder: ['#e6af42', '#f7d77c'],
  image: ['#9683c6', '#d2c6ed'],
  media: ['#bf7998', '#ecc0d3'],
  archive: ['#ce9955', '#efd0a2'],
  spreadsheet: ['#589c7c', '#b5d9c4'],
  database: ['#579eaa', '#b8dce0'],
  presentation: ['#ce8466', '#f1c8b5'],
  package: ['#bb905e', '#e6c59c'],
  secret: ['#658d91', '#c1dbdc'],
  code: ['#648fbe', '#bfd6ed'],
  document: ['#7d9cbb', '#d0e0ee'],
  generic: ['#929fad', '#dde4eb'],
}
const palette = computed(() => colors[kind.value])
</script>

<template>
  <svg
    class="file-entry-icon"
    :data-file-icon-kind="kind"
    :width="size"
    :height="size"
    viewBox="0 0 48 48"
    fill="none"
    aria-hidden="true"
    focusable="false"
  >
    <template v-if="kind === 'folder'">
      <path d="M4 13a4 4 0 0 1 4-4h10l4 5h18a4 4 0 0 1 4 4v18H4Z" fill="#c99637" />
      <path d="M8 17h31v17H8Z" fill="#fff0c6" />
      <path d="M6 19h36a3 3 0 0 1 3 3.3l-1.5 15a4 4 0 0 1-4 3.7h-31a4 4 0 0 1-4-3.7L3 22.3A3 3 0 0 1 6 19Z" :fill="palette[0]" />
      <path d="M6 19h36a3 3 0 0 1 3 3H3a3 3 0 0 1 3-3Z" :fill="palette[1]" />
      <path d="M5 36.5A4 4 0 0 0 9 40h30a4 4 0 0 0 4-3.5l-.1 1a4 4 0 0 1-4 3.5H9.1a4 4 0 0 1-4-3.5Z" fill="#aa7825" opacity=".35" />
    </template>
    <template v-else-if="kind === 'database'">
      <path d="M7 12h34v24c0 4.4-7.6 8-17 8S7 40.4 7 36Z" :fill="palette[0]" />
      <path d="M7 21c0 4.4 7.6 8 17 8s17-3.6 17-8v3c0 4.4-7.6 8-17 8S7 28.4 7 24Zm0 11c0 4.4 7.6 8 17 8s17-3.6 17-8v3c0 4.4-7.6 8-17 8S7 39.4 7 35Z" fill="#fff" opacity=".25" />
      <ellipse cx="24" cy="12" rx="17" ry="8" :fill="palette[1]" />
      <ellipse cx="24" cy="11" rx="12" ry="4" :fill="palette[0]" opacity=".35" />
    </template>
    <template v-else-if="kind === 'package'">
      <path d="m24 4 19 10v23L24 46 5 37V14Z" :fill="palette[0]" />
      <path d="m24 4 19 10-19 10L5 14Z" :fill="palette[1]" />
      <path d="m24 24 19-10v23l-19 9Z" fill="#825f38" opacity=".25" />
      <path d="m17 7.7 19 10v10l-7 3.5v-10l-19-10Z" fill="#fff0cd" opacity=".9" />
      <path d="m10 30 9 4v3l-9-4Z" fill="#fff" opacity=".55" />
    </template>
    <template v-else>
      <path d="M12 3h17l11 11v27a4 4 0 0 1-4 4H12a4 4 0 0 1-4-4V7a4 4 0 0 1 4-4Z" :fill="palette[0]" />
      <path d="M29 3v8a3 3 0 0 0 3 3h8Z" :fill="palette[1]" />
      <path d="M29 11v3a3 3 0 0 0 3 3h8v-3h-8a3 3 0 0 1-3-3Z" fill="#24354a" opacity=".1" />
      <path d="M8 39v2a4 4 0 0 0 4 4h24a4 4 0 0 0 4-4v-2a4 4 0 0 1-4 4H12a4 4 0 0 1-4-4Z" fill="#24354a" opacity=".12" />
      <g fill="#fff" fill-opacity=".94">
        <template v-if="kind === 'image'">
          <circle cx="18" cy="23" r="3" />
          <path d="m12 36 8-9 5 5 5-7 6 11Z" />
        </template>
        <template v-else-if="kind === 'media'">
          <circle cx="24" cy="29" r="10" fill-opacity=".2" />
          <path d="M21 22.8a1 1 0 0 1 1.5-.9l9 6.2a1.1 1.1 0 0 1 0 1.8l-9 6.2a1 1 0 0 1-1.5-.9Z" />
        </template>
        <template v-else-if="kind === 'archive'">
          <path d="M20 8h4v4h-4zm4 4h4v4h-4zm-4 4h4v4h-4zm4 4h4v4h-4zm-4 4h4v4h-4z" />
          <path fill-rule="evenodd" d="M20 29h8l1 7a3 3 0 0 1-3 3h-4a3 3 0 0 1-3-3Zm2 4-.3 3h4.6l-.3-3Z" />
        </template>
        <template v-else-if="kind === 'spreadsheet'">
          <rect x="13" y="21" width="22" height="16" rx="2" />
          <path d="M14 26h20M14 31h20M21 22v14" :stroke="palette[0]" stroke-width="2" />
        </template>
        <template v-else-if="kind === 'presentation'">
          <rect x="13" y="20" width="22" height="15" rx="2" />
          <path d="M19 39h10M24 35v4" stroke="#fff" stroke-width="2.5" stroke-linecap="round" />
          <path d="M23 23v5h5a5 5 0 1 1-5-5Z" :fill="palette[0]" />
          <path d="M25 22a5 5 0 0 1 5 5h-5Z" :fill="palette[0]" opacity=".55" />
        </template>
        <template v-else-if="kind === 'secret'">
          <path d="M18 27v-4a6 6 0 0 1 12 0v4" fill="none" stroke="#fff" stroke-width="3" />
          <rect x="15" y="26" width="18" height="13" rx="3" />
          <path d="M24 30a2 2 0 0 1 1 3.7V36h-2v-2.3a2 2 0 0 1 1-3.7Z" :fill="palette[0]" />
        </template>
        <template v-else-if="kind === 'code'">
          <path d="m19 22-7 7 7 7 2-2-5-5 5-5Zm10 0-2 2 5 5-5 5 2 2 7-7Z" />
        </template>
        <template v-else-if="kind === 'document'">
          <rect x="15" y="21" width="18" height="3" rx="1.5" />
          <rect x="15" y="28" width="18" height="3" rx="1.5" />
          <rect x="15" y="35" width="12" height="3" rx="1.5" />
        </template>
        <template v-else>
          <rect x="15" y="24" width="18" height="3" rx="1.5" fill-opacity=".65" />
          <rect x="15" y="31" width="12" height="3" rx="1.5" fill-opacity=".65" />
        </template>
      </g>
    </template>
  </svg>
</template>

<style scoped>
.file-entry-icon {
  display: block;
  flex: 0 0 auto;
  overflow: visible;
  filter: drop-shadow(0 1px 1px rgb(22 34 49 / 12%));
}
</style>
