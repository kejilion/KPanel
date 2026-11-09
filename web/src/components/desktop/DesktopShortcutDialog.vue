<script setup lang="ts">
import { isDesktopCommandShortcut } from '@/lib/desktopCommandShortcuts'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { AlignLeft, ImagePlus, Link2, Terminal, Trash2, Type } from '@lucide/vue'
import TerminalHostPicker from '@/components/terminal/TerminalHostPicker.vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { useI18n } from '@/i18n'
import { useTerminalCommands } from '@/stores/terminalCommands'
import type { DesktopShortcut } from '@/types/api'

export interface DesktopShortcutDraft {
  id: string
  name: string
  description: string
  targetType: 'url' | 'command' | 'remote-command'
  url?: string
  command?: string
  hostId?: string
}

export interface DesktopSiteDraft {
  id: string
  name: string
  defaultName: string
  url: string
  domain: string
  iconURL: string
  defaultIconURL: string
  customIcon: boolean
}

const props = defineProps<{
  open: boolean
  shortcut?: DesktopShortcut
  site?: DesktopSiteDraft
  saving?: boolean
  errorMessage?: string
}>()

const emit = defineEmits<{
  close: []
  save: [draft: DesktopShortcutDraft, icon: File | undefined, removeIcon: boolean]
}>()

const i18n = useI18n()
const name = ref('')
const description = ref('')
const url = ref('')
const targetType = ref<'url' | 'command'>('url')
const command = ref('')
const hostId = ref('local')
const hostValid = ref(true)
const commandStore = useTerminalCommands()
const importError = ref('')
const importing = ref(false)
let importController: AbortController | undefined
async function loadCommands(): Promise<void> {
  importController?.abort()
  const controller = new AbortController()
  importController = controller
  importError.value = ''
  importing.value = true
  try {
    const result = await commandStore.load(controller.signal)
    if (!controller.signal.aborted && !result.available) importError.value = i18n.t('terminal.quickCommandsLoadFailed')
  } catch {
    if (!controller.signal.aborted) importError.value = i18n.t('terminal.quickCommandsLoadFailed')
  } finally {
    if (importController === controller) importing.value = false
  }
}
function importCommand(event: Event): void {
  const select = event.target as HTMLSelectElement
  const item = commandStore.commands.value.items.find(item => item.id === select.value)
  if (item) { name.value = item.name; command.value = item.command }
  select.value = ''
}
watch(() => [props.open, targetType.value] as const, ([open, type]) => {
  if (open && type === 'command') void loadCommands()
  else { importController?.abort(); importing.value = false }
}, { immediate: true })
const icon = ref<File>()
const iconInput = ref<HTMLInputElement>()
const iconPreviewURL = ref('')
const removeIcon = ref(false)
const validationMessage = ref('')
let previewSequence = 0
let previewReader: FileReader | undefined

const title = computed(() => props.site ? i18n.t('desktop.siteEditTitle') : props.shortcut
  ? i18n.t('desktop.shortcutEditTitle')
  : i18n.t('desktop.shortcutAddTitle'))

const visibleIconURL = computed(() => iconPreviewURL.value
  || (props.site
    ? (removeIcon.value ? props.site.defaultIconURL : props.site.iconURL)
    : (!removeIcon.value ? props.shortcut?.iconURL : undefined)))
const canRemoveIcon = computed(() => props.site
  ? Boolean(icon.value || (props.site.customIcon && !removeIcon.value))
  : Boolean(visibleIconURL.value))
const iconFailed = ref(false)
watch(visibleIconURL, () => { iconFailed.value = false })

function clearPreview(): void {
  previewSequence += 1
  const reader = previewReader
  previewReader = undefined
  if (reader?.readyState === FileReader.LOADING) {
    try {
      reader.abort()
    } catch {
      // The read may have completed between the readyState check and abort.
    }
  }
  iconPreviewURL.value = ''
}

function reset(): void {
  clearPreview()
  name.value = props.site?.name || props.shortcut?.name || ''
  description.value = props.shortcut?.description || ''
  targetType.value = isDesktopCommandShortcut(props.shortcut) ? 'command' : 'url'
  command.value = props.shortcut?.command || ''
  hostId.value = props.shortcut?.hostId || 'local'
  hostValid.value = hostId.value === 'local'
  importError.value = ''
  url.value = props.site?.url || (props.shortcut?.targetType === 'url' ? props.shortcut.url || '' : '')
  icon.value = undefined
  removeIcon.value = false
  validationMessage.value = ''
  iconFailed.value = false
}

watch([() => props.open, () => props.shortcut?.id, () => props.site?.id], ([open]) => {
  if (open) reset()
  else clearPreview()
}, { immediate: true })

function readIconPreview(file: File): void {
  const sequence = ++previewSequence
  const reader = new FileReader()
  previewReader = reader

  const fail = (): void => {
    if (sequence !== previewSequence || previewReader !== reader) return
    previewReader = undefined
    icon.value = undefined
    iconPreviewURL.value = ''
    validationMessage.value = i18n.t('desktop.shortcutIconReadError')
  }

  reader.onload = () => {
    if (sequence !== previewSequence || previewReader !== reader || icon.value !== file || !props.open) return
    const result = reader.result
    if (typeof result !== 'string' || !result.startsWith('data:image/')) {
      fail()
      return
    }
    previewReader = undefined
    iconPreviewURL.value = result
  }
  reader.onerror = fail
  reader.onabort = () => {
    if (sequence === previewSequence && previewReader === reader) previewReader = undefined
  }
  try {
    reader.readAsDataURL(file)
  } catch {
    fail()
  }
}

function selectIcon(event: Event): void {
  const input = event.currentTarget as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type)) {
    validationMessage.value = i18n.t('desktop.shortcutIconTypeError')
    return
  }
  if (file.size < 1 || file.size > 256 * 1024) {
    validationMessage.value = i18n.t('desktop.shortcutIconSizeError')
    return
  }
  clearPreview()
  icon.value = file
  removeIcon.value = false
  validationMessage.value = ''
  readIconPreview(file)
}

function clearIcon(): void {
  clearPreview()
  icon.value = undefined
  removeIcon.value = Boolean(props.site || props.shortcut?.iconVersion || props.shortcut?.iconURL)
}

function normalizedURL(value: string): string | undefined {
  if (/[\u0000-\u001f\u007f]/.test(value)) return undefined
  try {
    const parsed = new URL(value)
    if (!['http:', 'https:'].includes(parsed.protocol) || parsed.username || parsed.password) return undefined
    return parsed.href
  } catch {
    return undefined
  }
}

function submit(): void {
  if (props.saving) return
  validationMessage.value = ''
  const trimmedName = name.value.trim()
  const trimmedDescription = description.value.replace(/[\s\p{Cc}]+/gu, ' ').trim()
  const normalized = normalizedURL(url.value.trim())
  if (!trimmedName) {
    validationMessage.value = i18n.t('desktop.shortcutNameRequired')
    return
  }
  if (targetType.value === 'command' && (!command.value.trim() || new TextEncoder().encode(command.value).length > 8192 || /[\u0000-\u0008\u000b-\u001f\u007f-\u009f]/.test(command.value))) {
    validationMessage.value = i18n.t('desktop.shortcutCommandInvalid')
    return
  }
  if (targetType.value === 'command' && !hostValid.value) {
    validationMessage.value = i18n.t('desktop.shortcutHostUnavailable')
    return
  }
  if (targetType.value === 'url' && !normalized) {
    validationMessage.value = i18n.t('desktop.shortcutURLInvalid')
    return
  }
  emit('save', {
    id: props.site?.id || props.shortcut?.id || '',
    name: trimmedName,
    description: trimmedDescription,
    targetType: targetType.value === 'command' && hostId.value !== 'local' ? 'remote-command' : targetType.value,
    ...(targetType.value === 'command' ? { command: command.value, hostId: hostId.value } : { url: normalized }),
  }, icon.value, removeIcon.value)
}

onBeforeUnmount(() => { clearPreview(); importController?.abort() })
</script>

<template>
  <ModalDialog :open="open" :title="title" size="small" @close="!saving && emit('close')">
    <form class="desktop-shortcut-form" @submit.prevent="submit">
      <div class="desktop-shortcut-form__identity">
        <span class="desktop-shortcut-form__preview" aria-hidden="true">
          <img v-if="visibleIconURL && !iconFailed" :src="visibleIconURL" alt="" @error="iconFailed = true" />
          <Terminal v-else-if="targetType === 'command'" :size="28" :stroke-width="1.8" />
          <Link2 v-else :size="28" :stroke-width="1.8" />
        </span>
        <div class="desktop-shortcut-form__icon-actions">
          <label class="button button--ghost" role="button" :tabindex="saving ? -1 : 0"
            @keydown.enter.prevent="!saving && iconInput?.click()" @keydown.space.prevent="!saving && iconInput?.click()">
            <ImagePlus :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.shortcutChooseIcon') }}
            <input
              ref="iconInput"
              type="file"
              accept="image/png,image/jpeg,image/webp"
              :disabled="saving"
              @change="selectIcon"
            />
          </label>
          <button
            v-if="canRemoveIcon"
            class="button button--ghost"
            type="button"
            :disabled="saving"
            @click="clearIcon"
          >
            <Trash2 :size="14" aria-hidden="true" />
            {{ i18n.t(site ? 'desktop.siteResetIcon' : 'desktop.shortcutRemoveIcon') }}
          </button>
          <small>{{ i18n.t('desktop.shortcutIconHint') }}</small>
        </div>
      </div>

      <label v-if="!site" class="desktop-shortcut-form__field">
        <span class="desktop-shortcut-form__field-heading">{{ i18n.t('desktop.shortcutType') }}</span>
        <select v-model="targetType" class="desktop-shortcut-form__control" :disabled="saving" data-testid="shortcut-type">
          <option value="url">{{ i18n.t('desktop.shortcutTypeURL') }}</option>
          <option value="command">{{ i18n.t('desktop.shortcutTypeCommand') }}</option>
        </select>
      </label>
      <TerminalHostPicker v-if="targetType === 'command'" v-model="hostId" :active="open" :disabled="saving" @validity="hostValid = $event" />
      <div v-if="targetType === 'command'" class="desktop-shortcut-form__field">
        <label class="desktop-shortcut-form__field-heading" for="shortcut-import">{{ i18n.t('desktop.shortcutImportCommand') }}</label>
        <select id="shortcut-import" class="desktop-shortcut-form__control" :disabled="saving || importing || !!importError" @change="importCommand">
          <option value="">{{ importing ? i18n.t('common.loading') : i18n.t('desktop.shortcutImportPlaceholder') }}</option>
          <option v-for="item in commandStore.commands.value.items" :key="item.id" :value="item.id">{{ item.name }}</option>
        </select>
        <p v-if="importError" role="alert">{{ importError }} <button type="button" class="button button--ghost" @click="loadCommands">{{ i18n.t('common.retry') }}</button></p>
        <small v-else-if="!importing && !commandStore.commands.value.items.length">{{ i18n.t('terminal.quickCommandsEmptyTitle') }}</small>
      </div>
      <label class="desktop-shortcut-form__field">
        <span class="desktop-shortcut-form__field-heading">
          <span>{{ i18n.t(site ? 'desktop.siteName' : 'desktop.shortcutName') }}</span>
        </span>
        <span class="desktop-shortcut-form__control">
          <Type :size="16" :stroke-width="1.9" aria-hidden="true" />
          <input
            v-model="name"
            maxlength="48"
            autocomplete="off"
            :placeholder="site?.defaultName || i18n.t('desktop.shortcutNamePlaceholder')"
            :disabled="saving"
            required
          />
        </span>
      </label>
      <button v-if="site && name !== site.defaultName" type="button" class="button button--ghost" :disabled="saving" @click="name = site.defaultName">
        {{ i18n.t('desktop.renameReset') }}
      </button>
      <div v-if="site" class="desktop-shortcut-form__field">
        <span class="desktop-shortcut-form__field-heading">{{ i18n.t('desktop.siteDomain') }}</span>
        <small class="desktop-site-domain">{{ site.domain }}</small>
      </div>
      <label v-if="!site && targetType === 'url'" class="desktop-shortcut-form__field">
        <span class="desktop-shortcut-form__field-heading">
          <span>{{ i18n.t('desktop.shortcutURL') }}</span>
        </span>
        <span class="desktop-shortcut-form__control">
          <Link2 :size="16" :stroke-width="1.9" aria-hidden="true" />
          <input
            v-model="url"
            type="url"
            maxlength="2048"
            placeholder="https://example.com"
            autocomplete="url"
            spellcheck="false"
            :disabled="saving"
            required
          />
        </span>
      </label>
      <label v-if="targetType === 'command'" class="desktop-shortcut-form__field">
        <span class="desktop-shortcut-form__field-heading">{{ i18n.t('desktop.shortcutTypeCommand') }}</span>
        <span class="desktop-shortcut-form__control desktop-shortcut-form__control--textarea">
          <Terminal :size="16" aria-hidden="true" />
          <textarea v-model="command" rows="4" maxlength="8192" spellcheck="false" :disabled="saving" :placeholder="i18n.t('desktop.shortcutCommandPlaceholder')" data-testid="shortcut-command" />
        </span>
        <small>{{ i18n.t('desktop.shortcutCommandHint') }}</small>
      </label>
      <label v-if="!site" class="desktop-shortcut-form__field">
        <span class="desktop-shortcut-form__field-heading">
          <span>{{ i18n.t('desktop.shortcutDescription') }}</span>
          <small>{{ description.length }}/160</small>
        </span>
        <span class="desktop-shortcut-form__control desktop-shortcut-form__control--textarea">
          <AlignLeft :size="16" :stroke-width="1.9" aria-hidden="true" />
          <textarea
            v-model="description"
            rows="3"
            maxlength="160"
            :placeholder="i18n.t('desktop.shortcutDescriptionPlaceholder')"
            :disabled="saving"
          />
        </span>
      </label>
      <p v-if="validationMessage || errorMessage" class="desktop-shortcut-form__error" role="alert">
        {{ validationMessage || errorMessage }}
      </p>
    </form>
    <template #footer>
      <button class="button button--ghost" type="button" :disabled="saving" @click="emit('close')">
        {{ i18n.t('common.cancel') }}
      </button>
      <button class="button button--primary" type="button" :disabled="saving" @click="submit">
        {{ saving ? i18n.t('common.saving') : i18n.t(site ? 'desktop.renameSave' : 'desktop.shortcutSave') }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.desktop-shortcut-form__field-heading { font-size: 14px; }
select.desktop-shortcut-form__control { width: 100%; min-width: 0; color: var(--text); font-size: 14px; }
.desktop-site-domain { overflow-wrap: anywhere; }
.desktop-shortcut-form__icon-actions small { font-size: 13px; }
.desktop-shortcut-form__error { font-size: 14px; }
.desktop-shortcut-form__field > small { color: var(--muted); font-size: 13px; line-height: 1.5; }
.desktop-shortcut-form__field > p { margin: 0; color: var(--danger); font-size: 14px; }
</style>
