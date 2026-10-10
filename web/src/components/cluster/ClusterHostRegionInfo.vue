<script setup lang="ts">
import { computed } from 'vue'
import { Globe2 } from '@lucide/vue'
import CountryFlagIcon from '@/components/overview/CountryFlagIcon.vue'
import ClusterHoverInfo from '@/components/cluster/ClusterHoverInfo.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { hostLocationLabel, splitAutonomousSystem } from '@/lib/clusterHostIdentity'
import type { PublicNetworkSummary } from '@/types/api'

/** `shared` words gaps as withheld, matching the anonymous share page and its globe. */
const props = defineProps<{ location?: PublicNetworkSummary; shared?: boolean }>()

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const countryCode = computed(() => props.location?.countryCode?.trim() || '')
const place = computed(() => hostLocationLabel(props.location))
const title = computed(() => place.value || phrase(props.shared ? '地区未公开' : '位置未获取'))
const rows = computed(() => {
  const network = splitAutonomousSystem(props.location?.isp)
  return [
    { label: 'ASN', value: network.asn || '' },
    { label: phrase('运营商'), value: network.organization || '' },
  ].filter((row) => row.value)
})
const note = computed(() => {
  if (props.location?.isp?.trim()) return ''
  return phrase(props.shared ? '网络信息未公开' : '运营商未知')
})
const label = computed(() => [phrase('地区'), title.value, props.location?.isp?.trim()].filter(Boolean).join(' · '))
</script>

<template>
  <ClusterHoverInfo class="cluster-host-region" :label="label" :title="title" :rows="rows" :note="note">
    <template #trigger>
      <CountryFlagIcon v-if="countryCode" :country-code="countryCode" :label="location?.country || countryCode" />
      <span v-else class="cluster-host-region__unknown"><Globe2 :size="14" /></span>
    </template>
  </ClusterHoverInfo>
</template>

<style scoped>
.cluster-host-region__unknown {
  display: inline-grid;
  width: 20px;
  height: 20px;
  place-items: center;
  color: var(--text-soft);
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: 50%;
}
</style>
