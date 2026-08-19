<script setup lang="ts">
import { Inbox } from '@lucide/vue'

withDefaults(
  defineProps<{
    title: string
    description?: string
  }>(),
  {
    description: '',
  },
)
</script>

<template>
  <div class="empty-state">
    <div class="empty-state__icon" aria-hidden="true">
      <!-- Callers with a subject-specific icon override this; the inbox is the
           generic "nothing here yet" default. -->
      <slot name="icon"><Inbox :size="24" :stroke-width="1.8" /></slot>
    </div>
    <h3>{{ title }}</h3>
    <p v-if="description">{{ description }}</p>
    <div v-if="$slots.default" class="empty-state__action">
      <slot />
    </div>
  </div>
</template>
