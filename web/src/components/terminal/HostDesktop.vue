<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { LoaderCircle, Monitor } from '@lucide/vue'
import type { UserInteraction } from '@devolutions/iron-remote-desktop'
import { api, ApiError } from '@/lib/api'
import { loadRemoteDesktop } from '@/lib/remoteDesktop'
import { translatePhrase } from '@/i18n/phrase'
import { useI18n } from '@/i18n'

const props = defineProps<{ hostId: string; hostName: string; active: boolean }>()
const emit = defineEmits<{ 'state-change': [state: 'connecting' | 'connected' | 'finished'] }>()
const { locale } = useI18n()
const phrase = (value: string): string => { locale.value; return translatePhrase(value) }
const canvasHost = ref<HTMLElement>()
const username = ref('')
const domain = ref('')
const password = ref('')
const connecting = ref(false)
const connected = ref(false)
const error = ref('')
let ui: UserInteraction | undefined
let sessionId = ''
let sequence = 0
let element: HTMLElement | undefined
let observer: ResizeObserver | undefined

function scheduleResize(): void {
  const host = canvasHost.value
  if (!host || !props.active) return
  ui?.resize(Math.max(640, Math.min(1920, host.clientWidth)), Math.max(480, Math.min(1080, host.clientHeight)))
}

function disposeView(): void {
  observer?.disconnect()
  observer = undefined
  try { ui?.shutdown() } catch { /* The server close below remains authoritative. */ }
  finally { ui = undefined; element?.remove(); element = undefined }
  connected.value = false
}

async function closeSession(): Promise<void> {
  sequence++
  password.value = ''
  disposeView()
  if (sessionId) {
    const id = sessionId
    try { await api.desktops.close(id) } catch (reason) {
      if (!(reason instanceof ApiError && reason.code === 'desktop_not_found')) throw reason
    }
    if (sessionId === id) sessionId = ''
  }
  emit('state-change', 'finished')
}

async function connect(): Promise<void> {
  if (connecting.value || !username.value.trim() || !password.value) return
  const attempt = ++sequence
  connecting.value = true
  error.value = ''
  emit('state-change', 'connecting')
  try {
    const rdp = await loadRemoteDesktop()
    if (attempt !== sequence) return
    const opened = await api.desktops.open(props.hostId)
    if (attempt !== sequence) { await api.desktops.close(opened.sessionId); return }
    sessionId = opened.sessionId
    element = document.createElement('iron-remote-desktop')
    Object.assign(element, { module: rdp.Backend })
    element.style.cssText = 'display:block;width:100%;height:100%;min-height:320px'
    const ready = new Promise<UserInteraction>((resolve, reject) => {
      const timer = window.setTimeout(() => reject(new Error('desktop initialization timeout')), 10000)
      element!.addEventListener('ready', (event) => { window.clearTimeout(timer); resolve((event as CustomEvent<{ irgUserInteraction: UserInteraction }>).detail.irgUserInteraction) }, { once: true })
    })
    canvasHost.value?.replaceChildren(element)
    ui = await ready
    if (attempt !== sequence) { disposeView(); return }
    ui.setEnableClipboard(false)
    ui.setEnableAutoClipboard(false)
    ui.setKeyboardUnicodeMode(true)
    const config = ui.configBuilder()
      .withUsername(username.value.trim()).withPassword(password.value).withServerDomain(domain.value.trim())
      .withDestination('localhost').withProxyAddress(api.desktops.socket(sessionId)).withAuthToken(opened.nonce)
      .withDesktopSize({ width: 1280, height: 800 })
      .withExtension(rdp.displayControl(true)).withExtension(rdp.enableCredssp(true))
      .withExtension(rdp.outboundMessageSizeLimit(60 * 1024)).build()
    password.value = ''
    const live = await ui.connect(config)
    if (attempt !== sequence) { disposeView(); return }
    connected.value = true
    connecting.value = false
    ui.setVisibility(props.active)
    observer = new ResizeObserver(scheduleResize)
    if (canvasHost.value) observer.observe(canvasHost.value)
    emit('state-change', 'connected')
    await live.run()
  } catch {
    if (attempt === sequence) error.value = phrase('远程桌面连接失败，请检查 Windows 账户、NLA、RDP 服务和证书。')
  } finally {
    password.value = ''
    if (attempt === sequence) {
      connecting.value = false
      await closeSession().catch(() => { error.value = phrase('关闭未确认，请重试关闭会话。') })
    }
  }
}

watch(() => props.active, (active) => { ui?.setVisibility(active); if (active) scheduleResize() })
onBeforeUnmount(() => { void closeSession().catch(() => undefined) })
defineExpose({ closeSession, scheduleResize, focusTerminal: () => canvasHost.value?.focus(), executeCommand: () => false })
</script>

<template>
  <section class="host-desktop">
    <form v-if="!connected && !connecting" class="host-desktop__login form-stack" autocomplete="off" @submit.prevent="connect">
      <Monitor :size="32" />
      <h2>{{ hostName }} · RDP</h2>
      <p>{{ phrase('使用目标 Windows 的登录账户连接，凭据仅用于当前会话。') }}</p>
      <p>{{ phrase('建议使用桌面浏览器。连接可能锁定本机用户的桌面，具体取决于 Windows 的会话策略。') }}</p>
      <label class="field">{{ phrase('用户名') }}<input v-model="username" autocomplete="off" maxlength="256" required /></label>
      <label class="field">{{ phrase('域（可选）') }}<input v-model="domain" autocomplete="off" maxlength="256" /></label>
      <label class="field">{{ phrase('密码') }}<input v-model="password" type="password" autocomplete="new-password" maxlength="1024" required /></label>
      <button class="button button--primary" type="submit">{{ phrase('连接远程桌面') }}</button>
      <small>{{ phrase('剪贴板、文件传输和打印重定向默认关闭。') }}</small>
    </form>
    <p v-if="error" class="host-desktop__error" role="alert">{{ error }}</p>
    <div v-if="connecting" class="host-desktop__status" role="status"><LoaderCircle class="spin" :size="20" />{{ phrase('正在连接远程桌面…') }}</div>
    <div v-if="connected" class="host-desktop__toolbar"><button class="button button--secondary button--small" @click="ui?.ctrlAltDel()">Ctrl + Alt + Del</button></div>
    <div ref="canvasHost" class="host-desktop__canvas" :class="{ 'is-hidden': !connected && !connecting }" tabindex="-1" />
  </section>
</template>

<style scoped>
.host-desktop { display: flex; flex: 1; flex-direction: column; min-height: 0; overflow: auto; background: var(--surface); color: var(--text); }
.host-desktop__login { width: min(100% - 40px, 380px); margin: auto; padding: 32px 0; }
.host-desktop__login h2 { font-size: 20px; margin: 0; }
.host-desktop__login .field,.host-desktop__login input { font-size: 14px; }
.host-desktop__login p,.host-desktop__login small { color: var(--muted); line-height: 1.6; }
.host-desktop__canvas { flex: 1; min-height: 320px; overflow: hidden; }
.host-desktop__canvas.is-hidden { display: none; }
.host-desktop__status,.host-desktop__toolbar { display: flex; align-items: center; gap: 8px; padding: 12px; }
.host-desktop__error { color: var(--danger); padding: 12px 20px; }
</style>
