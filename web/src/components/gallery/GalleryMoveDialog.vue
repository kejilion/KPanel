<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { ChevronRight, Folder, FolderInput, FolderPlus, Images, Loader2 } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { galleryAlbumNameProblem, galleryPathTrail } from '@/lib/gallery'

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

export interface GalleryMoveFolder {
  name: string
  path: string
}

const props = defineProps<{
  open: boolean
  /** What is being moved, already worded for the reader: a file name or "N 个文件". */
  summary: string
  /** Where the dialog starts, and the highest folder it lets you climb to. */
  start: string
  root: string
  /** Folders the files are in now; moving them to one of these changes nothing. */
  sourceFolders: string[]
  /** The gallery's own folder, shown as "图库" rather than by its name. */
  libraryRoot?: string
  busy?: boolean
  listFolders: (path: string) => Promise<GalleryMoveFolder[]>
  createFolder: (parent: string, name: string) => Promise<void>
}>()

const emit = defineEmits<{
  close: []
  confirm: [destination: string]
}>()

const path = ref(props.start)
const folders = ref<GalleryMoveFolder[]>([])
const loading = ref(false)
const failed = ref('')
const creating = ref(false)
const newName = ref('')
const createBusy = ref(false)
const createError = ref('')
const nameInput = ref<HTMLInputElement>()
let sequence = 0

const trail = computed(() => galleryPathTrail(props.root, path.value))
const unchanged = computed(() => props.sourceFolders.length > 0 && props.sourceFolders.every((folder) => folder === path.value))
const nameProblem = computed(() => (
  creating.value && newName.value ? galleryAlbumNameProblem(newName.value, folders.value.map((folder) => folder.name)) : undefined
))
const here = computed(() => (props.libraryRoot && path.value === props.libraryRoot ? phrase('图库') : trail.value.at(-1)?.name ?? path.value))

function segmentLabel(index: number, name: string): string {
  return index === 0 && props.libraryRoot === props.root ? phrase('图库') : name
}

async function openFolder(next: string): Promise<void> {
  path.value = next
  creating.value = false
  newName.value = ''
  createError.value = ''
  const current = ++sequence
  loading.value = true
  failed.value = ''
  try {
    const list = await props.listFolders(next)
    if (current !== sequence) return
    folders.value = list
  } catch (error) {
    if (current !== sequence) return
    folders.value = []
    failed.value = error instanceof Error ? error.message : phrase('操作未完成，请稍后重试。')
  } finally {
    if (current === sequence) loading.value = false
  }
}

async function startCreating(): Promise<void> {
  creating.value = true
  createError.value = ''
  await nextTick()
  nameInput.value?.focus()
}

async function submitCreate(): Promise<void> {
  const name = newName.value.trim()
  if (!name || nameProblem.value || createBusy.value) return
  createBusy.value = true
  createError.value = ''
  const current = sequence
  const parent = path.value
  try {
    await props.createFolder(parent, name)
    if (current !== sequence || !props.open) return
    await openFolder(`${parent === '/' ? '' : parent}/${name}`)
  } catch (error) {
    if (current !== sequence || !props.open) return
    createError.value = error instanceof Error ? error.message : phrase('操作未完成，请稍后重试。')
  } finally {
    createBusy.value = false
  }
}

watch(() => props.open, (open) => {
  if (open) void openFolder(props.start)
  else sequence += 1
}, { immediate: true })
onBeforeUnmount(() => { sequence += 1 })
</script>

<template>
  <ModalDialog
    :open="open"
    :title="phrase('移动到相册')"
    :description="phrase('照片和视频会移到所选文件夹，名称相同的文件不会被覆盖。')"
    size="small"
    :close-disabled="busy"
    @close="emit('close')"
  >
    <div class="gallery-move">
      <p class="gallery-move__summary"><FolderInput :size="16" aria-hidden="true" /> {{ summary }}</p>

      <nav class="gallery-move__trail" :aria-label="phrase('目标位置')">
        <template v-for="(segment, index) in trail" :key="segment.path">
          <ChevronRight v-if="index > 0" :size="13" aria-hidden="true" />
          <button
            v-if="index < trail.length - 1"
            type="button"
            class="gallery-move__crumb"
            @click="openFolder(segment.path)"
          >
            <Images v-if="index === 0 && libraryRoot === root" :size="14" aria-hidden="true" />
            {{ segmentLabel(index, segment.name) }}
          </button>
          <span v-else class="gallery-move__crumb gallery-move__crumb--current" aria-current="page">
            <Images v-if="index === 0 && libraryRoot === root" :size="14" aria-hidden="true" />
            {{ segmentLabel(index, segment.name) }}
          </span>
        </template>
      </nav>

      <div class="gallery-move__list" role="list" :aria-busy="loading">
        <p v-if="loading" class="gallery-move__state" role="status"><Loader2 :size="15" class="gallery-move__spin" aria-hidden="true" /> {{ phrase('正在读取文件夹…') }}</p>
        <p v-else-if="failed" class="gallery-move__state gallery-move__state--error" role="alert">
          {{ phrase('文件夹读取失败') }}：{{ failed }}
          <button type="button" class="gallery-move__retry" @click="openFolder(path)">{{ phrase('重试') }}</button>
        </p>
        <p v-else-if="!folders.length" class="gallery-move__state">{{ phrase('这里没有子相册，可以直接移到这里，或新建一个。') }}</p>
        <template v-else>
          <button
            v-for="folder in folders"
            :key="folder.path"
            type="button"
            role="listitem"
            class="gallery-move__folder"
            :disabled="busy"
            @click="openFolder(folder.path)"
          >
            <Folder :size="17" aria-hidden="true" />
            <span>{{ folder.name }}</span>
            <ChevronRight :size="15" aria-hidden="true" />
          </button>
        </template>
      </div>

      <form v-if="creating" class="gallery-move__create" @submit.prevent="submitCreate">
        <input
          ref="nameInput"
          v-model="newName"
          class="text-input"
          type="text"
          maxlength="120"
          autocomplete="off"
          :placeholder="phrase('新相册名称')"
          :aria-label="phrase('新相册名称')"
          :aria-invalid="Boolean(newName && nameProblem)"
          @keydown.esc.stop.prevent="creating = false"
        />
        <button class="button button--secondary button--small" type="submit" :disabled="createBusy || !newName.trim() || Boolean(nameProblem)">
          {{ phrase(createBusy ? '正在保存…' : '创建') }}
        </button>
        <button class="button button--secondary button--small" type="button" :disabled="createBusy" @click="creating = false">{{ phrase('取消') }}</button>
        <p v-if="nameProblem || createError" class="gallery-move__error" role="alert">{{ nameProblem ? phrase(nameProblem) : createError }}</p>
      </form>

      <p v-if="unchanged" class="gallery-move__hint">{{ phrase('文件已经在这个相册里') }}</p>

      <div class="gallery-move__footer">
        <button v-if="!creating" type="button" class="button button--secondary button--small" :disabled="loading || busy" @click="startCreating">
          <FolderPlus :size="16" /> {{ phrase('新建相册') }}
        </button>
        <span class="gallery-move__spacer" />
        <button class="button button--secondary" type="button" :disabled="busy" @click="emit('close')">{{ phrase('取消') }}</button>
        <button
          class="button button--primary"
          type="button"
          :disabled="busy || loading || Boolean(failed) || unchanged"
          @click="emit('confirm', path)"
        >
          {{ busy ? phrase('正在移动…') : phrase(`移到「${here}」`) }}
        </button>
      </div>
    </div>
  </ModalDialog>
</template>

<style scoped>
.gallery-move {
  display: grid;
  gap: 12px;
  min-width: 0;
}

.gallery-move__summary {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  margin: 0;
  color: var(--muted);
  font-size: 14px;
  overflow-wrap: anywhere;
}

.gallery-move__trail {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  align-items: center;
  gap: 2px 4px;
  color: var(--muted);
  font-size: 13px;
}

.gallery-move__crumb {
  display: inline-flex;
  min-height: 28px;
  align-items: center;
  gap: 5px;
  padding: 0 6px;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: inherit;
  font: inherit;
  cursor: pointer;
}

button.gallery-move__crumb:hover:not(:disabled) {
  background: var(--surface-raised);
  color: var(--text);
}

.gallery-move__crumb--current {
  color: var(--text);
  font-weight: 600;
  cursor: default;
}

.gallery-move__list {
  display: grid;
  max-height: min(320px, 44vh);
  align-content: start;
  gap: 2px;
  overflow-y: auto;
  padding: 4px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface-subtle);
}

.gallery-move__folder {
  display: flex;
  min-height: 40px;
  align-items: center;
  gap: 10px;
  padding: 0 10px;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--text);
  font: inherit;
  font-size: 14px;
  text-align: left;
  cursor: pointer;
}

.gallery-move__folder > span {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gallery-move__folder > svg:last-child {
  color: var(--muted);
}

.gallery-move__folder:hover:not(:disabled) {
  background: var(--surface-raised);
}

.gallery-move__folder:focus-visible,
.gallery-move__crumb:focus-visible {
  outline: 2px solid var(--brand);
  outline-offset: -2px;
}

.gallery-move__state {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  padding: 14px 10px;
  color: var(--muted);
  font-size: 14px;
}

.gallery-move__state--error {
  flex-wrap: wrap;
  color: var(--danger);
}

.gallery-move__spin {
  animation: gallery-move-spin 900ms linear infinite;
}

.gallery-move__create {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.gallery-move__create .text-input {
  min-width: 0;
  flex: 1 1 160px;
}

.gallery-move__error {
  flex: 1 1 100%;
  margin: 0;
  color: var(--danger, #c2410c);
  font-size: 13px;
}

.gallery-move__retry {
  min-height: 28px;
  padding: 0 6px;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--brand);
  font: inherit;
  font-weight: 600;
  cursor: pointer;
}

.gallery-move__retry:hover {
  background: var(--brand-soft);
}

.gallery-move__hint {
  margin: -4px 0 0;
  color: var(--muted);
  font-size: 13px;
}

.gallery-move__footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.gallery-move__spacer {
  flex: 1;
}

.gallery-move__footer .button--primary {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@keyframes gallery-move-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .gallery-move__spin {
    animation: none;
  }
}
</style>
