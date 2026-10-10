<script setup lang="ts">
import { computed } from 'vue'
import OperatingSystemIcon from '@/components/overview/OperatingSystemIcon.vue'
import ClusterHoverInfo from '@/components/cluster/ClusterHoverInfo.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { detectOperatingSystemIdentity } from '@/lib/operatingSystem'
import type { ClusterTelemetry } from '@/types/api'

const props = defineProps<{ telemetry?: ClusterTelemetry }>()

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const identity = computed(() => detectOperatingSystemIdentity(props.telemetry))
const title = computed(() => props.telemetry?.os?.trim() || phrase('系统信息未获取'))
const rows = computed(() => {
  const telemetry = props.telemetry
  if (!telemetry) return []
  const cores = telemetry.cpu?.cores ? `${telemetry.cpu.cores} ${phrase('核')}` : ''
  return [
    { label: phrase('架构'), value: telemetry.architecture?.trim() || '' },
    { label: phrase('内核'), value: telemetry.kernel?.trim() || '' },
    { label: phrase('处理器'), value: [telemetry.cpu?.model?.trim(), cores].filter(Boolean).join(' · ') },
  ].filter((row) => row.value)
})
const label = computed(() => [phrase('系统'), title.value, ...rows.value.map((row) => row.value)].join(' · '))
</script>

<template>
  <ClusterHoverInfo class="cluster-host-system" :label="label" :title="title" :rows="rows">
    <template #trigger>
      <OperatingSystemIcon :distro="identity.key" :label="identity.label" :show-tooltip="false" />
    </template>
  </ClusterHoverInfo>
</template>
