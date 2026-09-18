<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { LoaderCircle, Pencil, Play, Plus, RefreshCw, Trash2, X } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { ApiError } from '@/lib/api'
import { useI18n } from '@/i18n'
import { useTerminalCommands } from '@/stores/terminalCommands'
import { useToast } from '@/stores/toast'
import type { TerminalQuickCommand } from '@/types/api'

const props = defineProps<{
  open: boolean
  disabled?: boolean
  mode?: 'interactive' | 'batch'
}>()

const emit = defineEmits<{
  close: []
  execute: [command: string]
}>()

const { t } = useI18n()
const toast = useToast()
const commandStore = useTerminalCommands()
// In batch mode clicking a name fills the command box instead of executing,
// so run/empty guidance must say "fill", not "run".
const actionPhraseKey = computed(() => props.mode === 'batch' ? 'terminal.quickCommandsFill' : 'terminal.quickCommandsRun')
const emptyDescriptionKey = computed(() => props.mode === 'batch' ? 'terminal.quickCommandsEmptyDescriptionBatch' : 'terminal.quickCommandsEmptyDescription')
const loadError = ref('')
const formOpen = ref(false)
const editingCommand = ref<TerminalQuickCommand>()
const name = ref('')
const command = ref('')
const formError = ref('')
const deleteTarget = ref<TerminalQuickCommand>()
let loadController: AbortController | undefined

const snapshot = computed(() => commandStore.commands.value)
const items = computed(() => snapshot.value.items)
const readOnly = computed(() => !snapshot.value.available)
const modalTitle = computed(() => editingCommand.value
  ? t('terminal.quickCommandsEditTitle')
  : t('terminal.quickCommandsAddTitle'))

async function reload(): Promise<void> {
  loadController?.abort()
  loadController = new AbortController()
  loadError.value = ''
  try {
    await commandStore.load(loadController.signal)
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') return
    loadError.value = t('terminal.quickCommandsLoadFailed')
  }
}

watch(() => props.open, (open) => {
  if (open) void reload()
  else {
    loadController?.abort()
    formOpen.value = false
    editingCommand.value = undefined
    deleteTarget.value = undefined
    name.value = ''
    command.value = ''
    formError.value = ''
  }
}, { immediate: true })

function beginAdd(): void {
  if (readOnly.value || commandStore.saving.value) return
  if (items.value.length >= 64) {
    toast.danger(t('terminal.quickCommandsLimitReached'))
    return
  }
  editingCommand.value = undefined
  name.value = ''
  command.value = ''
  formError.value = ''
  formOpen.value = true
}

function beginEdit(item: TerminalQuickCommand): void {
  if (readOnly.value || commandStore.saving.value) return
  editingCommand.value = item
  name.value = item.name
  command.value = item.command
  formError.value = ''
  formOpen.value = true
}

function closeForm(): void {
  if (commandStore.saving.value) return
  formOpen.value = false
  formError.value = ''
}

function normalizeDraft(): Pick<TerminalQuickCommand, 'name' | 'command'> | undefined {
  const normalizedName = name.value.trim()
  const normalizedCommand = command.value.replace(/\r\n?/g, '\n')
  if (!normalizedName) {
    formError.value = t('terminal.quickCommandsNameRequired')
    return undefined
  }
  if ([...normalizedName].length > 48 || /[\p{Cc}]/u.test(normalizedName)) {
    formError.value = t('terminal.quickCommandsNameInvalid')
    return undefined
  }
  if (!normalizedCommand.trim()) {
    formError.value = t('terminal.quickCommandsCommandRequired')
    return undefined
  }
  if (/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f]/.test(normalizedCommand)) {
    formError.value = t('terminal.quickCommandsCommandInvalid')
    return undefined
  }
  if (new TextEncoder().encode(normalizedCommand).byteLength > 8192) {
    formError.value = t('terminal.quickCommandsCommandTooLong')
    return undefined
  }
  return { name: normalizedName, command: normalizedCommand }
}

async function save(): Promise<void> {
  formError.value = ''
  const normalized = normalizeDraft()
  if (!normalized) return
  const editingID = editingCommand.value?.id
  try {
    await commandStore.mutate((draft) => {
      if (!editingID) {
        draft.items.push({ id: commandStore.generateCommandID(), ...normalized })
        return
      }
      const index = draft.items.findIndex((item) => item.id === editingID)
      if (index < 0) return false
      draft.items[index] = { id: editingID, ...normalized }
    })
    formOpen.value = false
    toast.success(t('terminal.quickCommandsSaved'))
  } catch (error) {
    formError.value = error instanceof ApiError && error.status === 409
      ? t('terminal.quickCommandsConflict')
      : t('terminal.quickCommandsSaveFailed')
  }
}

function confirmDelete(item: TerminalQuickCommand): void {
  if (readOnly.value || commandStore.saving.value) return
  deleteTarget.value = item
}

async function remove(): Promise<void> {
  const target = deleteTarget.value
  if (!target) return
  try {
    await commandStore.mutate((draft) => {
      const next = draft.items.filter((item) => item.id !== target.id)
      if (next.length === draft.items.length) return false
      draft.items = next
    })
    deleteTarget.value = undefined
    toast.success(t('terminal.quickCommandsDeleted'))
  } catch (error) {
    if (error instanceof ApiError && error.status === 409) {
      deleteTarget.value = undefined
      toast.danger(t('terminal.quickCommandsConflict'))
      return
    }
    toast.danger(t('terminal.quickCommandsSaveFailed'))
  }
}
</script>

<template>
  <aside
    v-if="open"
    id="terminal-quick-commands"
    class="terminal-quick-commands"
    :aria-label="t('terminal.quickCommandsTitle')"
  >
    <header class="terminal-quick-commands__header">
      <div>
        <strong>{{ t('terminal.quickCommandsTitle') }}</strong>
        <small>{{ t('terminal.quickCommandsCount', { count: items.length }) }}</small>
      </div>
      <div class="terminal-quick-commands__actions">
        <button
          type="button"
          :title="t('terminal.quickCommandsAdd')"
          :aria-label="t('terminal.quickCommandsAdd')"
          :disabled="readOnly || commandStore.saving.value"
          @click="beginAdd"
        >
          <Plus :size="17" />
        </button>
        <button
          type="button"
          :title="t('terminal.quickCommandsCollapse')"
          :aria-label="t('terminal.quickCommandsCollapse')"
          @click="emit('close')"
        >
          <X :size="17" />
        </button>
      </div>
    </header>

    <div v-if="commandStore.loading.value && !commandStore.loaded.value" class="terminal-quick-commands__state">
      <LoaderCircle class="spin" :size="20" />
      <span>{{ t('terminal.quickCommandsLoading') }}</span>
    </div>
    <div v-else-if="loadError" class="terminal-quick-commands__state" role="alert">
      <span>{{ loadError }}</span>
      <button type="button" class="terminal-quick-commands__retry" @click="reload">
        <RefreshCw :size="15" />{{ t('terminal.quickCommandsRetry') }}
      </button>
    </div>
    <div v-else-if="readOnly" class="terminal-quick-commands__state" role="alert">
      <strong>{{ t('terminal.quickCommandsUnavailableTitle') }}</strong>
      <span>{{ t('terminal.quickCommandsUnavailableMessage') }}</span>
    </div>
    <div v-else-if="!items.length" class="terminal-quick-commands__state terminal-quick-commands__state--empty">
      <span class="terminal-quick-commands__empty-icon"><Play :size="20" /></span>
      <strong>{{ t('terminal.quickCommandsEmptyTitle') }}</strong>
      <span>{{ t(emptyDescriptionKey) }}</span>
      <button type="button" class="terminal-quick-commands__add-first" @click="beginAdd">
        <Plus :size="15" />{{ t('terminal.quickCommandsAdd') }}
      </button>
    </div>
    <div v-else class="terminal-quick-commands__list">
      <article v-for="item in items" :key="item.id" class="terminal-quick-command">
        <button
          type="button"
          class="terminal-quick-command__run"
          :disabled="disabled"
          :title="t(actionPhraseKey, { name: item.name })"
          @click="emit('execute', item.command)"
        >
          <Play :size="14" fill="currentColor" />
          <span>{{ item.name }}</span>
        </button>
        <div class="terminal-quick-command__actions">
          <button
            type="button"
            :title="t('terminal.quickCommandsEdit')"
            :aria-label="t('terminal.quickCommandsEditNamed', { name: item.name })"
            :disabled="commandStore.saving.value"
            @click="beginEdit(item)"
          >
            <Pencil :size="14" />
          </button>
          <button
            type="button"
            :title="t('terminal.quickCommandsDelete')"
            :aria-label="t('terminal.quickCommandsDeleteNamed', { name: item.name })"
            :disabled="commandStore.saving.value"
            @click="confirmDelete(item)"
          >
            <Trash2 :size="14" />
          </button>
        </div>
      </article>
    </div>

    <ModalDialog
      :open="formOpen"
      :title="modalTitle"
      size="small"
      :close-disabled="commandStore.saving.value"
      @close="closeForm"
    >
      <form class="terminal-command-form" @submit.prevent="save">
        <label>
          <span>{{ t('terminal.quickCommandsName') }}</span>
          <input
            v-model="name"
            maxlength="48"
            autocomplete="off"
            :disabled="commandStore.saving.value"
            :placeholder="t('terminal.quickCommandsNamePlaceholder')"
            required
          />
        </label>
        <label>
          <span>{{ t('terminal.quickCommandsCommand') }}</span>
          <textarea
            v-model="command"
            rows="6"
            maxlength="8192"
            autocomplete="off"
            autocapitalize="off"
            spellcheck="false"
            :disabled="commandStore.saving.value"
            :placeholder="t('terminal.quickCommandsCommandPlaceholder')"
            required
          />
        </label>
        <p class="terminal-command-form__hint">{{ t('terminal.quickCommandsSecurityHint') }}</p>
        <p v-if="formError" class="terminal-command-form__error" role="alert">{{ formError }}</p>
      </form>
      <template #footer>
        <button class="button button--ghost" type="button" :disabled="commandStore.saving.value" @click="closeForm">
          {{ t('common.cancel') }}
        </button>
        <button class="button button--primary" type="button" :disabled="commandStore.saving.value" @click="save">
          {{ commandStore.saving.value ? t('common.saving') : t('terminal.quickCommandsSave') }}
        </button>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="Boolean(deleteTarget)"
      :title="t('terminal.quickCommandsDeleteTitle')"
      size="compact"
      :close-disabled="commandStore.saving.value"
      @close="deleteTarget = undefined"
    >
      <p class="terminal-command-delete-message">
        {{ t('terminal.quickCommandsDeleteMessage', { name: deleteTarget?.name || '' }) }}
      </p>
      <template #footer>
        <button class="button button--ghost" type="button" :disabled="commandStore.saving.value" @click="deleteTarget = undefined">
          {{ t('common.cancel') }}
        </button>
        <button class="button button--danger" type="button" :disabled="commandStore.saving.value" @click="remove">
          {{ t('terminal.quickCommandsDelete') }}
        </button>
      </template>
    </ModalDialog>
  </aside>
</template>

<style scoped>
.terminal-quick-commands { display:grid; min-width:0; min-height:0; grid-template-rows:auto minmax(0,1fr); overflow:hidden; border-left:1px solid var(--terminal-shell-border,#29383a); color:var(--terminal-shell-text,#d8dddc); background:var(--terminal-shell-panel,#111a1d); }
.terminal-quick-commands__header { display:flex; min-height:51px; align-items:center; justify-content:space-between; gap:10px; padding:8px 10px 8px 13px; border-bottom:1px solid var(--terminal-shell-border,#29383a); }
.terminal-quick-commands__header>div:first-child { display:grid; min-width:0; gap:2px; }
.terminal-quick-commands__header strong { overflow:hidden; font-size:14px; text-overflow:ellipsis; white-space:nowrap; }
.terminal-quick-commands__header small { color:var(--terminal-shell-muted,#8a9695); font-size:12px; }
.terminal-quick-commands__actions,.terminal-quick-command__actions { display:flex; flex:0 0 auto; gap:5px; }
.terminal-quick-commands__actions button,.terminal-quick-command__actions button { display:grid; width:30px; height:30px; place-items:center; border:1px solid var(--terminal-shell-border,#29383a); border-radius:var(--radius-sm); color:var(--terminal-shell-muted,#8a9695); background:var(--terminal-shell-background,#0b1214); cursor:pointer; }
.terminal-quick-commands button:hover:not(:disabled),.terminal-quick-commands button:focus-visible { border-color:var(--brand); color:var(--terminal-shell-text,#d8dddc); outline:none; }
.terminal-quick-commands button:focus-visible { box-shadow:0 0 0 2px color-mix(in srgb,var(--brand) 20%,transparent); }
.terminal-quick-commands button:disabled { cursor:not-allowed; opacity:.48; }
.terminal-quick-commands__list { min-height:0; overflow-y:auto; overscroll-behavior:contain; padding:8px; scrollbar-color:var(--terminal-shell-scrollbar,#35474a) var(--terminal-shell-panel,#111a1d); scrollbar-width:thin; }
.terminal-quick-command { display:flex; min-width:0; align-items:center; gap:5px; margin-bottom:5px; border:1px solid transparent; border-radius:var(--radius-sm); padding:4px; }
.terminal-quick-command:hover { border-color:color-mix(in srgb,var(--brand) 24%,var(--terminal-shell-border,#29383a)); background:color-mix(in srgb,var(--brand) 6%,transparent); }
.terminal-quick-command__run { display:flex; min-width:0; min-height:34px; flex:1; align-items:center; gap:9px; border:0; border-radius:var(--radius-sm); padding:6px 8px; color:var(--terminal-shell-text,#d8dddc); background:transparent; font:inherit; font-size:13px; text-align:left; cursor:pointer; }
.terminal-quick-command__run>svg { flex:0 0 auto; color:var(--brand); }
.terminal-quick-command__run>span { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.terminal-quick-command__actions { opacity:.72; }
.terminal-quick-command:hover .terminal-quick-command__actions,.terminal-quick-command:focus-within .terminal-quick-command__actions { opacity:1; }
.terminal-quick-command__actions button { width:28px; height:28px; }
.terminal-quick-commands__state { display:flex; min-height:0; flex-direction:column; align-items:center; justify-content:center; gap:10px; padding:24px 18px; color:var(--terminal-shell-muted,#8a9695); font-size:13px; line-height:1.5; text-align:center; }
.terminal-quick-commands__state strong { color:var(--terminal-shell-text,#d8dddc); }
.terminal-quick-commands__empty-icon { display:grid; width:42px; height:42px; place-items:center; border-radius:var(--radius-md); color:var(--brand); background:color-mix(in srgb,var(--brand) 11%,var(--terminal-shell-background,#0b1214)); }
.terminal-quick-commands__retry,.terminal-quick-commands__add-first { display:flex; align-items:center; gap:6px; border:1px solid var(--terminal-shell-border,#29383a); border-radius:var(--radius-sm); padding:7px 10px; color:var(--terminal-shell-text,#d8dddc); background:var(--terminal-shell-background,#0b1214); font:inherit; font-size:12px; cursor:pointer; }
.terminal-command-form { display:grid; gap:16px; }
.terminal-command-form label { display:grid; gap:7px; color:var(--text); font-size:13px; font-weight:700; }
.terminal-command-form input,.terminal-command-form textarea { width:100%; border:1px solid var(--border); border-radius:var(--radius-sm); padding:10px 11px; color:var(--text); background:var(--surface); font:inherit; font-size:14px; }
.terminal-command-form textarea { min-height:132px; resize:vertical; font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace; font-size:13px; line-height:1.5; }
.terminal-command-form input:focus,.terminal-command-form textarea:focus { border-color:var(--brand); outline:none; box-shadow:0 0 0 2px color-mix(in srgb,var(--brand) 15%,transparent); }
.terminal-command-form__hint,.terminal-command-form__error,.terminal-command-delete-message { margin:0; font-size:13px; line-height:1.55; }
.terminal-command-form__hint { color:var(--text-muted); }
.terminal-command-form__error { color:var(--danger); }
.terminal-command-delete-message { color:var(--text-muted); }
.spin { animation:spin .8s linear infinite; }
@keyframes spin { to { transform:rotate(360deg); } }
@media (prefers-reduced-motion:reduce) { .spin { animation:none; } }
</style>
