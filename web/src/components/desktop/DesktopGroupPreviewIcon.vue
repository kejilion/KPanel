<script setup lang="ts">
import { ref, watch, type Component } from 'vue'

const props = defineProps<{ label: string; iconURL?: string; icon?: Component }>()
const failed = ref(false)
watch(() => props.iconURL, () => { failed.value = false })
</script>

<template>
  <span class="desktop-group-preview-icon" :title="label" aria-hidden="true">
    <img v-if="iconURL && !failed" :src="iconURL" alt="" width="26" height="26"
      draggable="false" decoding="async" referrerpolicy="no-referrer" @error="failed = true" />
    <component v-else-if="icon" :is="icon" :size="18" :stroke-width="1.7" />
    <span v-else>{{ label.trim().slice(0, 1).toLocaleUpperCase() || 'K' }}</span>
  </span>
</template>

<style scoped>
.desktop-group-preview-icon { display: grid; place-items: center; width: 26px; height: 26px; flex: 0 0 26px; border-radius: 6px; background: var(--desktop-group-hover); color: var(--desktop-group-text); overflow: hidden; font-size: 13px; font-weight: 600; }
.desktop-group-preview-icon img { display: block; width: 100%; height: 100%; object-fit: contain; }
</style>
