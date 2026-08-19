<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { AlignLeft, ImagePlus, Link2, Trash2, Type } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { useI18n } from '@/i18n'
import type { DesktopShortcut } from '@/types/api'

export interface DesktopShortcutDraft {
  id: string
  name: string
  description: string
  targetType: 'url'
  url: string
}

const props = defineProps<{
  open: boolean
  shortcut?: DesktopShortcut
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
const icon = ref<File>()
const iconPreviewURL = ref('')
const removeIcon = ref(false)
const validationMessage = ref('')
let previewSequence = 0
let previewReader: FileReader | undefined

const title = computed(() => props.shortcut
  ? i18n.t('desktop.shortcutEditTitle')
  : i18n.t('desktop.shortcutAddTitle'))

const visibleIconURL = computed(() => iconPreviewURL.value
  || (!removeIcon.value ? props.shortcut?.iconURL : undefined))

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
  name.value = props.shortcut?.name || ''
  description.value = props.shortcut?.description || ''
  url.value = props.shortcut?.targetType === 'url' ? props.shortcut.url || '' : ''
  icon.value = undefined
  removeIcon.value = false
  validationMessage.value = ''
}

watch(() => [props.open, props.shortcut?.id] as const, ([open]) => {
  if (open) reset()
  else clearPreview()
})

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
  removeIcon.value = Boolean(props.shortcut?.iconVersion || props.shortcut?.iconURL)
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
  validationMessage.value = ''
  const trimmedName = name.value.trim()
  const trimmedDescription = description.value.replace(/[\s\p{Cc}]+/gu, ' ').trim()
  const normalized = normalizedURL(url.value.trim())
  if (!trimmedName) {
    validationMessage.value = i18n.t('desktop.shortcutNameRequired')
    return
  }
  if (!normalized) {
    validationMessage.value = i18n.t('desktop.shortcutURLInvalid')
    return
  }
  emit('save', {
    id: props.shortcut?.id || '',
    name: trimmedName,
    description: trimmedDescription,
    targetType: 'url',
    url: normalized,
  }, icon.value, removeIcon.value)
}

onBeforeUnmount(clearPreview)
</script>

<template>
  <ModalDialog :open="open" :title="title" size="small" @close="emit('close')">
    <form class="desktop-shortcut-form" @submit.prevent="submit">
      <div class="desktop-shortcut-form__identity">
        <span class="desktop-shortcut-form__preview" aria-hidden="true">
          <img v-if="visibleIconURL" :src="visibleIconURL" alt="" />
          <Link2 v-else :size="28" :stroke-width="1.8" />
        </span>
        <div class="desktop-shortcut-form__icon-actions">
          <label class="button button--ghost">
            <ImagePlus :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.shortcutChooseIcon') }}
            <input
              type="file"
              accept="image/png,image/jpeg,image/webp"
              :disabled="saving"
              @change="selectIcon"
            />
          </label>
          <button
            v-if="visibleIconURL"
            class="button button--ghost"
            type="button"
            :disabled="saving"
            @click="clearIcon"
          >
            <Trash2 :size="14" aria-hidden="true" />
            {{ i18n.t('desktop.shortcutRemoveIcon') }}
          </button>
          <small>{{ i18n.t('desktop.shortcutIconHint') }}</small>
        </div>
      </div>

      <label class="desktop-shortcut-form__field">
        <span class="desktop-shortcut-form__field-heading">
          <span>{{ i18n.t('desktop.shortcutName') }}</span>
        </span>
        <span class="desktop-shortcut-form__control">
          <Type :size="16" :stroke-width="1.9" aria-hidden="true" />
          <input
            v-model="name"
            maxlength="48"
            autocomplete="off"
            :placeholder="i18n.t('desktop.shortcutNamePlaceholder')"
            :disabled="saving"
            required
          />
        </span>
      </label>
      <label class="desktop-shortcut-form__field">
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
      <label class="desktop-shortcut-form__field">
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
        {{ saving ? i18n.t('common.saving') : i18n.t('desktop.shortcutSave') }}
      </button>
    </template>
  </ModalDialog>
</template>
