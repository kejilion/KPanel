<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { formatDateTime } from '@/lib/format'
import { lightHealthFresh, lightServiceLabel, lightUpdateLabel } from '@/lib/lightNodeHealth'
import type { LightNodeHealth } from '@/types/api'

const props = defineProps<{ health?: LightNodeHealth }>()
const now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => { timer = setInterval(() => { now.value = Date.now() }, 15_000) })
onBeforeUnmount(() => { if (timer) clearInterval(timer) })
const fresh = computed(() => lightHealthFresh(props.health, now.value))
const rows = [ ['timer', '自动检查'], ['telemetry', '遥测服务'], ['terminal', '终端服务'], ['file', '文件服务'], ['sshLogin', 'SSH 登录采集'] ] as const
function phrase(value: string): string { phraseCatalogVersion.value; return translatePhrase(value) }
function timestamp(value?: number): string { return value ? formatDateTime(new Date(value * 1000).toISOString()) : phrase('暂无记录') }
</script>

<template>
  <section class="light-health" :aria-label="phrase('更新与服务')">
    <h3>{{ phrase('更新与服务') }}</h3>
    <p class="light-health__hint">{{ phrase('在线仅表示遥测连接正常，服务状态与可用权限分别判断。') }}</p>
    <dl>
      <dt>{{ phrase('最近检查结果') }}</dt><dd>{{ phrase(lightUpdateLabel(health, now)) }}</dd>
      <dt>{{ phrase('最近检查') }}</dt><dd>{{ timestamp(health?.update.checkedAt) }}</dd>
      <dt>{{ phrase('最近完成') }}</dt><dd>{{ timestamp(health?.update.finishedAt) }}</dd>
      <template v-if="health?.update.errorCode"><dt>{{ phrase('错误码') }}</dt><dd><code>{{ health.update.errorCode }}</code></dd></template>
      <dt>{{ phrase('实际运行版本') }}</dt><dd>{{ health?.runtimeVersion || phrase('尚未上报') }}</dd>
      <dt>{{ phrase('观测时间') }}</dt><dd>{{ health?.observedAt ? formatDateTime(health.observedAt) : phrase('暂无记录') }}</dd>
    </dl>
    <p v-if="!fresh" class="light-health__hint">{{ phrase(health ? '观测已过期，请等待节点重新上报。' : '等待支持健康上报的节点连接；未上报不代表正常。') }}</p>
    <details>
      <summary>{{ phrase('查看服务状态') }}</summary>
      <dl>
        <template v-for="[key, label] in rows" :key="key">
          <dt>{{ phrase(label) }}</dt><dd>{{ phrase(health ? lightServiceLabel(health.services[key], fresh, key === 'timer') : '尚未上报') }}</dd>
        </template>
      </dl>
    </details>
  </section>
</template>

<style scoped>
.light-health { border-top: 1px solid var(--border); padding-top: 16px; font-size: 14px; line-height: 1.5; }
.light-health h3 { margin: 0 0 8px; font-size: 16px; }
.light-health__hint { margin: 8px 0; color: var(--muted); font-size: 13px; }
.light-health dl { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 2fr); gap: 8px 12px; margin: 12px 0; }
.light-health dt { color: var(--muted); }
.light-health dd { margin: 0; overflow-wrap: anywhere; }
.light-health summary { cursor: pointer; padding: 8px 0; font-weight: 600; }
.light-health summary:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
</style>
