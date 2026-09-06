import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'
import type { DockerContainer } from '@/types/api'

export interface DockerImageUpdateResult {
  containerId: string
  image: string
  status: 'available' | 'current' | 'fixed'
  updateAvailable: boolean
  resourceVersion: string
  checkedAt: string
  localDigest?: string
  remoteDigest?: string
}

export type DockerUpdateStatus = DockerImageUpdateResult['status'] | 'checking' | 'unavailable' | 'expired'
interface Entry {
  status: DockerUpdateStatus
  resourceVersion: string
  image: string
  checkedAt?: string
}
export const dockerUpdateTTL = 5 * 60_000
export const dockerUpdateConcurrency = 2
const capacity = 200

export function useDockerImageUpdates(
  containers: Readonly<Ref<readonly DockerContainer[]>>,
  active: Readonly<Ref<boolean>>,
  request: (id: string, version: string, signal: AbortSignal) => Promise<DockerImageUpdateResult>,
) {
  const entries = ref<Record<string, Entry>>({})
  const running = ref(0)
  const controllers = new Map<string, AbortController>()
  const expiryTimers = new Map<string, ReturnType<typeof setTimeout>>()
  const busy = computed(() => running.value >= dockerUpdateConcurrency)

  function remove(id: string) {
    controllers.get(id)?.abort()
    controllers.delete(id)
    clearTimeout(expiryTimers.get(id))
    expiryTimers.delete(id)
    delete entries.value[id]
  }
  function clear() { for (const id of Object.keys(entries.value)) remove(id) }

  watch([containers, active], ([items, enabled]) => {
    if (!enabled) { clear(); return }
    const current = new Map(items.map(item => [item.id, item]))
    for (const [id, entry] of Object.entries(entries.value)) {
      const item = current.get(id)
      if (!item || item.resourceVersion !== entry.resourceVersion || item.image !== entry.image) remove(id)
    }
  }, { flush: 'sync' })
  onScopeDispose(clear)

  async function check(container: DockerContainer) {
    if (!active.value || busy.value || controllers.has(container.id) || !container.resourceVersion) return
    remove(container.id)
    if (Object.keys(entries.value).length >= capacity) {
      const oldest = Object.keys(entries.value).find(id => !controllers.has(id))
      if (oldest) remove(oldest)
    }
    const controller = new AbortController()
    controllers.set(container.id, controller)
    running.value++
    const entry: Entry = { status: 'checking', resourceVersion: container.resourceVersion, image: container.image }
    entries.value[container.id] = entry
    const timeout = setTimeout(() => controller.abort(), 25_000)
    try {
      const result = await request(container.id, container.resourceVersion, controller.signal)
      if (controllers.get(container.id) !== controller) return
      if (controller.signal.aborted || result.containerId !== container.id || result.resourceVersion !== container.resourceVersion
        || !['available', 'current', 'fixed'].includes(result.status)
        || result.updateAvailable !== (result.status === 'available') || !Number.isFinite(Date.parse(result.checkedAt))) {
        throw new Error('Unconfirmed image update response')
      }
      entries.value[container.id] = { ...entry, status: result.status, checkedAt: result.checkedAt }
    } catch {
      if (controllers.get(container.id) !== controller) return
      entries.value[container.id] = { ...entry, status: 'unavailable', checkedAt: new Date().toISOString() }
    } finally {
      clearTimeout(timeout)
      running.value--
      if (controllers.get(container.id) === controller) {
        controllers.delete(container.id)
        expiryTimers.set(container.id, setTimeout(() => {
          const current = entries.value[container.id]
          if (current) current.status = 'expired'
          expiryTimers.delete(container.id)
        }, dockerUpdateTTL))
      }
    }
  }
  return { entries, busy, check, clear }
}
