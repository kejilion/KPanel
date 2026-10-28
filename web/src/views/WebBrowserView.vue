<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft, ArrowRight, Ellipsis, ExternalLink, Globe2, Home, LoaderCircle, Network, Plus, RefreshCw, Search, Settings2, X } from '@lucide/vue'
import { api } from '@/lib/api'
import { BROWSER_MAX_TABS, browserCoordinates, browserEvent, browserInput, browserTextChunks, browserLinksKey, browserLaunchKey, type BrowserEvent, type BrowserTab } from '@/lib/webBrowser'
import { desktopWindowTitleKey, desktopWindowTitlebarTargetKey, desktopWindowVisibleKey } from '@/lib/desktopRouteKeys'
import { useBrowserText } from '@/i18n/webBrowser'
import { usePhraseCatalog } from '@/i18n/phrase'
import ModalDialog from '@/components/common/ModalDialog.vue'
import BrowserSettings from '@/components/browser/BrowserSettings.vue'
import type RFB from '@novnc/novnc'

const t = useBrowserText()
usePhraseCatalog(locale => locale === 'en-US' ? import('@/i18n/pages/WebBrowserView/en-US').then(m => m.default) : import('@/i18n/pages/WebBrowserView/zh-TW').then(m => m.default))
const route = useRoute()
const windowVisible = inject(desktopWindowVisibleKey, ref(true))
const setTitle = inject(desktopWindowTitleKey, undefined)
const titlebarTarget = inject(desktopWindowTitlebarTargetKey, ref<HTMLElement>())
const links = inject(browserLinksKey, ref([]))
const launch = inject(browserLaunchKey, ref(undefined))
const tabs = ref<BrowserTab[]>([])
const activeID = ref('')
const address = ref('')
const editingAddress = ref(false)
const startPage = ref(true)
const newTabPending = ref(true)
const status = ref<'connecting' | 'connected' | 'disconnected' | 'unconfigured' | 'unavailable'>('connecting')
const notice = ref('')
const frame = ref<HTMLElement>()
const vncScreen = ref<HTMLElement>()
const tabStrip = ref<HTMLElement>()
const screen = ref<HTMLElement>()
const addressInput = ref<HTMLInputElement>()
const typing = ref<HTMLTextAreaElement>()
const dialog = ref<BrowserEvent>()
const mobile = ref(false)
const tabsOpen = ref(false)
const moreOpen = ref(false)
const settingsOpen = ref(false)
const startURL = ref('')
let preferencesController: AbortController | undefined
const tabCount = computed(() => tabs.value.length + Number(startPage.value && newTabPending.value))
const imageReady = ref(false)
const width = ref(1280)
const height = ref(720)
const activeTab = computed(() => tabs.value.find(tab => tab.id === activeID.value))
const workspace = Array.from(crypto.getRandomValues(new Uint8Array(16)), value => value.toString(16).padStart(2, '0')).join('')
const visible = computed(() => windowVisible.value && !startPage.value && documentVisible.value && !tabsOpen.value && !moreOpen.value && !settingsOpen.value)
const documentVisible = ref(!document.hidden)
let socket: WebSocket | undefined
let epoch = 0
let rfb: RFB | undefined
let vncSocket: WebSocket | undefined
let vncEpoch = 0
let vncTimer: ReturnType<typeof setTimeout> | undefined
let vncObserver: MutationObserver | undefined
let controller: AbortController | undefined
let observer: ResizeObserver | undefined
let resizeTimer: ReturnType<typeof setTimeout> | undefined
let connectTimer: ReturnType<typeof setTimeout> | undefined
let lastPointer = 0
let requestedViewport: { width: number; height: number } | undefined
const composing = ref(false)
let pendingURL: string | undefined
const closingTabIDs = new Set<string>()
let mobileQuery: MediaQueryList | undefined
let touchGesture: { id: number; startX: number; startY: number; x: number; y: number; sentX: number; sentY: number; moved: boolean; sentAt: number } | undefined

function send(command: Record<string, unknown>): boolean {
  if (socket?.readyState !== WebSocket.OPEN || status.value !== 'connected') return false
  if (socket.bufferedAmount > 128 * 1024) { notice.value = t('failed'); return false }
  socket.send(JSON.stringify(command)); return true
}
function errorMessage(code = ''): string {
  if (code === 'tab_limit') return t('limit')
  if (code === 'invalid_url') return t('invalid')
  if (code === 'browser_start_failed') return t('startFailed')
  if (code === 'upload_unavailable') return t('upload')
  return t('failed')
}
function disconnectVNC(): void {
  vncEpoch++; clearTimeout(vncTimer); vncObserver?.disconnect(); vncObserver = undefined
  vncSocket?.close(); vncSocket = undefined; rfb?.disconnect(); rfb = undefined; imageReady.value = false
}
function updateVNCSize(): void {
  const canvas = vncScreen.value?.querySelector('canvas')
  if (canvas && canvas.width >= 320 && canvas.height >= 240) { width.value = canvas.width; height.value = canvas.height }
}
async function connectVNC(attempt = 0): Promise<void> {
  disconnectVNC(); const current = vncEpoch
  if (!visible.value || !activeID.value || status.value !== 'connected') return
  try {
    const { default: Client } = await import('@novnc/novnc')
    await nextTick()
    if (current !== vncEpoch || !vncScreen.value) return
    const url = new URL(`/api/v1/browser/vnc/${workspace}`, location.href); url.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const channel = new WebSocket(url); vncSocket = channel; channel.binaryType = 'arraybuffer'
    const failed = () => {
      if (current !== vncEpoch) return
      disconnectVNC()
      if (attempt < 3 && visible.value && activeID.value) vncTimer = setTimeout(() => void connectVNC(attempt + 1), 500)
      else notice.value = t('failed')
    }
    vncTimer = setTimeout(failed, 15000)
    channel.onopen = () => channel.send(JSON.stringify({ csrf: api.browser.csrf() }))
    channel.onerror = failed
    channel.onclose = failed
    channel.onmessage = event => {
      if (current !== vncEpoch || typeof event.data !== 'string' || !vncScreen.value) return
      try { if (JSON.parse(event.data).ready !== true) throw new Error('invalid handshake') } catch { failed(); return }
      channel.onmessage = null
      rfb = new Client(vncScreen.value, channel); rfb.viewOnly = true; rfb.scaleViewport = true; rfb.resizeSession = false
      vncObserver = new MutationObserver(updateVNCSize)
      vncObserver.observe(vncScreen.value, { childList: true, subtree: true, attributes: true, attributeFilter: ['width', 'height'] })
      rfb.addEventListener('connect', () => { if (current === vncEpoch) { clearTimeout(vncTimer); updateVNCSize(); imageReady.value = true } })
      rfb.addEventListener('disconnect', failed)
      rfb.addEventListener('securityfailure', failed)
    }
  } catch { if (current === vncEpoch) notice.value = t('failed') }
}
function canvasBox(): DOMRect | undefined { return vncScreen.value?.querySelector('canvas')?.getBoundingClientRect() }
function receive(raw: unknown): void {
  const event = browserEvent(raw)
  if (!event) return
  if (event.type === 'error') { notice.value = errorMessage(event.code); return }
  if (event.type === 'dialog') { dialog.value = event; return }
  if (event.type !== 'state') return
  const reported = event.tabs!
  for (const id of closingTabIDs) { if (!reported.some(tab => tab.id === id)) closingTabIDs.delete(id) }
  const remaining = reported.filter(tab => !closingTabIDs.has(tab.id))
  const nextActive = remaining.some(tab => tab.id === event.active) ? event.active! : remaining[remaining.length - 1]?.id || ''
  const changed = activeID.value !== nextActive
  tabs.value = remaining; activeID.value = nextActive
  if (tabs.value.length >= BROWSER_MAX_TABS && newTabPending.value) { newTabPending.value = false; startPage.value = false }
  if (changed) void connectVNC()
  if (!editingAddress.value) address.value = activeTab.value?.url === 'about:blank' ? '' : activeTab.value?.url || ''
  setTitle?.(activeTab.value?.title || t('title'))
  if (pendingURL) { const target = pendingURL; pendingURL = undefined; openURL(target) }
}
function disconnect(end = false): void {
  if (end) send({ type: 'end' })
  epoch++; clearTimeout(connectTimer); controller?.abort(); socket?.close(); socket = undefined; disconnectVNC(); requestedViewport = undefined
}
async function connect(): Promise<void> {
  disconnect(); const current = epoch; status.value = 'connecting'; notice.value = ''; controller = new AbortController()
  closingTabIDs.clear()
  try {
    const health = await api.browser.status(controller.signal)
    if (current !== epoch) return
    if (!health.ready) { status.value = health.reason === 'browser_unconfigured' ? 'unconfigured' : 'unavailable'; return }
    const url = new URL(`/api/v1/browser/stream/${workspace}`, location.href); url.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const channel = new WebSocket(url); socket = channel
    connectTimer = setTimeout(() => { if (current === epoch) { disconnect(); status.value = 'unavailable' } }, 15000)
    channel.onopen = () => { if (current !== epoch) return; channel.send(JSON.stringify({ csrf: api.browser.csrf() })) }
    channel.onmessage = event => {
      if (current !== epoch) return
      const parsed = browserEvent(event.data)
      if (!parsed) return
      if (status.value !== 'connected') { clearTimeout(connectTimer); status.value = 'connected'; send({ type: 'visible', visible: visible.value }); resize() }
      receive(event.data)
    }
    channel.onerror = () => { if (current === epoch) status.value = 'disconnected' }
    channel.onclose = () => { if (current === epoch) { clearTimeout(connectTimer); status.value = 'disconnected'; imageReady.value = false } }
  } catch { if (current === epoch) status.value = 'unavailable' }
}
function openURL(url: string): void {
  if (status.value !== 'connected') { address.value = url; pendingURL = url; startPage.value = false; return }
  if (newTabPending.value || !activeTab.value) {
    if (tabs.value.length >= BROWSER_MAX_TABS) { notice.value = t('limit'); return }
    send({ type: 'new', url })
  } else send({ type: 'navigate', tab: activeID.value, url })
  startPage.value = false; newTabPending.value = false; notice.value = ''; address.value = url
  void nextTick(() => { resize(); if (!mobile.value) typing.value?.focus() })
}
function submit(): void { const url = browserInput(address.value); if (!url) { notice.value = t('invalid'); return }; openURL(url); if (mobile.value) addressInput.value?.blur() }
function select(tab: BrowserTab): void { startPage.value = false; newTabPending.value = false; if (tab.id !== activeID.value) disconnectVNC(); send({ type: 'select', tab: tab.id }); void nextTick(resize) }
function close(tab: BrowserTab): void {
  if (!send({ type: 'close', tab: tab.id })) return
  closingTabIDs.add(tab.id)
  tabs.value = tabs.value.filter(item => item.id !== tab.id)
  if (activeID.value === tab.id) disconnectVNC()
  if (!tabs.value.length) { startPage.value = true; newTabPending.value = true }
}
function home(newPage = false): void { startPage.value = true; newTabPending.value = newPage || !activeTab.value; address.value = ''; if (startURL.value) openURL(startURL.value); else void nextTick(() => addressInput.value?.focus()) }
function closeStartTab(): void { const previous = tabs.value[tabs.value.length - 1]; if (previous) select(previous); else { pendingURL = undefined; notice.value = ''; home() } }
function newTab(): void { if (tabs.value.length >= BROWSER_MAX_TABS) { notice.value = t('limit'); return }; home(true) }
function external(): void { const url = browserInput(address.value || activeTab.value?.url || pendingURL || ''); if (url) window.open(url, '_blank', 'noopener,noreferrer') }
function tabHost(url: string): string { try { return new URL(url).host } catch { return '' } }
function chooseTab(tab: BrowserTab): void { select(tab); tabsOpen.value = false }
function addTab(): void { newTab(); tabsOpen.value = false }
function reload(): void { send({ type: activeTab.value?.loading ? 'stop' : 'reload', tab: activeID.value }); moreOpen.value = false }
function mobileChanged(): void { mobile.value = mobileQuery?.matches || false; tabsOpen.value = false; moreOpen.value = false; touchGesture = undefined; void nextTick(resize) }
function resize(): void {
  if (!screen.value || !visible.value) return
  const box = screen.value.getBoundingClientRect()
  if (box.width <= 0 || box.height <= 0) return
  const scale = Math.max(1, box.width / 1600, box.height / 1000)
  const next = { width: Math.max(320, Math.round(box.width / scale)), height: Math.max(240, Math.round(box.height / scale)) }
  clearTimeout(resizeTimer)
  if (next.width === requestedViewport?.width && next.height === requestedViewport.height) return
  resizeTimer = setTimeout(() => { if (send({ type: 'resize', ...next })) { requestedViewport = next; touchGesture = undefined } }, 140)
}
function viewportReady(): boolean { return !requestedViewport || (requestedViewport.width === width.value && requestedViewport.height === height.value) }
function pointer(event: PointerEvent, action: string): void {
  if (!imageReady.value || !frame.value || startPage.value) return
  // Frames can lag a successful resize. Do not interpret old pixels against
  // the new viewport; an outstanding mouse press may still be released safely.
  if (!viewportReady() && (event.pointerType === 'touch' || action !== 'mouseReleased')) return
  if (event.pointerType === 'touch') { touch(event, action); return }
  if (action === 'mouseMoved' && performance.now() - lastPointer < 40) return
  lastPointer = performance.now()
  const box = canvasBox(); if (!box) return
  const point = browserCoordinates(event.clientX, event.clientY, box, requestedViewport?.width || width.value, requestedViewport?.height || height.value)
  if (!point) return
  if (action === 'mousePressed') { typing.value?.focus({ preventScroll: true }); frame.value.setPointerCapture?.(event.pointerId) }
  send({ type: 'mouse', action, ...point, button: ['left', 'middle', 'right'][event.button] || 'none', modifiers: modifiers(event) })
}
function touch(event: PointerEvent, action: string): void {
  const canvas = frame.value
  if (!canvas) return
  if (action === 'mousePressed') {
    if (touchGesture) { touchGesture = undefined; return }
    touchGesture = { id: event.pointerId, startX: event.clientX, startY: event.clientY, x: event.clientX, y: event.clientY, sentX: event.clientX, sentY: event.clientY, moved: false, sentAt: 0 }
    canvas.setPointerCapture?.(event.pointerId)
    return
  }
  const gesture = touchGesture
  if (!gesture || gesture.id !== event.pointerId) return
  gesture.x = event.clientX; gesture.y = event.clientY
  gesture.moved ||= Math.hypot(gesture.x - gesture.startX, gesture.y - gesture.startY) > 8
  const box = canvasBox(); if (!box) return
  const point = browserCoordinates(gesture.x, gesture.y, box, width.value, height.value)
  if (!point) { touchGesture = undefined; return }
  if (gesture.moved && (gesture.sentX !== gesture.x || gesture.sentY !== gesture.y) && (action === 'mouseReleased' || performance.now() - gesture.sentAt >= 40)) {
    send({ type: 'mouse', action: 'mouseWheel', ...point, button: 'none', deltaX: Math.max(-3000, Math.min(3000, (gesture.sentX - gesture.x) * width.value / box.width)), deltaY: Math.max(-3000, Math.min(3000, (gesture.sentY - gesture.y) * height.value / box.height)) })
    gesture.sentX = gesture.x; gesture.sentY = gesture.y; gesture.sentAt = performance.now()
  }
  if (action !== 'mouseReleased') return
  if (!gesture.moved) {
    if (typing.value) {
      typing.value.style.left = `${Math.max(0, Math.min(box.width - 2, gesture.x - box.left))}px`
      typing.value.style.top = `${Math.max(0, Math.min(box.height - 20, gesture.y - box.top))}px`
    }
    typing.value?.focus({ preventScroll: true })
    send({ type: 'mouse', action: 'mousePressed', ...point, button: 'left' })
    send({ type: 'mouse', action: 'mouseReleased', ...point, button: 'left' })
  }
  touchGesture = undefined
}
function wheel(event: WheelEvent): void {
  if (!frame.value || !imageReady.value || !viewportReady()) return
  const box = canvasBox(); if (!box) return
  const point = browserCoordinates(event.clientX, event.clientY, box, width.value, height.value)
  const unit = event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? box.height : 1
  if (point) send({ type: 'mouse', action: 'mouseWheel', ...point, deltaX: Math.max(-3000, Math.min(3000, event.deltaX * unit * width.value / box.width)), deltaY: Math.max(-3000, Math.min(3000, event.deltaY * unit * height.value / box.height)), button: 'none', modifiers: modifiers(event) })
}
function modifiers(event: { altKey: boolean; ctrlKey: boolean; metaKey: boolean; shiftKey: boolean }): number { return Number(event.altKey) + Number(event.ctrlKey) * 2 + Number(event.metaKey) * 4 + Number(event.shiftKey) * 8 }
function key(event: KeyboardEvent, action: string): void {
  if (composing.value || event.isComposing || event.key === 'Process') return
  if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) return
  if (event.key === 'Tab' && event.shiftKey) return
  event.preventDefault(); send({ type: 'key', action, key: event.key, code: event.code, modifiers: modifiers(event) })
}
function insertText(value: string): void { for (const text of browserTextChunks(value)) { if (!send({ type: 'text', text })) break } }
function textInput(event: Event): void { if (composing.value || (event as InputEvent).isComposing) return; const value = typing.value?.value; if (value) insertText(value); if (typing.value) typing.value.value = '' }
function beforeTextInput(event: InputEvent): void {
  if (composing.value || event.isComposing || !['insertLineBreak', 'insertParagraph'].includes(event.inputType)) return
  event.preventDefault()
  send({ type: 'key', action: 'keyDown', key: 'Enter', code: 'Enter' })
  send({ type: 'key', action: 'keyUp', key: 'Enter', code: 'Enter' })
}
function answer(accept: boolean): void { if (dialog.value) send({ type: 'dialog', tab: dialog.value.tab, accept }); dialog.value = undefined }
function documentVisibility(): void { documentVisible.value = !document.hidden }
async function initialize(): Promise<void> {
  preferencesController = new AbortController()
  try { const preferences = await api.preferences.get(preferencesController.signal); startURL.value = preferences.browser?.startURL || '' } catch { /* Browsing remains available when preference storage is temporarily unavailable. */ }
  if (preferencesController.signal.aborted) return
  if (!pendingURL && !tabs.value.length && startURL.value) openURL(startURL.value)
  await connect()
}
function serviceChanged(action: string): void { if (['install', 'start', 'restart'].includes(action)) void connect(); else { disconnect(); status.value = 'unavailable'; imageReady.value = false } }
watch(() => [route.query.settings, route.query.request], () => { if (route.query.settings === '1') settingsOpen.value = true }, { immediate: true })
watch(visible, value => { send({ type: 'visible', visible: value }); if (value) { void nextTick(resize); void connectVNC() } else { touchGesture = undefined; disconnectVNC() } })
watch([activeID, startPage, newTabPending], () => void nextTick(() => tabStrip.value?.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]')?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })))
watch(launch, value => { if (!value) return; const url = browserInput(value.url); if (!url) return; const tab = tabs.value.find(tab => tab.url === url); if (tab) select(tab); else { newTabPending.value = true; openURL(url) } }, { immediate: true })
watch(() => route.query.url, raw => { const value = Array.isArray(raw) ? raw[0] : raw; if (value) { const url = browserInput(value); if (url) openURL(url) } }, { immediate: true })
watch(() => route.query.request, () => { const value = route.query.url; if (typeof value === 'string') { const url = browserInput(value); if (url) openURL(url) } })
onMounted(() => { mobileQuery = window.matchMedia?.('(max-width: 600px), (max-height: 500px) and (pointer: coarse)'); mobileChanged(); mobileQuery?.addEventListener('change', mobileChanged); observer = new ResizeObserver(resize); if (screen.value) observer.observe(screen.value); document.addEventListener('visibilitychange', documentVisibility); void initialize() })
onBeforeUnmount(() => { preferencesController?.abort(); disconnect(true); observer?.disconnect(); mobileQuery?.removeEventListener('change', mobileChanged); clearTimeout(resizeTimer); document.removeEventListener('visibilitychange', documentVisibility) })
</script>

<template>
  <section class="web-browser" :aria-label="t('title')">
    <Teleport :to="titlebarTarget || 'body'" :disabled="!titlebarTarget || mobile">
    <div v-if="!mobile" ref="tabStrip" class="web-browser__tabs" :class="{ 'web-browser__tabs--titlebar': titlebarTarget }" role="tablist" :aria-label="t('title')" @dblclick.stop>
      <div class="web-browser__tab-track">
      <div v-for="tab in tabs" :key="tab.id" class="web-browser__tab" :class="{ 'is-active': (!startPage || !newTabPending) && tab.id === activeID }">
        <button role="tab" :aria-selected="(!startPage || !newTabPending) && tab.id === activeID" :title="tab.title || t('untitled')" @click="select(tab)"><LoaderCircle v-if="tab.loading" :size="15" class="is-spinning" /><Globe2 v-else :size="15" /><span>{{ startPage && !newTabPending && tab.id === activeID ? t('untitled') : tab.title || t('untitled') }}</span></button>
        <button :aria-label="`${t('closeTab')} · ${tab.title || t('untitled')}`" @click="close(tab)"><X :size="14" /></button>
      </div>
      <div v-if="startPage && newTabPending" class="web-browser__tab is-active"><button role="tab" aria-selected="true" @click="addressInput?.focus()"><Globe2 :size="15" /><span>{{ t('untitled') }}</span></button><button :aria-label="t('closeTab')" @click="closeStartTab"><X :size="14" /></button></div>
      </div>
      <button class="web-browser__icon" :disabled="tabs.length >= BROWSER_MAX_TABS" :title="t('newTab')" :aria-label="t('newTab')" @click="newTab"><Plus :size="18" /></button>
      <span class="web-browser__tab-count">{{ tabs.length + Number(startPage && newTabPending) }}/{{ BROWSER_MAX_TABS }}</span>
    </div>
    </Teleport>
    <form class="web-browser__toolbar" @submit.prevent="submit">
      <button type="button" class="web-browser__icon web-browser__back" :disabled="!activeTab?.back || startPage || status !== 'connected'" :title="t('back')" :aria-label="t('back')" @click="send({ type: 'back', tab: activeID })"><ArrowLeft :size="18" /></button>
      <button type="button" class="web-browser__icon web-browser__forward" :disabled="!activeTab?.forward || startPage || status !== 'connected'" :title="t('forward')" :aria-label="t('forward')" @click="send({ type: 'forward', tab: activeID })"><ArrowRight :size="18" /></button>
      <button type="button" class="web-browser__icon web-browser__reload" :disabled="!activeTab || startPage || status !== 'connected'" :title="t(activeTab?.loading ? 'stop' : 'reload')" :aria-label="t(activeTab?.loading ? 'stop' : 'reload')" @click="reload"><X v-if="activeTab?.loading" :size="18" /><RefreshCw v-else :size="17" /></button>
      <label class="web-browser__address"><Search :size="16" aria-hidden="true" /><input ref="addressInput" v-model="address" :aria-label="t('address')" :placeholder="t('address')" maxlength="4096" autocomplete="off" spellcheck="false" enterkeyhint="go" @focus="editingAddress = true" @blur="editingAddress = false" /><button type="submit" :aria-label="t('go')"><ArrowRight :size="17" /></button></label>
      <button type="button" class="web-browser__icon web-browser__home" :title="t('home')" :aria-label="t('home')" @click="home()"><Home :size="17" /></button>
      <button v-if="!mobile" type="button" class="web-browser__icon" :title="t('settings')" :aria-label="t('settings')" @click="settingsOpen = true"><Settings2 :size="17" /></button>
      <button v-if="!mobile" type="button" class="web-browser__icon" :disabled="!address && !activeTab" :title="t('external')" :aria-label="t('external')" @click="external"><ExternalLink :size="17" /></button>
      <button v-if="mobile" type="button" class="web-browser__icon web-browser__tab-switch" :aria-label="`${t('tabs')} · ${tabCount}/${BROWSER_MAX_TABS}`" aria-haspopup="dialog" @click="tabsOpen = true"><span>{{ tabCount }}</span></button>
      <button v-if="mobile" type="button" class="web-browser__icon web-browser__more" :aria-label="t('more')" aria-haspopup="dialog" @click="moreOpen = true"><Ellipsis :size="22" /></button>
    </form>
    <div v-if="notice" class="web-browser__notice" role="alert"><span>{{ notice }}</span><button class="web-browser__icon" :aria-label="t('clear')" @click="notice = ''"><X :size="15" /></button></div>
    <main ref="screen" class="web-browser__content">
      <div v-if="status !== 'connected'" class="web-browser__state">
        <LoaderCircle v-if="status === 'connecting'" :size="32" class="is-spinning" /><Globe2 v-else :size="40" />
        <h2>{{ t(status === 'connecting' ? 'connecting' : status === 'unconfigured' ? 'unconfigured' : status === 'disconnected' ? 'disconnected' : 'unavailable') }}</h2>
        <p v-if="status !== 'connecting'">{{ t(status === 'unconfigured' ? 'unconfiguredHint' : 'unavailableHint') }}</p>
        <div v-if="status !== 'connecting'" class="web-browser__actions"><button class="button button--primary" @click="settingsOpen = true">{{ t('settings') }}</button><button class="button button--secondary" @click="connect">{{ t('retry') }}</button><button v-if="address || activeTab" class="button button--secondary" @click="external">{{ t('external') }}</button></div>
      </div>
      <div v-else-if="startPage || !activeTab" class="web-browser__start">
        <span class="web-browser__emblem"><Globe2 :size="40" :stroke-width="1.5" /></span>
        <h1>{{ t('startTitle') }}</h1><p>{{ t('startHint') }}</p>
        <form class="web-browser__search" @submit.prevent="submit"><Search :size="20" /><input v-model="address" :aria-label="t('address')" :placeholder="t('address')" maxlength="4096" /><button class="button button--primary" type="submit">{{ t('go') }}<ArrowRight :size="16" /></button></form>
        <div v-if="links.length" class="web-browser__quick"><h2>{{ t('quick') }}</h2><div><button v-for="link in links.slice(0, 12)" :key="link.id" @click="openURL(link.url)"><img v-if="link.iconURL" :src="link.iconURL" alt="" /><Globe2 v-else :size="24" /><span>{{ link.name }}</span></button></div></div>
      </div>
      <div v-else class="web-browser__viewport">
        <div v-if="!imageReady" class="web-browser__loading" role="status"><LoaderCircle :size="24" class="is-spinning" /><span>{{ t('loading') }}</span></div>
        <div ref="frame" class="web-browser__frame" :aria-label="t('screen')" @pointerdown.prevent="pointer($event, 'mousePressed')" @pointerup.prevent="pointer($event, 'mouseReleased')" @pointermove.prevent="pointer($event, 'mouseMoved')" @pointercancel="touchGesture = undefined" @wheel.prevent="wheel" @contextmenu.prevent><div ref="vncScreen" class="web-browser__vnc" :class="{ 'is-ready': imageReady }" /></div>
        <textarea ref="typing" class="web-browser__keyboard" :aria-label="t('input')" autocapitalize="off" autocomplete="off" spellcheck="false" @keydown="key($event, 'keyDown')" @keyup="key($event, 'keyUp')" @beforeinput="beforeTextInput" @compositionstart="composing = true" @compositionend="composing = false; textInput($event)" @input="textInput" />
      </div>
    </main>
    <footer class="web-browser__footer"><span :title="t('sessionHint')"><span class="web-browser__dot" :class="{ 'is-connected': status === 'connected' }" />{{ t(status === 'connected' ? 'connected' : status === 'connecting' ? 'connecting' : 'disconnected') }}</span><span class="web-browser__network"><Network :size="13" />{{ t('server') }}</span></footer>
    <BrowserSettings :open="settingsOpen" @close="settingsOpen = false" @saved="startURL = $event" @changed="serviceChanged" />
    <ModalDialog :open="tabsOpen" :title="`${t('tabs')} · ${tabCount}/${BROWSER_MAX_TABS}`" size="medium" @close="tabsOpen = false">
      <div class="web-browser__tab-grid">
        <div v-for="tab in tabs" :key="tab.id" class="web-browser__tab-card" :class="{ 'is-active': !startPage && tab.id === activeID }">
          <button class="web-browser__tab-choice" :title="tab.title || t('untitled')" :aria-current="!startPage && tab.id === activeID ? 'page' : undefined" @click="chooseTab(tab)"><Globe2 :size="28" /><strong>{{ tab.title || t('untitled') }}</strong><span>{{ tabHost(tab.url) }}</span></button>
          <button class="web-browser__card-close" :aria-label="`${t('closeTab')} · ${tab.title || t('untitled')}`" @click="close(tab)"><X :size="18" /></button>
        </div>
        <div v-if="startPage && newTabPending" class="web-browser__tab-card is-active"><button class="web-browser__tab-choice" aria-current="page" @click="tabsOpen = false"><Globe2 :size="28" /><strong>{{ t('untitled') }}</strong></button><button class="web-browser__card-close" :aria-label="t('closeTab')" @click="closeStartTab"><X :size="18" /></button></div>
      </div>
      <template #footer><button class="button button--secondary web-browser__sheet-action" :disabled="tabs.length >= BROWSER_MAX_TABS" @click="addTab"><Plus :size="18" />{{ t('newTab') }}</button><button class="button button--primary web-browser__sheet-action" @click="tabsOpen = false">{{ t('done') }}</button></template>
    </ModalDialog>
    <ModalDialog :open="moreOpen" :title="t('more')" size="compact" @close="moreOpen = false">
      <div class="web-browser__menu-actions"><button @click="settingsOpen = true; moreOpen = false"><Settings2 :size="19" />{{ t('settings') }}</button><button :disabled="!activeTab || startPage || status !== 'connected'" @click="reload"><RefreshCw :size="19" />{{ t(activeTab?.loading ? 'stop' : 'reload') }}</button><button :disabled="!address && !activeTab" @click="external(); moreOpen = false"><ExternalLink :size="19" />{{ t('external') }}</button></div>
      <p class="web-browser__menu-info"><Network :size="15" />{{ t('server') }} · {{ t(status === 'connected' ? 'connected' : status === 'connecting' ? 'connecting' : 'disconnected') }}</p>
      <p class="web-browser__menu-hint">{{ t('sessionHint') }}</p>
    </ModalDialog>
    <ModalDialog :open="Boolean(dialog)" :title="t('dialog')" size="compact" @close="answer(false)"><p class="web-browser__dialog">{{ dialog?.text }}</p><template #footer><button class="button button--secondary" @click="answer(false)">{{ t('cancel') }}</button><button class="button button--primary" @click="answer(true)">{{ t('accept') }}</button></template></ModalDialog>
  </section>
</template>

<style scoped>
.web-browser{display:flex;flex-direction:column;width:100%;height:100%;min-height:24rem;overflow:hidden;background:var(--surface);color:var(--text);font-size:.875rem}
.web-browser__tabs{display:flex;align-items:center;gap:.25rem;padding:.4rem .65rem 0;min-width:0;min-height:2.6rem;background:var(--surface-subtle);font-size:.875rem}
.web-browser__tabs--titlebar{height:42px;min-height:42px;padding:0 .25rem;background:transparent}.web-browser__tab-track{display:flex;align-items:center;flex:0 1 auto;min-width:0;gap:.25rem;overflow-x:auto;scrollbar-width:thin}.web-browser__tab-count{margin-left:auto;flex-shrink:0;font-size:.8125rem;color:var(--muted);padding:0 .25rem}.web-browser__tabs--titlebar .web-browser__tab{border-radius:var(--radius-sm);height:32px}.web-browser__tabs--titlebar .web-browser__tab button{padding:.4rem}.web-browser__tabs--titlebar .web-browser__tab.is-active{border-color:var(--border)}
.web-browser__tab{display:flex;align-items:center;flex:0 0 12rem;width:12rem;min-width:0;border:1px solid transparent;border-radius:var(--radius-sm) var(--radius-sm) 0 0}
.web-browser__tab.is-active{background:var(--surface);border-color:var(--border);border-bottom-color:var(--surface)}
.web-browser__tab button{display:flex;align-items:center;gap:.5rem;border:0;background:none;color:inherit;padding:.65rem .5rem;font:inherit;min-width:0;cursor:pointer}.web-browser__tab button:first-child{flex:1}.web-browser__tab button:last-child,.web-browser__tab svg{flex-shrink:0}.web-browser__tab span{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.web-browser__network{display:flex;align-items:center;gap:.4rem;margin-left:auto;white-space:nowrap;font-size:.8125rem;color:var(--muted);padding:0 .5rem}
.web-browser__toolbar{display:flex;align-items:center;gap:.3rem;padding:.65rem;border-bottom:1px solid var(--border)}
.web-browser__icon{display:inline-flex;align-items:center;justify-content:center;flex-shrink:0;width:2.1rem;min-height:2.1rem;border:0;border-radius:var(--radius-sm);background:none;color:var(--text);cursor:pointer}.web-browser__icon:hover{background:var(--surface-subtle)}.web-browser__icon:disabled{opacity:.4;cursor:default}
.web-browser__address{display:flex;align-items:center;gap:.5rem;min-width:0;flex:1;background:var(--surface-subtle);border:1px solid var(--border);border-radius:999px;padding:.25rem .65rem;color:var(--muted)}
.web-browser__address input{min-width:0;width:100%;border:0;background:none;color:var(--text);font:inherit;outline:none;padding:.3rem 0}.web-browser__address:focus-within{border-color:var(--brand);outline:2px solid var(--brand-soft)}.web-browser__address button{display:flex;border:0;background:none;color:var(--muted);padding:.25rem;cursor:pointer}
.web-browser__content{position:relative;min-height:0;flex:1;overflow:auto}.web-browser__state,.web-browser__start{display:flex;flex-direction:column;align-items:center;justify-content:center;min-height:100%;padding:2rem;text-align:center;box-sizing:border-box}.web-browser__state>svg{color:var(--muted)}.web-browser__state h2{font-size:1.125rem;margin:1rem 0 .5rem}.web-browser__state p,.web-browser__start>p{max-width:34rem;line-height:1.7;color:var(--muted);margin:0 0 1.5rem;font-size:.875rem}.web-browser__actions{display:flex;flex-wrap:wrap;gap:.65rem;justify-content:center}
.web-browser__emblem{display:grid;place-items:center;width:5rem;height:5rem;border-radius:var(--radius-lg);background:var(--brand-soft);color:var(--brand);margin-bottom:1rem}.web-browser__start h1{font-size:clamp(1.25rem,2vw,1.65rem);line-height:1.4;font-weight:600;margin:0 0 .6rem}
.web-browser__search{display:flex;align-items:center;gap:.75rem;width:min(100%,36rem);padding:.55rem .6rem .55rem 1rem;border:1px solid var(--border);border-radius:var(--radius-lg);background:var(--surface-raised);box-shadow:var(--shadow-sm)}.web-browser__search>svg{flex-shrink:0;color:var(--muted)}.web-browser__search input{min-width:0;flex:1;border:0;background:none;color:var(--text);font:inherit;outline:none}.web-browser__search:focus-within{border-color:var(--brand)}.web-browser__search .button{display:flex;align-items:center;gap:.4rem;white-space:nowrap;font-size:.875rem}
.web-browser__quick{width:min(100%,36rem);margin-top:2rem;text-align:left}.web-browser__quick h2{font-size:.8125rem;font-weight:500;color:var(--muted);margin:0 0 .75rem}.web-browser__quick>div{display:grid;grid-template-columns:repeat(auto-fit,minmax(5rem,1fr));gap:.5rem}.web-browser__quick button{display:flex;flex-direction:column;align-items:center;gap:.6rem;min-width:0;padding:.8rem .4rem;border:0;border-radius:var(--radius-sm);background:none;color:var(--text);font:inherit;cursor:pointer}.web-browser__quick button:hover{background:var(--surface-subtle)}.web-browser__quick img{width:2rem;height:2rem;object-fit:contain}.web-browser__quick span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:100%}
.web-browser__viewport{position:absolute;inset:0;overflow:hidden;display:flex;align-items:flex-start;justify-content:center;background:var(--surface-subtle)}.web-browser__frame,.web-browser__vnc{width:100%;height:100%;touch-action:none}.web-browser__vnc{visibility:hidden;pointer-events:none}.web-browser__vnc.is-ready{visibility:visible}.web-browser__loading{position:absolute;inset:0;display:flex;gap:.75rem;align-items:center;justify-content:center;color:var(--muted);pointer-events:none}.web-browser__keyboard{position:absolute;left:0;top:0;width:1px;height:1px;opacity:0;resize:none;padding:0;border:0}
.web-browser__footer{display:flex;align-items:center;flex-shrink:0;gap:1rem;min-height:1.75rem;box-sizing:border-box;border-top:1px solid var(--border);padding:.25rem .75rem;color:var(--muted);font-size:.8125rem}.web-browser__footer>span:first-child{display:flex;gap:.4rem;align-items:center;white-space:nowrap}.web-browser__dot{width:.4rem;height:.4rem;border-radius:50%;background:var(--muted)}.web-browser__dot.is-connected{background:var(--brand)}.web-browser__notice{display:flex;gap:.75rem;align-items:center;justify-content:space-between;padding:.4rem .85rem;border-bottom:1px solid var(--border);background:var(--brand-soft);font-size:.875rem;line-height:1.5}.web-browser__dialog{white-space:pre-wrap;overflow-wrap:anywhere;font-size:.875rem;line-height:1.6}
.web-browser__sheet-action{display:flex;align-items:center;gap:.5rem;min-height:2.75rem}.web-browser__tab-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:.75rem}.web-browser__tab-card{position:relative;min-width:0;border:1px solid var(--border);border-radius:var(--radius-lg);background:var(--surface-subtle);overflow:hidden}.web-browser__tab-card.is-active{border:2px solid var(--brand);background:var(--brand-soft)}.web-browser__tab-choice{display:flex;flex-direction:column;align-items:flex-start;gap:.75rem;width:100%;min-height:10rem;padding:1rem;border:0;background:none;color:var(--text);text-align:left;cursor:pointer}.web-browser__tab-choice>svg{margin:.25rem 0;color:var(--brand)}.web-browser__tab-choice strong{display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden;overflow-wrap:anywhere;font-size:.875rem;line-height:1.5;font-weight:500}.web-browser__tab-choice span{max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--muted);font-size:.8125rem}.web-browser__card-close{position:absolute;right:0;top:0;display:grid;place-items:center;width:2.75rem;height:2.75rem;border:0;background:none;color:var(--text);cursor:pointer}.web-browser__card-close:hover{background:var(--surface-raised)}
.web-browser__menu-actions{display:grid;gap:.25rem}.web-browser__menu-actions button{display:flex;align-items:center;gap:.75rem;min-height:2.75rem;padding:.65rem .5rem;border:0;border-radius:var(--radius-sm);background:none;color:var(--text);font-size:.875rem;text-align:left;cursor:pointer}.web-browser__menu-actions button:hover{background:var(--surface-subtle)}.web-browser__menu-actions button:disabled{opacity:.4;cursor:default}.web-browser__menu-info{display:flex;align-items:center;gap:.4rem;margin:1rem 0 .5rem;color:var(--muted);font-size:.8125rem}.web-browser__menu-hint{margin:0;color:var(--muted);font-size:.8125rem;line-height:1.6}
.web-browser button:focus-visible,.web-browser__tabs button:focus-visible,.web-browser__tab-grid button:focus-visible,.web-browser__menu-actions button:focus-visible{outline:2px solid var(--brand);outline-offset:2px}.is-spinning{animation:browser-spin calc(var(--motion-duration-ambient) * 2) linear infinite}@keyframes browser-spin{to{transform:rotate(360deg)}}@media(prefers-reduced-motion:reduce){.is-spinning{animation:none}}
@media(max-width:600px),(max-height:500px) and (pointer:coarse){
  .web-browser{min-height:0}.web-browser__tabs{display:none}.web-browser__content{order:1}.web-browser__notice{order:0}.web-browser__footer{display:none}
  .web-browser__toolbar{position:relative;order:2;display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:.25rem;padding:.5rem .5rem max(.35rem,env(safe-area-inset-bottom));border-top:1px solid var(--border);border-bottom:0;background:var(--surface-raised);flex-shrink:0}
  .web-browser__toolbar>.web-browser__icon{width:100%;height:2.75rem;min-height:2.75rem}.web-browser__back{grid-column:1;grid-row:2}.web-browser__forward{grid-column:2;grid-row:2}.web-browser__home{grid-column:3;grid-row:2}.web-browser__tab-switch{grid-column:4;grid-row:2}.web-browser__more{grid-column:5;grid-row:2}
  .web-browser__tab-switch span{display:grid;place-items:center;min-width:1.35rem;height:1.35rem;border:1.5px solid currentColor;border-radius:.3rem;font-size:.875rem;font-weight:600}
  .web-browser__address{grid-column:1/-1;grid-row:1;min-height:2.75rem;box-sizing:border-box;padding:.25rem 3rem .25rem .85rem;border-radius:var(--radius-lg)}.web-browser__address input{font-size:1rem;min-height:2rem}.web-browser__address>button{display:none}.web-browser__address>svg{flex-shrink:0}.web-browser__toolbar>.web-browser__reload{position:absolute;right:.5rem;top:.5rem;width:2.75rem;height:2.75rem}
  .web-browser__start,.web-browser__state{padding:1.5rem 1rem}.web-browser__start h1{font-size:1.25rem}.web-browser__search{flex-wrap:wrap;padding:.6rem;gap:.5rem}.web-browser__search input{font-size:1rem;min-height:2.75rem;width:0}.web-browser__search .button{min-height:2.75rem}.web-browser__keyboard{font-size:1rem}
}
@media(min-width:601px) and (max-height:500px) and (pointer:coarse){
  .web-browser__toolbar{grid-template-columns:2.75rem 2.75rem minmax(0,1fr) 2.75rem 2.75rem 2.75rem}.web-browser__back{grid-column:1;grid-row:1}.web-browser__forward{grid-column:2;grid-row:1}.web-browser__address{grid-column:3;grid-row:1}.web-browser__home{grid-column:4;grid-row:1}.web-browser__tab-switch{grid-column:5;grid-row:1}.web-browser__more{grid-column:6;grid-row:1}.web-browser__toolbar>.web-browser__reload{right:9.5rem}
}
</style>
