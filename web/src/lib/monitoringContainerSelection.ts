import type { MonitoringContainerSeries } from '@/types/api'

export const monitoringContainerSelectionLimit = 5
export const monitoringContainerDefaultCount = 3
export const monitoringContainerColors = ['#2563eb', '#0f766e', '#8b5cf6', '#d97706', '#db2777'] as const

const preferenceKey = 'kejilion-panel-monitoring-containers-v1'
const hostPreferenceKey = 'kejilion-panel-monitoring-host-containers-v1'

function readHostPreferences(storage: PreferenceReader): Record<string, MonitoringContainerPreference> {
  try {
    const raw = storage.getItem(hostPreferenceKey)
    if (!raw || raw.length > 128 * 1024) return {}
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    const result: Record<string, MonitoringContainerPreference> = {}
    for (const [id, value] of Object.entries(parsed).slice(-100)) {
      if (!/^[a-f0-9]{32}$/.test(id)) continue
      const preference = readMonitoringContainerPreference({ getItem: () => JSON.stringify(value) })
      if (preference) result[id] = preference
    }
    return result
  } catch { return {} }
}

export function readMonitoringHostContainerPreference(
  hostId: string,
  storage: PreferenceReader = localStorage,
): MonitoringContainerPreference | undefined {
  return hostId === 'local' ? readMonitoringContainerPreference(storage) : readHostPreferences(storage)[hostId]
}

export function writeMonitoringHostContainerPreference(
  hostId: string,
  preference: MonitoringContainerPreference,
  storage: PreferenceReader & PreferenceWriter = localStorage,
): void {
  if (hostId === 'local') { writeMonitoringContainerPreference(preference, storage); return }
  if (!/^[a-f0-9]{32}$/.test(hostId)) return
  try {
    const preferences = readHostPreferences(storage)
    delete preferences[hostId]
    writeMonitoringContainerPreference(preference, { setItem: (_key, value) => { preferences[hostId] = JSON.parse(value) } })
    storage.setItem(hostPreferenceKey, JSON.stringify(Object.fromEntries(Object.entries(preferences).slice(-100))))
  } catch { /* Storage may be disabled; selection still works in memory. */ }
}

interface PreferenceReader {
  getItem: (key: string) => string | null
}

interface PreferenceWriter {
  setItem: (key: string, value: string) => void
}

export interface MonitoringContainerPreference {
  ids: string[]
  slots: Record<string, number>
}

function validContainerID(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0 && value.length <= 64
}

function latestPointTime(container: MonitoringContainerSeries): number | undefined {
  const value = Date.parse(container.points.at(-1)?.collectedAt || '')
  return Number.isFinite(value) ? value : undefined
}

export function defaultMonitoringContainerIDs(
  containers: MonitoringContainerSeries[],
  count = monitoringContainerDefaultCount,
): string[] {
  const available = containers.filter((container) => latestPointTime(container) !== undefined)
  let newest: number | undefined
  for (const container of available) {
    const latest = latestPointTime(container)
    if (latest !== undefined && (newest === undefined || latest > newest)) newest = latest
  }
  const current = newest === undefined
    ? []
    : available.filter((container) => latestPointTime(container) === newest)
  const candidates = current.length ? current : available.slice(0, 1)
  return candidates.slice(0, Math.max(1, count)).map((container) => container.containerId)
}

export function reconcileMonitoringContainerIDs(
  containers: MonitoringContainerSeries[],
  preferred: readonly string[],
): string[] {
  const available = new Set(containers.map((container) => container.containerId))
  const retained = Array.from(new Set(preferred))
    .filter((id) => available.has(id))
    .slice(0, monitoringContainerSelectionLimit)
  return retained.length ? retained : defaultMonitoringContainerIDs(containers)
}

export function assignMonitoringContainerColorSlots(
  ids: readonly string[],
  previous: Readonly<Record<string, number>> = {},
): Record<string, number> {
  const selected = Array.from(new Set(ids)).slice(0, monitoringContainerSelectionLimit)
  const next: Record<string, number> = {}
  const used = new Set<number>()
  for (const id of selected) {
    const slot = previous[id]
    if (typeof slot === 'number' && Number.isInteger(slot) &&
      slot >= 0 && slot < monitoringContainerColors.length && !used.has(slot)) {
      next[id] = slot
      used.add(slot)
    }
  }
  for (const id of selected) {
    if (id in next) continue
    const slot = monitoringContainerColors.findIndex((_, index) => !used.has(index))
    if (slot < 0) break
    next[id] = slot
    used.add(slot)
  }
  return next
}

export function readMonitoringContainerPreference(
  storage: PreferenceReader = localStorage,
): MonitoringContainerPreference | undefined {
  try {
    const raw = storage.getItem(preferenceKey)
    if (!raw) return undefined
    const parsed = JSON.parse(raw) as { ids?: unknown; slots?: unknown }
    if (!Array.isArray(parsed.ids)) return undefined
    const ids = Array.from(new Set(parsed.ids.filter(validContainerID))).slice(0, monitoringContainerSelectionLimit)
    if (!ids.length) return undefined
    const rawSlots = parsed.slots && typeof parsed.slots === 'object'
      ? parsed.slots as Record<string, unknown>
      : {}
    const slots: Record<string, number> = {}
    for (const id of ids) {
      const slot = rawSlots[id]
      if (Number.isInteger(slot) && Number(slot) >= 0 && Number(slot) < monitoringContainerColors.length) {
        slots[id] = Number(slot)
      }
    }
    return { ids, slots: assignMonitoringContainerColorSlots(ids, slots) }
  } catch {
    return undefined
  }
}

export function writeMonitoringContainerPreference(
  preference: MonitoringContainerPreference,
  storage: PreferenceWriter = localStorage,
): void {
  try {
    const ids = Array.from(new Set(preference.ids.filter(validContainerID)))
      .slice(0, monitoringContainerSelectionLimit)
    if (!ids.length) return
    storage.setItem(preferenceKey, JSON.stringify({
      ids,
      slots: assignMonitoringContainerColorSlots(ids, preference.slots),
    }))
  } catch {
    // Hardened browser contexts can disable storage; the current in-memory selection remains valid.
  }
}
