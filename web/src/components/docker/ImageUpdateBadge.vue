<script setup lang="ts">
import { CircleArrowUp } from '@lucide/vue'
defineProps<{ label: string }>()
</script>

<template>
  <span class="image-update-badge" role="img" :aria-label="label" :title="label" @click.stop>
    <CircleArrowUp :size="14" aria-hidden="true" />
  </span>
</template>

<style scoped>
.image-update-badge { position: absolute; top: -3px; right: -3px; display: grid; place-items: center; width: 18px; height: 18px; border-radius: 50%; background: var(--surface); color: var(--warning); cursor: default; }
</style>
