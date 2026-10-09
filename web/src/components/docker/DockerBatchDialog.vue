<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { CircleCheck, CircleDashed, CircleHelp, CircleMinus, CircleX, LoaderCircle, TriangleAlert } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import {
  dockerBatchDescription,
  dockerBatchIsDanger,
  dockerBatchNotes,
  dockerBatchSelfWarning,
  dockerBatchTitle,
  summarizeDockerBatch,
  type DockerBatchItemStatus,
  type DockerBatchPlan,
  type DockerBatchRun,
} from '@/lib/dockerBatch'
import type { DockerContainer } from '@/types/api'

const props = defineProps<{
  open: boolean
  plan: DockerBatchPlan
  run?: DockerBatchRun
  readOnly?: boolean
}>()

const emit = defineEmits<{
  confirm: []
  close: []
  stop: []
  retry: []
}>()

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

// Modals render outside the page's phrase observer, so badges get explicit labels.
const stateLabels: Record<DockerContainer['state'], string> = {
  running: '运行中',
  paused: '已暂停',
  restarting: '重启中',
  exited: '已退出',
  created: '已创建',
  dead: '异常退出',
  unknown: '未知',
}

const statusLabels: Record<DockerBatchItemStatus, string> = {
  pending: '等待执行',
  running: '正在执行',
  succeeded: '已完成',
  failed: '失败',
  cancelled: '未执行',
  unknown: '结果未确认',
}

const items = computed(() => props.run?.items ?? props.plan.items.map((item) => ({ ...item, status: 'pending' as const, message: '' })))
const summary = computed(() => summarizeDockerBatch({ items: items.value }))
const phase = computed(() => props.run?.phase ?? 'confirm')
const danger = computed(() => dockerBatchIsDanger(props.plan.action))
const notes = computed(() => dockerBatchNotes(props.plan))
const selfWarning = computed(() => dockerBatchSelfWarning(props.plan))
const description = computed(() => phrase(dockerBatchDescription(props.plan)))
const retryable = computed(() => phase.value === 'finished' && summary.value.failed > 0)
const progressText = computed(() => {
  const { done, total, succeeded, failed, cancelled, unknown } = summary.value
  const parts = [phrase(`已处理 ${done}/${total}`), phrase(`成功 ${succeeded}`)]
  if (failed) parts.push(phrase(`失败 ${failed}`))
  if (unknown) parts.push(phrase(`未确认 ${unknown}`))
  if (cancelled) parts.push(phrase(`未执行 ${cancelled}`))
  return parts.join(' · ')
})
const resultAlert = computed(() => {
  if (phase.value !== 'finished') return undefined
  const { total, succeeded, failed, unknown } = summary.value
  if (succeeded === total) return { tone: 'success', text: phrase('全部完成。') }
  if (failed) return { tone: 'warning', text: phrase('部分项目没有成功，原因见下方列表；可以重试失败项。') }
  if (unknown) return { tone: 'warning', text: phrase('部分项目的结果未能确认，请刷新后核对实际状态。') }
  return { tone: 'info', text: phrase('已按要求停止，剩余项目没有执行。') }
})

const list = ref<HTMLElement>()
// Follow the item being worked on, then land on the first one that needs attention.
const focusKey = computed(() => {
  if (!props.run) return ''
  if (phase.value === 'finished') return items.value.find((item) => item.status === 'failed' || item.status === 'unknown')?.key || ''
  return items.value.find((item) => item.status === 'running')?.key || ''
})
watch([focusKey, () => props.open], async ([key, open]) => {
  if (!key || !open) return
  await nextTick()
  const index = items.value.findIndex((item) => item.key === key)
  const element = list.value?.children[index] as HTMLElement | undefined
  element?.scrollIntoView?.({ block: 'nearest' })
})

function requestClose(): void {
  emit('close')
}
</script>

<template>
  <ModalDialog
    :open="open"
    :title="phrase(dockerBatchTitle(plan.action))"
    :description="description"
    size="medium"
    @close="requestClose"
  >
    <div class="docker-batch" :class="{ 'is-running': run }">
      <div v-if="selfWarning && phase === 'confirm'" class="inline-alert inline-alert--warning docker-batch__alert" role="note">
        <TriangleAlert :size="17" aria-hidden="true" />
        <span>{{ phrase(selfWarning) }}</span>
      </div>

      <div v-if="run" class="docker-batch__progress" role="status" aria-live="polite">
        <progress :value="summary.done" :max="Math.max(summary.total, 1)">{{ summary.done }}/{{ summary.total }}</progress>
        <span>{{ progressText }}</span>
      </div>

      <div
        v-if="resultAlert"
        class="inline-alert docker-batch__alert"
        :class="`inline-alert--${resultAlert.tone}`"
        role="status"
      >{{ resultAlert.text }}</div>

      <ol v-if="items.length" ref="list" class="docker-batch__list" :aria-label="phrase('批量操作项目')">
        <li v-for="item in items" :key="item.key" class="docker-batch__item" :class="`is-${item.status}`">
          <span class="docker-batch__status-icon" aria-hidden="true">
            <LoaderCircle v-if="item.status === 'running'" class="spin" :size="17" />
            <CircleCheck v-else-if="item.status === 'succeeded'" :size="17" />
            <CircleX v-else-if="item.status === 'failed'" :size="17" />
            <CircleHelp v-else-if="item.status === 'unknown'" :size="17" />
            <CircleMinus v-else-if="item.status === 'cancelled'" :size="17" />
            <CircleDashed v-else :size="17" />
          </span>
          <span class="docker-batch__copy">
            <strong>{{ phrase(item.label) }}</strong>
            <small :title="item.detail">{{ item.detail }}</small>
            <small v-if="item.note" class="docker-batch__note">{{ phrase(item.note) }}</small>
            <small v-if="item.message" class="docker-batch__message">{{ phrase(item.message) }}</small>
          </span>
          <span class="docker-batch__state">
            <StatusBadge v-if="phase === 'confirm' && item.state" :status="item.state" :label="phrase(stateLabels[item.state])" subtle />
            <span v-else-if="run">{{ phrase(statusLabels[item.status]) }}</span>
          </span>
        </li>
      </ol>

      <details v-if="plan.skipped.length" class="docker-batch__skipped" :open="!items.length">
        <summary>{{ phrase(`跳过 ${plan.skipped.length} 项（不适用此操作）`) }}</summary>
        <ul>
          <li v-for="skip in plan.skipped" :key="skip.key">
            <strong>{{ phrase(skip.label) }}</strong>
            <small>{{ phrase(skip.reason) }}</small>
          </li>
        </ul>
      </details>

      <ul v-if="phase === 'confirm'" class="docker-batch__notes">
        <li v-for="note in notes" :key="note">{{ phrase(note) }}</li>
      </ul>
      <p v-else-if="phase !== 'finished'" class="docker-batch__hint">{{ phrase('关闭窗口后会在后台继续；离开 Docker 页面会停止派发剩余项目，已发出的操作照常完成。') }}</p>
    </div>

    <template #footer>
      <template v-if="phase === 'confirm'">
        <button class="button button--secondary" type="button" @click="requestClose">{{ phrase('取消') }}</button>
        <button
          class="button"
          :class="danger ? 'button--danger' : 'button--primary'"
          type="button"
          :disabled="!items.length || readOnly"
          :title="readOnly ? phrase('Agent 当前只读或不可用，不能执行写入操作') : undefined"
          @click="emit('confirm')"
        >{{ phrase(`确认执行（${items.length} 项）`) }}</button>
      </template>
      <template v-else-if="phase !== 'finished'">
        <button class="button button--secondary" type="button" :disabled="phase === 'stopping'" @click="emit('stop')">
          {{ phrase(phase === 'stopping' ? '正在停止…' : '停止后续操作') }}
        </button>
        <button class="button button--primary" type="button" @click="requestClose">{{ phrase('后台运行') }}</button>
      </template>
      <template v-else>
        <button v-if="retryable" class="button button--secondary" type="button" :disabled="readOnly" @click="emit('retry')">{{ phrase(`重试失败项（${summary.failed}）`) }}</button>
        <button class="button button--primary" type="button" @click="requestClose">{{ phrase('完成') }}</button>
      </template>
    </template>
  </ModalDialog>
</template>

<style scoped>
.docker-batch {
  display: grid;
  gap: 12px;
}

.docker-batch__alert {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 14px;
  line-height: 1.55;
}

.docker-batch__alert.inline-alert--success {
  color: var(--success);
  background: var(--success-soft);
  border-color: color-mix(in srgb, var(--success) 22%, transparent);
}

.docker-batch__alert svg {
  flex: 0 0 auto;
  margin-top: 2px;
}

.docker-batch__progress {
  display: grid;
  gap: 6px;
  color: var(--text-soft);
  font-size: 14px;
  font-variant-numeric: tabular-nums;
}

.docker-batch__progress progress {
  width: 100%;
  height: 8px;
  overflow: hidden;
  border: 0;
  border-radius: 999px;
  background: var(--surface-subtle);
  accent-color: var(--brand);
}

.docker-batch__progress progress::-webkit-progress-bar {
  border-radius: 999px;
  background: var(--surface-subtle);
}

.docker-batch__progress progress::-webkit-progress-value {
  border-radius: 999px;
  background: var(--brand);
}

.docker-batch__progress progress::-moz-progress-bar {
  border-radius: 999px;
  background: var(--brand);
}

.docker-batch__list {
  display: grid;
  max-height: min(46vh, 420px);
  margin: 0;
  padding: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  list-style: none;
}

.docker-batch__item {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: start;
  gap: 10px;
  padding: 10px 12px;
}

.docker-batch__item + .docker-batch__item {
  border-top: 1px solid var(--border);
}

.docker-batch__status-icon {
  display: grid;
  width: 20px;
  height: 22px;
  place-items: center;
  color: var(--muted);
}

.docker-batch__item.is-running .docker-batch__status-icon { color: var(--brand); }
.docker-batch__item.is-succeeded .docker-batch__status-icon { color: var(--success); }
.docker-batch__item.is-failed .docker-batch__status-icon { color: var(--danger); }
.docker-batch__item.is-unknown .docker-batch__status-icon { color: var(--amber); }

.docker-batch__copy {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.docker-batch__copy strong {
  color: var(--text);
  font-size: 14px;
  overflow-wrap: anywhere;
}

.docker-batch__copy small {
  overflow: hidden;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.45;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.docker-batch__copy .docker-batch__note {
  color: var(--text-soft);
  white-space: normal;
}

.docker-batch__copy .docker-batch__message {
  color: var(--danger);
  white-space: normal;
  overflow-wrap: anywhere;
}

.docker-batch__item.is-unknown .docker-batch__message {
  color: var(--text-soft);
}

.docker-batch__state {
  display: flex;
  min-height: 22px;
  align-items: center;
  color: var(--muted);
  font-size: 13px;
  white-space: nowrap;
}

.docker-batch__item.is-failed .docker-batch__state { color: var(--danger); }
.docker-batch__item.is-succeeded .docker-batch__state { color: var(--success); }

.docker-batch__skipped {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface-subtle);
}

.docker-batch__skipped summary {
  padding: 10px 12px;
  color: var(--text-soft);
  font-size: 14px;
  cursor: pointer;
}

.docker-batch__skipped ul {
  display: grid;
  max-height: 180px;
  margin: 0;
  padding: 0 12px 10px;
  overflow-y: auto;
  gap: 6px;
  list-style: none;
}

.docker-batch__skipped li {
  display: grid;
  gap: 1px;
}

.docker-batch__skipped strong {
  color: var(--text);
  font-size: 14px;
  overflow-wrap: anywhere;
}

.docker-batch__skipped small {
  color: var(--muted);
  font-size: 13px;
  line-height: 1.45;
}

.docker-batch__notes {
  display: grid;
  gap: 4px;
  margin: 0;
  padding-left: 20px;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.55;
}

.docker-batch__hint {
  margin: 0;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.55;
}

@media (max-width: 720px) {
  .docker-batch__item {
    grid-template-columns: auto minmax(0, 1fr);
  }

  .docker-batch__state {
    grid-column: 2;
    min-height: 0;
  }
}
</style>
