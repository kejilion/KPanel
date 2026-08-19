import { readonly, ref } from 'vue'
import { api, ApiError } from '@/lib/api'
import type {
  DesktopShortcut,
  DesktopWorkspace,
  DesktopWorkspaceUpdate,
} from '@/types/api'

export type DesktopWorkspaceDraft = Omit<DesktopWorkspaceUpdate, 'expectedResourceVersion'>
/** Return false to explicitly skip the network write after inspecting the latest draft. */
export type DesktopWorkspaceMutation = (draft: DesktopWorkspaceDraft) => unknown

const EMPTY_RESOURCE_VERSION = 'sha256:' + '0'.repeat(64)

function emptyWorkspace(): DesktopWorkspace {
  return {
    schemaVersion: 2,
    resourceVersion: EMPTY_RESOURCE_VERSION,
    available: true,
    hiddenEntryKeys: [],
    positions: {},
    labels: {},
    shortcuts: [],
  }
}

function cloneShortcuts(shortcuts: DesktopShortcut[]): DesktopWorkspaceDraft['shortcuts'] {
  return shortcuts.map(({ id, name, description, targetType, url, path }) => ({
    id,
    name,
    description,
    targetType,
    ...(url ? { url } : {}),
    ...(path ? { path } : {}),
  }))
}

function draftFrom(value: DesktopWorkspace): DesktopWorkspaceDraft {
  return {
    hiddenEntryKeys: [...value.hiddenEntryKeys],
    positions: Object.fromEntries(
      Object.entries(value.positions).map(([key, position]) => [key, { ...position }]),
    ),
    labels: { ...value.labels },
    shortcuts: cloneShortcuts(value.shortcuts),
  }
}

const workspace = ref<DesktopWorkspace>(emptyWorkspace())
const loaded = ref(false)
const loading = ref(false)
const saving = ref(false)
let loadSequence = 0
let saveTail: Promise<unknown> = Promise.resolve()

async function load(signal?: AbortSignal): Promise<DesktopWorkspace> {
  const sequence = ++loadSequence
  loading.value = true
  try {
    const value = await api.desktop.workspace(signal)
    if (sequence === loadSequence) {
      workspace.value = value
      loaded.value = true
    }
    return value
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

/**
 * Serialize desktop writes so quick consecutive drops cannot arrive out of
 * order. The mutation is applied to the latest confirmed server snapshot.
 */
function mutate(change: DesktopWorkspaceMutation): Promise<DesktopWorkspace> {
  const operation = saveTail.then(async () => {
    if (!loaded.value) await load()
    if (!workspace.value.available) throw new Error('desktop_workspace_unavailable')
    const base = workspace.value
    const draft = draftFrom(base)
    if (change(draft) === false) return base
    saving.value = true
    try {
      const saved = await api.desktop.updateWorkspace({
        expectedResourceVersion: base.resourceVersion,
        ...draft,
      })
      workspace.value = saved
      return saved
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        // Preserve the remote winner. The caller still receives the conflict
        // and can explain that its local action was not committed.
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

function generateShortcutID(): string {
  const cryptoObject = globalThis.crypto
  if (typeof cryptoObject?.randomUUID === 'function') {
    return cryptoObject.randomUUID().replaceAll('-', '').toLowerCase()
  }
  const bytes = new Uint8Array(16)
  cryptoObject?.getRandomValues?.(bytes)
  if (bytes.some(Boolean)) return [...bytes].map((value) => value.toString(16).padStart(2, '0')).join('')
  // The fallback is only for restricted test/legacy browser environments; the
  // server still validates uniqueness and the 32-hex identifier contract.
  return `${Date.now().toString(16).padStart(12, '0')}${Math.random().toString(16).slice(2).padEnd(20, '0')}`.slice(0, 32)
}

function resetDesktopIconsForTest(): void {
  loadSequence += 1
  workspace.value = emptyWorkspace()
  loaded.value = false
  loading.value = false
  saving.value = false
  saveTail = Promise.resolve()
}

export function useDesktopIcons() {
  return {
    workspace: readonly(workspace),
    loaded: readonly(loaded),
    loading: readonly(loading),
    saving: readonly(saving),
    load,
    mutate,
    generateShortcutID,
  }
}

export { resetDesktopIconsForTest }
