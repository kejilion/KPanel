import { api } from '@/lib/api'
import { appAccessURL, matchingAppProxySites } from '@/lib/appAccess'
import { kpanelAppID, kpanelAppToken } from '@/lib/kpanelUpdate'
import type { Component } from 'vue'
import type { AppMarketItem, DesktopShortcut, PublicNetworkSummary, Site } from '@/types/api'

/**
 * Desktop dynamic entries: installed apps and configured sites surfaced as
 * desktop icons. Sites backed by an installed app, or entries that resolve to
 * the same URL, collapse to a single app icon (the app entry wins), keeping the
 * desktop uncluttered.
 */

export type DesktopEntryKind = 'app' | 'site' | 'shortcut'
export type DesktopEntryLaunch = 'external' | 'script' | 'file' | 'directory'

export interface DesktopEntry {
  /** Stable key: `${kind}:${id}` so Vue can key icons reliably. */
  key: string
  kind: DesktopEntryKind
  id: string
  name: string
  launch: DesktopEntryLaunch
  /** External entry URL opened when the icon is activated. */
  url?: string
  /** Internal Agent file-manager path for file and directory shortcuts. */
  path?: string
  /** Optional user-authored description for a custom shortcut. */
  description?: string
  /** App market icon URL (apps) or site favicon endpoint (sites). */
  iconURL?: string
  /** Built-in presentation icon used by file and directory shortcuts. */
  icon?: Component
  /** Source item for the detail dialog. */
  app?: AppMarketItem
  site?: Site
  shortcut?: DesktopShortcut
}

export interface DesktopEntries {
  apps: DesktopEntry[]
  sites: DesktopEntry[]
  /** All entries with app-backed sites and duplicate URLs removed (app wins). */
  visible: DesktopEntry[]
  /** Reused by the desktop clock to avoid a duplicate public-network call. */
  publicNetwork?: PublicNetworkSummary
  loadedAt: number
}

const DESKTOP_ENTRIES_CACHE_TTL = 5 * 60_000
const desktopEntriesCache = new Map<string, DesktopEntries>()

function cacheKey(directHost?: string): string {
  return directHost?.trim() || 'auto'
}

export function getCachedDesktopEntries(directHost?: string): DesktopEntries | undefined {
  const cached = desktopEntriesCache.get(cacheKey(directHost))
  if (!cached || Date.now() - cached.loadedAt > DESKTOP_ENTRIES_CACHE_TTL) return undefined
  return cached
}

export function clearDesktopEntriesCacheForTest(): void {
  desktopEntriesCache.clear()
}

function normalizeSiteURL(site: Site): string {
  const secure = site.certificate?.status === 'valid' || site.certificate?.status === 'expiring'
  return `${secure ? 'https' : 'http'}://${site.primaryDomain}`
}

function isConfiguredSite(site: Site): boolean {
  // Desktop entries represent configured websites, not only sites whose
  // certificate and runtime checks are fully healthy. Warning/unknown sites
  // must remain reachable from the desktop so users can inspect or repair them.
  return site.enabled && Boolean(site.primaryDomain.trim())
}

function appEntryName(item: AppMarketItem): string {
  return item.name_zh || item.name_en || item.id
}

function buildAppEntries(items: AppMarketItem[], sites: Site[], directHost: string): DesktopEntry[] {
  const entries: DesktopEntry[] = []
  for (const item of items) {
    if (!item.runtime.installed) continue
    const url = appAccessURL(item, sites, directHost)
    const scriptManage = Boolean(
      !url &&
      item.runtime.resourceVersion &&
      item.capabilities.manage?.enabled,
    )
    if (!url && !scriptManage) continue
    entries.push({
      key: `app:${item.id}`,
      kind: 'app',
      id: item.id,
      name: appEntryName(item),
      launch: scriptManage ? 'script' : 'external',
      url: url || undefined,
      iconURL: item.icon,
      app: item,
    })
  }
  return entries
}

function buildSiteEntries(sites: Site[]): DesktopEntry[] {
  return sites
    .filter(isConfiguredSite)
    .map((site) => ({
      key: `site:${site.id}`,
      kind: 'site' as const,
      id: site.id,
      name: site.primaryDomain,
      launch: 'external' as const,
      url: normalizeSiteURL(site),
      iconURL: api.sites.iconURL(site.id),
      site,
    }))
}

/**
 * Collapse sites that are known proxy entry points for an installed app, then
 * collapse entries that point at the same URL. Apps win in both cases.
 *
 * URL-only deduplication is insufficient when one app has several proxy
 * domains: `appAccessURL` picks one preferred domain, but every proxy targeting
 * that app's local public port represents the same app and should not produce
 * an additional desktop icon.
 */
function dedupeDesktopEntries(apps: DesktopEntry[], sites: DesktopEntry[]): DesktopEntry[] {
  const appBackedSiteIDs = new Set<string>()
  const sourceSites = sites.flatMap((entry) => entry.site ? [entry.site] : [])
  for (const entry of apps) {
    if (!entry.app) continue
    for (const site of matchingAppProxySites(entry.app, sourceSites)) {
      appBackedSiteIDs.add(site.id)
    }
  }

  const seen = new Set<string>()
  const visible: DesktopEntry[] = []
  for (const entry of [...apps, ...sites]) {
    if (entry.kind === 'site' && appBackedSiteIDs.has(entry.id)) continue
    const isKPanelSelf = entry.kind === 'app'
      && (entry.app?.id === kpanelAppID || entry.app?.token === kpanelAppToken)
    if (isKPanelSelf) {
      // KPanel is already represented by the desktop shell and taskbar. Keep
      // its URL reserved for deduplication so a matching site does not replace
      // the intentionally hidden self-entry.
      if (entry.url) seen.add(entry.url.replace(/\/+$/, '').toLowerCase())
      continue
    }
    if (!entry.url) {
      visible.push(entry)
      continue
    }
    const normalized = entry.url.replace(/\/+$/, '').toLowerCase()
    if (seen.has(normalized)) continue
    seen.add(normalized)
    visible.push(entry)
  }
  return visible
}

/**
 * Load installed apps and configured sites and compute the visible entry set.
 * Returns undefined when both sources fail, so the desktop can show an empty
 * state rather than stale data.
 *
 * `directHost` is the host used to build direct-access app URLs. Like the app
 * market view, the panel's public IP is preferred (so apps are reachable even
 * when the panel is accessed through a domain); tests pass a literal IP because
 * `appAccessURL` only emits direct URLs for IP hosts.
 */
export async function loadDesktopEntries(
  signal?: AbortSignal,
  directHost?: string,
  force = false,
): Promise<DesktopEntries> {
  if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')
  const key = cacheKey(directHost)
  const cached = getCachedDesktopEntries(directHost)
  if (!force && cached) return cached
  const previous = desktopEntriesCache.get(key)

  const publicNetwork = directHost
    ? undefined
    : await api.system.publicNetwork(signal).catch(() => previous?.publicNetwork)
  const host =
    directHost ||
    publicNetwork?.ipv4 ||
    publicNetwork?.ipv6 ||
    window.location.hostname

  const [inventory, sites] = await Promise.all([
    api.apps.inventory(signal).catch(() => undefined),
    api.sites.list(undefined, signal).catch(() => undefined),
  ])

  if (!inventory && !sites && previous) return previous
  const siteItems = sites?.items || previous?.sites.flatMap((entry) => entry.site ? [entry.site] : []) || []
  const apps = inventory
    ? buildAppEntries(inventory.items, siteItems, host)
    : previous?.apps || []
  const siteEntries = sites ? buildSiteEntries(sites.items) : previous?.sites || []
  const visible = dedupeDesktopEntries(apps, siteEntries)
  const result = { apps, sites: siteEntries, visible, publicNetwork, loadedAt: Date.now() }
  if (inventory || sites) desktopEntriesCache.set(key, result)
  return result
}
