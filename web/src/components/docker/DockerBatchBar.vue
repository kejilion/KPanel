<script setup lang="ts">
import type { Component } from 'vue'
import { LoaderCircle, TriangleAlert, X } from '@lucide/vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'

export interface DockerBatchBarAction {
  id: string
  label: string
  icon: Component
  /** Selected items this action applies to; 0 disables it. */
  count: number
  danger?: boolean
  disabledReason?: string
}

export interface DockerBatchBarProgress {
  label: string
  done: number
  total: number
  /** A finished run that still has results to review. */
  finished: boolean
}

defineProps<{
  count: number
  actions: DockerBatchBarAction[]
  progress?: DockerBatchBarProgress
}>()

const emit = defineEmits<{
  run: [id: string]
  clear: []
  showProgress: []
}>()

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

function blocked(progress?: DockerBatchBarProgress): boolean {
  return Boolean(progress && !progress.finished)
}

function actionTitle(action: DockerBatchBarAction, progress?: DockerBatchBarProgress): string | undefined {
  if (blocked(progress)) return phrase('请等待当前批量操作完成')
  if (action.disabledReason) return phrase(action.disabledReason)
  if (!action.count) return phrase('所选项都不适用此操作')
  return undefined
}
</script>

<template>
  <div class="docker-batch-bar" role="toolbar" :aria-label="phrase('Docker 批量操作')">
    <strong v-if="count" class="docker-batch-bar__count">已选 {{ count }} 项</strong>
    <button
      v-if="progress"
      class="docker-batch-bar__progress"
      :class="{ 'is-finished': progress.finished }"
      type="button"
      :title="phrase(progress.finished ? '查看批量操作结果' : '查看批量操作进度')"
      @click="emit('showProgress')"
    >
      <TriangleAlert v-if="progress.finished" :size="15" aria-hidden="true" />
      <LoaderCircle v-else class="spin" :size="15" aria-hidden="true" />
      <span v-if="progress.finished">查看结果</span>
      <span v-else>{{ phrase(progress.label) }} {{ progress.done }}/{{ progress.total }}</span>
    </button>
    <template v-if="count">
      <button
        v-for="action in actions"
        :key="action.id"
        type="button"
        :class="{ 'is-danger': action.danger }"
        :disabled="blocked(progress) || !action.count || Boolean(action.disabledReason)"
        :title="actionTitle(action, progress)"
        @click="emit('run', action.id)"
      >
        <component :is="action.icon" :size="15" aria-hidden="true" />
        <span>{{ phrase(action.label) }}</span>
        <small v-if="action.count" class="docker-batch-bar__chip">{{ action.count }}</small>
      </button>
      <button class="docker-batch-bar__clear" type="button" :title="phrase('取消选择')" @click="emit('clear')">
        <X :size="15" aria-hidden="true" />
        <span>取消</span>
      </button>
    </template>
  </div>
</template>

<style scoped>
.docker-batch-bar {
  position: fixed;
  z-index: 45;
  bottom: max(16px, env(safe-area-inset-bottom));
  left: calc(var(--app-shell-inline-offset, 0px) + (100vw - var(--app-shell-inline-offset, 0px)) / 2);
  display: flex;
  width: max-content;
  max-width: min(820px, calc(100vw - var(--app-shell-inline-offset, 0px) - 32px));
  min-width: 0;
  align-items: center;
  gap: 4px;
  overflow-x: auto;
  padding: 8px 10px;
  border: 1px solid color-mix(in srgb, var(--brand) 30%, var(--border));
  border-radius: var(--radius);
  background: color-mix(in srgb, var(--brand) 8%, var(--surface));
  box-shadow: var(--shadow-md);
  transform: translateX(-50%);
  scrollbar-width: thin;
}

.docker-batch-bar__count {
  flex: 0 0 auto;
  margin-right: 6px;
  padding-left: 4px;
  color: var(--text);
  font-size: 14px;
  white-space: nowrap;
}

.docker-batch-bar button {
  display: inline-flex;
  min-height: 36px;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
  padding: 0 10px;
  border: 0;
  border-radius: var(--radius-sm);
  color: var(--text-soft);
  background: transparent;
  font: inherit;
  font-size: 14px;
  white-space: nowrap;
  cursor: pointer;
  transition: background-color .14s ease, color .14s ease;
}

.docker-batch-bar button:hover:not(:disabled) {
  color: var(--text);
  background: var(--interaction-hover-surface);
}

.docker-batch-bar button:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--brand) 55%, transparent);
  outline-offset: 1px;
}

.docker-batch-bar button.is-danger:not(:disabled) {
  color: var(--danger);
}

.docker-batch-bar button:disabled {
  opacity: .48;
  cursor: not-allowed;
}

.docker-batch-bar__chip {
  display: inline-grid;
  min-width: 22px;
  height: 20px;
  place-items: center;
  padding: 0 6px;
  border-radius: 999px;
  color: var(--brand);
  background: var(--brand-soft);
  font-size: 12px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.docker-batch-bar button.is-danger .docker-batch-bar__chip {
  color: var(--danger);
  background: color-mix(in srgb, var(--danger) 12%, transparent);
}

.docker-batch-bar .docker-batch-bar__progress {
  color: var(--brand);
  font-variant-numeric: tabular-nums;
}

.docker-batch-bar .docker-batch-bar__progress.is-finished {
  color: var(--amber);
}

/* A desktop window scrolls its own body, so the bar rides along at its bottom edge. */
.desktop-window__body .docker-batch-bar {
  position: sticky;
  z-index: 70;
  bottom: 10px;
  left: auto;
  max-width: calc(100% - 24px);
  margin: 12px auto 0;
  transform: none;
}

/* A narrow desktop window never reaches the phone breakpoint; wrap like it. */
@container desktop-window (max-width: 560px) {
  .docker-batch-bar {
    display: grid;
    width: calc(100% - 20px);
    grid-template-columns: repeat(3, minmax(0, 1fr));
    overflow: visible;
    padding: 8px;
  }

  .docker-batch-bar__count,
  .docker-batch-bar .docker-batch-bar__progress {
    grid-column: 1 / -1;
  }

  .docker-batch-bar__count {
    margin: 0;
    padding: 2px 6px 4px;
  }

  .docker-batch-bar button {
    justify-content: center;
    gap: 4px;
    padding: 0 4px;
  }
}

/* The sidebar turns into a drawer here and the content spans the viewport. */
@media (max-width: 920px) {
  .docker-batch-bar {
    left: 50%;
    max-width: calc(100vw - 32px);
  }
}

@media (max-width: 720px) {
  .docker-batch-bar {
    bottom: max(10px, env(safe-area-inset-bottom));
    display: grid;
    width: calc(100vw - 20px);
    max-width: none;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    overflow: visible;
    padding: 8px;
  }

  .docker-batch-bar__count,
  .docker-batch-bar .docker-batch-bar__progress {
    grid-column: 1 / -1;
  }

  .docker-batch-bar__count {
    margin: 0;
    padding: 2px 6px 4px;
  }

  .docker-batch-bar button {
    min-height: 44px;
    justify-content: center;
    gap: 4px;
    padding: 0 4px;
  }

  .desktop-window__body .docker-batch-bar {
    width: calc(100% - 20px);
  }
}
</style>
