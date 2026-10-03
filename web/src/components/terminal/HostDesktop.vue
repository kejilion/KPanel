<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { LoaderCircle, Monitor } from '@lucide/vue'
import type { UserInteraction } from '@devolutions/iron-remote-desktop'
import { api, ApiError } from '@/lib/api'
import { loadRemoteDesktop } from '@/lib/remoteDesktop'
import { translatePhrase } from '@/i18n/phrase'
import { useI18n } from '@/i18n'

const props = defineProps<{ hostId: string; hostName: string; active: boolean }>()
const emit = defineEmits<{ 'state-change': [state: 'connecting' | 'connected' | 'finished'] }>()
const { locale, t } = useI18n()
const phrase = (value: string): string => { locale.value; return translatePhrase(value) }
const canvasHost = ref<HTMLElement>()
const username = ref('')
const domain = ref('')
const password = ref('')
const checking = ref(true)
const managing = ref(false)
const manualLogin = ref(true)
const saved = ref(false)
const statusFailed = ref(false)
const connecting = ref(false)
const saving = ref(false)
const connected = ref(false)
const error = ref('')
let ui: UserInteraction | undefined
let sessionId = ''
let sequence = 0
let unmounted = false
let element: HTMLElement | undefined
let observer: ResizeObserver | undefined
let cancelReady: (() => void) | undefined
let statusController: AbortController | undefined
let connectionTimer: number | undefined

function clearConnectionTimer(): void {
  if (connectionTimer !== undefined) window.clearTimeout(connectionTimer)
  connectionTimer = undefined
}

function scheduleResize(): void {
  const host = canvasHost.value
  if (!host || !props.active) return
  ui?.resize(Math.max(640, Math.min(1920, host.clientWidth)), Math.max(480, Math.min(1080, host.clientHeight)))
}

function disposeView(): void {
  cancelReady?.()
  cancelReady = undefined
  observer?.disconnect()
  observer = undefined
  try { ui?.shutdown() } catch { /* The server close below remains authoritative. */ }
  finally { ui = undefined; element?.remove(); element = undefined }
  connected.value = false
}

async function releaseSession(): Promise<void> {
  if (!sessionId) return
  const id = sessionId
  try { await api.desktops.close(id) } catch (reason) {
    if (!(reason instanceof ApiError && reason.code === 'desktop_not_found')) throw reason
  }
  if (sessionId === id) sessionId = ''
}

async function closeSession(): Promise<void> {
  const closing = ++sequence
  clearConnectionTimer()
  statusController?.abort()
  password.value = ''
  connecting.value = false
  disposeView()
  await releaseSession()
  if (!unmounted && closing === sequence) emit('state-change', 'finished')
}

async function checkCredentials(): Promise<void> {
  if (unmounted || connecting.value || managing.value) return
  const attempt = ++sequence
  statusController?.abort()
  statusController = new AbortController()
  checking.value = true
  statusFailed.value = false
  error.value = ''
  try {
    const result = await api.desktops.credentialStatus(props.hostId, statusController.signal)
    if (attempt !== sequence || unmounted) return
    saved.value = result.saved
    username.value = result.username || ''
    domain.value = result.domain || ''
    manualLogin.value = !result.saved
    checking.value = false
    if (result.saved) void connect(true)
  } catch {
    if (attempt !== sequence || unmounted) return
    statusFailed.value = true
    error.value = phrase('无法读取已保存的登录信息。可以重试或仅连接本次。')
  } finally {
    if (attempt === sequence) checking.value = false
  }
}

async function connect(useSaved: boolean, saveFirst = false): Promise<void> {
  if (unmounted || connecting.value || managing.value || checking.value) return
  if (!useSaved && (!username.value.trim() || !password.value)) return
  const utf8 = new TextEncoder()
  if (!useSaved && (utf8.encode(username.value.trim()).length > 256 || utf8.encode(domain.value.trim()).length > 256 || utf8.encode(password.value).length > 1024)) {
    error.value = phrase('登录字段过长，请缩短用户名、域或密码。')
    return
  }
  const attempt = ++sequence
  // Passwords leave reactive state after submission and never enter browser storage.
  let credentials = { username: username.value.trim(), domain: domain.value.trim(), password: password.value }
  password.value = ''
  connecting.value = true
  error.value = ''
  emit('state-change', 'connecting')
  try {
    // A failed close keeps its ID; retry must close it before creating another session.
    await releaseSession()
    if (attempt !== sequence || unmounted) return
    if (saveFirst) {
      saving.value = true
      try {
        const result = await api.desktops.saveCredentials(props.hostId, credentials)
        if (attempt !== sequence || unmounted) return
        if (!result.saved) throw new Error('credentials were not saved')
        saved.value = true
        manualLogin.value = false
        statusFailed.value = false
      } catch {
        if (attempt === sequence) error.value = phrase('登录信息保存失败，尚未连接。请重新输入密码后重试。')
        return
      } finally {
        saving.value = false
      }
    }
    // Bound loading, session allocation and negotiation, not the live desktop.
    // Invalidating this attempt also makes any late result close itself.
    connectionTimer = window.setTimeout(() => {
      if (attempt !== sequence || unmounted) return
      credentials.password = ''
      error.value = phrase('远程桌面连接超时，请重试或更换账户。')
      const closing = closeSession()
      const closingSequence = sequence
      void closing.catch(() => { if (!unmounted && closingSequence === sequence) error.value = phrase('关闭未确认，请重试关闭会话。') })
    }, 30_000)
    const rdp = await loadRemoteDesktop()
    if (attempt !== sequence || unmounted) return
    const opened = await api.desktops.open(props.hostId, useSaved)
    if (attempt !== sequence || unmounted) {
      if (opened.credentials) opened.credentials.password = ''
      await api.desktops.close(opened.sessionId)
      return
    }
    sessionId = opened.sessionId
    if (useSaved) {
      if (!opened.credentials?.username || !opened.credentials.password) throw new Error('saved credentials missing')
      credentials = { ...opened.credentials, domain: opened.credentials.domain || '' }
      opened.credentials.password = ''
      username.value = credentials.username
      domain.value = credentials.domain
    }
    element = document.createElement('iron-remote-desktop')
    Object.assign(element, { module: rdp.Backend })
    element.style.cssText = 'display:block;width:100%;height:100%;min-height:320px'
    const view = element
    const ready = new Promise<UserInteraction>((resolve, reject) => {
      const finish = () => { window.clearTimeout(timer); view.removeEventListener('ready', onReady); cancelReady = undefined }
      const onReady = (event: Event) => { finish(); resolve((event as CustomEvent<{ irgUserInteraction: UserInteraction }>).detail.irgUserInteraction) }
      const timer = window.setTimeout(() => { finish(); reject(new Error('desktop initialization timeout')) }, 10000)
      cancelReady = () => { finish(); reject(new Error('desktop initialization cancelled')) }
      view.addEventListener('ready', onReady, { once: true })
    })
    canvasHost.value?.replaceChildren(view)
    const interaction = await ready
    if (attempt !== sequence || unmounted) { interaction.shutdown(); return }
    ui = interaction
    interaction.setEnableClipboard(false)
    interaction.setEnableAutoClipboard(false)
    interaction.setKeyboardUnicodeMode(true)
    const config = interaction.configBuilder()
      .withUsername(credentials.username).withPassword(credentials.password).withServerDomain(credentials.domain)
      .withDestination('localhost').withProxyAddress(api.desktops.socket(sessionId)).withAuthToken(opened.nonce)
      .withDesktopSize({ width: 1280, height: 800 })
      .withExtension(rdp.displayControl(true)).withExtension(rdp.enableCredssp(true))
      .withExtension(rdp.outboundMessageSizeLimit(60 * 1024)).build()
    credentials.password = ''
    const live = await interaction.connect(config)
    if (attempt !== sequence || unmounted) { interaction.shutdown(); return }
    clearConnectionTimer()
    connected.value = true
    connecting.value = false
    interaction.setVisibility(props.active)
    observer = new ResizeObserver(scheduleResize)
    if (canvasHost.value) observer.observe(canvasHost.value)
    emit('state-change', 'connected')
    await live.run()
  } catch (reason) {
    if (attempt === sequence) {
      if (reason instanceof ApiError && reason.code === 'desktop_credentials_missing') {
        saved.value = false
        manualLogin.value = true
        error.value = phrase('已保存的登录信息不存在，请重新输入。')
      } else if (useSaved && reason instanceof ApiError && reason.code === 'desktop_credentials_unavailable') {
        statusFailed.value = true
        manualLogin.value = true
        error.value = phrase('无法读取已保存的登录信息。可以重试或仅连接本次。')
      } else error.value = phrase('远程桌面连接失败，请检查 Windows 账户、NLA、RDP 服务和证书。')
    }
  } finally {
    credentials.password = ''
    if (attempt === sequence) {
      connecting.value = false
      const closing = closeSession()
      const closingSequence = sequence
      await closing.catch(() => { if (!unmounted && closingSequence === sequence) error.value = phrase('关闭未确认，请重试关闭会话。') })
    }
  }
}

async function manageCredentials(clear: boolean): Promise<void> {
  if (managing.value || checking.value || saving.value) return
  managing.value = true
  error.value = ''
  try {
    await closeSession()
    const attempt = sequence
    if (unmounted) return
    if (clear) {
      try {
        const result = await api.desktops.clearCredentials(props.hostId)
        if (attempt !== sequence || unmounted) return
        if (result.saved) throw new Error('credentials were not cleared')
        saved.value = false
        username.value = ''
        domain.value = ''
      } catch {
        if (attempt === sequence && !unmounted) error.value = phrase('登录信息清除失败，请重试。当前远程桌面已断开。')
        return
      }
    }
    manualLogin.value = true
  } catch {
    if (!unmounted) error.value = phrase('关闭未确认，请重试关闭会话。')
  } finally {
    if (!unmounted) managing.value = false
  }
}

watch(() => props.active, (active) => { ui?.setVisibility(active); if (active) scheduleResize() })
onMounted(() => { void checkCredentials() })
onBeforeUnmount(() => { unmounted = true; void closeSession().catch(() => undefined) })
defineExpose({ closeSession, scheduleResize, focusTerminal: () => canvasHost.value?.focus(), executeCommand: () => false })
</script>

<template>
  <section class="host-desktop">
    <p v-if="error" class="host-desktop__error" role="alert">{{ error }}</p>
    <div v-if="checking" class="host-desktop__status" role="status"><LoaderCircle class="spin" :size="20" />{{ phrase('正在读取登录信息…') }}</div>
    <form v-if="!checking && !connected && !connecting && manualLogin" class="host-desktop__login form-stack" autocomplete="off" @submit.prevent="connect(false, !statusFailed)">
      <Monitor :size="32" />
      <h2>{{ hostName }} · RDP</h2>
      <p>{{ phrase('首次保存 Windows 登录信息后，再次打开 RDP 将自动连接。登录信息在中心加密保存，仅供当前 KPanel 用户连接此主机。') }}</p>
      <p>{{ phrase('建议使用桌面浏览器。连接可能锁定本机用户的桌面，具体取决于 Windows 的会话策略。') }}</p>
      <label class="field">{{ phrase('用户名') }}<input v-model="username" autocomplete="off" maxlength="256" :disabled="managing" required /></label>
      <label class="field">{{ phrase('域（可选）') }}<input v-model="domain" autocomplete="off" maxlength="256" :disabled="managing" /></label>
      <label class="field">{{ t('auth.password') }}<input v-model="password" type="password" autocomplete="new-password" maxlength="1024" :disabled="managing" required /></label>
      <div class="host-desktop__actions">
        <button v-if="!statusFailed" class="button button--primary" type="submit" :disabled="managing">{{ phrase('保存并连接') }}</button>
        <button class="button button--secondary" :type="statusFailed ? 'submit' : 'button'" :disabled="managing" @click="!statusFailed && connect(false)">{{ phrase('仅连接本次') }}</button>
        <button v-if="statusFailed" class="button button--secondary" type="button" @click="checkCredentials">{{ phrase('重新读取登录信息') }}</button>
      </div>
      <small v-if="saved">{{ phrase('仅连接本次不会替换已保存的账户。') }}</small>
      <small>{{ phrase('剪贴板、文件传输和打印重定向默认关闭。') }}</small>
    </form>
    <div v-if="!checking && !connected && !connecting && !manualLogin" class="host-desktop__login form-stack">
      <Monitor :size="32" /><h2>{{ hostName }} · RDP</h2>
      <p>{{ phrase('已保存登录账户') }}: {{ domain ? `${domain}\\${username}` : username }}</p>
      <button class="button button--primary" :disabled="managing" @click="connect(true)">{{ phrase('使用已保存账户连接') }}</button>
    </div>
    <div v-if="connecting" class="host-desktop__status" role="status"><LoaderCircle class="spin" :size="20" />{{ saving ? phrase('正在保存登录信息…') : phrase('正在连接远程桌面…') }}</div>
    <div v-if="!checking && (saved || connected || connecting)" class="host-desktop__toolbar">
      <button v-if="connected" class="button button--secondary button--small" :disabled="managing" @click="ui?.ctrlAltDel()">Ctrl + Alt + Del</button>
      <button v-if="!manualLogin || connected || connecting" class="button button--secondary button--small" :disabled="managing || saving" @click="manageCredentials(false)">{{ connected || connecting ? phrase('断开并更换账户') : phrase('更换账户') }}</button>
      <button v-if="saved" class="button button--secondary button--small" :disabled="managing || saving" @click="manageCredentials(true)">{{ connected || connecting ? phrase('清除登录信息并断开') : phrase('清除登录信息') }}</button>
    </div>
    <div ref="canvasHost" class="host-desktop__canvas" :class="{ 'is-hidden': !connected && !connecting }" tabindex="-1" />
  </section>
</template>

<style scoped>
.host-desktop { display: flex; flex: 1; flex-direction: column; min-height: 0; overflow: auto; background: var(--surface); color: var(--text); }
.host-desktop__login { width: min(100% - 40px, 380px); margin: auto; padding: 32px 0; }
.host-desktop__login h2 { font-size: 20px; margin: 0; overflow-wrap: anywhere; }
.host-desktop__login .field,.host-desktop__login input,.host-desktop__toolbar,.host-desktop__status,.host-desktop__error { font-size: 14px; }
.host-desktop__login p,.host-desktop__login small { color: var(--muted); line-height: 1.6; overflow-wrap: anywhere; }
.host-desktop__canvas { flex: 1; min-height: 320px; overflow: hidden; }
.host-desktop__canvas.is-hidden { display: none; }
.host-desktop__actions,.host-desktop__status,.host-desktop__toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.host-desktop__status,.host-desktop__toolbar { padding: 12px; }
.host-desktop__error { color: var(--danger); padding: 12px 20px; }
</style>
