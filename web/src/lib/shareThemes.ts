import type { PublicClusterShareSnapshot } from '@/types/api'
import type { ScenePackList, LocalizedText } from './scenePacks'
import { clusterTrafficCounters, formatNetworkTrafficCounter, monthlyTrafficUsage, clusterTrafficHint } from './networkTraffic'
import type { useI18n } from '@/i18n'
import { formatClusterMoney, estimateRemainingValue, summarizeRemainingValue } from './clusterRemainingValue'
import { formatPercent, formatDuration, formatBytes } from './format'
import { shareThemeLabels } from './shareThemeLabels'

export interface ShareTheme { id: string; name: LocalizedText; fileBase: string }
export interface ShareThemeList extends ScenePackList { selected?: string; resourceVersion: string }

export function shareThemeURL(theme?: ShareTheme): string | undefined {
  if (!theme || !/^[a-z0-9][a-z0-9-]{0,39}$/.test(theme.id)) return
  const prefix = `/api/v1/cluster/share-themes/${theme.id}/files/`
  if (!theme.fileBase.startsWith(prefix) || !/^[a-f0-9]{32}\/$/.test(theme.fileBase.slice(prefix.length))) return
  return `${theme.fileBase}index.html`
}

// Explicit display DTO: never forward a session, share URL/token, arbitrary API
// additions, or management objects to a community package.
export function shareThemeModel(snapshot: PublicClusterShareSnapshot, locale: string, now = new Date(), t?: ReturnType<typeof useI18n>['t']) {
  const details = Object.fromEntries(snapshot.items.map(host => [host.id, host]))
  const summary = summarizeRemainingValue(snapshot.items, details, now)
  return {
    title: snapshot.title, description: snapshot.description || '', generatedAt: snapshot.generatedAt,
    total: snapshot.total, online: snapshot.online, attention: snapshot.attention,
    value: { included: summary.included, excluded: summary.excluded,
      groups: summary.groups.map(group => ({ currency: group.currency, text: formatClusterMoney(group.remaining, group.currency, locale) })) },
    hosts: snapshot.items.map(host => {
      const usage = monthlyTrafficUsage(host.trafficPeriod, host)
      const counters = clusterTrafficCounters(host)
      const estimate = estimateRemainingValue(host, now)
      return {
        id: host.id, name: host.name, state: host.state, os: host.os || '',
        location: [host.location.country, host.location.city].filter(Boolean).join(' · '),
        cpu: host.collectedAt ? formatPercent(host.cpu.usagePercent) : '—',
        memory: host.collectedAt ? formatPercent(host.memory.usagePercent) : '—',
        disk: host.collectedAt ? formatPercent(host.disk.usagePercent) : '—',
        uptime: host.collectedAt ? formatDuration(host.uptimeSeconds || 0) : '—',
        traffic: { monthly: Boolean(host.trafficPeriod || host.trafficResetDay), percent: usage?.text || '', tone: usage?.tone || 'normal',
          hint: t ? clusterTrafficHint(host.trafficPeriod, host, t) || '' : '',
          available: host.trafficPeriod?.available ?? Boolean(host.collectedAt), partial: host.trafficPeriod?.partial || false,
          estimated: host.trafficPeriod?.estimated || false,
          received: host.collectedAt ? formatNetworkTrafficCounter(counters, 'received') : '—',
          sent: host.collectedAt ? formatNetworkTrafficCounter(counters, 'sent') : '—' },
        price: host.price || '', expiresOn: host.expiresOn || '',
        remaining: estimate.remaining === undefined ? '' : formatClusterMoney(estimate.remaining, estimate.currency, locale),
      }
    }),
  }
}

/** Highest protocol a package declares in its `ready` message; anything unrecognised is the legacy protocol 1. */
export function shareThemeProtocol(value: unknown): 1 | 2 { return value === 2 ? 2 : 1 }

const ratio = (percent: number | undefined, known: boolean) => known && Number.isFinite(percent) ? Math.min(1, Math.max(0, (percent as number) / 100)) : null
const finite = (value: number | undefined, known: boolean) => known && Number.isFinite(value) ? value as number : null

/**
 * Protocol 2 snapshot. Same whitelist as protocol 1, but every measurement also
 * travels as a raw number (`ratio` is 0..1, `null` means unknown) beside its
 * formatted text, so a theme may draw gauges, bars, heat maps or plain prose
 * however it likes. Themes get vocabulary through `labels` and a stable list of
 * `states`, and never need to parse formatted strings.
 */
export type RegionCenters = Readonly<Record<string, readonly [number, number]>>

/** Country anchors are lazy-loaded by the caller so the public page never pays for them without a protocol 2 theme. */
export function shareThemeModelV2(snapshot: PublicClusterShareSnapshot, locale: string, now = new Date(), t?: ReturnType<typeof useI18n>['t'], regionCenters: RegionCenters = {}) {
  const v1 = shareThemeModel(snapshot, locale, now, t)
  const summary = summarizeRemainingValue(snapshot.items, Object.fromEntries(snapshot.items.map(host => [host.id, host])), now)
  const labels = shareThemeLabels(locale)
  return {
    ...v1,
    counts: { total: snapshot.total, online: snapshot.online, attention: snapshot.attention, offline: snapshot.items.filter(host => host.state === 'offline').length },
    states: { online: labels.online, degraded: labels.degraded, offline: labels.offline, pending: labels.pending },
    value: { ...v1.value, groups: v1.value.groups.map((group, index) => ({ ...group, amount: summary.groups[index]?.remaining ?? null })) },
    hosts: v1.hosts.map((host, index) => {
      const raw = snapshot.items[index]!, known = Boolean(raw.collectedAt), countryCode = (raw.location.countryCode || '').trim().toUpperCase()
      return {
        ...host,
        stateLabel: labels[raw.state] || labels.pending,
        architecture: raw.architecture || '',
        location: { text: host.location, country: raw.location.country || '', countryCode, city: raw.location.city || '', region: raw.location.region || '', isp: raw.location.isp || '',
          // Country-level anchor shared with the built-in globe; never a measured host position.
          latitude: regionCenters[countryCode]?.[0] ?? null, longitude: regionCenters[countryCode]?.[1] ?? null },
        cores: known ? raw.cpu.cores : null,
        collected: known,
        cpu: { text: host.cpu, ratio: ratio(raw.cpu.usagePercent, known) },
        memory: { text: host.memory, ratio: ratio(raw.memory.usagePercent, known), usedBytes: finite(raw.memory.usedBytes, known), totalBytes: finite(raw.memory.totalBytes, known),
          usedText: known ? formatBytes(raw.memory.usedBytes) : '—', totalText: known ? formatBytes(raw.memory.totalBytes) : '—' },
        disk: { text: host.disk, ratio: ratio(raw.disk.usagePercent, known), usedBytes: finite(raw.disk.usedBytes, known), totalBytes: finite(raw.disk.totalBytes, known),
          usedText: known ? formatBytes(raw.disk.usedBytes) : '—', totalText: known ? formatBytes(raw.disk.totalBytes) : '—' },
        load: known ? { one: raw.load.one, five: raw.load.five, fifteen: raw.load.fifteen } : null,
        uptimeSeconds: finite(raw.uptimeSeconds, known),
        network: { down: { bytesPerSecond: finite(raw.network.receiveBytesPerSecond, known), text: known ? `${formatBytes(raw.network.receiveBytesPerSecond)}/s` : '—' },
          up: { bytesPerSecond: finite(raw.network.transmitBytesPerSecond, known), text: known ? `${formatBytes(raw.network.transmitBytesPerSecond)}/s` : '—' } },
        traffic: { ...host.traffic, ratio: ratio(monthlyTrafficUsage(raw.trafficPeriod, raw)?.percent, true),
          quotaGiB: raw.trafficMonthlyQuotaGiB || null },
      }
    }),
  }
}
