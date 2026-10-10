<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ChevronLeft, ChevronRight, ExternalLink, Info, LayoutGrid, Megaphone, Pause, Play, RefreshCw, TriangleAlert } from '@lucide/vue'
import { phraseCatalogVersion, translatePhrase, usePhraseCatalog } from '@/i18n/phrase'
import { getLocale } from '@/i18n'
import { ApiError, api } from '@/lib/api'
import { desktopWindowVisibleKey } from '@/lib/desktopRouteKeys'
import type { OfferItem, OffersSnapshot } from '@/types/api'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/OffersView/en-US').then((module) => module.default)
  : import('@/i18n/pages/OffersView/zh-TW').then((module) => module.default))

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

/**
 * 广告专栏: sponsored server and domain banners published with the app catalogue.
 * The page shows vendor artwork only; KPanel draws the frame, the "广告" label,
 * the target host and the carousel controls so mixed artwork still reads as one page.
 */
const MORE_OFFERS_URL = 'https://kejilion.pro/topvps/'
const AUTOPLAY_MS = 6000
const MAX_FEATURED = 3
// Breakpoints follow the page container, not the viewport, so a desktop
// window lays out like a classic page of the same width.
const WIDE_LAYOUT_MIN = 720
const THREE_COLUMN_MIN = 1120

const root = ref<HTMLElement>()
const snapshot = ref<OffersSnapshot>()
const loading = ref(true)
const refreshing = ref(false)
const loadError = ref('')
const failedImages = reactive(new Set<string>())
let controller: AbortController | undefined

async function load(refresh = false): Promise<void> {
  controller?.abort()
  const current = new AbortController()
  controller = current
  if (refresh) refreshing.value = true
  else loading.value = !snapshot.value
  loadError.value = ''
  try {
    snapshot.value = await api.offers.list({ refresh, signal: current.signal })
  } catch (error) {
    if (current.signal.aborted) return
    loadError.value = error instanceof ApiError && error.message
      ? error.message
      : phrase('无法读取广告专栏，请稍后重试。')
  } finally {
    if (controller === current) {
      loading.value = false
      refreshing.value = false
    }
  }
}

const items = computed(() => snapshot.value?.items || [])
const featured = computed(() => items.value.filter((item) => item.featured && item.wide).slice(0, MAX_FEATURED))
const wall = computed(() => items.value.filter((item) => !featured.value.includes(item)))

const width = ref(1200)
const narrow = computed(() => width.value < WIDE_LAYOUT_MIN)
const columns = computed(() => width.value >= THREE_COLUMN_MIN ? 3 : narrow.value ? 1 : 2)
/** The "more offers" tile fills the last wall row instead of leaving a hole. */
const moreLayout = computed<'tile' | 'span' | 'bar'>(() => {
  if (columns.value === 1) return 'bar'
  const used = wall.value.length % columns.value
  const free = used === 0 ? columns.value : columns.value - used
  if (free === columns.value) return 'bar'
  return free === 1 ? 'tile' : 'span'
})

const dateFormatter = computed(() => new Intl.DateTimeFormat(getLocale(), { year: 'numeric', month: 'short', day: 'numeric' }))
function formatDate(value?: string): string {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : dateFormatter.value.format(date)
}
const updatedLabel = computed(() => formatDate(snapshot.value?.updatedAt))
const fetchedLabel = computed(() => formatDate(snapshot.value?.fetchedAt))

function linkLabel(item: OfferItem): string {
  return `${item.vendor}：${item.alt}（广告，在新标签页打开 ${item.host}）`
}

function imageFailed(src?: string): boolean {
  return !src || failedImages.has(src)
}

// Carousel: one slide at a time, paused whenever the visitor is reading or
// the page is out of sight; reduced motion turns autoplay off entirely.
const current = ref(0)
const pausedByUser = ref(false)
const pointerInside = ref(false)
const focusInside = ref(false)
const documentVisible = ref(typeof document === 'undefined' || document.visibilityState !== 'hidden')
const motionQuery = typeof window !== 'undefined' && window.matchMedia
  ? window.matchMedia('(prefers-reduced-motion: reduce)')
  : undefined
const reducedMotion = ref(Boolean(motionQuery?.matches))
const windowVisible = inject(desktopWindowVisibleKey, computed(() => true))
const activeSlide = computed(() => featured.value[current.value])
const autoplay = computed(() => featured.value.length > 1 && !pausedByUser.value && !pointerInside.value &&
  !focusInside.value && !reducedMotion.value && documentVisible.value && windowVisible.value)
let autoplayTimer: number | undefined

function showSlide(index: number): void {
  const count = featured.value.length
  if (!count) return
  current.value = (index + count) % count
}

watch(featured, (list) => {
  if (current.value >= list.length) current.value = 0
})
watch([autoplay, current], () => {
  window.clearTimeout(autoplayTimer)
  autoplayTimer = autoplay.value ? window.setTimeout(() => showSlide(current.value + 1), AUTOPLAY_MS) : undefined
}, { immediate: true })

function onShowcaseFocusOut(event: FocusEvent): void {
  const next = event.relatedTarget
  focusInside.value = next instanceof Node && Boolean((event.currentTarget as HTMLElement).contains(next))
}

function onVisibilityChange(): void {
  documentVisible.value = document.visibilityState !== 'hidden'
}

function onMotionChange(event: MediaQueryListEvent): void {
  reducedMotion.value = event.matches
}

let resizeObserver: ResizeObserver | undefined
onMounted(() => {
  if (root.value) {
    width.value = root.value.clientWidth || width.value
    if (typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver(([entry]) => {
        if (entry) width.value = entry.contentRect.width
      })
      resizeObserver.observe(root.value)
    }
  }
  document.addEventListener('visibilitychange', onVisibilityChange)
  motionQuery?.addEventListener?.('change', onMotionChange)
  void load()
})

onBeforeUnmount(() => {
  controller?.abort()
  window.clearTimeout(autoplayTimer)
  resizeObserver?.disconnect()
  document.removeEventListener('visibilitychange', onVisibilityChange)
  motionQuery?.removeEventListener?.('change', onMotionChange)
})
</script>

<template>
  <div ref="root" class="offers-page" :class="{ 'offers-page--narrow': narrow, 'offers-page--stacked': columns < 3 }">
    <header class="offers-intro">
      <p class="offers-intro__text">
        科技lion 挑选的服务器与域名厂商。这里的链接都含推广返利（AFF），你支付的价格不变，返利用于支持 KPanel 持续开发。
      </p>
      <div class="offers-intro__meta">
        <span v-if="updatedLabel">更新于 {{ updatedLabel }}</span>
        <button
          class="icon-button"
          type="button"
          aria-label="刷新"
          title="刷新"
          :disabled="refreshing || loading"
          @click="load(true)"
        >
          <RefreshCw :size="17" :class="{ spin: refreshing }" aria-hidden="true" />
        </button>
      </div>
    </header>

    <div v-if="!loading && !loadError && snapshot?.state === 'stale'" class="offers-notice" role="status">
      <TriangleAlert :size="17" aria-hidden="true" />
      <p>
        <template v-if="fetchedLabel">暂时无法获取最新广告，下面是 {{ fetchedLabel }} 获取的内容。</template>
        <template v-else>暂时无法获取最新广告，下面是上次获取的内容。</template>
      </p>
      <button class="button button--ghost" type="button" :disabled="refreshing" @click="load(true)">重试</button>
    </div>

    <div v-if="loading" class="offers-skeleton" role="status" aria-label="正在加载广告专栏">
      <span class="offers-skeleton__block offers-skeleton__block--wide" />
      <div class="offers-wall" :class="`offers-wall--cols-${columns}`">
        <span v-for="index in columns" :key="index" class="offers-skeleton__block" />
      </div>
    </div>

    <div v-else-if="loadError" class="offers-empty" role="alert">
      <TriangleAlert :size="22" aria-hidden="true" />
      <strong>广告专栏暂时打不开</strong>
      <p>{{ loadError }}</p>
      <button class="button" type="button" @click="load()">重试</button>
    </div>

    <div v-else-if="snapshot?.state === 'unavailable'" class="offers-empty" role="status">
      <Megaphone :size="22" aria-hidden="true" />
      <strong>暂时无法获取广告内容</strong>
      <p>广告内容来自 app.kejilion.sh。请确认面板所在服务器可以访问外网，然后重试。</p>
      <button class="button" type="button" :disabled="refreshing" @click="load(true)">重试</button>
    </div>

    <div v-else-if="!items.length" class="offers-empty" role="status">
      <Megaphone :size="22" aria-hidden="true" />
      <strong>暂时没有可展示的广告</strong>
      <a class="button" :href="MORE_OFFERS_URL" target="_blank" rel="noopener noreferrer">
        去 kejilion.pro 查看更多 VPS 优惠
        <ExternalLink :size="14" aria-hidden="true" />
      </a>
    </div>

    <template v-else>
      <section
        v-if="featured.length"
        class="offers-showcase"
        aria-roledescription="轮播"
        aria-label="主推广告"
        @pointerenter="pointerInside = true"
        @pointerleave="pointerInside = false"
        @focusin="focusInside = true"
        @focusout="onShowcaseFocusOut"
      >
        <div class="offers-showcase__stage">
          <a
            v-for="(item, index) in featured"
            :key="item.id"
            class="offers-showcase__slide"
            :class="{ 'is-active': index === current }"
            :href="item.url"
            target="_blank"
            rel="sponsored noopener noreferrer"
            :aria-hidden="index === current ? undefined : 'true'"
            :tabindex="index === current ? undefined : -1"
            :aria-label="linkLabel(item)"
          >
            <img
              v-if="!imageFailed(narrow ? item.card : item.wide)"
              :src="narrow ? item.card : item.wide"
              alt=""
              decoding="async"
              @error="failedImages.add((narrow ? item.card : item.wide) || '')"
            />
            <span v-else class="offers-fallback" data-i18n-ignore>
              <strong>{{ item.vendor }}</strong>
              <span>{{ item.alt }}</span>
            </span>
          </a>
        </div>
        <div v-if="activeSlide" class="offers-showcase__bar">
          <span class="offers-disclosure">广告</span>
          <p class="offers-showcase__caption" aria-live="polite" data-i18n-ignore>
            <strong>{{ activeSlide.vendor }}</strong>
            <span>{{ activeSlide.alt }}</span>
          </p>
          <span class="offers-host" data-i18n-ignore>
            {{ activeSlide.host }}
            <ExternalLink :size="13" aria-hidden="true" />
          </span>
          <div v-if="featured.length > 1" class="offers-controls">
            <button type="button" aria-label="上一张" @click="showSlide(current - 1)">
              <ChevronLeft :size="18" aria-hidden="true" />
            </button>
            <div class="offers-dots">
              <button
                v-for="(item, index) in featured"
                :key="item.id"
                type="button"
                :aria-label="`第 ${index + 1} 张`"
                :aria-current="index === current ? 'true' : undefined"
                @click="showSlide(index)"
              />
            </div>
            <button type="button" aria-label="下一张" @click="showSlide(current + 1)">
              <ChevronRight :size="18" aria-hidden="true" />
            </button>
            <button
              class="offers-controls__pause"
              type="button"
              :aria-label="pausedByUser ? '继续自动切换' : '暂停自动切换'"
              :aria-pressed="pausedByUser"
              @click="pausedByUser = !pausedByUser"
            >
              <Play v-if="pausedByUser" :size="16" aria-hidden="true" />
              <Pause v-else :size="16" aria-hidden="true" />
            </button>
          </div>
        </div>
      </section>

      <section class="offers-wall-section" aria-labelledby="offers-wall-title">
        <div class="offers-section-head">
          <h2 id="offers-wall-title">更多厂商</h2>
          <span v-if="wall.length">{{ wall.length }} 家</span>
        </div>
        <div class="offers-wall" :class="`offers-wall--cols-${columns}`">
          <a
            v-for="item in wall"
            :key="item.id"
            class="offers-tile"
            :href="item.url"
            target="_blank"
            rel="sponsored noopener noreferrer"
            :aria-label="linkLabel(item)"
          >
            <span class="offers-tile__image">
              <img
                v-if="!imageFailed(item.card)"
                :src="item.card"
                alt=""
                loading="lazy"
                decoding="async"
                @error="failedImages.add(item.card)"
              />
              <span v-else class="offers-fallback" data-i18n-ignore>
                <strong>{{ item.vendor }}</strong>
                <span>{{ item.alt }}</span>
              </span>
            </span>
            <span class="offers-tile__caption">
              <span class="offers-disclosure">广告</span>
              <span class="offers-tile__vendor" data-i18n-ignore>{{ item.vendor }}</span>
              <span class="offers-host" data-i18n-ignore>
                {{ item.host }}
                <ExternalLink :size="13" aria-hidden="true" />
              </span>
            </span>
          </a>
          <a
            class="offers-tile offers-tile--more"
            :class="`offers-tile--${moreLayout}`"
            :href="MORE_OFFERS_URL"
            target="_blank"
            rel="noopener noreferrer"
          >
            <span class="offers-tile__image">
              <LayoutGrid :size="22" aria-hidden="true" />
              <strong>更多 VPS 优惠</strong>
              <span>kejilion.pro/topvps 整理的热门套餐</span>
            </span>
            <span class="offers-tile__caption">
              <span class="offers-tag">外部链接</span>
              <span class="offers-tile__vendor">科技lion</span>
              <span class="offers-host">
                kejilion.pro
                <ExternalLink :size="13" aria-hidden="true" />
              </span>
            </span>
          </a>
        </div>
      </section>
    </template>

    <aside class="offers-about" aria-labelledby="offers-about-title">
      <Info :size="18" aria-hidden="true" />
      <div>
        <h2 id="offers-about-title">关于广告</h2>
        <ul>
          <li>本页链接含推广返利（AFF），你支付的价格不变。</li>
          <li>横幅上的价格和配置以厂商页面为准；KPanel 不经手订单和付款。</li>
          <li>KPanel 不记录你点了哪些横幅。</li>
        </ul>
      </div>
    </aside>
  </div>
</template>

<style scoped>
/*
 * Class names avoid ad/banner/sponsor/promo so content blockers do not hide
 * unrelated panel chrome by selector. Vendor colour lives only inside images.
 */
.offers-page {
  display: grid;
  gap: 24px;
  min-width: 0;
}

.offers-intro {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px 24px;
}

.offers-intro__text {
  flex: 1 1 420px;
  min-width: 0;
  max-width: 68ch;
  margin: 0;
  color: var(--text-soft);
  font-size: 14px;
  line-height: 1.6;
}

.offers-intro__meta {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--muted);
  font-size: 13px;
}

.offers-notice {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px 12px;
  padding: 10px 12px 10px 14px;
  color: var(--text);
  background: var(--amber-soft);
  border: 1px solid color-mix(in srgb, var(--amber) 30%, var(--border));
  border-radius: var(--radius);
  font-size: 14px;
}

.offers-notice > svg {
  color: var(--amber);
}

.offers-notice p {
  margin: 0;
}

.offers-skeleton {
  display: grid;
  gap: 24px;
}

.offers-skeleton__block {
  display: block;
  aspect-ratio: 2 / 1;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}

.offers-skeleton__block--wide {
  aspect-ratio: 4 / 1;
  border-radius: var(--radius-lg);
}

.offers-page--narrow .offers-skeleton__block--wide {
  aspect-ratio: 2 / 1;
}

.offers-empty {
  display: grid;
  justify-items: center;
  gap: 10px;
  padding: 40px 16px;
  color: var(--text-soft);
  text-align: center;
  background: var(--surface);
  border: 1px dashed var(--border-strong);
  border-radius: var(--radius-lg);
}

.offers-empty > svg {
  color: var(--muted);
}

.offers-empty strong {
  color: var(--text);
  font-size: 16px;
}

.offers-empty p {
  max-width: 52ch;
  margin: 0;
  font-size: 14px;
}

.offers-empty .button {
  text-decoration: none;
}

/* Carousel: the image and the caption bar form one surface. */
.offers-showcase {
  overflow: hidden;
  background: var(--surface-raised);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
}

.offers-showcase__stage {
  position: relative;
  aspect-ratio: 4 / 1;
  background: var(--surface-subtle);
}

.offers-page--narrow .offers-showcase__stage {
  aspect-ratio: 2 / 1;
}

.offers-showcase__slide {
  position: absolute;
  inset: 0;
  display: block;
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--motion-duration-base) var(--motion-ease-fade);
}

.offers-showcase__slide.is-active {
  opacity: 1;
  pointer-events: auto;
}

.offers-showcase__slide img,
.offers-tile__image img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.offers-showcase__slide:focus-visible {
  outline: 2px solid var(--brand);
  outline-offset: -4px;
}

.offers-showcase__bar {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 8px 14px;
  padding: 8px 10px 8px 16px;
  border-top: 1px solid var(--border);
}

.offers-showcase__caption {
  display: flex;
  flex-wrap: wrap;
  column-gap: 8px;
  min-width: 0;
  margin: 0;
  font-size: 14px;
}

.offers-showcase__caption span {
  color: var(--text-soft);
}

.offers-disclosure,
.offers-tag {
  display: inline-flex;
  flex: none;
  align-items: center;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  line-height: 1.4;
  white-space: nowrap;
}

.offers-disclosure {
  color: var(--amber);
  background: var(--amber-soft);
}

.offers-tag {
  color: var(--text-soft);
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  font-weight: 500;
}

.offers-host {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--muted);
  font-size: 13px;
  white-space: nowrap;
}

.offers-controls {
  display: flex;
  align-items: center;
  gap: 2px;
}

.offers-controls > button {
  display: grid;
  width: 36px;
  height: 36px;
  place-items: center;
  padding: 0;
  color: var(--text-soft);
  background: none;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition:
    color var(--motion-duration-fast) var(--motion-ease-fade),
    background var(--motion-duration-fast) var(--motion-ease-fade);
}

.offers-controls > button:hover {
  color: var(--brand);
  background: var(--interaction-hover-surface);
}

.offers-dots {
  display: flex;
}

.offers-dots button {
  width: 24px;
  height: 36px;
  padding: 0;
  background: none;
  border: 0;
  cursor: pointer;
}

.offers-dots button::before {
  display: block;
  width: 8px;
  height: 8px;
  margin: auto;
  background: var(--border-strong);
  border-radius: 999px;
  content: '';
  transition:
    width var(--motion-duration-fast) var(--motion-ease-standard),
    background var(--motion-duration-fast) var(--motion-ease-fade);
}

.offers-dots button[aria-current='true']::before {
  width: 18px;
  background: var(--brand);
}

/* Wall */
.offers-wall-section {
  display: grid;
  gap: 14px;
}

.offers-section-head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px 12px;
}

.offers-section-head h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  line-height: 1.3;
}

.offers-section-head span {
  color: var(--muted);
  font-size: 13px;
}

.offers-wall {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 20px 16px;
}

.offers-wall--cols-2 {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.offers-wall--cols-1 {
  grid-template-columns: minmax(0, 1fr);
}

.offers-tile {
  display: grid;
  align-content: start;
  gap: 8px;
  min-width: 0;
  color: inherit;
  text-decoration: none;
  border-radius: var(--radius);
}

.offers-tile__image {
  position: relative;
  display: block;
  overflow: hidden;
  aspect-ratio: 2 / 1;
  max-width: 100%;
  background: var(--surface-subtle);
  border-radius: var(--radius);
  box-shadow: var(--shadow-sm);
  transition:
    transform var(--motion-duration-fast) var(--motion-ease-standard),
    box-shadow var(--motion-duration-fast) var(--motion-ease-fade);
}

/* Inset edge keeps dark artwork from melting into a dark page. */
.offers-tile__image::after {
  position: absolute;
  inset: 0;
  border-radius: inherit;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--text) 10%, transparent);
  content: '';
  pointer-events: none;
}

.offers-tile:hover .offers-tile__image {
  box-shadow: var(--shadow-md);
  transform: translateY(-2px);
}

.offers-tile:hover .offers-tile__vendor {
  color: var(--brand);
}

.offers-tile:focus-visible {
  outline: 2px solid var(--brand);
  outline-offset: 4px;
}

.offers-tile__caption {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  padding-inline: 2px;
}

.offers-tile__vendor {
  min-width: 0;
  overflow-wrap: anywhere;
  font-size: 14px;
  font-weight: 600;
  transition: color var(--motion-duration-fast) var(--motion-ease-fade);
}

.offers-tile__caption .offers-host {
  margin-left: auto;
}

.offers-tile--more .offers-tile__image {
  display: grid;
  align-content: center;
  justify-items: center;
  gap: 4px;
  padding: 16px;
  color: var(--text-soft);
  text-align: center;
  background: var(--surface);
  border: 1px dashed var(--border-strong);
  box-shadow: none;
  font-size: 13px;
}

.offers-tile--more .offers-tile__image::after {
  display: none;
}

.offers-tile--more .offers-tile__image > svg {
  color: var(--brand);
}

.offers-tile--more strong {
  color: var(--text);
  font-size: 16px;
}

.offers-tile--span {
  grid-column: span 2;
  grid-template-rows: minmax(0, 1fr) auto;
  align-content: stretch;
}

.offers-tile--span .offers-tile__image {
  aspect-ratio: auto;
  min-height: 96px;
}

.offers-tile--bar {
  grid-column: 1 / -1;
}

.offers-tile--bar .offers-tile__image {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-start;
  gap: 4px 12px;
  aspect-ratio: auto;
  padding: 14px 18px;
  text-align: left;
}

.offers-tile--bar .offers-tile__caption {
  display: none;
}

.offers-fallback {
  display: grid;
  align-content: center;
  gap: 4px;
  width: 100%;
  height: 100%;
  padding: 0 16px;
  color: var(--text-soft);
  background: var(--surface-subtle);
  font-size: 14px;
}

.offers-fallback strong {
  color: var(--text);
  font-size: 16px;
}

.offers-about {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 12px;
  padding: 16px 18px;
  color: var(--text-soft);
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}

.offers-about > svg {
  margin-top: 2px;
  color: var(--muted);
}

.offers-about h2 {
  margin: 0 0 4px;
  color: var(--text);
  font-size: 14px;
  font-weight: 600;
}

.offers-about ul {
  display: grid;
  gap: 2px;
  margin: 0;
  padding-left: 18px;
  font-size: 14px;
}

/* Below three wall columns the caption bar takes two rows: label and text, then host and controls. */
.offers-page--stacked .offers-showcase__bar {
  grid-template-columns: auto minmax(0, 1fr) auto;
}

.offers-page--stacked .offers-showcase__caption {
  grid-column: 2 / -1;
}

.offers-page--stacked .offers-showcase__bar .offers-host {
  grid-column: 1 / 3;
  grid-row: 2;
}

.offers-page--stacked .offers-controls {
  grid-column: 3;
  grid-row: 2;
}

.offers-page--narrow .offers-showcase__bar {
  grid-template-columns: auto minmax(0, 1fr);
}

.offers-page--narrow .offers-showcase__bar .offers-host {
  grid-column: 1 / -1;
  grid-row: 2;
}

.offers-page--narrow .offers-controls {
  grid-column: 1 / -1;
  grid-row: 3;
}

.offers-page--narrow .offers-controls__pause {
  margin-left: auto;
}

@media (prefers-reduced-motion: reduce) {
  .offers-tile:hover .offers-tile__image {
    transform: none;
  }
}
</style>
