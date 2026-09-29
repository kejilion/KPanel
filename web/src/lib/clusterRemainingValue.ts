import { parseClusterHostPrice } from './clusterHostPrice'
import type { ClusterHostDetails } from '@/types/api'

const dayMs = 86_400_000
export type RemainingValueStatus = 'active' | 'expired' | 'missingPrice' | 'unknownPrice' | 'missingExpiry' | 'invalidExpiry'

export function estimateRemainingValue(details: ClusterHostDetails | undefined, now: Date) {
  if (!details?.price?.trim()) return { status: 'missingPrice' as const }
  const price = parseClusterHostPrice(details.price)
  if (!price || !price.currency || !price.cycleMonths || !Number.isSafeInteger(price.cycleMonths)
    || price.cycleMonths > 1200 || !Number.isFinite(price.amount) || price.amount > 1_000_000_000_000) {
    return { status: 'unknownPrice' as const }
  }
  if (!details.expiresOn) return { status: 'missingExpiry' as const }
  const expires = new Date(`${details.expiresOn}T00:00:00Z`)
  if (!/^(?!0000)\d{4}-\d{2}-\d{2}$/.test(details.expiresOn) || !Number.isFinite(expires.getTime())
    || expires.toISOString().slice(0, 10) !== details.expiresOn || !Number.isFinite(now.getTime())) {
    return { status: 'invalidExpiry' as const }
  }
  // Compare calendar dates in the viewer's timezone, without DST-length days.
  const today = Date.UTC(now.getFullYear(), now.getMonth(), now.getDate())
  const remainingDays = Math.max(0, Math.round((expires.getTime() - today) / dayMs))
  // Fixed-day estimate: complete years use 365 days, remaining months use 30.
  const cycleDays = Math.floor(price.cycleMonths / 12) * 365 + (price.cycleMonths % 12) * 30
  return {
    status: remainingDays ? 'active' as const : 'expired' as const,
    currency: price.currency, price: price.amount, monthlyCost: price.monthlyAmount,
    remainingDays, cycleDays,
    remaining: price.amount * Math.min(1, remainingDays / cycleDays),
  }
}

export function summarizeRemainingValue<T extends { id: string; name: string }>(
  hosts: readonly T[], details: Readonly<Record<string, ClusterHostDetails>>, now: Date,
) {
  const rows = hosts.map(host => ({ host, details: details[host.id], estimate: estimateRemainingValue(details[host.id], now) }))
  const groups = new Map<string, { currency: string; remaining: number; price: number; monthlyCost: number; count: number }>()
  let included = 0
  let expired = 0
  for (const { estimate } of rows) {
    if (estimate.remaining === undefined) continue
    included++
    if (estimate.status === 'expired') expired++
    const group = groups.get(estimate.currency) || { currency: estimate.currency, remaining: 0, price: 0, monthlyCost: 0, count: 0 }
    group.remaining += estimate.remaining
    group.price += estimate.price
    group.monthlyCost += estimate.monthlyCost
    group.count++
    groups.set(group.currency, group)
  }
  return { rows, included, expired, excluded: rows.length - included,
    groups: [...groups.values()].sort((a, b) => a.currency.localeCompare(b.currency, 'en')) }
}

export function formatClusterMoney(amount: number, currency: string, locale: string) {
  return new Intl.NumberFormat(locale, { style: 'currency', currency, minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(amount)
}
