import { formatBytes } from '@/lib/format'
import type { PublicNetworkSummary } from '@/types/api'

/** The system fields a hover card shows; panel telemetry and public share hosts both fit. */
export interface HostSystemSummary {
  os?: string
  osId?: string
  osLike?: string[]
  architecture?: string
  kernel?: string
  cpu?: { model?: string; cores?: number }
}

export interface AutonomousSystemLabel {
  asn?: string
  organization?: string
}

/**
 * IP geolocation sources report the network as one string such as
 * "AS31898 Oracle Corporation". The cluster hover card shows the ASN and the
 * operator on separate rows; values without an ASN prefix stay intact.
 */
export function splitAutonomousSystem(isp?: string): AutonomousSystemLabel {
  const value = isp?.trim()
  if (!value) return {}
  const match = /^(AS\d{1,10})(?:[\s:·,-]+(.*))?$/i.exec(value)
  if (!match) return { organization: value }
  return { asn: match[1]!.toUpperCase(), organization: match[2]?.trim() || undefined }
}

/** Country, region and city without repeating a city-state such as "HK · Hong Kong · Hong Kong". */
export function hostLocationLabel(location?: PublicNetworkSummary): string {
  const parts = [location?.country, location?.region, location?.city]
    .map((value) => value?.trim() || '')
    .filter(Boolean)
  return parts.filter((value, index) => parts.indexOf(value) === index).join(' · ')
}

export type UsageTone = 'normal' | 'warning' | 'danger'

/** Meter colour only; every meter also prints its percentage, so tone is never the sole signal. */
export function usageTone(percent?: number): UsageTone {
  if (typeof percent !== 'number' || !Number.isFinite(percent)) return 'normal'
  if (percent >= 90) return 'danger'
  return percent >= 75 ? 'warning' : 'normal'
}

/** "41.3 / 146.5 GB" when both sides share a unit, so a narrow meter caption stays on one line. */
export function formatCapacityPair(used?: number, total?: number): string {
  const [usedValue, usedUnit] = formatBytes(used).split(' ')
  const totalText = formatBytes(total)
  if (usedUnit && usedUnit === totalText.split(' ')[1]) return `${usedValue} / ${totalText}`
  return `${formatBytes(used)} / ${totalText}`
}
