<script setup lang="ts">
import { inject } from 'vue'
import { Maximize2, Minimize2, X } from '@lucide/vue'
import { useI18n } from '@/i18n'
import { modalWindowKey } from './modalWindow'

const controls = inject(modalWindowKey, undefined)
const i18n = useI18n()
</script>

<template>
  <div v-if="controls" class="modal-panel__actions modal-panel__actions--window">
    <button
      v-if="controls.allowFullscreen.value"
      class="modal-panel__window-action"
      type="button"
      :title="i18n.t(controls.fullscreen.value ? 'common.exitFullscreen' : 'common.enterFullscreen')"
      :aria-label="i18n.t(controls.fullscreen.value ? 'common.exitFullscreen' : 'common.enterFullscreen')"
      @click="controls.toggleFullscreen"
    >
      <Minimize2 v-if="controls.fullscreen.value" :size="15" aria-hidden="true" />
      <Maximize2 v-else :size="15" aria-hidden="true" />
    </button>
    <button
      class="modal-panel__window-action modal-panel__window-action--close"
      type="button"
      :title="i18n.t('common.closeDialog')"
      :aria-label="i18n.t('common.closeDialog')"
      :disabled="controls.closeDisabled.value"
      @click="controls.close"
    >
      <X :size="17" aria-hidden="true" />
    </button>
  </div>
</template>
