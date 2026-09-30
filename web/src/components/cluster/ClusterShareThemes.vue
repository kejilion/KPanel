<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import { formatBytes } from '@/lib/format'
import type { ShareThemeList } from '@/lib/shareThemes'
import { isOfficialScenePack, type ScenePack } from '@/lib/scenePacks'

const { t, locale } = useI18n()
const catalog = ref<ShareThemeList>()
const busy = ref('')
const loading = ref(true)
const error = ref('')
const notice = ref('')
const controller = new AbortController()
async function load() {
  loading.value = true
  try { catalog.value = await api.cluster.shareThemes(controller.signal); error.value = '' }
  catch { if (!controller.signal.aborted) error.value = t('cluster.themes.loadFailed') }
  finally { loading.value = false }
}
async function act(action: 'install' | 'delete' | 'select', pack?: ScenePack) {
  if (!catalog.value || busy.value) return
  busy.value = `${action}:${pack?.id || 'default'}`; error.value = ''; notice.value = ''
  try {
    if (action === 'install' && pack) await api.cluster.installShareTheme(pack.id, pack.resourceVersion)
    else if (action === 'delete' && pack) await api.cluster.deleteShareTheme(pack.id, pack.resourceVersion)
    else if (action === 'select') await api.cluster.selectShareTheme(pack?.id || '', catalog.value.resourceVersion)
    notice.value = t(action === 'install' ? 'cluster.themes.downloaded' : action === 'delete' ? 'cluster.themes.deleted' : 'cluster.themes.applied')
    await load()
  } catch { error.value = t('cluster.themes.actionFailed'); await loadAfterError() }
  finally { busy.value = '' }
}
async function loadAfterError() {
  try { catalog.value = await api.cluster.shareThemes(controller.signal) } catch { /* Keep actionable retry and last known state. */ }
}
onMounted(load)
onBeforeUnmount(() => controller.abort())
const text = (value?: Partial<Record<string, string>>) => value?.[locale.value] || value?.['en-US'] || ''
const swatch = (pack: ScenePack) => pack.theme ? { '--swatch-bg': pack.theme.neutral, '--swatch-brand': pack.theme.brand, '--swatch-signature': pack.theme.signature } : {}
</script>

<template>
  <section class="themes" :aria-label="t('cluster.themes.title')" :aria-busy="Boolean(busy) || loading">
    <header class="themes__head">
      <div>
        <h3>{{ t('cluster.themes.title') }}</h3>
        <p>{{ t('cluster.themes.hint') }}</p>
      </div>
      <a href="https://github.com/kejilion/KPanel/tree/main/share-themes" target="_blank" rel="noopener noreferrer">{{ t('cluster.themes.create') }}</a>
    </header>
    <p class="themes__safe">{{ t('cluster.themes.safe') }}</p>

    <ul class="themes__grid">
      <li class="theme-card" :class="{ 'is-current': catalog && !catalog.selected }">
        <div class="theme-card__swatch theme-card__swatch--default" role="img" :aria-label="t('cluster.themes.preview')"><span /><span /><span /></div>
        <div class="theme-card__body">
          <h4>{{ t('cluster.themes.default') }} <span class="theme-card__tag">{{ t('cluster.themes.builtin') }}</span></h4>
          <p>{{ t('cluster.themes.defaultDescription') }}</p>
        </div>
        <div class="theme-card__actions">
          <span v-if="catalog && !catalog.selected" class="theme-card__current">{{ t('cluster.themes.current') }}</span>
          <button v-else class="button button--secondary" type="button" :disabled="!catalog || Boolean(busy) || loading" @click="act('select')">{{ t('cluster.themes.apply') }}</button>
        </div>
      </li>

      <li v-for="pack in catalog?.packs || []" :key="pack.id" class="theme-card" :class="{ 'is-current': catalog?.selected === pack.id }">
        <div class="theme-card__swatch" :style="swatch(pack)" role="img" :aria-label="t('cluster.themes.preview')"><span /><span /><span /></div>
        <div class="theme-card__body">
          <h4>{{ text(pack.name) }}
            <span v-if="pack.installed && pack.installedVersion !== pack.version" class="theme-card__tag theme-card__tag--update">{{ t('cluster.themes.updateAvailable') }}</span>
            <span v-else-if="pack.installed" class="theme-card__tag">{{ t('cluster.themes.installed') }}</span>
          </h4>
          <p>{{ text(pack.description) }}</p>
          <p class="theme-card__meta">
            {{ isOfficialScenePack(pack) ? t('cluster.themes.official') : t('cluster.themes.community') }}
            · {{ t('cluster.themes.by', { name: pack.author?.name || '—' }) }} · v{{ pack.version }} · {{ formatBytes(pack.sizeBytes) }}
          </p>
        </div>
        <div class="theme-card__actions">
          <span v-if="catalog?.selected === pack.id" class="theme-card__current">{{ t('cluster.themes.current') }}</span>
          <button v-if="!pack.installed || pack.installedVersion !== pack.version" class="button button--secondary" type="button" :disabled="Boolean(busy) || loading" @click="act('install', pack)">{{ busy === `install:${pack.id}` ? t('cluster.themes.downloading') : t(pack.installed ? 'cluster.themes.update' : 'cluster.themes.download') }}</button>
          <button v-if="pack.installed && catalog?.selected !== pack.id" class="button button--secondary" type="button" :disabled="Boolean(busy) || loading" @click="act('select', pack)">{{ t('cluster.themes.apply') }}</button>
          <button v-if="pack.installed" class="button button--ghost" type="button" :disabled="Boolean(busy) || loading" @click="act('delete', pack)">{{ t('cluster.themes.delete') }}</button>
        </div>
      </li>
    </ul>

    <p v-if="loading" role="status">{{ t('cluster.themes.loading') }}</p>
    <p v-else-if="catalog && !catalog.packs.length" class="themes__note">{{ t('cluster.themes.empty') }}</p>
    <p v-if="catalog?.warning" class="themes__note" role="status">{{ t('cluster.themes.offline') }}</p>
    <div v-if="error" class="themes__error" role="alert">{{ error }} <button class="button button--secondary" type="button" :disabled="Boolean(busy) || loading" @click="load">{{ t('cluster.themes.retry') }}</button></div>
    <p v-else-if="notice" class="themes__note" role="status">{{ notice }}</p>
  </section>
</template>

<style scoped>
.themes { display: grid; gap: 0.75rem; padding: 1rem; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface-subtle); font-size: 0.875rem; line-height: 1.6; }
.themes__head { display: flex; align-items: flex-start; justify-content: space-between; flex-wrap: wrap; gap: 0.5rem 1rem; }
h3 { margin: 0; font-size: 1rem; line-height: 1.4; }
.themes__head p, .themes__safe, .themes__note, .theme-card__meta { margin: 0; color: var(--text-soft); font-size: 0.8125rem; }
.themes__head a { color: var(--brand-strong); font-size: 0.875rem; }
.themes__safe { padding-left: 0.75rem; border-left: 3px solid var(--border); }
.themes__grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 15rem), 1fr)); gap: 0.75rem; margin: 0; padding: 0; list-style: none; }
.theme-card { display: flex; flex-direction: column; gap: 0.75rem; min-width: 0; padding: 0.75rem; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface); }
.theme-card.is-current { border-color: var(--brand-strong); box-shadow: 0 0 0 1px var(--brand-strong); }
.theme-card__swatch { display: grid; align-content: center; gap: 0.375rem; height: 5.5rem; padding: 0 1rem; border-radius: calc(var(--radius) - 2px); background: var(--swatch-bg, var(--surface-subtle)); border: 1px solid var(--border); }
.theme-card__swatch span { display: block; height: 0.5rem; border-radius: 999px; background: var(--swatch-brand, var(--brand-strong)); }
.theme-card__swatch span:nth-child(1) { width: 62%; }
.theme-card__swatch span:nth-child(2) { width: 84%; opacity: 0.45; }
.theme-card__swatch span:nth-child(3) { width: 38%; background: var(--swatch-signature, var(--text-soft)); }
.theme-card__body { display: grid; gap: 0.25rem; flex: 1; min-width: 0; }
h4 { margin: 0; font-size: 0.9375rem; line-height: 1.4; overflow-wrap: anywhere; }
.theme-card__body p { margin: 0; overflow-wrap: anywhere; }
.theme-card__tag { display: inline-block; margin-left: 0.25rem; padding: 0 0.5rem; border: 1px solid var(--border); border-radius: 999px; color: var(--text-soft); font-size: 0.75rem; font-weight: 500; vertical-align: 0.0625rem; }
.theme-card__tag--update { border-color: var(--brand-strong); color: var(--brand-strong); }
.theme-card__actions { display: flex; align-items: center; flex-wrap: wrap; gap: 0.5rem; }
.theme-card__current { color: var(--brand-strong); font-size: 0.875rem; font-weight: 600; }
.theme-card__current::before { content: "✓ "; }
.themes__error { color: var(--danger); }
.button { font-size: 0.875rem; min-height: 2.25rem; padding: 0.375rem 0.75rem; }
</style>
