<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { LoaderCircle } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import HostTerminal from '@/components/terminal/HostTerminal.vue'
import { api, ApiError } from '@/lib/api'
import { closeTerminalForOwner, terminalCleanupOwner } from '@/lib/terminalClose'
import { fileAPIForHost } from '@/lib/fileHostContext'
import { fileExecutionCommand, fileExecutionDisplayCommand } from '@/lib/fileExecution'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import type { FileEntry, TerminalSession } from '@/types/api'

const props = defineProps<{ entry: FileEntry; hostId: string; hostName: string; shell?: 'posix' | 'powershell' }>()
const emit = defineEmits<{ close: [] }>()
const terminal = ref<InstanceType<typeof HostTerminal>>()
const session = ref<TerminalSession>()
const opening = ref(true)
const closing = ref(false)
const error = ref('')
const controller = new AbortController()
// Snapshot the target: changing the file pane's host must never retarget a pending launch.
const entry = { ...props.entry }
const hostId = props.hostId
const owner = terminalCleanupOwner()
let disposed = false
let submitted = false
let command: string | undefined
let displayCommand: string | undefined

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

async function reclaim(opened: TerminalSession): Promise<void> {
  const result = await api.terminals.close(opened.sessionId)
  if (!result.closed) throw new Error('Terminal close was not confirmed')
}

async function open(): Promise<void> {
  try {
    const fresh = await fileAPIForHost(hostId).entry(entry.path, controller.signal)
    if (disposed || owner && terminalCleanupOwner() !== owner) return
    if (!entry.resourceVersion || fresh.resourceVersion !== entry.resourceVersion || fresh.path !== entry.path) {
      error.value = phrase('文件已变化，请刷新列表后重新执行。')
      return
    }
    command = fileExecutionCommand(fresh, {
      unavailable: phrase('文件不存在或已不再是普通文件。'),
      permission: phrase('文件没有执行权限，请先在文件管理中修改权限。'),
      interpreter: (name) => phrase('缺少解释器：{0}，请安装后重新执行。').replace('{0}', name),
      tool: (name) => phrase('缺少工具：{0}，请安装后重新执行。').replace('{0}', name),
    }, props.shell)
    displayCommand = fileExecutionDisplayCommand(fresh, props.shell)
    if (!command || !displayCommand) {
      error.value = phrase('当前文件不支持在终端中执行。')
      return
    }
    const opened = await api.terminals.open(hostId || 'local', 30, 120)
    if (disposed || owner && terminalCleanupOwner() !== owner) {
      await closeTerminalForOwner(opened.sessionId, owner, () => reclaim(opened))
      return
    }
    session.value = opened
  } catch (reason) {
    if (!disposed) error.value = reason instanceof ApiError && reason.code === 'terminal_limit'
      ? phrase('已达到终端会话上限，请先关闭不用的终端。')
      : phrase('终端启动失败，请检查主机连接后重新打开。')
  } finally {
    opening.value = false
  }
}

async function onStateChange(state: string): Promise<void> {
  if (state !== 'connected' || submitted || disposed || closing.value || owner && terminalCleanupOwner() !== owner || !command || !displayCommand) return
  // A reconnect must never repeat a script or installation. Mark before awaiting Vue's ref update.
  submitted = true
  await nextTick()
  if (disposed || closing.value || owner && terminalCleanupOwner() !== owner) return
  const sent = await terminal.value?.executePresentedCommand(command, displayCommand)
  if (!sent && !disposed && !closing.value) error.value = phrase('命令未发送，请关闭窗口后重新执行。')
}

async function close(): Promise<void> {
  if (closing.value) return
  closing.value = true
  try {
    if (session.value) {
      const target = session.value
      const outcome = await closeTerminalForOwner(target.sessionId, owner, () => terminal.value ? terminal.value.closeSession() : reclaim(target))
      if (outcome !== 'closed') terminal.value?.markDetached?.()
      session.value = undefined
    }
    disposed = true
    controller.abort()
    emit('close')
  } catch {
    disposed = true
    controller.abort()
    emit('close')
  } finally {
    closing.value = false
  }
}

onMounted(open)
onBeforeUnmount(() => {
  disposed = true
  controller.abort()
  // HostTerminal owns mounted sessions, including page-hide cleanup.
  if (session.value && !terminal.value) {
    const opened = session.value
    void closeTerminalForOwner(opened.sessionId, owner, () => reclaim(opened))
  }
})
</script>

<template>
  <ModalDialog :open="true" :title="`${phrase('在终端中执行')} · ${entry.name}`" :description="hostName" variant="workspace" allow-fullscreen :close-disabled="closing" @close="close">
    <div class="file-execution-terminal">
      <div v-if="opening" class="file-execution-terminal__state" role="status"><LoaderCircle class="spin" :size="20" />{{ phrase('正在连接终端…') }}</div>
      <div v-if="error" class="inline-alert inline-alert--danger" role="alert">{{ error }}</div>
      <HostTerminal v-if="session" ref="terminal" :session-id="session.sessionId" :host-name="hostName" :initial-offset="session.offset" @state-change="onStateChange" />
    </div>
  </ModalDialog>
</template>

<style scoped>
.file-execution-terminal { display: flex; flex: 1 1 auto; min-height: 0; flex-direction: column; }
.file-execution-terminal__state { display: flex; flex: 1; align-items: center; justify-content: center; gap: 8px; color: var(--muted); font-size: 14px; }
.file-execution-terminal > .inline-alert { margin: 16px; }
.file-execution-terminal > :deep(.host-terminal) { flex: 1 1 auto; height: auto; }
</style>
