<script setup lang="ts">
import { computed, markRaw, nextTick, onBeforeUnmount, onMounted, ref, useId, type Raw } from 'vue'
import {
  ArrowUp,
  Check,
  Code2,
  FileCode,
  Folder,
  PanelLeft,
  RefreshCw,
  Save,
  Search,
  WrapText,
  X,
} from '@lucide/vue'
import CodeEditor from './CodeEditor.vue'
import { fileAPIForHost } from '@/lib/fileHostContext'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import type { CodeEditorSession } from '@/lib/code-editor-session'
import type { FileDirectory, FileEntry } from '@/types/api'

const props = defineProps<{ entry: FileEntry; content: string; hostId: string }>()
const emit = defineEmits<{
  dirty: [value: boolean]
  saving: [value: boolean]
  saved: [entry: FileEntry]
  close: []
}>()
const files = fileAPIForHost(props.hostId)
const MAX_TABS = 12
const MAX_ENTRIES = 500
interface EditorTab {
  entry: FileEntry
  content: string
  session?: Raw<CodeEditorSession>
  dirty: boolean
  revision: number
  saving: boolean
  loading: boolean
  error: string
  lineWrap: boolean
}
function createTab(entry: FileEntry, content = '', loading = false): EditorTab {
  return {
    entry,
    content,
    dirty: false,
    revision: 0,
    saving: false,
    loading,
    error: '',
    lineWrap: false,
  }
}
const tabs = ref<EditorTab[]>([createTab(props.entry, props.content)])
const activePath = ref(props.entry.path)
const active = computed(() => tabs.value.find((tab) => tab.entry.path === activePath.value)!)
const editor = ref<InstanceType<typeof CodeEditor>>()
const editorPath = ref('')
const editorReady = computed(
  () => editorPath.value === activePath.value && Boolean(editor.value) && !active.value.loading,
)
const status = ref({ line: 1, column: 1, lines: 1 })
const language = ref('')
const root = ref<HTMLElement>()
const tabList = ref<HTMLElement>()
const sidebarToggle = ref<HTMLButtonElement>()
const sidebarSearch = ref<HTMLInputElement>()
const compact = ref(false)
const sidebarOpen = ref(true)
const directory = ref<FileDirectory>()
const directoryPath = ref(parentPath(props.entry.path))
const directoryLoading = ref(false)
const directoryError = ref('')
const query = ref('')
const notice = ref('')
const id = `file-editor-${useId()}`
let directoryRequest = 0
let directoryController: AbortController | undefined
let searchTimer: ReturnType<typeof setTimeout> | undefined
let observer: ResizeObserver | undefined
let disposed = false

function phrase(text: string): string {
  phraseCatalogVersion.value
  return translatePhrase(text)
}
function parentPath(path: string): string {
  return path.slice(0, path.lastIndexOf('/')) || '/'
}
function errorText(error: unknown): string {
  return error instanceof Error ? error.message : phrase('请求失败，请重试。')
}
function publishState(): void {
  emit(
    'dirty',
    tabs.value.some((tab) => tab.dirty),
  )
  emit(
    'saving',
    tabs.value.some((tab) => tab.saving),
  )
}
function capture(): void {
  const session = editor.value?.getSession()
  const tab = tabs.value.find((item) => item.entry.path === editorPath.value)
  if (session && tab) tab.session = markRaw(session)
}
function focusTab(): void {
  void nextTick(() => {
    const button = tabList.value?.querySelector<HTMLButtonElement>('[aria-selected="true"]')
    button?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
    button?.focus({ preventScroll: true })
  })
}
function selectTab(tab: EditorTab, keyboard = false): void {
  if (tab.entry.path !== activePath.value) {
    capture()
    language.value = ''
    status.value = { line: 1, column: 1, lines: 1 }
    activePath.value = tab.entry.path
  }
  notice.value = ''
  if (compact.value) sidebarOpen.value = false
  if (keyboard || compact.value) focusTab()
  else
    void nextTick(() =>
      tabList.value
        ?.querySelector('[aria-selected="true"]')
        ?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' }),
    )
}
function tabsKeydown(event: KeyboardEvent): void {
  if (!(event.target as HTMLElement).matches('[role="tab"]')) return
  const index = tabs.value.indexOf(active.value)
  const target =
    event.key === 'ArrowRight'
      ? (index + 1) % tabs.value.length
      : event.key === 'ArrowLeft'
        ? (index - 1 + tabs.value.length) % tabs.value.length
        : event.key === 'Home'
          ? 0
          : event.key === 'End'
            ? tabs.value.length - 1
            : -1
  if (target >= 0) {
    event.preventDefault()
    selectTab(tabs.value[target]!, true)
  } else if (event.key === 'Delete') {
    event.preventDefault()
    closeTab(active.value)
  }
}
function closeTab(tab: EditorTab): void {
  if (tab.saving) return
  if (tab.dirty && !window.confirm(phrase(`放弃 ${tab.entry.name} 的未保存修改并关闭？`))) return
  const index = tabs.value.indexOf(tab)
  if (tabs.value.length === 1) {
    tab.dirty = false
    publishState()
    emit('close')
    return
  }
  if (tab === active.value) selectTab(tabs.value[index + 1] || tabs.value[index - 1]!, true)
  tabs.value.splice(index, 1)
  publishState()
  focusTab()
}
async function loadTab(tab: EditorTab): Promise<void> {
  tab.loading = true
  tab.error = ''
  try {
    const content = await files.text(tab.entry.path)
    if (!disposed && tabs.value.includes(tab)) tab.content = content
  } catch (error) {
    if (!disposed && tabs.value.includes(tab)) tab.error = errorText(error)
  } finally {
    if (!disposed) tab.loading = false
  }
}
async function openFile(entry: FileEntry): Promise<void> {
  if (entry.kind === 'directory') {
    await browse(entry.path)
    return
  }
  const existing = tabs.value.find((tab) => tab.entry.path === entry.path)
  if (existing) {
    selectTab(existing)
    return
  }
  if (!entry.editable) return
  if (tabs.value.length >= MAX_TABS) {
    notice.value = phrase('最多打开 12 个文件，请先关闭一个标签。')
    return
  }
  tabs.value.push(createTab(entry, '', true))
  const tab = tabs.value[tabs.value.length - 1]!
  selectTab(tab)
  await loadTab(tab)
}
function changed(): void {
  active.value.dirty = true
  active.value.revision += 1
  publishState()
}
function ready(info: { label: string }): void {
  editorPath.value = activePath.value
  language.value = info.label
}
async function save(): Promise<void> {
  const tab = active.value
  if (!editorReady.value || !tab.dirty || tab.saving || !editor.value) return
  const content = editor.value.getValue()
  const revision = tab.revision
  tab.saving = true
  tab.error = ''
  publishState()
  try {
    const result = await files.write(tab.entry.path, content, tab.entry.resourceVersion)
    if (disposed) return
    tab.entry = result.entry
    const listed = directory.value?.entries.findIndex((entry) => entry.path === result.entry.path)
    if (directory.value && listed !== undefined && listed >= 0) {
      directory.value.entries[listed] = result.entry
    }
    if (tab.revision === revision) {
      tab.dirty = false
      if (active.value === tab) editor.value?.markClean()
    }
    emit('saved', result.entry)
  } catch (error) {
    if (!disposed) tab.error = errorText(error)
  } finally {
    if (!disposed) {
      tab.saving = false
      publishState()
    }
  }
}
async function loadDirectory(path = directoryPath.value, append = false): Promise<void> {
  const request = ++directoryRequest
  directoryController?.abort()
  directoryController = new AbortController()
  directoryLoading.value = true
  directoryError.value = ''
  try {
    const result = await files.list(
      path,
      { search: query.value.trim(), offset: append ? directory.value?.nextOffset : 0 },
      directoryController.signal,
    )
    if (disposed || request !== directoryRequest) return
    directory.value = {
      ...result,
      entries: (append
        ? [...(directory.value?.entries || []), ...result.entries]
        : result.entries
      ).slice(0, MAX_ENTRIES),
    }
    directoryPath.value = result.path
  } catch (error) {
    if (!disposed && request === directoryRequest) directoryError.value = errorText(error)
  } finally {
    if (!disposed && request === directoryRequest) directoryLoading.value = false
  }
}
async function browse(path: string): Promise<void> {
  clearTimeout(searchTimer)
  query.value = ''
  await loadDirectory(path)
}
function searchDirectory(): void {
  clearTimeout(searchTimer)
  directoryController?.abort()
  directoryRequest += 1
  searchTimer = setTimeout(() => {
    void loadDirectory()
  }, 220)
}
const entries = computed(() =>
  [...(directory.value?.entries || [])].sort(
    (a, b) =>
      Number(b.kind === 'directory') - Number(a.kind === 'directory') ||
      a.name.localeCompare(b.name),
  ),
)
function toggleSidebar(): void {
  sidebarOpen.value = !sidebarOpen.value
  if (sidebarOpen.value && compact.value) void nextTick(() => sidebarSearch.value?.focus())
}
function dismissSidebar(): void {
  sidebarOpen.value = false
  sidebarToggle.value?.focus({ preventScroll: true })
}
function beforeUnload(event: BeforeUnloadEvent): void {
  if (tabs.value.some((tab) => tab.dirty || tab.saving)) {
    event.preventDefault()
    event.returnValue = ''
  }
}
onMounted(() => {
  void loadDirectory()
  if (typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver(([entry]) => {
      if (!entry) return
      const next = entry.contentRect.width < 700
      if (next !== compact.value) {
        compact.value = next
        sidebarOpen.value = !next
      }
    })
    if (root.value) observer.observe(root.value)
  }
  window.addEventListener('beforeunload', beforeUnload)
})
onBeforeUnmount(() => {
  disposed = true
  directoryController?.abort()
  clearTimeout(searchTimer)
  observer?.disconnect()
  window.removeEventListener('beforeunload', beforeUnload)
})
defineExpose({ openFile })
</script>

<template>
  <section
    ref="root"
    class="editor-workspace"
    :class="{ 'is-compact': compact }"
    :aria-label="phrase('文件编辑器')"
    @keydown.ctrl.s.prevent.stop="save"
    @keydown.meta.s.prevent.stop="save"
  >
    <header class="editor-toolbar">
      <button
        ref="sidebarToggle"
        class="editor-tool"
        :class="{ 'is-active': sidebarOpen }"
        type="button"
        :title="phrase('文件列表')"
        :aria-label="phrase('文件列表')"
        :aria-expanded="sidebarOpen"
        :aria-controls="`${id}-files`"
        @click="toggleSidebar"
      >
        <PanelLeft :size="18" />
      </button>
      <span class="editor-toolbar__path" :title="active.entry.path" data-i18n-ignore>{{
        active.entry.path
      }}</span>
      <button
        class="editor-tool"
        type="button"
        :title="phrase('查找或替换（Ctrl+F）')"
        :aria-label="phrase('查找或替换')"
        :disabled="!editorReady"
        @click="editor?.openSearch()"
      >
        <Search :size="17" />
      </button>
      <button
        class="editor-tool"
        :class="{ 'is-active': active.lineWrap }"
        type="button"
        :title="phrase('切换自动换行')"
        :aria-label="phrase('切换自动换行')"
        :aria-pressed="active.lineWrap"
        @click="active.lineWrap = !active.lineWrap"
      >
        <WrapText :size="18" />
      </button>
      <button
        class="button button--primary editor-save"
        type="button"
        :disabled="!editorReady || !active.dirty || active.saving"
        title="Ctrl / ⌘ + S"
        @click="save"
      >
        <Save :size="16" />{{ phrase(active.saving ? '保存中…' : '保存') }}
      </button>
    </header>
    <div class="editor-workspace__body">
      <button
        v-if="compact && sidebarOpen"
        class="editor-sidebar-backdrop"
        type="button"
        :aria-label="phrase('收起文件列表')"
        @click="dismissSidebar"
      />
      <aside
        v-if="sidebarOpen"
        :id="`${id}-files`"
        class="editor-sidebar"
        :aria-label="phrase('文件列表')"
        @keydown.esc.stop.prevent="dismissSidebar"
      >
        <header class="editor-sidebar__header">
          <strong>{{ phrase('文件') }}</strong
          ><button
            class="editor-tool"
            type="button"
            :disabled="directoryLoading || directoryPath === '/'"
            :aria-label="phrase('上级目录')"
            :title="phrase('上级目录')"
            @click="browse(parentPath(directoryPath))"
          >
            <ArrowUp :size="16" /></button
          ><button
            class="editor-tool"
            type="button"
            :disabled="directoryLoading"
            :aria-label="phrase('刷新文件列表')"
            :title="phrase('刷新文件列表')"
            @click="loadDirectory()"
          >
            <RefreshCw :size="16" :class="{ spinning: directoryLoading }" /></button
          ><button
            v-if="compact"
            class="editor-tool"
            type="button"
            :aria-label="phrase('收起文件列表')"
            @click="dismissSidebar"
          >
            <X :size="17" />
          </button>
        </header>
        <div class="editor-sidebar__path" :title="directoryPath" data-i18n-ignore>
          {{ directoryPath }}
        </div>
        <label class="editor-filter"
          ><Search :size="15" /><input
            ref="sidebarSearch"
            v-model="query"
            :placeholder="phrase('搜索当前目录')"
            :aria-label="phrase('搜索当前目录')"
            @input="searchDirectory"
        /></label>
        <div v-if="directoryError" class="editor-message" role="alert">
          <span>{{ directoryError }}</span
          ><button type="button" @click="loadDirectory()">{{ phrase('重试') }}</button>
        </div>
        <div class="editor-file-list" :aria-busy="directoryLoading">
          <div v-if="directoryLoading && !directory" class="editor-empty" role="status">
            {{ phrase('正在读取目录…') }}
          </div>
          <div
            v-else-if="!entries.length && !directoryLoading && !directoryError"
            class="editor-empty"
          >
            {{ phrase('没有匹配的文件') }}
          </div>
          <button
            v-for="entry in entries"
            :key="entry.path"
            class="editor-file"
            :class="{ 'is-active': entry.path === activePath }"
            type="button"
            :disabled="entry.kind !== 'directory' && !entry.editable"
            :aria-current="entry.path === activePath ? 'true' : undefined"
            :title="
              entry.editable || entry.kind === 'directory'
                ? entry.name
                : phrase('此文件不支持文本编辑')
            "
            @click="openFile(entry)"
          >
            <Folder v-if="entry.kind === 'directory'" :size="17" /><FileCode
              v-else
              :size="17"
            /><span data-i18n-ignore>{{ entry.name }}</span
            ><span
              v-if="tabs.some((tab) => tab.entry.path === entry.path && tab.dirty)"
              class="editor-dirty"
              :aria-label="phrase('未保存')"
            />
          </button>
          <button
            v-if="directory?.nextOffset !== undefined && entries.length < MAX_ENTRIES"
            class="editor-load-more"
            type="button"
            :disabled="directoryLoading"
            @click="loadDirectory(directoryPath, true)"
          >
            {{ phrase(directoryLoading ? '正在读取目录…' : '加载更多') }}
          </button>
          <p
            v-if="
              directory?.scanTruncated ||
              (directory?.nextOffset !== undefined && entries.length >= MAX_ENTRIES)
            "
            class="editor-empty"
          >
            {{ phrase('目录较大，请搜索文件名缩小范围。') }}
          </p>
        </div>
      </aside>
      <main class="editor-main" :inert="compact && sidebarOpen">
        <div
          ref="tabList"
          class="editor-tabs"
          role="tablist"
          :aria-label="phrase('已打开的文件')"
          @keydown="tabsKeydown"
        >
          <div
            v-for="(tab, index) in tabs"
            :key="tab.entry.path"
            class="editor-tab"
            :class="{ 'is-active': tab === active }"
          >
            <button
              :id="`${id}-tab-${index}`"
              class="editor-tab__select"
              type="button"
              role="tab"
              :aria-selected="tab === active"
              :aria-controls="`${id}-content`"
              :tabindex="tab === active ? 0 : -1"
              :title="tab.entry.path"
              @click="selectTab(tab)"
            >
              <Code2 :size="15" /><span data-i18n-ignore>{{ tab.entry.name }}</span
              ><span v-if="tab.dirty" class="editor-dirty" :aria-label="phrase('未保存')" />
            </button>
            <button
              class="editor-tab__close"
              type="button"
              :disabled="tab.saving"
              :aria-label="phrase(`关闭 ${tab.entry.name}`)"
              :title="phrase('关闭标签')"
              @click="closeTab(tab)"
            >
              <RefreshCw v-if="tab.saving || tab.loading" :size="14" class="spinning" /><X
                v-else
                :size="15"
              />
            </button>
          </div>
        </div>
        <div v-if="notice" class="editor-message" role="status">{{ notice }}</div>
        <div v-if="active.error" class="editor-message" role="alert">
          <span>{{ active.error }}</span
          ><button v-if="!active.dirty" type="button" @click="loadTab(active)">
            {{ phrase('重试') }}</button
          ><span v-else>{{ phrase('修改仍保留在此标签，请检查后重试保存。') }}</span>
        </div>
        <div
          :id="`${id}-content`"
          class="editor-canvas"
          role="tabpanel"
          :aria-labelledby="`${id}-tab-${tabs.indexOf(active)}`"
          :aria-busy="active.loading"
        >
          <div v-if="active.loading" class="editor-empty editor-empty--center" role="status">
            <RefreshCw :size="20" class="spinning" />{{ phrase('正在打开文件…') }}
          </div>
          <CodeEditor
            v-else-if="!active.error || active.dirty"
            :key="active.entry.path"
            ref="editor"
            :model-value="active.content"
            :file-name="active.entry.name"
            :mime="active.entry.mime"
            :size-bytes="active.entry.sizeBytes"
            :session="active.session"
            :line-wrap="active.lineWrap"
            :auto-focus="false"
            @change="changed"
            @save="save"
            @status="status = $event"
            @ready="ready"
          />
        </div>
        <footer class="editor-status">
          <span class="editor-status__position"
            >{{ phrase('行') }} {{ status.line }}<span> : {{ status.column }}</span
            ><span class="editor-status__extra">
              · {{ status.lines }} {{ phrase('行') }}</span
            ></span
          ><span class="editor-status__language"
            >{{ language || 'UTF-8' }}<span class="editor-status__extra"> · UTF-8</span></span
          ><span class="editor-status__saved" role="status"
            ><span v-if="active.dirty" class="editor-dirty" /><Check v-else :size="14" />{{
              phrase(active.saving ? '保存中…' : active.dirty ? '未保存' : '已保存')
            }}</span
          >
        </footer>
      </main>
    </div>
  </section>
</template>

<style scoped>
.editor-workspace {
  display: flex;
  flex-direction: column;
  height: min(72dvh, 780px);
  min-height: 280px;
  overflow: hidden;
  border: 1px solid var(--file-preview-border);
  border-radius: var(--radius);
  color: var(--file-preview-text);
  background: var(--file-preview-background);
}
.editor-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 54px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--file-preview-border);
  background: var(--file-preview-panel);
}
.editor-toolbar__path {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font:
    13px/1.5 ui-monospace,
    Consolas,
    monospace;
  color: var(--file-preview-muted);
}
.editor-tool {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  padding: 0;
  border: 0;
  border-radius: var(--radius-sm);
  color: var(--file-preview-muted);
  background: transparent;
  cursor: pointer;
}
.editor-tool:hover:not(:disabled),
.editor-tool.is-active {
  color: var(--file-preview-text);
  background: var(--file-preview-panel-raised);
}
.editor-workspace button:focus-visible,
.editor-workspace input:focus-visible {
  outline: 2px solid var(--file-preview-accent);
  outline-offset: -2px;
}
.editor-workspace button:disabled {
  opacity: 0.5;
  cursor: default;
}
.editor-save {
  flex: 0 0 auto;
  min-height: 36px;
  padding: 6px 12px;
  font-size: 14px;
}
.editor-workspace__body {
  position: relative;
  display: flex;
  flex: 1;
  min-height: 0;
}
.editor-sidebar {
  display: flex;
  flex-direction: column;
  flex: 0 0 220px;
  width: 220px;
  min-height: 0;
  border-right: 1px solid var(--file-preview-border);
  background: var(--file-preview-panel);
}
.editor-sidebar__header {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 6px 8px 0 14px;
}
.editor-sidebar__header strong {
  flex: 1;
  font-size: 14px;
  font-weight: 600;
}
.editor-sidebar__path {
  padding: 4px 14px 8px;
  color: var(--file-preview-muted);
  font-size: 13px;
  line-height: 1.5;
  overflow-wrap: anywhere;
  max-height: 80px;
  overflow: auto;
}
.editor-filter {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0 10px 10px;
  padding: 0 8px;
  border: 1px solid var(--file-preview-border);
  border-radius: var(--radius-sm);
  color: var(--file-preview-muted);
  background: var(--file-preview-background);
}
.editor-filter input {
  width: 100%;
  min-width: 0;
  height: 36px;
  border: 0;
  outline: 0;
  color: var(--file-preview-text);
  background: transparent;
  font-size: 14px;
}
.editor-file-list {
  flex: 1;
  min-height: 0;
  overflow: auto;
  overscroll-behavior: contain;
  padding: 0 6px 8px;
}
.editor-file {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 38px;
  padding: 8px;
  border: 0;
  border-radius: var(--radius-sm);
  color: var(--file-preview-text);
  background: transparent;
  text-align: left;
  font-size: 14px;
  cursor: pointer;
}
.editor-file svg {
  flex: 0 0 auto;
  color: var(--file-preview-muted);
}
.editor-file > span:first-of-type {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.editor-file:hover:not(:disabled),
.editor-file.is-active {
  background: var(--file-preview-panel-raised);
}
.editor-file.is-active {
  color: var(--file-preview-text);
  box-shadow: inset 2px 0 var(--file-preview-accent);
}
.editor-main {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  min-height: 0;
}
.editor-tabs {
  display: flex;
  flex: 0 0 auto;
  overflow-x: auto;
  overscroll-behavior-x: contain;
  border-bottom: 1px solid var(--file-preview-border);
  background: var(--file-preview-panel);
  scrollbar-width: thin;
}
.editor-tab {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  max-width: 250px;
  padding-right: 4px;
  border-right: 1px solid var(--file-preview-border);
  border-top: 2px solid transparent;
}
.editor-tab.is-active {
  border-top-color: var(--file-preview-accent);
  background: var(--file-preview-background);
}
.editor-tab__select {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  min-height: 42px;
  padding: 8px 10px;
  border: 0;
  color: var(--file-preview-muted);
  background: transparent;
  font-size: 14px;
  cursor: pointer;
}
.editor-tab.is-active .editor-tab__select {
  color: var(--file-preview-text);
}
.editor-tab__select > span:first-of-type {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.editor-tab__select svg {
  flex-shrink: 0;
}
.editor-tab__close {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  width: 28px;
  height: 32px;
  padding: 0;
  border: 0;
  border-radius: var(--radius-sm);
  color: var(--file-preview-muted);
  background: transparent;
  cursor: pointer;
}
.editor-tab__close:hover {
  color: var(--file-preview-text);
  background: var(--file-preview-panel-raised);
}
.editor-dirty {
  display: inline-block;
  flex: 0 0 auto;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--file-preview-accent);
}
.editor-canvas {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
.editor-status {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  min-height: 34px;
  padding: 6px 12px;
  border-top: 1px solid var(--file-preview-border);
  color: var(--file-preview-muted);
  background: var(--file-preview-panel);
  font-size: 13px;
  line-height: 1.5;
}
.editor-status__saved {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-left: auto;
}
.editor-message {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  max-height: 140px;
  overflow: auto;
  overflow-wrap: anywhere;
  border-bottom: 1px solid var(--file-preview-border);
  color: var(--file-preview-text);
  background: var(--file-preview-panel-raised);
  font-size: 14px;
  line-height: 1.5;
}
.editor-message button,
.editor-load-more {
  padding: 8px;
  border: 0;
  border-radius: var(--radius-sm);
  color: var(--file-preview-text);
  background: var(--file-preview-panel);
  font-size: 14px;
  cursor: pointer;
}
.editor-load-more {
  width: 100%;
}
.editor-empty {
  padding: 20px 12px;
  color: var(--file-preview-muted);
  font-size: 14px;
  line-height: 1.6;
}
.editor-empty--center {
  display: flex;
  height: 100%;
  align-items: center;
  justify-content: center;
  gap: 10px;
}
.editor-sidebar-backdrop {
  position: absolute;
  z-index: 2;
  inset: 0;
  border: 0;
  background: color-mix(in srgb, var(--file-preview-background) 60%, transparent);
}
.is-compact .editor-sidebar {
  position: absolute;
  z-index: 3;
  inset: 0 auto 0 0;
  width: min(290px, 88%);
  box-shadow: var(--file-preview-shadow);
}
.is-compact .editor-toolbar {
  gap: 4px;
  padding: 6px;
  flex-wrap: wrap;
}
.is-compact .editor-toolbar__path {
  order: 5;
  flex-basis: 100%;
  padding: 0 6px 2px;
}
.is-compact .editor-save {
  margin-left: auto;
  min-height: 40px;
}
.is-compact .editor-tool {
  width: 40px;
  height: 40px;
}
.is-compact .editor-file {
  min-height: 44px;
}
.is-compact .editor-filter input {
  min-height: 40px;
  font-size: 16px;
}
.is-compact .editor-tab__close {
  width: 36px;
  height: 42px;
}
.is-compact .editor-status {
  gap: 8px;
}
.is-compact .editor-status__extra {
  display: none;
}
:global(.modal-panel--fullscreen .editor-workspace) {
  height: 100%;
  min-height: 0;
}
@media (max-width: 620px) {
  .editor-workspace {
    height: 72dvh;
  }
  :global(.modal-panel--wide:has(.editor-workspace) .modal-panel__body) {
    padding: 8px;
  }
}
</style>
