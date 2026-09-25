<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { LoaderCircle, Type } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { ApiError, api } from '@/lib/api'
import { formatPackSize } from '@/lib/scenePacks'
import {
  prepareWallpaperImage,
  wallpaperNameFromFile,
  WallpaperFileError,
  type PreparedWallpaperImage,
  type WallpaperFileProblem,
} from '@/lib/wallpaperImage'
import { useI18n } from '@/i18n'
import type { MessageKey } from '@/i18n/messages/zh-CN'
import type { CustomWallpaper } from '@/types/api'

/**
 * Prepares a picked picture, lets the administrator set its focal point, name and
 * colors while previewing it on a desktop, in classic mode and on a phone, then
 * uploads it. Opens whenever `file` is set.
 */
const props = defineProps<{ file?: File }>()
const emit = defineEmits<{
  close: []
  uploaded: [wallpaper: CustomWallpaper]
}>()

const i18n = useI18n()
const prepared = ref<PreparedWallpaperImage>()
const previewURL = ref('')
const processing = ref(false)
const problem = ref('')
const focusX = ref(500)
const focusY = ref(500)
const name = ref('')
const applyColors = ref(true)
const uploading = ref(false)
const progress = ref(0)
let preparation = 0

const FILE_PROBLEMS: Record<WallpaperFileProblem, MessageKey> = {
  type: 'desktop.customWallpaperErrorType',
  size: 'desktop.customWallpaperErrorSize',
  decode: 'desktop.customWallpaperErrorDecode',
  small: 'desktop.customWallpaperErrorSmall',
  encode: 'desktop.customWallpaperErrorEncode',
}

const focusPosition = computed(() => `${focusX.value / 10}% ${focusY.value / 10}%`)
const previewStyle = computed(() => ({ backgroundImage: `url("${previewURL.value}")`, backgroundPosition: focusPosition.value }))
const stageRatio = computed(() => prepared.value ? `${prepared.value.width} / ${prepared.value.height}` : '16 / 9')
const canSave = computed(() => Boolean(prepared.value) && name.value.trim().length > 0 && !uploading.value)
const swatches = computed(() => {
  const theme = prepared.value?.theme
  if (!theme) return []
  return [
    { key: 'desktop.customWallpaperColorBrand' as const, color: theme.brand },
    { key: 'desktop.customWallpaperColorNeutral' as const, color: theme.neutral },
    { key: 'desktop.customWallpaperColorSignature' as const, color: theme.signature },
  ]
})

function releasePreview(): void {
  if (previewURL.value) URL.revokeObjectURL(previewURL.value)
  previewURL.value = ''
}

async function prepare(file: File): Promise<void> {
  const current = ++preparation
  releasePreview()
  prepared.value = undefined
  problem.value = ''
  focusX.value = 500
  focusY.value = 500
  applyColors.value = true
  progress.value = 0
  name.value = wallpaperNameFromFile(file.name)
  processing.value = true
  try {
    const result = await prepareWallpaperImage(file)
    if (current !== preparation) return
    prepared.value = result
    previewURL.value = URL.createObjectURL(result.image)
  } catch (error) {
    if (current !== preparation) return
    problem.value = i18n.t(error instanceof WallpaperFileError ? FILE_PROBLEMS[error.problem] : 'desktop.customWallpaperErrorDecode')
  } finally {
    if (current === preparation) processing.value = false
  }
}

watch(() => props.file, (file) => {
  if (file) void prepare(file)
  else {
    preparation++
    releasePreview()
    prepared.value = undefined
  }
}, { immediate: true })

onBeforeUnmount(releasePreview)

function clamp(value: number): number {
  return Math.min(1000, Math.max(0, Math.round(value)))
}

function onStagePointer(event: PointerEvent): void {
  const box = (event.currentTarget as HTMLElement).getBoundingClientRect()
  if (!box.width || !box.height) return
  focusX.value = clamp(((event.clientX - box.left) / box.width) * 1000)
  focusY.value = clamp(((event.clientY - box.top) / box.height) * 1000)
}

function onStageKey(event: KeyboardEvent): void {
  const step = event.shiftKey ? 100 : 20
  const moves: Record<string, [number, number]> = {
    ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step],
  }
  const move = moves[event.key]
  if (!move) return
  event.preventDefault()
  focusX.value = clamp(focusX.value + move[0])
  focusY.value = clamp(focusY.value + move[1])
}

function uploadProblem(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === 'desktop_wallpaper_quota_exceeded') return i18n.t('desktop.customWallpaperFull')
    if (error.code === 'desktop_wallpaper_busy') return i18n.t('desktop.customWallpaperErrorBusy')
    if (error.code === 'desktop_wallpaper_image_invalid') return i18n.t('desktop.customWallpaperErrorType')
    if (error.code === 'desktop_wallpaper_too_large') return i18n.t('desktop.customWallpaperErrorSize')
    return i18n.t('desktop.customWallpaperErrorUpload', { message: error.message })
  }
  return i18n.t('desktop.customWallpaperErrorUpload', { message: String(error) })
}

async function save(): Promise<void> {
  const ready = prepared.value
  if (!ready || !canSave.value) return
  uploading.value = true
  problem.value = ''
  progress.value = 0
  try {
    const wallpaper = await api.desktop.uploadWallpaper({
      name: name.value.trim(),
      focusX: focusX.value,
      focusY: focusY.value,
      theme: applyColors.value ? ready.theme : undefined,
      image: ready.image,
      thumb: ready.thumb,
    }, (fraction) => { progress.value = fraction })
    emit('uploaded', wallpaper)
  } catch (error) {
    problem.value = uploadProblem(error)
  } finally {
    uploading.value = false
  }
}
</script>

<template>
  <ModalDialog
    :open="Boolean(file)"
    :title="i18n.t('desktop.customWallpaperUploadTitle')"
    :description="i18n.t('desktop.customWallpaperUploadDescription')"
    size="large"
    :close-disabled="uploading"
    @close="emit('close')"
  >
    <div class="custom-wallpaper-dialog">
      <p v-if="processing" class="custom-wallpaper-dialog__status" role="status">
        <LoaderCircle class="spin" :size="16" aria-hidden="true" />
        {{ i18n.t('desktop.customWallpaperProcessing') }}
      </p>
      <template v-if="prepared && previewURL">
        <div class="custom-wallpaper-dialog__layout">
          <div class="custom-wallpaper-dialog__focus">
            <div
              class="custom-wallpaper-dialog__stage"
              :style="{ aspectRatio: stageRatio }"
              role="group"
              tabindex="0"
              :aria-label="`${i18n.t('desktop.customWallpaperFocus')} ${Math.round(focusX / 10)}% · ${Math.round(focusY / 10)}%`"
              data-custom-wallpaper-stage
              @pointerdown="onStagePointer"
              @keydown="onStageKey"
            >
              <img :src="previewURL" alt="" draggable="false" />
              <span
                class="custom-wallpaper-dialog__marker"
                :style="{ left: `${focusX / 10}%`, top: `${focusY / 10}%` }"
                aria-hidden="true"
              />
            </div>
            <small>{{ i18n.t('desktop.customWallpaperFocusHint') }}</small>
          </div>
          <div class="custom-wallpaper-dialog__side">
            <section>
              <h3>{{ i18n.t('desktop.customWallpaperPreview') }}</h3>
              <div class="custom-wallpaper-dialog__previews">
                <figure>
                  <span class="custom-wallpaper-dialog__frame custom-wallpaper-dialog__frame--desktop" :style="previewStyle" />
                  <figcaption>{{ i18n.t('desktop.customWallpaperPreviewDesktop') }}</figcaption>
                </figure>
                <figure>
                  <span class="custom-wallpaper-dialog__frame custom-wallpaper-dialog__frame--classic" :style="previewStyle">
                    <i aria-hidden="true" />
                  </span>
                  <figcaption>{{ i18n.t('desktop.customWallpaperPreviewClassic') }}</figcaption>
                </figure>
                <figure>
                  <span class="custom-wallpaper-dialog__frame custom-wallpaper-dialog__frame--phone" :style="previewStyle" />
                  <figcaption>{{ i18n.t('desktop.customWallpaperPreviewPhone') }}</figcaption>
                </figure>
              </div>
            </section>
            <section>
              <h3>{{ i18n.t('desktop.customWallpaperColors') }}</h3>
              <template v-if="swatches.length">
                <div class="custom-wallpaper-dialog__swatches">
                  <span v-for="swatch in swatches" :key="swatch.key" :title="`${i18n.t(swatch.key)} ${swatch.color}`">
                    <i :style="{ background: swatch.color }" aria-hidden="true" />
                    {{ i18n.t(swatch.key) }}
                  </span>
                </div>
                <label class="custom-wallpaper-dialog__apply">
                  <input v-model="applyColors" type="checkbox" data-custom-wallpaper-apply-colors :disabled="uploading" />
                  <span>{{ i18n.t('desktop.customWallpaperColorsApply') }}</span>
                </label>
              </template>
              <p v-else class="custom-wallpaper-dialog__muted">{{ i18n.t('desktop.customWallpaperColorsNone') }}</p>
            </section>
            <label class="desktop-shortcut-form__field">
              <span class="desktop-shortcut-form__field-heading">
                <span>{{ i18n.t('desktop.customWallpaperName') }}</span>
                <small>{{ Array.from(name).length }}/40</small>
              </span>
              <span class="desktop-shortcut-form__control">
                <Type :size="16" :stroke-width="1.9" aria-hidden="true" />
                <input v-model="name" maxlength="40" autocomplete="off" data-custom-wallpaper-name :disabled="uploading" required />
              </span>
            </label>
            <p class="custom-wallpaper-dialog__muted">
              {{ i18n.t('desktop.customWallpaperInfo', { width: prepared.width, height: prepared.height, size: formatPackSize(prepared.image.size + prepared.thumb.size) }) }}
            </p>
          </div>
        </div>
      </template>
      <p v-if="problem" class="desktop-shortcut-form__error" role="alert">{{ problem }}</p>
    </div>
    <template #footer>
      <button class="button button--ghost" type="button" :disabled="uploading" @click="emit('close')">
        {{ i18n.t('common.cancel') }}
      </button>
      <button class="button button--primary" type="button" :disabled="!canSave" data-custom-wallpaper-save @click="save">
        <LoaderCircle v-if="uploading" class="spin" :size="15" aria-hidden="true" />
        {{ uploading ? i18n.t('desktop.customWallpaperSaving', { percent: Math.round(progress * 100) }) : i18n.t('desktop.customWallpaperSave') }}
      </button>
    </template>
  </ModalDialog>
</template>
