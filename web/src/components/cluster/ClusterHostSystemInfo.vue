<script setup lang="ts">
import { computed } from 'vue'
import OperatingSystemIcon from '@/components/overview/OperatingSystemIcon.vue'
import ClusterHoverInfo from '@/components/cluster/ClusterHoverInfo.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import type { HostSystemSummary } from '@/lib/clusterHostIdentity'
import { detectOperatingSystemIdentity } from '@/lib/operatingSystem'

const props = defineProps<{ system?: HostSystemSummary }>()

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const identity = computed(() => detectOperatingSystemIdentity(props.system))
const title = computed(() => props.system?.os?.trim() || phrase('系统信息未获取'))
const rows = computed(() => {
  const system = props.system
  if (!system) return []
  const cores = system.cpu?.cores ? `${system.cpu.cores} ${phrase('核')}` : ''
  return [
    { label: phrase('架构'), value: system.architecture?.trim() || '' },
    { label: phrase('内核'), value: system.kernel?.trim() || '' },
    { label: phrase('处理器'), value: [system.cpu?.model?.trim(), cores].filter(Boolean).join(' · ') },
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
