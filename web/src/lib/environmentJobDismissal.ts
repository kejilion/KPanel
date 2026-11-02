import type { WebEnvironmentJob } from '@/types/api'

const storageKey = 'kpanel:environment-dismissed-results:v1'
const maxEntries = 50
const validResult = /^[a-f0-9]{32}:(succeeded|failed|needs_attention)$/

export function environmentJobNoticeKey(job: WebEnvironmentJob): string {
  return `${job.id}:${job.status}`
}

export function readDismissedEnvironmentResults(): string[] {
  try {
    const raw = window.localStorage.getItem(storageKey)
    if (!raw || raw.length > 4096) return []
    const values: unknown = JSON.parse(raw)
    if (!Array.isArray(values) || values.length > maxEntries) return []
    return [...new Set(values.filter((value): value is string =>
      typeof value === 'string' && validResult.test(value),
    ))]
  } catch {
    return []
  }
}

export function dismissEnvironmentResult(job: WebEnvironmentJob, current: string[]): string[] {
  const key = environmentJobNoticeKey(job)
  if (!validResult.test(key)) return current
  // Merge other windows' acknowledgements; retain only bounded presentation preferences.
  const next = [...new Set([key, ...readDismissedEnvironmentResults(), ...current])].slice(0, maxEntries)
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(next))
  } catch {
    // Hiding still works in this view when browser storage is unavailable.
  }
  return next
}
