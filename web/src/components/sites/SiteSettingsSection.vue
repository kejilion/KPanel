<script setup lang="ts">
import { useId } from 'vue'
import { ChevronDown } from '@lucide/vue'

withDefaults(defineProps<{ title: string; description: string; expanded: boolean; disabled?: boolean }>(), { disabled: false })
defineEmits<{ toggle: [] }>()
const contentId = `site-settings-section-${useId()}`
</script>

<template>
  <section class="site-settings-section">
    <button class="site-settings-section__toggle" type="button" :aria-expanded="expanded" :aria-controls="contentId" :disabled="disabled" @click="$emit('toggle')">
      <span class="site-settings-section__label">
        <strong><slot name="icon" /> {{ title }}</strong>
        <small>{{ description }}</small>
      </span>
      <ChevronDown :size="18" :class="{ 'is-open': expanded }" />
    </button>
    <div v-show="expanded" :id="contentId" class="site-settings-section__content">
      <slot v-if="expanded" />
    </div>
  </section>
</template>

<style scoped>
.site-settings-section { border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface-subtle); }
.site-settings-section__toggle { display: flex; width: 100%; align-items: center; justify-content: space-between; gap: 12px; padding: 16px; border: 0; border-radius: inherit; background: transparent; color: var(--text); text-align: left; cursor: pointer; }
.site-settings-section__toggle:disabled { cursor: not-allowed; opacity: .6; }
.site-settings-section__toggle:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
.site-settings-section__toggle > svg { flex: 0 0 auto; transition: transform var(--motion-duration-fast) var(--motion-ease-standard); }
.site-settings-section__toggle > svg.is-open { transform: rotate(180deg); }
.site-settings-section__label { display: grid; gap: 4px; min-width: 0; }
.site-settings-section__label strong { display: flex; align-items: center; gap: 8px; font-size: 14px; }
.site-settings-section__label small { color: var(--muted); font-size: 13px; line-height: 1.5; }
.site-settings-section__content { display: grid; gap: 12px; padding: 0 16px 16px; }
</style>
