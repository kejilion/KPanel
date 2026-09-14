<script setup lang="ts">
import { computed } from 'vue'
import {
  ArrowRight,
  Copy,
  ExternalLink,
  LoaderCircle,
  RefreshCw,
  ShieldCheck,
  TriangleAlert,
} from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { useI18n } from '@/i18n'
import {
  normalizedKPanelVersion,
  officialKPanelReleaseURL,
  releaseMatchesTarget,
} from '@/lib/kpanelUpdate'
import { useToast } from '@/stores/toast'
import type { KPanelReleaseInfo, KPanelReleaseNoteKind } from '@/types/api'

const props = withDefaults(defineProps<{
  open: boolean
  release?: KPanelReleaseInfo
  loading?: boolean
  error?: string
  currentVersion?: string
  targetVersion?: string
  targetDigest?: string
  channel?: 'stable' | 'preview'
  icon?: string
  busy?: boolean
  strategy?: 'automatic' | 'script'
}>(), {
  release: undefined,
  loading: false,
  error: '',
  currentVersion: '',
  targetVersion: '',
  targetDigest: '',
  channel: 'stable',
  icon: '/app-icons/kpanel.webp',
  busy: false,
  strategy: 'script',
})

const emit = defineEmits<{
  close: []
  confirm: []
  retry: []
}>()

const i18n = useI18n()
const toast = useToast()

const matchedRelease = computed(() => (
  props.release && releaseMatchesTarget(props.release, props.targetVersion, props.targetDigest)
    ? props.release
    : undefined
))
const releaseMismatch = computed(() => Boolean(props.release && !matchedRelease.value))
const currentLabel = computed(() => versionLabel(props.currentVersion, i18n.t('kpanelUpdate.currentUnknown')))
const targetLabel = computed(() => versionLabel(
  props.targetVersion || matchedRelease.value?.version,
  i18n.t('kpanelUpdate.latestVersion'),
))
const releaseURL = computed(() => {
  const version = matchedRelease.value?.version || normalizedKPanelVersion(props.targetVersion)
  return officialKPanelReleaseURL(version)
})
const channelLabel = computed(() => i18n.t(
  (matchedRelease.value?.channel || props.channel) === 'preview'
    ? 'kpanelUpdate.channel.preview'
    : 'kpanelUpdate.channel.stable',
))
const publishedLabel = computed(() => {
  const value = matchedRelease.value?.publishedAt
  if (!value) return ''
  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) ? new Date(timestamp).toLocaleDateString() : ''
})
const safetyKey = computed(() => (
  props.strategy === 'automatic'
    ? 'kpanelUpdate.safety.automatic'
    : 'kpanelUpdate.safety.script'
))
const primaryLabel = computed(() => (
  props.busy
    ? i18n.t('kpanelUpdate.starting')
    : matchedRelease.value?.version || normalizedKPanelVersion(props.targetVersion)
      ? i18n.t('kpanelUpdate.updateTo', { version: targetLabel.value })
      : i18n.t('kpanelUpdate.start')
))

function versionLabel(value: string | undefined, fallback: string): string {
  const normalized = normalizedKPanelVersion(value)
  return normalized ? `v${normalized}` : fallback
}

function kindLabel(kind: KPanelReleaseNoteKind): string {
  return i18n.t(`kpanelUpdate.kind.${kind}`)
}

async function copyReleaseURL(): Promise<void> {
  try {
    if (!navigator.clipboard?.writeText) throw new Error('clipboard_unavailable')
    await navigator.clipboard.writeText(releaseURL.value)
    toast.success(i18n.t('kpanelUpdate.linkCopied'))
  } catch {
    toast.danger(i18n.t('kpanelUpdate.copyFailed'), i18n.t('kpanelUpdate.copyFailedDetail'))
  }
}
</script>

<template>
  <ModalDialog
    :open="open"
    :title="i18n.t('kpanelUpdate.title')"
    :description="i18n.t('kpanelUpdate.description')"
    size="large"
    :close-disabled="busy"
    @close="emit('close')"
  >
    <div class="kpanel-update-dialog" data-testid="kpanel-update-dialog">
      <section class="kpanel-update-identity" aria-label="KPanel update version">
        <img :src="icon" alt="" />
        <div class="kpanel-update-identity__product">
          <strong>KPanel</strong>
          <small>{{ channelLabel }}</small>
        </div>
        <div class="kpanel-update-version">
          <span><small>{{ i18n.t('kpanelUpdate.current') }}</small><strong>{{ currentLabel }}</strong></span>
          <ArrowRight :size="18" aria-hidden="true" />
          <span class="is-target"><small>{{ i18n.t('kpanelUpdate.target') }}</small><strong>{{ targetLabel }}</strong></span>
        </div>
      </section>

      <section class="kpanel-release-card" aria-live="polite">
        <header>
          <div>
            <strong>{{ i18n.t('kpanelUpdate.releaseNotes') }}</strong>
            <small v-if="publishedLabel">
              {{ publishedLabel }}
            </small>
          </div>
          <span v-if="matchedRelease?.stale" class="kpanel-release-source is-stale">
            {{ i18n.t('kpanelUpdate.source.stale') }}
          </span>
          <span v-else-if="matchedRelease?.cached" class="kpanel-release-source">
            {{ i18n.t('kpanelUpdate.source.cached') }}
          </span>
        </header>

        <div v-if="loading" class="kpanel-release-loading">
          <LoaderCircle class="spin" :size="18" />
          <span>{{ i18n.t('kpanelUpdate.loadingNotes') }}</span>
        </div>
        <ul v-else-if="matchedRelease?.notes?.length" class="kpanel-release-notes">
          <li v-for="(note, index) in matchedRelease.notes" :key="`${note.kind}-${index}`">
            <span :class="`is-${note.kind}`">{{ kindLabel(note.kind) }}</span>
            <p>{{ note.text }}</p>
          </li>
        </ul>
        <div v-else class="kpanel-release-unavailable">
          <TriangleAlert :size="18" />
          <div>
            <strong>{{ i18n.t(releaseMismatch ? 'kpanelUpdate.notesMismatch' : 'kpanelUpdate.notesUnavailable') }}</strong>
            <p>{{ error || i18n.t('kpanelUpdate.notesUnavailableDetail') }}</p>
          </div>
          <button v-if="error" class="button button--ghost button--small" type="button" @click="emit('retry')">
            <RefreshCw :size="14" /> {{ i18n.t('kpanelUpdate.retry') }}
          </button>
        </div>
      </section>

      <section v-if="matchedRelease?.upgradeNotes?.length" class="kpanel-upgrade-notes">
        <header><TriangleAlert :size="17" /><strong>{{ i18n.t('kpanelUpdate.upgradeNotes') }}</strong></header>
        <ul>
          <li v-for="(note, index) in matchedRelease.upgradeNotes" :key="index">{{ note }}</li>
        </ul>
      </section>

      <div class="kpanel-update-safety">
        <ShieldCheck :size="18" />
        <span>{{ i18n.t(safetyKey) }}</span>
      </div>

      <section class="kpanel-release-link">
        <div>
          <strong>{{ i18n.t('kpanelUpdate.fullNotes') }}</strong>
          <code data-testid="kpanel-release-url">{{ releaseURL }}</code>
        </div>
        <button class="icon-button" type="button" :title="i18n.t('kpanelUpdate.copyLink')" :aria-label="i18n.t('kpanelUpdate.copyLink')" @click="copyReleaseURL">
          <Copy :size="16" />
        </button>
        <a class="button button--secondary button--small" :href="releaseURL" target="_blank" rel="noopener noreferrer">
          <ExternalLink :size="15" /> {{ i18n.t('kpanelUpdate.openFullNotes') }}
        </a>
      </section>
      <p class="kpanel-network-note">{{ i18n.t('kpanelUpdate.networkNote') }}</p>
    </div>

    <template #footer>
      <button class="button button--secondary" type="button" :disabled="busy" @click="emit('close')">
        {{ i18n.t('common.cancel') }}
      </button>
      <button class="button button--primary" type="button" :disabled="busy" @click="emit('confirm')">
        <LoaderCircle v-if="busy" class="spin" :size="16" />
        {{ primaryLabel }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.kpanel-update-dialog {
  display: grid;
  gap: 14px;
}

.kpanel-update-identity {
  display: grid;
  grid-template-columns: 48px minmax(110px, 1fr) minmax(270px, auto);
  align-items: center;
  gap: 12px;
  padding: 13px 14px;
  border: 1px solid color-mix(in srgb, var(--brand) 20%, var(--border));
  border-radius: var(--radius);
  background: color-mix(in srgb, var(--brand) 6%, var(--surface));
}

.kpanel-update-identity > img {
  width: 48px;
  height: 48px;
  border-radius: var(--radius);
}

.kpanel-update-identity__product,
.kpanel-update-identity__product strong,
.kpanel-update-identity__product small,
.kpanel-update-version span {
  display: grid;
}

.kpanel-update-identity__product strong {
  font-size: 15px;
}

.kpanel-update-identity__product small,
.kpanel-update-version small,
.kpanel-release-card header small,
.kpanel-network-note {
  color: var(--text-muted);
}

.kpanel-update-version {
  display: flex;
  align-items: center;
  gap: 12px;
}

.kpanel-update-version span {
  min-width: 96px;
  gap: 2px;
}

.kpanel-update-version strong {
  font-variant-numeric: tabular-nums;
}

.kpanel-update-version .is-target strong {
  color: var(--brand-strong);
}

.kpanel-release-card,
.kpanel-upgrade-notes {
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
}

.kpanel-release-card > header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 11px 14px;
  border-bottom: 1px solid var(--border-subtle);
  background: var(--surface-subtle);
}

.kpanel-release-card > header > div {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.kpanel-release-source {
  padding: 3px 7px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--brand) 9%, var(--surface));
  color: var(--brand-strong);
  font-size: 12px;
}

.kpanel-release-source.is-stale {
  background: color-mix(in srgb, var(--warning) 12%, var(--surface));
  color: var(--warning);
}

.kpanel-release-loading,
.kpanel-release-unavailable {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 84px;
  padding: 14px;
  color: var(--text-muted);
}

.kpanel-release-unavailable > div {
  flex: 1;
}

.kpanel-release-unavailable strong {
  color: var(--text);
}

.kpanel-release-unavailable p {
  margin: 3px 0 0;
  font-size: 13px;
}

.kpanel-release-notes {
  display: grid;
  gap: 0;
  max-height: 270px;
  margin: 0;
  padding: 5px 14px;
  overflow-y: auto;
  list-style: none;
}

.kpanel-release-notes li {
  display: grid;
  grid-template-columns: 48px 1fr;
  align-items: start;
  gap: 10px;
  padding: 9px 0;
  border-bottom: 1px solid var(--border-subtle);
}

.kpanel-release-notes li:last-child {
  border-bottom: 0;
}

.kpanel-release-notes li > span {
  margin-top: 1px;
  padding: 2px 6px;
  border-radius: var(--radius-sm);
  background: var(--surface-subtle);
  color: var(--text-muted);
  font-size: 12px;
  text-align: center;
}

.kpanel-release-notes li > span.is-added,
.kpanel-release-notes li > span.is-performance {
  background: color-mix(in srgb, var(--success) 12%, var(--surface));
  color: var(--success);
}

.kpanel-release-notes li > span.is-fixed,
.kpanel-release-notes li > span.is-security {
  background: color-mix(in srgb, var(--brand) 10%, var(--surface));
  color: var(--brand-strong);
}

.kpanel-release-notes p {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.55;
}

.kpanel-upgrade-notes {
  padding: 11px 14px;
  border-color: color-mix(in srgb, var(--warning) 30%, var(--border));
  background: color-mix(in srgb, var(--warning) 6%, var(--surface));
}

.kpanel-upgrade-notes header,
.kpanel-update-safety {
  display: flex;
  align-items: center;
  gap: 8px;
}

.kpanel-upgrade-notes ul {
  margin: 7px 0 0 25px;
  padding: 0;
  color: var(--text-secondary);
  font-size: 13px;
}

.kpanel-upgrade-notes li + li {
  margin-top: 5px;
}

.kpanel-update-safety {
  padding: 10px 12px;
  border-radius: var(--radius);
  background: color-mix(in srgb, var(--brand) 6%, var(--surface-subtle));
  color: var(--text-secondary);
  font-size: 13px;
}

.kpanel-update-safety svg {
  flex: 0 0 auto;
  color: var(--brand-strong);
}

.kpanel-release-link {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 9px;
  padding: 10px 11px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface-subtle);
}

.kpanel-release-link > div {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.kpanel-release-link strong {
  font-size: 12px;
}

.kpanel-release-link code {
  overflow: hidden;
  color: var(--text-muted);
  font: 12px/1.4 var(--font-mono);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.kpanel-network-note {
  margin: -4px 2px 0;
  font-size: 12px;
  line-height: 1.5;
}

@media (max-width: 680px) {
  .kpanel-update-identity {
    grid-template-columns: 44px 1fr;
  }

  .kpanel-update-identity > img {
    width: 44px;
    height: 44px;
  }

  .kpanel-update-version {
    grid-column: 1 / -1;
    justify-content: space-between;
    padding-top: 4px;
  }

  .kpanel-release-link {
    grid-template-columns: minmax(0, 1fr) auto;
  }

  .kpanel-release-link .button {
    grid-column: 1 / -1;
    justify-content: center;
  }
}
</style>
