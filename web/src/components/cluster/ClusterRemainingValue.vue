<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { ChevronRight } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { useI18n } from '@/i18n'
import { formatClusterMoney, summarizeRemainingValue } from '@/lib/clusterRemainingValue'
import type { ClusterHost, ClusterHostDetails } from '@/types/api'

const props = defineProps<{ hosts: readonly ClusterHost[]; details: Readonly<Record<string, ClusterHostDetails>> }>()
const emit = defineEmits<{ manage: [host: ClusterHost] }>()
const { t, locale } = useI18n()
const open = ref(false)
const now = ref(new Date())
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => { timer = setInterval(() => { now.value = new Date() }, 60_000) })
onBeforeUnmount(() => { if (timer) clearInterval(timer) })
const summary = computed(() => summarizeRemainingValue(props.hosts, props.details, now.value))
const primary = computed(() => summary.value.groups[0])
const money = (amount: number, currency: string) => formatClusterMoney(amount, currency, locale.value)
const coverage = computed(() => t('cluster.value.coverage', {
  included: summary.value.included, total: props.hosts.length, excluded: summary.value.excluded,
}))
function show() { now.value = new Date(); open.value = true }
async function manage(host: ClusterHost) {
  open.value = false
  await nextTick()
  emit('manage', host)
}
</script>

<template>
  <div class="cluster-value">
    <button class="cluster-value__trigger" type="button" aria-haspopup="dialog" :title="coverage" @click="show">
      <strong>{{ primary ? money(primary.remaining, primary.currency) : '—' }}</strong>
      <span>{{ t('cluster.value.title') }} <ChevronRight :size="13" aria-hidden="true" /></span>
      <small v-if="summary.groups.length > 1">{{ t('cluster.value.otherCurrencies', { currency: primary?.currency || '', count: summary.groups.length - 1 }) }}</small>
    </button>
    <ModalDialog :open="open" :title="t('cluster.value.title')" :description="t('cluster.value.description')" size="large" @close="open = false">
      <div class="cluster-value__content">
        <p class="cluster-value__coverage">{{ coverage }}</p>
        <div v-if="summary.groups.length" class="cluster-value__groups">
          <section v-for="group in summary.groups" :key="group.currency" class="cluster-value__group" :aria-label="group.currency">
            <h3>{{ group.currency }} <small>{{ t('cluster.value.hostCount', { count: group.count }) }}</small></h3>
            <strong>{{ money(group.remaining, group.currency) }}</strong>
            <dl>
              <dt>{{ t('cluster.value.cycleTotal') }}</dt><dd>{{ money(group.price, group.currency) }}</dd>
              <dt>{{ t('cluster.value.monthlyCost') }}</dt><dd>{{ money(group.monthlyCost, group.currency) }}</dd>
            </dl>
          </section>
        </div>
        <p v-else class="cluster-value__empty" role="status">{{ t('cluster.value.empty') }}</p>
        <p class="cluster-value__formula">{{ t('cluster.value.formula') }}</p>
        <ul class="cluster-value__hosts">
          <li v-for="row in summary.rows" :key="row.host.id">
            <div class="cluster-value__identity">
              <strong>{{ row.host.name }}</strong>
              <small>{{ row.details?.price || t('cluster.value.missingPrice') }}</small>
              <small>{{ row.details?.expiresOn ? t('cluster.details.expirySummary', { date: row.details.expiresOn }) : t('cluster.value.missingExpiry') }}</small>
            </div>
            <div class="cluster-value__result">
              <strong v-if="row.estimate.remaining !== undefined">{{ money(row.estimate.remaining, row.estimate.currency) }}</strong>
              <small>{{ row.estimate.status === 'active' ? t('cluster.value.days', { days: row.estimate.remainingDays }) : t(`cluster.value.${row.estimate.status}`) }}</small>
              <button class="button button--secondary button--small" type="button" :aria-label="t('cluster.value.manageHost', { name: row.host.name })" @click="manage(row.host)">{{ t('cluster.value.manage') }}</button>
            </div>
          </li>
        </ul>
      </div>
      <template #footer><button class="button button--secondary" type="button" @click="open = false">{{ t('cluster.value.close') }}</button></template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.cluster-value { min-width: 0; }
.cluster-value__trigger {
  display: grid; justify-items: center; gap: .125rem; min-height: 3rem; width: 100%; padding: .125rem .25rem;
  border: 0; border-radius: var(--radius-sm); background: transparent; color: var(--brand); cursor: pointer;
}
.cluster-value__trigger:hover { background: var(--brand-soft); }
.cluster-value__trigger:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
.cluster-value__trigger strong { font-size: 1.1875rem; overflow-wrap: anywhere; }
.cluster-value__trigger span { display: flex; align-items: center; gap: .125rem; font-size: .875rem; }
.cluster-value__trigger small { color: var(--text-soft); font-size: .8125rem; }
.cluster-value__content { display: grid; gap: 1rem; font-size: .875rem; line-height: 1.5; }
.cluster-value__coverage, .cluster-value__formula, .cluster-value__empty { margin: 0; color: var(--text-soft); }
.cluster-value__groups { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 14rem), 1fr)); gap: .75rem; }
.cluster-value__group { min-width: 0; padding: 1rem; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface); }
.cluster-value__group h3 { display: flex; flex-wrap: wrap; justify-content: space-between; gap: .5rem; margin: 0 0 .5rem; font-size: 1rem; }
.cluster-value__group > strong { color: var(--brand); font-size: 1.375rem; overflow-wrap: anywhere; }
.cluster-value__group dl { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: .25rem .75rem; margin: .75rem 0 0; }
.cluster-value__group dt { color: var(--text-soft); }
.cluster-value__group dd { margin: 0; text-align: right; overflow-wrap: anywhere; }
.cluster-value__hosts { display: grid; gap: .75rem; margin: 0; padding: 0; list-style: none; }
.cluster-value__hosts li { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: .75rem; border-top: 1px solid var(--border); padding-top: .75rem; }
.cluster-value__identity, .cluster-value__result { display: grid; align-content: start; gap: .25rem; min-width: 0; overflow-wrap: anywhere; }
.cluster-value__result { justify-items: end; text-align: right; }
.cluster-value__content small { font-size: .8125rem; color: var(--text-soft); }
.cluster-value__result .button { margin-top: .25rem; font-size: .875rem; }
</style>
