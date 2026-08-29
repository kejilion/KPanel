<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type {
  Compartment as CompartmentType,
  EditorState as EditorStateType,
  Extension,
} from '@codemirror/state'
import type { EditorView as EditorViewType } from '@codemirror/view'
import { loadCodeLanguage, type CodeLanguage } from '@/lib/code-editor-language'

const props = withDefaults(
  defineProps<{
    modelValue: string
    fileName: string
    mime?: string
    sizeBytes: number
    editable?: boolean
    lineWrap?: boolean
  }>(),
  {
    mime: '',
    editable: true,
    lineWrap: false,
  },
)

interface CodeEditorStatus {
  line: number
  column: number
  lines: number
}

const emit = defineEmits<{
  'update:modelValue': [value: string]
  dirty: []
  save: [value: string]
  status: [value: CodeEditorStatus]
  ready: [value: CodeLanguage & { loadMs: number }]
}>()

const host = ref<HTMLElement>()
const searchInput = ref<HTMLInputElement>()
const loading = ref(true)
const loadError = ref('')
const searchOpen = ref(false)
const replaceOpen = ref(false)
const searchValue = ref('')
const replaceValue = ref('')
const searchCaseSensitive = ref(false)
const searchRegexp = ref(false)
const searchWholeWord = ref(false)
const searchMessage = ref('')
let editor: EditorViewType | undefined
let lineWrapCompartment: CompartmentType | undefined
let lineWrappingExtension: Extension | undefined
let searchRuntime: Pick<
  typeof import('@codemirror/search'),
  'SearchQuery' | 'setSearchQuery' | 'findNext' | 'findPrevious' | 'replaceNext' | 'replaceAll'
> | undefined
let applyingExternalValue = false
let dirty = false
let cancelled = false

function currentValue(): string {
  return editor?.state.doc.toString() ?? props.modelValue
}

function markClean(): void {
  dirty = false
}

function emitStatus(state: EditorStateType): void {
  const head = state.selection.main.head
  const line = state.doc.lineAt(head)
  emit('status', {
    line: line.number,
    column: head - line.from + 1,
    lines: state.doc.lines,
  })
}

function requestSave(): void {
  const value = currentValue()
  emit('update:modelValue', value)
  emit('save', value)
}

function updateSearchQuery(): boolean {
  if (!editor || !searchRuntime) return false
  const query = new searchRuntime.SearchQuery({
    search: searchValue.value,
    replace: replaceValue.value,
    caseSensitive: searchCaseSensitive.value,
    regexp: searchRegexp.value,
    wholeWord: searchWholeWord.value,
  })
  editor.dispatch({ effects: searchRuntime.setSearchQuery.of(query) })
  if (!searchValue.value) {
    searchMessage.value = ''
    return false
  }
  searchMessage.value = query.valid ? '' : '表达式无效'
  return query.valid
}

function runSearch(direction: 'next' | 'previous'): void {
  if (!editor || !searchRuntime || !updateSearchQuery()) return
  const found = direction === 'next'
    ? searchRuntime.findNext(editor)
    : searchRuntime.findPrevious(editor)
  searchMessage.value = found ? '' : '无匹配'
}

function runReplace(all: boolean): void {
  if (!editor || !searchRuntime || !props.editable || !updateSearchQuery()) return
  const replaced = all
    ? searchRuntime.replaceAll(editor)
    : searchRuntime.replaceNext(editor)
  searchMessage.value = replaced ? '' : '无匹配'
}

function toggleSearchOption(option: 'case' | 'regexp' | 'word'): void {
  if (option === 'case') searchCaseSensitive.value = !searchCaseSensitive.value
  else if (option === 'regexp') searchRegexp.value = !searchRegexp.value
  else searchWholeWord.value = !searchWholeWord.value
  updateSearchQuery()
}

function openSearch(showReplace = false): void {
  if (!editor) return
  const selection = editor.state.selection.main
  const selectedText = selection.empty
    ? ''
    : editor.state.sliceDoc(selection.from, selection.to)
  if (!searchValue.value && selectedText && !/[\r\n]/.test(selectedText)) {
    searchValue.value = selectedText
  }
  searchOpen.value = true
  replaceOpen.value = replaceOpen.value || showReplace
  searchMessage.value = ''
  updateSearchQuery()
  void nextTick(() => {
    searchInput.value?.focus()
    searchInput.value?.select()
  })
}

function closeSearch(): void {
  searchOpen.value = false
  searchMessage.value = ''
  if (editor && searchRuntime) {
    editor.dispatch({
      effects: searchRuntime.setSearchQuery.of(new searchRuntime.SearchQuery({ search: '' })),
    })
    editor.focus()
  }
}

defineExpose({
  getValue: currentValue,
  markClean,
  openSearch,
  focus: () => editor?.focus(),
})

async function initialize(): Promise<void> {
  const startedAt = performance.now()
  try {
    const [
      { Compartment, EditorState },
      {
        EditorView,
        drawSelection,
        dropCursor,
        highlightActiveLine,
        highlightActiveLineGutter,
        highlightSpecialChars,
        keymap,
        lineNumbers,
      },
      { defaultKeymap, history, historyKeymap, indentWithTab },
      { bracketMatching, foldGutter, foldKeymap, HighlightStyle, indentOnInput, syntaxHighlighting },
      {
        findNext,
        findPrevious,
        highlightSelectionMatches,
        replaceAll,
        replaceNext,
        search,
        SearchQuery,
        setSearchQuery,
      },
      { tags },
      language,
    ] = await Promise.all([
      import('@codemirror/state'),
      import('@codemirror/view'),
      import('@codemirror/commands'),
      import('@codemirror/language'),
      import('@codemirror/search'),
      import('@lezer/highlight'),
      loadCodeLanguage(props.fileName, props.mime, props.sizeBytes),
    ])
    if (cancelled || !host.value) return

    const highlightStyle = HighlightStyle.define([
      { tag: tags.comment, color: 'var(--code-comment)' },
      { tag: [tags.keyword, tags.controlKeyword, tags.operatorKeyword], color: 'var(--code-keyword)' },
      { tag: [tags.string, tags.special(tags.string)], color: 'var(--code-string)' },
      { tag: [tags.number, tags.bool, tags.null], color: 'var(--code-number)' },
      { tag: [tags.function(tags.variableName), tags.definition(tags.variableName)], color: 'var(--code-function)' },
      { tag: [tags.typeName, tags.className], color: 'var(--code-type)' },
      { tag: tags.tagName, color: 'var(--code-tag)' },
      { tag: [tags.attributeName, tags.propertyName], color: 'var(--code-property)' },
      { tag: tags.invalid, color: 'var(--danger)', textDecoration: 'underline' },
    ])

    lineWrapCompartment = new Compartment()
    lineWrappingExtension = EditorView.lineWrapping
    searchRuntime = {
      SearchQuery,
      setSearchQuery,
      findNext,
      findPrevious,
      replaceNext,
      replaceAll,
    }
    const extensions = [
      lineNumbers(),
      ...(language.highlighted ? [foldGutter()] : []),
      highlightActiveLineGutter(),
      highlightSpecialChars(),
      history(),
      drawSelection(),
      dropCursor(),
      indentOnInput(),
      bracketMatching(),
      highlightActiveLine(),
      search({ top: true }),
      highlightSelectionMatches(),
      syntaxHighlighting(highlightStyle),
      EditorState.tabSize.of(2),
      EditorState.readOnly.of(!props.editable),
      EditorView.editable.of(props.editable),
      lineWrapCompartment.of(props.lineWrap ? lineWrappingExtension : []),
      EditorView.contentAttributes.of({ 'aria-label': '文件内容' }),
      EditorView.updateListener.of((update) => {
        if (update.docChanged && !applyingExternalValue && !dirty) {
          dirty = true
          emit('dirty')
        }
        if (update.docChanged || update.selectionSet) emitStatus(update.state)
      }),
      EditorView.theme({
        '&': {
          height: '100%',
          color: 'var(--code-text)',
          backgroundColor: 'var(--code-background)',
          fontSize: '13px',
        },
        '.cm-scroller': {
          overflow: 'auto',
          fontFamily: 'ui-monospace, SFMono-Regular, Consolas, monospace',
          lineHeight: '1.65',
        },
        '.cm-content': {
          minWidth: 'max-content',
          padding: '14px 0',
          caretColor: 'var(--code-caret)',
        },
        '.cm-cursor, .cm-dropCursor': {
          borderLeftColor: 'var(--code-caret)',
          borderLeftWidth: '2px',
        },
        '.cm-line': { padding: '0 16px' },
        '.cm-gutters': {
          minWidth: '52px',
          color: 'var(--code-line-number)',
          backgroundColor: 'var(--code-gutter)',
          border: '0',
        },
        '.cm-lineNumbers .cm-gutterElement': {
          minWidth: '46px',
          padding: '0 10px 0 6px',
        },
        '.cm-activeLine': { backgroundColor: 'var(--code-active-line)' },
        '.cm-activeLineGutter': {
          color: 'var(--code-text)',
          backgroundColor: 'var(--code-active-line)',
        },
        '&.cm-focused': { outline: 'none' },
        '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
          backgroundColor: 'var(--code-selection) !important',
        },
        '.cm-panels': {
          color: 'var(--code-text)',
          backgroundColor: 'var(--code-gutter)',
        },
        '.cm-panel.cm-search': {
          padding: '8px 10px',
        },
        '.cm-panel.cm-search input': {
          color: 'var(--code-text)',
          backgroundColor: 'var(--code-background)',
          border: '1px solid var(--code-border)',
          borderRadius: '6px',
        },
        '.cm-searchMatch': {
          backgroundColor: 'var(--code-search-match)',
          outline: '1px solid var(--code-search-match-border)',
        },
      }),
      keymap.of([
        {
          key: 'Mod-s',
          preventDefault: true,
          run: () => {
            requestSave()
            return true
          },
        },
        indentWithTab,
        {
          key: 'Mod-f',
          preventDefault: true,
          run: () => {
            openSearch()
            return true
          },
        },
        {
          key: 'Mod-h',
          preventDefault: true,
          run: () => {
            openSearch(true)
            return true
          },
        },
        {
          key: 'F3',
          run: () => {
            runSearch('next')
            return true
          },
        },
        {
          key: 'Shift-F3',
          run: () => {
            runSearch('previous')
            return true
          },
        },
        ...foldKeymap,
        ...defaultKeymap,
        ...historyKeymap,
      ]),
    ]
    if (language.extension) extensions.push(language.extension)

    editor = new EditorView({
      parent: host.value,
      state: EditorState.create({ doc: props.modelValue, extensions }),
    })
    editor.focus()
    emitStatus(editor.state)
    emit('ready', { ...language, loadMs: performance.now() - startedAt })
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '编辑器加载失败'
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  (value) => {
    if (!editor || editor.state.doc.toString() === value) return
    applyingExternalValue = true
    editor.dispatch({
      changes: { from: 0, to: editor.state.doc.length, insert: value },
    })
    applyingExternalValue = false
  },
)

watch(
  () => props.lineWrap,
  (enabled) => {
    if (!editor || !lineWrapCompartment || !lineWrappingExtension) return
    editor.dispatch({
      effects: lineWrapCompartment.reconfigure(enabled ? lineWrappingExtension : []),
    })
  },
)

onMounted(() => {
  void initialize()
})

onBeforeUnmount(() => {
  cancelled = true
  editor?.destroy()
  editor = undefined
})
</script>

<template>
  <div class="code-editor-shell">
    <div ref="host" class="code-editor-host" />
    <div
      v-if="searchOpen"
      class="code-search"
      role="dialog"
      aria-label="查找或替换"
      @keydown.esc.stop.prevent="closeSearch"
    >
      <div class="code-search__row">
        <button
          class="code-search__icon code-search__toggle"
          type="button"
          :title="replaceOpen ? '收起替换' : '展开替换'"
          :aria-label="replaceOpen ? '收起替换' : '展开替换'"
          :aria-expanded="replaceOpen"
          @click="replaceOpen = !replaceOpen"
        >
          <span aria-hidden="true">{{ replaceOpen ? '⌄' : '›' }}</span>
        </button>
        <div class="code-search__field">
          <input
            ref="searchInput"
            v-model="searchValue"
            type="text"
            placeholder="查找"
            aria-label="查找内容"
            spellcheck="false"
            @input="updateSearchQuery"
            @keydown.enter.exact.prevent="runSearch('next')"
            @keydown.shift.enter.prevent="runSearch('previous')"
          />
          <span v-if="searchMessage" class="code-search__message">{{ searchMessage }}</span>
          <button
            class="code-search__option"
            :class="{ 'is-active': searchCaseSensitive }"
            type="button"
            title="区分大小写"
            aria-label="区分大小写"
            :aria-pressed="searchCaseSensitive"
            @click="toggleSearchOption('case')"
          ><span aria-hidden="true">Aa</span></button>
          <button
            class="code-search__option"
            :class="{ 'is-active': searchWholeWord }"
            type="button"
            title="全字匹配"
            aria-label="全字匹配"
            :aria-pressed="searchWholeWord"
            @click="toggleSearchOption('word')"
          ><span aria-hidden="true">ab</span></button>
          <button
            class="code-search__option"
            :class="{ 'is-active': searchRegexp }"
            type="button"
            title="使用正则表达式"
            aria-label="使用正则表达式"
            :aria-pressed="searchRegexp"
            @click="toggleSearchOption('regexp')"
          ><span aria-hidden="true">.*</span></button>
        </div>
        <button class="code-search__icon" type="button" title="上一个（Shift+F3）" aria-label="上一个匹配" @click="runSearch('previous')">
          <span aria-hidden="true">↑</span>
        </button>
        <button class="code-search__icon" type="button" title="下一个（F3）" aria-label="下一个匹配" @click="runSearch('next')">
          <span aria-hidden="true">↓</span>
        </button>
        <button class="code-search__icon" type="button" title="关闭（Esc）" aria-label="关闭查找" @click="closeSearch">
          <span aria-hidden="true">×</span>
        </button>
      </div>
      <div v-if="replaceOpen" class="code-search__row code-search__replace">
        <span class="code-search__toggle" aria-hidden="true" />
        <div class="code-search__field">
          <input
            v-model="replaceValue"
            type="text"
            placeholder="替换"
            aria-label="替换内容"
            spellcheck="false"
            :disabled="!props.editable"
            @input="updateSearchQuery"
            @keydown.enter.prevent="runReplace(false)"
          />
        </div>
        <button class="code-search__icon" type="button" title="替换" aria-label="替换当前匹配" :disabled="!props.editable" @click="runReplace(false)">
          <span aria-hidden="true">↪</span>
        </button>
        <button class="code-search__icon" type="button" title="全部替换" aria-label="替换全部匹配" :disabled="!props.editable" @click="runReplace(true)">
          <span aria-hidden="true">⇉</span>
        </button>
        <span class="code-search__icon" aria-hidden="true" />
      </div>
    </div>
    <div v-if="loading" class="code-editor-state">正在加载代码编辑器…</div>
    <div v-else-if="loadError" class="code-editor-state code-editor-state--error">
      代码编辑器加载失败：{{ loadError }}
    </div>
  </div>
</template>

<style scoped>
.code-editor-shell {
  --code-background: var(--file-preview-background, var(--terminal-shell-background, #0b1214));
  --code-gutter: color-mix(in srgb, var(--file-preview-panel, var(--terminal-shell-panel, #111a1d)) 72%, var(--code-background) 28%);
  --code-panel: var(--file-preview-panel, var(--terminal-shell-panel, #111a1d));
  --code-text: var(--file-preview-text, var(--terminal-shell-text, #d8dddc));
  --code-caret: var(--file-preview-accent, var(--brand, #35cba6));
  --code-line-number: var(--file-preview-muted, var(--terminal-shell-muted, #8a9695));
  --code-active-line: var(--file-preview-active-line, rgb(53 203 166 / 8%));
  --code-selection: var(--file-preview-selection, rgb(53 203 166 / 27%));
  --code-border: var(--file-preview-border, var(--terminal-shell-border, #29383a));
  --code-comment: color-mix(in srgb, var(--code-line-number) 78%, var(--code-caret) 22%);
  --code-keyword: var(--violet);
  --code-string: var(--success);
  --code-number: var(--amber);
  --code-function: var(--blue);
  --code-type: color-mix(in srgb, var(--amber) 68%, var(--code-caret) 32%);
  --code-tag: var(--danger);
  --code-property: var(--code-caret);
  --code-search-match: color-mix(in srgb, var(--amber) 22%, transparent);
  --code-search-match-border: color-mix(in srgb, var(--amber) 62%, transparent);
  --scrollbar-track: var(--code-background);
  --scrollbar-thumb: var(--file-preview-scrollbar, var(--terminal-shell-scrollbar, #35474a));
  --scrollbar-thumb-hover: var(--file-preview-scrollbar-hover, var(--terminal-shell-scrollbar-hover, #506367));
  --scrollbar-thumb-active: var(--code-caret);
  position: relative;
  height: 100%;
  overflow: hidden;
  background: var(--code-background);
  box-shadow: inset 0 0 0 1px var(--code-border);
}

.code-editor-host {
  height: 100%;
}

.code-search {
  position: absolute;
  z-index: 5;
  top: 10px;
  right: 14px;
  width: min(430px, calc(100% - 28px));
  padding: 6px;
  border: 1px solid var(--code-border);
  border-radius: 8px;
  color: var(--code-text);
  background: var(--code-panel);
  box-shadow: 0 10px 28px rgb(0 0 0 / 38%);
}

.code-search__row {
  display: grid;
  grid-template-columns: 24px minmax(130px, 1fr) 26px 26px 26px;
  align-items: center;
  gap: 3px;
}

.code-search__replace {
  margin-top: 4px;
}

.code-search__field {
  display: flex;
  min-width: 0;
  height: 27px;
  align-items: center;
  gap: 1px;
  padding-right: 2px;
  overflow: hidden;
  border: 1px solid var(--code-border);
  border-radius: 5px;
  background: var(--code-background);
}

.code-search__field:focus-within {
  border-color: var(--code-caret);
  box-shadow: 0 0 0 1px var(--code-caret);
}

.code-search__field input {
  min-width: 0;
  height: 100%;
  flex: 1;
  padding: 0 7px;
  border: 0;
  outline: 0;
  color: var(--code-text);
  background: transparent;
  font: 12px/1.4 ui-monospace, SFMono-Regular, Consolas, monospace;
}

.code-search__field input::placeholder {
  color: var(--code-line-number);
}

.code-search__message {
  flex: 0 0 auto;
  color: var(--danger, #ef7a7a);
  font-size: 10px;
  white-space: nowrap;
}

.code-search__icon,
.code-search__option {
  display: inline-grid;
  width: 26px;
  height: 26px;
  place-items: center;
  padding: 0;
  border: 0;
  border-radius: 4px;
  color: var(--code-line-number);
  background: transparent;
  cursor: pointer;
}

.code-search__toggle {
  width: 24px;
}

.code-search__icon:hover:not(:disabled),
.code-search__option:hover,
.code-search__option.is-active {
  color: var(--code-text);
  background: color-mix(in srgb, var(--code-caret) 14%, var(--code-panel));
}

.code-search__option.is-active {
  color: var(--file-preview-accent-strong, var(--brand-strong, #5adaba));
}

.code-search__icon:disabled {
  opacity: 0.35;
  cursor: default;
}

@media (max-width: 620px) {
  .code-search {
    top: 7px;
    right: 7px;
    width: calc(100% - 14px);
  }

  .code-search__row {
    grid-template-columns: 22px minmax(110px, 1fr) 26px 26px 26px;
  }
}

.code-editor-state {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  color: var(--code-line-number);
  background: var(--code-background);
}

.code-editor-state--error {
  color: var(--danger);
}
</style>
