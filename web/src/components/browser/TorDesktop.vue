<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import type RFB from '@novnc/novnc'
import { api } from '@/lib/api'
import { useBrowserText } from '@/i18n/webBrowser'

const props = defineProps<{ visible: boolean; workspace: string }>()
const t = useBrowserText()
const screen = ref<HTMLElement>()
const state = ref<'connecting' | 'connected' | 'disconnected'>('connecting')
const text = ref('')
let rfb: RFB | undefined, socket: WebSocket | undefined
let epoch = 0, timer: ReturnType<typeof setTimeout> | undefined

function disconnect(): void {
  epoch++; clearTimeout(timer)
  const previous = socket; socket = undefined
  rfb?.disconnect(); rfb = undefined; previous?.close()
  screen.value?.replaceChildren()
}
async function connect(): Promise<void> {
  disconnect()
  if (!props.visible) return
  state.value = 'connecting'
  const current = epoch
  const failed = () => { if (current === epoch) { disconnect(); state.value = 'disconnected' } }
  try {
    const { default: Client } = await import('@novnc/novnc')
    await nextTick()
    if (current !== epoch || !screen.value) return
    const url = new URL(`/api/v1/browser/vnc/${props.workspace}`, location.href)
    url.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const channel = new WebSocket(url); socket = channel; channel.binaryType = 'arraybuffer'
    timer = setTimeout(failed, 15000)
    channel.onopen = () => channel.send(JSON.stringify({ csrf: api.browser.csrf() }))
    channel.onerror = failed; channel.onclose = failed
    channel.onmessage = event => {
      if (current !== epoch || !screen.value) return
      try { if (typeof event.data !== 'string' || JSON.parse(event.data).ready !== true) throw Error() } catch { failed(); return }
      channel.onmessage = null
      rfb = new Client(screen.value, channel)
      rfb.viewOnly = false; rfb.scaleViewport = true; rfb.resizeSession = false
      rfb.addEventListener('connect', () => { if (current === epoch) { clearTimeout(timer); state.value = 'connected' } })
      rfb.addEventListener('disconnect', failed); rfb.addEventListener('securityfailure', failed)
      rfb.addEventListener('credentialsrequired', failed)
    }
  } catch { failed() }
}
function insert(): void {
  if (!rfb || !text.value || state.value !== 'connected') return
  rfb.clipboardPasteFrom(text.value)
  rfb.sendKey(0xffe3, 'ControlLeft', true); rfb.sendKey(0x76, 'KeyV'); rfb.sendKey(0xffe3, 'ControlLeft', false)
  text.value = ''
}
watch(() => props.visible, value => { if (value) void connect(); else disconnect() }, { immediate: true })
onBeforeUnmount(disconnect)
</script>

<template>
  <div class="tor-desktop">
    <p class="tor-desktop__hint">{{ t('torSessionHint') }}</p>
    <div class="tor-desktop__screen"><div ref="screen" class="tor-desktop__viewport" :aria-label="t('torScreen')" /></div>
    <div v-if="!visible || state !== 'connected'" role="status" class="tor-desktop__state">
      <p>{{ t(!visible ? 'paused' : state === 'connecting' ? 'connecting' : 'disconnected') }}</p>
      <button v-if="visible && state === 'disconnected'" class="button button--secondary" @click="connect">{{ t('retry') }}</button>
    </div>
    <form class="tor-desktop__input" @submit.prevent="insert">
      <input v-model="text" :aria-label="t('input')" :placeholder="t('torInputHint')" maxlength="8192" :disabled="state !== 'connected' || !visible" />
      <button class="button button--secondary" :disabled="!text || state !== 'connected' || !visible">{{ t('send') }}</button>
    </form>
  </div>
</template>

<style scoped>
.tor-desktop{position:relative;display:flex;flex:1;min-height:18rem;flex-direction:column;min-width:0}
.tor-desktop__hint{margin:.5rem .75rem;font-size:13px;line-height:1.5;color:var(--text-soft)}
.tor-desktop__screen{position:relative;flex:1;min-height:18rem;overflow:hidden;background:var(--surface-subtle)}
.tor-desktop__viewport{position:absolute;inset:0}
.tor-desktop__state{position:absolute;inset:3rem 0 4rem;display:grid;place-content:center;justify-items:center;gap:.75rem;background:var(--surface);font-size:14px}
.tor-desktop__input{display:flex;gap:.5rem;padding:.5rem;border-top:1px solid var(--border)}
.tor-desktop__input input{flex:1;min-width:0;min-height:44px;padding:.5rem .75rem;border:1px solid var(--border);border-radius:var(--radius-sm);font:inherit;color:var(--text);background:var(--surface)}
.tor-desktop__input .button{font-size:14px;min-height:44px}
.tor-desktop__input input:focus-visible{outline:2px solid var(--brand);outline-offset:2px}
</style>
