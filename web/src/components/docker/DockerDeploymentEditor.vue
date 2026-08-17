<script setup lang="ts">
import { computed, ref } from 'vue'
import { CircleAlert, Crosshair, ScanLine } from '@lucide/vue'
import type { DockerDeploymentDiagnostic } from '@/lib/dockerDeployment'

const props = withDefaults(defineProps<{
  modelValue: string
  diagnostics?: DockerDeploymentDiagnostic[]
  placeholder?: string
  rows?: number
  ariaLabel?: string
}>(), {
  diagnostics: () => [],
  placeholder: '',
  rows: 9,
  ariaLabel: 'Docker 部署内容',
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const input = ref<HTMLTextAreaElement>()
const scrollTop = ref(0)
const lineCount = computed(() => Math.max(1, props.modelValue.split(/\r?\n/).length))

function update(event: Event): void {
  emit('update:modelValue', (event.target as HTMLTextAreaElement).value)
}

function syncScroll(event: Event): void {
  scrollTop.value = (event.target as HTMLTextAreaElement).scrollTop
}

function focusDiagnostic(item: DockerDeploymentDiagnostic): void {
  const editor = input.value
  if (!editor) return
  editor.focus()
  editor.setSelectionRange(item.from, Math.max(item.from + 1, item.to))
}

defineExpose({ focusDiagnostic, focus: () => input.value?.focus() })
</script>

<template>
  <div class="deployment-editor" :class="{ 'has-errors': diagnostics.length }">
    <div class="deployment-editor__surface">
      <div class="deployment-editor__gutter" aria-hidden="true">
        <div :style="{ transform: `translateY(${-scrollTop}px)` }">
          <span v-for="line in lineCount" :key="line">{{ line }}</span>
        </div>
      </div>
      <textarea
        ref="input"
        :value="modelValue"
        class="deployment-editor__input"
        :rows="rows"
        maxlength="24576"
        wrap="off"
        spellcheck="false"
        autocomplete="off"
        :aria-label="ariaLabel"
        :aria-invalid="diagnostics.length > 0"
        :placeholder="placeholder"
        @input="update"
        @scroll="syncScroll"
      />
      <span class="deployment-editor__scanner" aria-hidden="true"><ScanLine :size="15" /></span>
    </div>
    <div v-if="diagnostics.length" class="deployment-diagnostics" role="list" aria-label="语法问题">
      <button
        v-for="item in diagnostics"
        :key="`${item.code}-${item.from}-${item.to}`"
        type="button"
        role="listitem"
        @click="focusDiagnostic(item)"
      >
        <CircleAlert :size="16" />
        <span>
          <strong>第 {{ item.line }} 行 · 第 {{ item.column }} 列</strong>
          <small>{{ item.message }}<template v-if="item.hint"> {{ item.hint }}</template></small>
        </span>
        <Crosshair :size="15" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.deployment-editor { display: grid; gap: 9px; }
.deployment-editor__surface {
  position: relative;
  display: grid;
  min-height: 190px;
  grid-template-columns: 46px minmax(0, 1fr);
  overflow: hidden;
  border: 1px solid var(--border-strong);
  border-radius: 11px;
  background: var(--surface-subtle);
  box-shadow: inset 0 1px 2px rgb(20 48 42 / 4%);
  transition: border-color .16s ease, box-shadow .16s ease;
}
.deployment-editor__surface:focus-within { border-color: var(--brand); box-shadow: 0 0 0 3px color-mix(in srgb, var(--brand) 12%, transparent); }
.has-errors .deployment-editor__surface { border-color: color-mix(in srgb, var(--danger) 64%, var(--border)); }
.deployment-editor__gutter {
  height: 100%;
  overflow: hidden;
  border-right: 1px solid var(--border);
  color: var(--muted);
  background: color-mix(in srgb, var(--surface-raised) 72%, transparent);
  font: 11.5px/1.65 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  text-align: right;
  user-select: none;
}
.deployment-editor__gutter > div { padding: 12px 10px 12px 4px; }
.deployment-editor__gutter span { display: block; height: 18.975px; }
.deployment-editor__input {
  width: 100%;
  min-height: 190px;
  resize: vertical;
  overflow: auto;
  border: 0;
  outline: 0;
  padding: 12px 34px 12px 13px;
  color: var(--text);
  background: transparent;
  caret-color: var(--brand);
  font: 11.5px/1.65 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  tab-size: 2;
}
.deployment-editor__input::placeholder { color: var(--muted); }
.deployment-editor__scanner {
  position: absolute;
  top: 11px;
  right: 11px;
  display: grid;
  width: 25px;
  height: 25px;
  place-items: center;
  border-radius: 8px;
  color: var(--brand);
  background: color-mix(in srgb, var(--brand) 10%, var(--surface));
}
.deployment-editor__surface:focus-within .deployment-editor__scanner { animation: scanner-pulse 1.5s ease-in-out infinite; }
.deployment-diagnostics { display: grid; gap: 7px; }
.deployment-diagnostics button {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 9px;
  width: 100%;
  padding: 9px 10px;
  border: 1px solid color-mix(in srgb, var(--danger) 32%, var(--border));
  border-radius: 10px;
  color: var(--danger);
  background: color-mix(in srgb, var(--danger) 6%, var(--surface));
  text-align: left;
  cursor: pointer;
}
.deployment-diagnostics button:hover,
.deployment-diagnostics button:focus-visible { border-color: var(--danger); outline: none; box-shadow: 0 0 0 3px color-mix(in srgb, var(--danger) 10%, transparent); }
.deployment-diagnostics button > span { display: grid; min-width: 0; gap: 2px; }
.deployment-diagnostics strong { font-size: .72rem; }
.deployment-diagnostics small { color: var(--text); font-size: .75rem; line-height: 1.45; }
@keyframes scanner-pulse { 0%, 100% { opacity: .55; transform: scale(.94); } 50% { opacity: 1; transform: scale(1); } }
:global(:root[data-theme='dark']) .deployment-editor__surface { background: var(--terminal-shell-background, #0b1214); color: var(--terminal-shell-text, #d8dddc); border-color: var(--terminal-shell-border, #29383a); box-shadow: var(--terminal-shell-shadow, inset 0 1px 0 rgb(255 255 255 / 3%)); }
:global(:root[data-theme='dark']) .deployment-editor__input { color: var(--terminal-shell-text, #d8dddc); }
:global(:root[data-theme='dark']) .deployment-editor__gutter { border-color: var(--terminal-shell-border, #29383a); background: var(--terminal-shell-panel, #111a1d); }
@media (max-width: 720px) {
  .deployment-editor__surface, .deployment-editor__input { min-height: 220px; }
  .deployment-editor__surface { grid-template-columns: 40px minmax(0, 1fr); }
  .deployment-editor__gutter > div { padding-right: 8px; }
}
@media (prefers-reduced-motion: reduce) {
  .deployment-editor__surface:focus-within .deployment-editor__scanner { animation: none; }
}
</style>
