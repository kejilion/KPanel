import { readonly, ref } from 'vue'
import { api, ApiError } from '@/lib/api'
import type {
  TerminalQuickCommand,
  TerminalQuickCommands,
  TerminalQuickCommandsUpdate,
} from '@/types/api'

export type TerminalCommandsDraft = Pick<TerminalQuickCommandsUpdate, 'items'>
/** Return false to skip the network write after inspecting the latest draft. */
export type TerminalCommandsMutation = (draft: TerminalCommandsDraft) => unknown

const EMPTY_RESOURCE_VERSION = 'sha256:' + '0'.repeat(64)

function emptyCommands(): TerminalQuickCommands {
  return {
    schemaVersion: 1,
    resourceVersion: EMPTY_RESOURCE_VERSION,
    available: true,
    items: [],
  }
}

function cloneItems(items: TerminalQuickCommand[]): TerminalQuickCommand[] {
  return items.map(({ id, name, command }) => ({ id, name, command }))
}

const commands = ref<TerminalQuickCommands>(emptyCommands())
const loaded = ref(false)
const loading = ref(false)
const saving = ref(false)
let loadSequence = 0
let saveTail: Promise<unknown> = Promise.resolve()

async function load(signal?: AbortSignal): Promise<TerminalQuickCommands> {
  const sequence = ++loadSequence
  loading.value = true
  try {
    const value = await api.terminals.commands(signal)
    if (sequence === loadSequence) {
      commands.value = value
      loaded.value = true
    }
    return value
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

function mutate(change: TerminalCommandsMutation): Promise<TerminalQuickCommands> {
  const operation = saveTail.then(async () => {
    if (!loaded.value) await load()
    if (!commands.value.available) throw new Error('terminal_commands_unavailable')
    const base = commands.value
    const draft: TerminalCommandsDraft = { items: cloneItems(base.items) }
    if (change(draft) === false) return base
    saving.value = true
    try {
      const saved = await api.terminals.updateCommands({
        expectedResourceVersion: base.resourceVersion,
        items: draft.items,
      })
      commands.value = saved
      return saved
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        await load().catch(() => undefined)
      }
      throw error
    } finally {
      saving.value = false
    }
  })
  saveTail = operation.catch(() => undefined)
  return operation
}

function generateCommandID(): string {
  const cryptoObject = globalThis.crypto
  if (typeof cryptoObject?.randomUUID === 'function') {
    return cryptoObject.randomUUID().replaceAll('-', '').toLowerCase()
  }
  const bytes = new Uint8Array(16)
  cryptoObject?.getRandomValues?.(bytes)
  if (bytes.some(Boolean)) return [...bytes].map((value) => value.toString(16).padStart(2, '0')).join('')
  return `${Date.now().toString(16).padStart(12, '0')}${Math.random().toString(16).slice(2).padEnd(20, '0')}`.slice(0, 32)
}

function resetTerminalCommandsForTest(): void {
  loadSequence += 1
  commands.value = emptyCommands()
  loaded.value = false
  loading.value = false
  saving.value = false
  saveTail = Promise.resolve()
}

export function useTerminalCommands() {
  return {
    commands: readonly(commands),
    loaded: readonly(loaded),
    loading: readonly(loading),
    saving: readonly(saving),
    load,
    mutate,
    generateCommandID,
  }
}

export { resetTerminalCommandsForTest }
