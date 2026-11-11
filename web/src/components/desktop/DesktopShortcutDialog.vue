<script setup lang="ts">
import { isDesktopCommandShortcut } from '@/lib/desktopCommandShortcuts'
import { api } from '@/lib/api'
import { computed, onBeforeUnmount, ref, useId, watch } from 'vue'
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
const iconSourceURL = ref('')
const iconImporting = ref(false)
const iconImportError = ref('')
const iconURLInputID = `desktop-icon-source-${useId()}`
let iconImportController: AbortController | undefined

function abortIconImport(): void {
  iconImportController?.abort()
  iconImportController = undefined
  iconImporting.value = false
}

watch(iconSourceURL, () => { abortIconImport(); iconImportError.value = '' })

async function importIconFromURL(): Promise<void> {
  if (props.saving || iconImporting.value) return
  iconImportError.value = ''
  const normalized = normalizedURL(iconSourceURL.value.trim())
  if (!normalized) {
    iconImportError.value = i18n.t('desktop.iconSourceInvalid')
    return
  }
  abortIconFetch()
  const controller = new AbortController()
  iconImportController = controller
  iconImporting.value = true
  try {
    const blob = await api.desktop.importIconFromURL(normalized, controller.signal)
    if (controller.signal.aborted || iconImportController !== controller || !props.open || props.saving) return
    if (!['image/png', 'image/jpeg', 'image/webp'].includes(blob.type) || blob.size < 1 || blob.size > 256 * 1024) {
      throw new Error('invalid icon response')
    }
    const extension = blob.type === 'image/png' ? 'png' : blob.type === 'image/jpeg' ? 'jpg' : 'webp'
    clearPreview()
    icon.value = new File([blob], `icon.${extension}`, { type: blob.type })
    iconManuallySet.value = true
    iconAutoFetched.value = false
    removeIcon.value = false
    validationMessage.value = ''
    readIconPreview(icon.value)
  } catch {
    if (!controller.signal.aborted && iconImportController === controller && props.open) {
      iconImportError.value = i18n.t('desktop.iconSourceFailed')
    }
  } finally {
    if (iconImportController === controller) {
      iconImportController = undefined
      iconImporting.value = false
    }
  }
}
// Auto-filled favicon state: a fetched icon only ever replaces an empty slot
// or a previously fetched icon. Once the user picks a file or removes the
// icon, the automatic fill stops for the rest of this dialog session.
const iconAutoFetched = ref(false)
const iconManuallySet = ref(false)
const lastIconFetchURL = ref('')
let iconFetchTimer: ReturnType<typeof setTimeout> | undefined
let iconFetchController: AbortController | undefined

function abortIconFetch(): void {
  if (iconFetchTimer !== undefined) {
    clearTimeout(iconFetchTimer)
    iconFetchTimer = undefined
  }
  iconFetchController?.abort()
  iconFetchController = undefined
}
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
  abortIconImport()
  iconSourceURL.value = ''
  iconImportError.value = ''
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
  abortIconFetch()
  iconAutoFetched.value = false
  iconManuallySet.value = false
  lastIconFetchURL.value = ''
  scheduleIconFetch()
}

watch([() => props.open, () => props.shortcut?.id, () => props.site?.id], ([open]) => {
  if (open) reset()
  else { abortIconImport(); abortIconFetch(); clearPreview() }
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
  abortIconImport()
  iconImportError.value = ''
  abortIconFetch()
  icon.value = file
  iconManuallySet.value = true
  iconAutoFetched.value = false
  removeIcon.value = false
  validationMessage.value = ''
  readIconPreview(file)
}

function clearIcon(): void {
  abortIconImport()
  iconImportError.value = ''
  abortIconFetch()
  clearPreview()
  icon.value = undefined
  iconManuallySet.value = true
  iconAutoFetched.value = false
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

function canAutoFillIcon(): boolean {
  if (iconImporting.value) return false
  if (props.site || targetType.value !== 'url' || !props.open) return false
  if (iconManuallySet.value) return false
  if (iconAutoFetched.value) return true
  return !icon.value && !visibleIconURL.value
}

async function fetchIconPreview(normalized: string, controller: AbortController): Promise<void> {
  let blob: Blob
  try {
    blob = await api.desktop.previewShortcutIcon(normalized, controller.signal)
  } catch {
    // A failed fetch silently keeps the current icon; the form stays usable.
    return
  }
  if (controller.signal.aborted || iconFetchController !== controller || !canAutoFillIcon()) return
  if (normalizedURL(url.value.trim()) !== normalized) return
  if (iconManuallySet.value || (icon.value && !iconAutoFetched.value)) return
  const extension = blob.type === 'image/png' ? 'png' : blob.type === 'image/jpeg' ? 'jpg' : 'webp'
  const file = new File([blob], `favicon.${extension}`, { type: blob.type })
  clearPreview()
  icon.value = file
  iconAutoFetched.value = true
  removeIcon.value = false
  lastIconFetchURL.value = normalized
  readIconPreview(file)
}

function scheduleIconFetch(): void {
  abortIconFetch()
  if (iconAutoFetched.value) {
    clearPreview()
    icon.value = undefined
    iconAutoFetched.value = false
    lastIconFetchURL.value = ''
  }
  if (!canAutoFillIcon()) return
  const normalized = normalizedURL(url.value.trim())
  if (!normalized || normalized === lastIconFetchURL.value) return
  iconFetchTimer = setTimeout(() => {
    iconFetchTimer = undefined
    const controller = new AbortController()
    iconFetchController = controller
    void fetchIconPreview(normalized, controller)
  }, 450)
}

watch(url, () => { scheduleIconFetch() })
watch(targetType, (type) => { if (type !== 'url') abortIconFetch() })

function submit(): void {
  if (props.saving || iconImporting.value) return
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
  abortIconFetch()
  emit('save', {
    id: props.site?.id || props.shortcut?.id || '',
    name: trimmedName,
    description: trimmedDescription,
    targetType: targetType.value === 'command' && hostId.value !== 'local' ? 'remote-command' : targetType.value,
    ...(targetType.value === 'command' ? { command: command.value, hostId: hostId.value } : { url: normalized }),
  }, icon.value, removeIcon.value)
}

onBeforeUnmount(() => { abortIconImport(); abortIconFetch(); clearPreview(); importController?.abort() })
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
        </div>
        <div class="desktop-shortcut-form__icon-source">
          <label :for="iconURLInputID">{{ i18n.t('desktop.iconSourceURL') }}</label>
          <div class="desktop-shortcut-form__icon-source-row">
            <span class="desktop-shortcut-form__control">
              <Link2 :size="16" aria-hidden="true" />
              <input :id="iconURLInputID" v-model="iconSourceURL" type="url" data-testid="shortcut-icon-url"
                maxlength="2048" placeholder="https://example.com/icon.png" autocomplete="off" spellcheck="false"
                :disabled="saving" :aria-invalid="Boolean(iconImportError)" :aria-describedby="`${iconURLInputID}-hint`"
                @keydown.enter.prevent="importIconFromURL" />
            </span>
            <button class="button button--secondary" type="button" :disabled="saving || iconImporting || !iconSourceURL.trim()"
              :aria-busy="iconImporting" @click="importIconFromURL">
              {{ i18n.t(iconImporting ? 'desktop.iconSourceLoading' : 'desktop.iconSourceImport') }}
            </button>
          </div>
          <small :id="`${iconURLInputID}-hint`">{{ i18n.t('desktop.shortcutIconHint') }}</small>
          <p v-if="iconImportError" class="desktop-shortcut-form__icon-source-error" role="alert">{{ iconImportError }}</p>
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
            data-testid="shortcut-name"
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
      <button class="button button--primary" type="button" :disabled="saving || iconImporting" @click="submit">
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
.desktop-shortcut-form__icon-source { grid-column: 1 / -1; display: grid; min-width: 0; gap: 8px; }
.desktop-shortcut-form__icon-source > label { font-size: 14px; font-weight: 600; }
.desktop-shortcut-form__icon-source-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 8px; align-items: start; }
.desktop-shortcut-form__icon-source-row > .button { min-height: 44px; font-size: 14px; white-space: nowrap; }
.desktop-shortcut-form__icon-source > small { font-size: 13px; color: var(--muted); line-height: 1.5; }
.desktop-shortcut-form__icon-source-error { margin: 0; color: var(--danger); font-size: 14px; line-height: 1.5; overflow-wrap: anywhere; }
@media (max-width: 420px) {
  .desktop-shortcut-form__icon-source-row { grid-template-columns: minmax(0, 1fr); }
  .desktop-shortcut-form__icon-source-row > .button { justify-self: end; }
}
</style>
