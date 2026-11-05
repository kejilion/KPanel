<script setup lang="ts">
import { inject, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { LoaderCircle } from '@lucide/vue'
import { desktopWindowCloseGuardKey } from '@/lib/desktopRouteKeys'
import HostTerminal from '@/components/terminal/HostTerminal.vue'
import { api, ApiError } from '@/lib/api'
import { closeTerminalQuietly, deferTerminalClose, queueTerminalLaunchClose } from '@/lib/terminalClose'
import { useSession } from '@/stores/session'
import { useI18n } from '@/i18n'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import type { TerminalSession } from '@/types/api'

const props = defineProps<{ name: string; command: string; hostName: string; hostId?: string; shortcutId?: string; shortcutVersion?: string; purpose?: 'site-management'; existingSession?: TerminalSession }>()
const { t } = useI18n()
const emit = defineEmits<{ close: [] }>()
const closeGuards = inject(desktopWindowCloseGuardKey, undefined)
let unregisterCloseGuard: (() => void) | undefined
let openingRequest: Promise<void> | undefined
const terminal = ref<InstanceType<typeof HostTerminal>>()
const session = ref<TerminalSession>()
const opening = ref(true)
const closing = ref(false)
const error = ref('')
// Snapshot the command for this terminal session.
const command = props.command
const hostName = props.hostName
const hostId = props.hostId || 'local'
// Saved shortcuts and fixed built-in entries share server-side launch fencing.
const launchTarget = { shortcutId: props.shortcutId, shortcutVersion: props.shortcutVersion, purpose: props.purpose }
const namedLaunch = Boolean(launchTarget.shortcutId || launchTarget.purpose)
let disposed = false
let userClosed = false
let submitted = false
let launchId: string | undefined
const account = useSession()
const owner = account.state.authenticated ? account.state.user?.id || '' : ''

function queueLaunchClose(): void {
  if (launchId && namedLaunch) queueTerminalLaunchClose({ launchId, hostId, ...launchTarget }, owner)
}

function resolveLaunch(signal?: AbortSignal): Promise<TerminalSession> {
  return launchTarget.purpose
    ? api.terminals.resolvePurpose(launchTarget.purpose, launchId, signal)
    : api.terminals.resolveShortcut(hostId, launchTarget.shortcutId!, launchId, signal)
}

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

async function reclaim(opened: TerminalSession): Promise<void> {
  const result = await api.terminals.close(opened.sessionId)
  if (!result.closed) throw new Error('Terminal close was not confirmed')
}

async function open(): Promise<void> {
  opening.value = true
  error.value = ''
  try {
    if (namedLaunch && !props.existingSession && !launchId) {
      launchId = (await (launchTarget.purpose ? api.terminals.preparePurpose(launchTarget.purpose) : api.terminals.prepareShortcut(hostId, launchTarget.shortcutId!))).launchId
      if (!launchId) throw new Error('Terminal launch identity is missing')
      if (disposed) {
        if (userClosed) queueLaunchClose()
        else void resolveLaunch().catch(() => undefined)
        return
      }
      if (closing.value) return
    }
    if (owner && (!account.state.authenticated || account.state.user?.id !== owner)) {
      if (userClosed) queueLaunchClose()
      return
    }
    const opened = props.existingSession || await (namedLaunch
      ? api.terminals.open(hostId, 30, 120, { ...launchTarget, retained: true, launchId }).catch(reason => {
        if (disposed || closing.value || !(reason instanceof ApiError) || reason.code !== 'terminal_retention_unavailable') throw reason
        // The server rejected retention before launching the command. Reuse its identity; never retry an uncertain launch.
        return api.terminals.open(hostId, 30, 120, { ...launchTarget, retained: false, launchId })
      })
      : api.terminals.open(hostId, 30, 120))
    if (disposed) {
      if (userClosed || !opened.retained) {
        if (owner && (!account.state.authenticated || account.state.user?.id !== owner)) deferTerminalClose(opened.sessionId, owner)
        else await closeTerminalQuietly(opened.sessionId, () => reclaim(opened))
      }
      return
    }
    session.value = opened
    if (opened.launchUncertain) error.value = t('terminal.launchUncertain')
  } catch (reason) {
    if (!disposed) error.value = reason instanceof ApiError && reason.code === 'terminal_limit'
      ? phrase('已达到终端会话上限，请先关闭不用的终端。')
      : phrase('终端启动失败，请检查主机连接后重新打开。')
  } finally {
    opening.value = false
  }
}

async function onStateChange(state: string): Promise<void> {
  if (namedLaunch || state !== 'connected' || submitted || disposed || closing.value || !command) return
  // A reconnect must never repeat a command. Mark before awaiting Vue's ref update.
  submitted = true
  await nextTick()
  if (disposed || closing.value) return
  const sent = await terminal.value?.executeCommand(command)
  if (!sent && !disposed && !closing.value) error.value = phrase('命令未发送，请关闭窗口后重新执行。')
}

async function close(): Promise<boolean> {
  if (closing.value) return false
  closing.value = true
  userClosed = true
  if (owner && (!account.state.authenticated || account.state.user?.id !== owner)) {
    if (session.value) deferTerminalClose(session.value.sessionId, owner)
    else queueLaunchClose()
    disposed = true
    emit('close')
    return true
  }
  if (opening.value) {
    disposed = true
    queueLaunchClose()
    emit('close')
    return true
  }
  if (!session.value && namedLaunch) {
    disposed = true
    queueLaunchClose()
    emit('close')
    return true
  }
  try {
    await openingRequest
    let target = session.value
    if (target) {
      try {
        const outcome = await closeTerminalQuietly(target.sessionId, () => session.value && terminal.value ? terminal.value.closeSession() : reclaim(target))
        if (outcome !== 'closed') terminal.value?.markDetached?.()
      } catch (reason) {
        session.value = target
        throw reason
      }
      session.value = undefined
    }
    disposed = true
    emit('close')
    return true
  } catch {
    queueLaunchClose()
    disposed = true
    emit('close')
    return true
  } finally {
    closing.value = false
  }
}

defineExpose({ close })

onMounted(() => {
  unregisterCloseGuard = closeGuards?.register(close)
  openingRequest = open()
})
onBeforeUnmount(() => {
  unregisterCloseGuard?.()
  disposed = true
  // HostTerminal owns mounted sessions, including page-hide cleanup.
  if (session.value && !session.value.retained && !terminal.value) {
    const opened = session.value
    void closeTerminalQuietly(opened.sessionId, () => reclaim(opened))
  }
})
</script>

<template>
  <section class="shortcut-command-terminal" :aria-label="name">
      <div v-if="opening" class="shortcut-command-terminal__state" role="status"><LoaderCircle class="spin" :size="20" />{{ phrase('正在连接终端…') }}</div>
      <div v-if="error" class="inline-alert inline-alert--danger" role="alert">
        {{ error }}
      </div>
      <HostTerminal v-if="session" ref="terminal" :session-id="session.sessionId" :retained="session.retained" :host-name="hostName" :initial-offset="session.retained ? 0 : session.offset" @state-change="onStateChange" />
  </section>
</template>

<style scoped>
.shortcut-command-terminal { height: 100%; overflow: hidden; background: var(--terminal-wallpaper-background, var(--terminal-shell-background)); display: flex; flex: 1 1 auto; min-height: 0; flex-direction: column; }
.shortcut-command-terminal__state { display: flex; flex: 1; align-items: center; justify-content: center; gap: 8px; color: var(--muted); font-size: 14px; }
.shortcut-command-terminal > .inline-alert { margin: 16px; }
.shortcut-command-terminal > :deep(.host-terminal) { flex: 1 1 auto; height: auto; min-width: 0; border: 0; border-radius: 0; box-shadow: none; }
</style>
