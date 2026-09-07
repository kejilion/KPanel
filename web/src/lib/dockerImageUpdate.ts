import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'
import type { DockerContainer, DockerImageUpdateResult } from '@/types/api'
export type { DockerImageUpdateResult } from '@/types/api'

export function isConfirmedImageUpdate(result: DockerImageUpdateResult): boolean {
  return Boolean(result && typeof result.containerId === 'string' && result.containerId
    && typeof result.resourceVersion === 'string' && result.resourceVersion
    && ['available', 'current', 'fixed'].includes(result.status)
    && result.updateAvailable === (result.status === 'available')
    && Number.isFinite(Date.parse(result.checkedAt)))
}

export type DockerUpdateStatus = DockerImageUpdateResult['status'] | 'checking' | 'unavailable' | 'expired'
interface Entry {
  status: DockerUpdateStatus
  resourceVersion: string
  image: string
  checkedAt?: string
  reason?: string
  expiresAt?: number
}
export const dockerUpdateTTL = 30 * 60_000
export const dockerUpdateConcurrency = 2
export const dockerUpdateInterval = 1_000
const capacity = 200

export function useDockerImageUpdates(
  containers: Readonly<Ref<readonly DockerContainer[]>>,
  active: Readonly<Ref<boolean>>,
  request: (id: string, version: string, signal: AbortSignal) => Promise<DockerImageUpdateResult>,
) {
  const entries = ref<Record<string, Entry>>({})
  const running = ref(0)
  const controllers = new Map<string, AbortController>()
  let timer: ReturnType<typeof setTimeout> | undefined
  let disposed = false
  const busy = computed(() => running.value >= dockerUpdateConcurrency)

  function remove(id: string) {
    controllers.get(id)?.abort()
    controllers.delete(id)
    delete entries.value[id]
  }
  function clear() { for (const id of Object.keys(entries.value)) remove(id) }

  function schedule() {
    clearTimeout(timer)
    timer = undefined
    if (disposed || !active.value) return
    timer = setTimeout(pump, dockerUpdateInterval)
  }
  function pump() {
    timer = undefined
    if (disposed || !active.value) return
    const now = Date.now()
    for (const entry of Object.values(entries.value)) {
      if (entry.expiresAt && entry.expiresAt <= now) entry.status = 'expired'
    }
    const next = containers.value.slice(0, capacity).find(item => {
      const entry = entries.value[item.id]
      return item.resourceVersion && !controllers.has(item.id)
        && (entry?.status === 'expired' || (!entry && Object.keys(entries.value).length < capacity))
    })
    if (next && !busy.value) {
      void check(next)
      schedule()
    } else if (running.value > 0) {
      // Completion resumes the bounded scan; never build a growing request queue.
    } else {
      const expiry = Math.min(...Object.values(entries.value).flatMap(entry => entry.expiresAt && entry.expiresAt > now ? [entry.expiresAt] : []))
      if (Number.isFinite(expiry)) timer = setTimeout(pump, Math.max(dockerUpdateInterval, expiry - now))
    }
  }

  watch([containers, active], ([items, enabled]) => {
    clearTimeout(timer)
    if (!enabled) {
      for (const id of controllers.keys()) remove(id)
    }
    const current = new Map(items.map(item => [item.id, item]))
    for (const [id, entry] of Object.entries(entries.value)) {
      const item = current.get(id)
      if (!item || item.resourceVersion !== entry.resourceVersion || item.image !== entry.image) remove(id)
      else if (entry.expiresAt && entry.expiresAt <= Date.now()) entry.status = 'expired'
    }
    if (enabled) schedule()
  }, { flush: 'sync', immediate: true })
  onScopeDispose(() => { disposed = true; clearTimeout(timer); clear() })

  async function check(container: DockerContainer) {
    if (disposed || !active.value || busy.value || controllers.has(container.id) || !container.resourceVersion) return
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
      if (controller.signal.aborted || !isConfirmedImageUpdate(result) || result.containerId !== container.id) {
        throw new Error('Unconfirmed image update response')
      }
      // The server may have refreshed a stale read-only snapshot. Keep the
      // list version as our invalidation key; never advance mutation versions.
      entries.value[container.id] = { ...entry, status: result.status, checkedAt: result.checkedAt }
    } catch (error) {
      if (controllers.get(container.id) !== controller) return
      const code = error && typeof error === 'object' && 'code' in error && typeof error.code === 'string' ? error.code : ''
      entries.value[container.id] = { ...entry, status: 'unavailable', checkedAt: new Date().toISOString(), reason: controller.signal.aborted ? 'docker_update_timeout' : code }
    } finally {
      clearTimeout(timeout)
      running.value--
      if (controllers.get(container.id) === controller) {
        controllers.delete(container.id)
        const current = entries.value[container.id]
        if (current) current.expiresAt = Date.now() + (current.reason === 'docker_update_busy' ? 30_000 : dockerUpdateTTL)
      }
      schedule()
    }
  }
  return { entries, busy, check, clear }
}
