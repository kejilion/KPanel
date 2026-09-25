<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import { Check, Download, LoaderCircle, Orbit, Trash2 } from '@lucide/vue'
import { api } from '@/lib/api'
import { useSceneMotionPreference } from '@/lib/desktopScenes/motionPreference'
import { DESKTOP_WALLPAPERS, useDesktopWallpaper, type DesktopWallpaperID } from '@/lib/desktopWallpapers'
import {
  formatPackSize,
  isOfficialScenePack,
  localizedText,
  scenePackFromWallpaper,
  scenePackWallpaper,
  type ScenePack,
  type ScenePackSource,
} from '@/lib/scenePacks'
import { useI18n } from '@/i18n'

/**
 * Static wallpapers and 3D scene packs (download, update, apply, delete). Shared by the desktop
 * wallpaper dialog and Settings; the parent decides how a choice is applied.
 */
const props = defineProps<{
  /** Loads the pack list each time the picker is shown. */
  visible: boolean
}>()

const emit = defineEmits<{
  select: [id: DesktopWallpaperID, pack?: ScenePack]
  /** The active pack was installed again; its running scene should restart. */
  reinstalled: [id: string]
}>()

const i18n = useI18n()
const uid = useId()
const wallpaperChoice = useDesktopWallpaper()
const selectedID = computed(() => wallpaperChoice.id.value)
const activeScenePack = computed(() => scenePackFromWallpaper(selectedID.value))
const scenePacks = ref<ScenePack[]>([])
const scenePacksLoading = ref(false)
const scenePacksError = ref(false)
const scenePackBusy = ref<{ id: string, action: 'install' | 'delete' }>()
const scenePackFailure = ref<{ id: string, action: 'install' | 'delete' }>()
const scenePackConfirmDelete = ref<string>()
const scenePackSource = ref<ScenePackSource>('auto')
const scenePackSources = ref<ScenePackSource[]>(['auto', 'github', 'mirror'])
const scenePackFilter = ref<'all' | 'installed'>('all')
const visibleScenePacks = computed(() =>
  scenePackFilter.value === 'installed' ? scenePacks.value.filter((pack) => pack.installed) : scenePacks.value,
)
const SCENE_PACK_SOURCE_LABELS = {
  auto: 'desktop.scenePacksSourceAuto',
  github: 'desktop.scenePacksSourceGithub',
  mirror: 'desktop.scenePacksSourceMirror',
} as const
const sceneMotion = useSceneMotionPreference()

async function loadScenePacks(): Promise<void> {
  scenePacksLoading.value = true
  scenePacksError.value = false
  try {
    const list = await api.desktop.scenePacks()
    scenePacks.value = list.packs
    scenePacksError.value = Boolean(list.warning)
    scenePackSource.value = list.source
    if (list.sources?.length) scenePackSources.value = list.sources
  } catch {
    scenePacksError.value = true
  } finally {
    scenePacksLoading.value = false
  }
}

async function installScenePack(pack: ScenePack): Promise<void> {
  if (scenePackBusy.value) return
  scenePackBusy.value = { id: pack.id, action: 'install' }
  scenePackFailure.value = undefined
  try {
    const installed = await api.desktop.installScenePack(pack.id, pack.resourceVersion)
    scenePacks.value = scenePacks.value.map((candidate) => (candidate.id === installed.id ? installed : candidate))
    if (activeScenePack.value === installed.id) emit('reinstalled', installed.id)
  } catch {
    scenePackFailure.value = { id: pack.id, action: 'install' }
    await loadScenePacks()
  } finally {
    scenePackBusy.value = undefined
  }
}

async function deleteScenePack(pack: ScenePack): Promise<void> {
  if (scenePackBusy.value) return
  // The first click arms the delete; the second one confirms it.
  if (scenePackConfirmDelete.value !== pack.id) {
    scenePackConfirmDelete.value = pack.id
    return
  }
  scenePackConfirmDelete.value = undefined
  scenePackBusy.value = { id: pack.id, action: 'delete' }
  scenePackFailure.value = undefined
  try {
    await api.desktop.deleteScenePack(pack.id, pack.resourceVersion)
    scenePacks.value = scenePacks.value.map((candidate) => (candidate.id === pack.id ? { ...candidate, installed: false, installedVersion: null, fileBase: null } : candidate))
    if (activeScenePack.value === pack.id) wallpaperChoice.resetToClassic()
    await loadScenePacks()
  } catch {
    scenePackFailure.value = { id: pack.id, action: 'delete' }
    await loadScenePacks()
  } finally {
    scenePackBusy.value = undefined
  }
}

async function onScenePackSourceChange(event: Event): Promise<void> {
  const next = (event.target as HTMLSelectElement).value as ScenePackSource
  const previous = scenePackSource.value
  scenePackSource.value = next
  try {
    await api.desktop.setScenePackSource(next)
    await loadScenePacks()
  } catch {
    scenePackSource.value = previous
    scenePacksError.value = true
  }
}

function onSceneMotionAlwaysChange(event: Event): void {
  sceneMotion.setMotionAlways((event.target as HTMLInputElement).checked)
}

watch(() => props.visible, (visible) => {
  scenePackConfirmDelete.value = undefined
  if (visible) void loadScenePacks()
}, { immediate: true })
</script>

<template>
  <div class="desktop-wallpaper-chooser">
    <h3 :id="`${uid}-static`" class="desktop-wallpaper-section__title">
      {{ i18n.t('desktop.wallpaperStaticTitle') }}
    </h3>
    <div
      class="desktop-wallpaper-picker"
      role="radiogroup"
      :aria-labelledby="`${uid}-static`"
    >
      <button
        v-for="wallpaper in DESKTOP_WALLPAPERS"
        :key="wallpaper.id"
        class="desktop-wallpaper-picker__option"
        :class="{ 'desktop-wallpaper-picker__option--selected': wallpaper.id === selectedID }"
        type="button"
        role="radio"
        :aria-checked="wallpaper.id === selectedID"
        :data-wallpaper-option="wallpaper.id"
        @click="emit('select', wallpaper.id)"
      >
        <span
          class="desktop-wallpaper-picker__preview"
          :style="{ backgroundImage: `url('${wallpaper.src}')` }"
          aria-hidden="true"
        />
        <span class="desktop-wallpaper-picker__copy">
          <strong>{{ i18n.t(wallpaper.nameKey) }}</strong>
          <small>{{ i18n.t(wallpaper.descriptionKey) }}</small>
        </span>
        <Check
          v-if="wallpaper.id === selectedID"
          class="desktop-wallpaper-picker__check"
          :size="17"
          aria-hidden="true"
        />
      </button>
    </div>
    <section class="desktop-scene-packs" :aria-labelledby="`${uid}-packs`">
      <div class="desktop-scene-packs__head">
        <div class="desktop-scene-packs__intro">
          <h3 :id="`${uid}-packs`" class="desktop-wallpaper-section__title">
            {{ i18n.t('desktop.scenePacksTitle') }}
          </h3>
          <p class="desktop-wallpaper-section__hint">{{ i18n.t('desktop.scenePacksHint') }}</p>
        </div>
        <div class="desktop-scene-packs__tools">
          <label class="desktop-scene-packs__source">
            <span>{{ i18n.t('desktop.scenePacksSource') }}</span>
            <select :value="scenePackSource" data-scene-pack-source @change="onScenePackSourceChange">
              <option v-for="source in scenePackSources" :key="source" :value="source">
                {{ i18n.t(SCENE_PACK_SOURCE_LABELS[source]) }}
              </option>
            </select>
          </label>
          <div class="desktop-scene-packs__filter" role="group" :aria-label="i18n.t('desktop.scenePacksTitle')">
            <button
              v-for="filter in (['all', 'installed'] as const)"
              :key="filter"
              type="button"
              :aria-pressed="scenePackFilter === filter"
              :data-scene-pack-filter="filter"
              @click="scenePackFilter = filter"
            >
              {{ i18n.t(filter === 'all' ? 'desktop.scenePacksFilterAll' : 'desktop.scenePacksFilterInstalled') }}
            </button>
          </div>
        </div>
      </div>
      <div v-if="sceneMotion.systemReducedMotion.value" class="desktop-wallpaper-motion" role="note">
        <p>{{ i18n.t('desktop.wallpaperSceneReducedMotion') }}</p>
        <label class="desktop-wallpaper-motion__toggle">
          <input
            type="checkbox"
            data-scene-motion-always
            :checked="sceneMotion.motionAlways.value"
            @change="onSceneMotionAlwaysChange"
          />
          <span>{{ i18n.t('desktop.wallpaperSceneMotionAlways') }}</span>
        </label>
      </div>
      <p v-if="scenePacksLoading && !scenePacks.length" class="desktop-scene-packs__status" role="status">
        <LoaderCircle class="spin" :size="16" aria-hidden="true" />
        {{ i18n.t('desktop.scenePacksLoading') }}
      </p>
      <div v-else-if="scenePacksError" class="desktop-scene-packs__status" role="alert">
        <span>{{ i18n.t('desktop.scenePacksLoadFailed') }}</span>
        <button type="button" class="button button--small" @click="loadScenePacks">{{ i18n.t('desktop.scenePacksRetry') }}</button>
      </div>
      <p v-if="!scenePacksLoading && !scenePacksError && !visibleScenePacks.length" class="desktop-scene-packs__status">
        {{ i18n.t(scenePackFilter === 'installed' ? 'desktop.scenePacksEmptyInstalled' : 'desktop.scenePacksEmpty') }}
      </p>
      <div v-if="visibleScenePacks.length" class="desktop-scene-packs__grid">
        <article
          v-for="pack in visibleScenePacks"
          :key="pack.id"
          class="desktop-scene-pack-card"
          :class="{ 'desktop-scene-pack-card--active': activeScenePack === pack.id }"
          :data-scene-pack-card="pack.id"
        >
          <div class="desktop-scene-pack-card__preview">
            <img :src="api.desktop.scenePackThumbURL(pack.id, pack.version)" alt="" loading="lazy" decoding="async" />
            <span class="desktop-scene-pack-card__badges">
              <span class="desktop-scene-pack-card__badge"><Orbit :size="13" aria-hidden="true" />3D</span>
              <span class="desktop-scene-pack-card__badge">
                {{ i18n.t(isOfficialScenePack(pack) ? 'desktop.scenePackOfficial' : 'desktop.scenePackCommunity') }}
              </span>
            </span>
            <Check v-if="activeScenePack === pack.id" class="desktop-wallpaper-picker__check" :size="17" aria-hidden="true" />
          </div>
          <div class="desktop-scene-pack-card__body">
            <strong>{{ localizedText(pack.name, i18n.locale.value) }}</strong>
            <p>{{ localizedText(pack.description, i18n.locale.value) }}</p>
            <span class="desktop-scene-pack-card__meta">
              {{ i18n.t('desktop.scenePackCameras', { count: pack.cameras.length }) }}
              · {{ formatPackSize(pack.sizeBytes) }} · v{{ pack.version }}<template v-if="pack.author"> · {{ pack.author.name }}</template>
            </span>
          </div>
          <div class="desktop-scene-pack-card__actions">
            <button
              v-if="!pack.installed || pack.installedVersion !== pack.version"
              type="button"
              class="button button--small button--primary"
              :disabled="Boolean(scenePackBusy)"
              :data-scene-pack-action="'install'"
              @click="installScenePack(pack)"
            >
              <LoaderCircle v-if="scenePackBusy?.id === pack.id" class="spin" :size="15" aria-hidden="true" />
              <Download v-else :size="15" aria-hidden="true" />
              {{ i18n.t(scenePackBusy?.id === pack.id ? 'desktop.scenePackDownloading' : pack.installed ? 'desktop.scenePackUpdate' : 'desktop.scenePackDownload') }}
            </button>
            <template v-if="pack.installed">
              <button
                type="button"
                class="button button--small button--primary"
                :disabled="activeScenePack === pack.id || Boolean(scenePackBusy)"
                :data-scene-pack-action="'apply'"
                @click="emit('select', scenePackWallpaper(pack.id) as DesktopWallpaperID, pack)"
              >
                {{ i18n.t(activeScenePack === pack.id ? 'desktop.scenePackActive' : 'desktop.scenePackApply') }}
              </button>
              <button
                type="button"
                class="button button--small"
                :class="{ 'button--danger': scenePackConfirmDelete === pack.id }"
                :disabled="Boolean(scenePackBusy)"
                :data-scene-pack-action="'delete'"
                @click="deleteScenePack(pack)"
              >
                <LoaderCircle v-if="scenePackBusy?.id === pack.id" class="spin" :size="15" aria-hidden="true" />
                <Trash2 v-else :size="15" aria-hidden="true" />
                {{ i18n.t(scenePackConfirmDelete === pack.id ? 'desktop.scenePackConfirmDelete' : 'desktop.scenePackDelete') }}
              </button>
            </template>
          </div>
          <p v-if="scenePackFailure?.id === pack.id" class="desktop-scene-pack-card__error" role="alert">
            {{ i18n.t(scenePackFailure.action === 'install' ? 'desktop.scenePackInstallFailed' : 'desktop.scenePackDeleteFailed') }}
          </p>
        </article>
      </div>
    </section>
  </div>
</template>
