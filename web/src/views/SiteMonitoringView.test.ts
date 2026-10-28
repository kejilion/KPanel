import type { Ref } from 'vue'
import { createSSRApp, ssrContextKey, nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import SiteMonitoringView from './SiteMonitoringView.vue'
import { api } from '@/lib/api'
import type { SiteTrafficSnapshot } from '@/types/siteTraffic'
import type { Site, WebEnvironmentSummary } from '@/types/api'
vi.mock('@/lib/api', () => ({ api: { siteTraffic: { summary: vi.fn(), action: vi.fn() }, webEnvironment: { summary: vi.fn() }, sites: { list: vi.fn() } } }))
interface Bindings { data: Ref<SiteTrafficSnapshot | undefined>; protectionStatus: Ref<WebEnvironmentSummary | undefined>; protectionStatusError: Ref<string>; protectionStatusLoading: Ref<boolean>; fail2banLogEntries: Readonly<Ref<string[]>>; view: Ref<'traffic' | 'security'>; schedule: () => void; load: () => Promise<void>; loadProtectionStatus: () => Promise<void>; error: Ref<string>; loading: Ref<boolean>; host: Ref<string>; busy: Ref<boolean>; loadSiteIcons: () => Promise<void>; siteID: (host: string) => string | undefined; perform: (action: 'enable' | 'pause' | 'clear') => Promise<void> }
function setup(): Bindings {
 const app = createSSRApp({ render: () => null }); app.provide(ssrContextKey, { modules: new Set<string>() })
 const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
 try { return app.runWithContext(() => (SiteMonitoringView as unknown as { setup: (props: object, ctx: object) => Bindings }).setup({}, { expose: () => undefined })) } finally { warn.mockRestore() }
}
function snapshot(requests = 1): SiteTrafficSnapshot { return { requests, enabled: true } as SiteTrafficSnapshot }
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(r => { resolve = r }); return { promise, resolve } }
afterEach(() => { vi.clearAllTimers(); vi.unstubAllGlobals(); vi.useRealTimers(); vi.resetAllMocks() })
describe('website monitoring request lifecycle', () => {
 it('resolves primary and alias hosts to managed icons without treating external referrers as site IDs', async () => {
  vi.mocked(api.sites.list).mockResolvedValue({ total: 1, items: [{ id: 'managed-site', primaryDomain: 'example.com', domains: ['www.example.com'] } as Site] })
  const view = setup(); await view.loadSiteIcons()
  expect(view.siteID('EXAMPLE.COM.')).toBe('managed-site')
  expect(view.siteID('www.example.com')).toBe('managed-site')
  expect(view.siteID('external.example')).toBeUndefined()
 })
 it('keeps monitoring usable with fallback icons when optional site metadata is unavailable', async () => {
  vi.mocked(api.sites.list).mockRejectedValue(new Error('sites unavailable'))
  vi.mocked(api.siteTraffic.summary).mockResolvedValue(snapshot(9))
  const view = setup(); await Promise.all([view.load(), view.loadSiteIcons()])
  expect(view.data.value?.requests).toBe(9); expect(view.error.value).toBe('')
  expect(view.siteID('example.com')).toBeUndefined()
 })
 it('loads managed defense state separately from traffic and shows site-only Fail2Ban logs', async () => {
  const summary = {
   protection: {
    fail2ban: true,
    fail2banInstalled: true,
    cloudflareStatus: { mode: 'cloudflare', configured: true, running: true, healthy: true, pending: false, cleanupPending: false, activeBans: 2 },
    siteJails: [{ name: 'waf-attack', available: true, currentlyBanned: 1, totalBanned: 5 }],
    fail2banLogs: { available: true, entries: ['[waf-attack] Ban 192.0.2.1'] },
   },
  } as WebEnvironmentSummary
  vi.mocked(api.siteTraffic.summary).mockResolvedValue(snapshot(3))
  vi.mocked(api.webEnvironment.summary).mockResolvedValue(summary)
  const view = setup()

  await view.load()
  expect(api.webEnvironment.summary).not.toHaveBeenCalled()
  expect(view.data.value?.requests).toBe(3)

  await view.loadProtectionStatus()
  expect(api.webEnvironment.summary).toHaveBeenCalledTimes(1)
  expect(view.protectionStatus.value?.protection.siteJails?.[0]?.currentlyBanned).toBe(1)
  expect(view.fail2banLogEntries.value).toEqual(['[waf-attack] Ban 192.0.2.1'])
  expect(view.protectionStatusError.value).toBe('')
 })
 it('does not abort a slow defense status request when the refresh interval elapses', async () => {
  vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-28T00:00:00Z'))
  vi.stubGlobal('document', { visibilityState: 'visible' })
  const pending = deferred<WebEnvironmentSummary>()
  vi.mocked(api.webEnvironment.summary).mockReturnValue(pending.promise)
  const view = setup(); view.view.value = 'security'; await nextTick()
  expect(api.webEnvironment.summary).toHaveBeenCalledTimes(1)
  view.schedule()
  await vi.advanceTimersByTimeAsync(30000)
  expect(api.webEnvironment.summary).toHaveBeenCalledTimes(1)
  pending.resolve({ protection: {} } as WebEnvironmentSummary)
  await nextTick()
 })
 it('discards an older response after a newer request has completed', async () => {
  const old = deferred<SiteTrafficSnapshot>(), latest = deferred<SiteTrafficSnapshot>()
  vi.mocked(api.siteTraffic.summary).mockReturnValueOnce(old.promise).mockReturnValueOnce(latest.promise)
  const view = setup(); const first = view.load(), second = view.load()
  latest.resolve(snapshot(22)); await second; old.resolve(snapshot(11)); await first
  expect(view.data.value?.requests).toBe(22); expect(view.loading.value).toBe(false)
 })
 it('keeps same-filter history available when a refresh fails', async () => {
  vi.mocked(api.siteTraffic.summary).mockResolvedValueOnce(snapshot(4)).mockRejectedValueOnce(new Error('source unavailable'))
  const view = setup(); await view.load(); await view.load()
  expect(view.data.value?.requests).toBe(4); expect(view.error.value).toBe('source unavailable')
 })
 it('clears old site data before loading a new filter and does not show it after failure', async () => {
  vi.mocked(api.siteTraffic.summary).mockResolvedValueOnce(snapshot(9)).mockRejectedValueOnce(new Error('new site unavailable'))
  const view = setup(); await view.load(); view.host.value = 'other.example'; await nextTick(); await nextTick()
  expect(view.data.value).toBeUndefined(); expect(view.error.value).toBe('new site unavailable')
 })
 it('serializes controls while an action is pending and reloads the result', async () => {
  const action = deferred<{ ok: boolean }>(); vi.mocked(api.siteTraffic.action).mockReturnValue(action.promise)
  vi.mocked(api.siteTraffic.summary).mockResolvedValue(snapshot(0))
  const view = setup(); const pending = view.perform('clear'); await view.perform('enable')
  expect(api.siteTraffic.action).toHaveBeenCalledTimes(1); expect(view.busy.value).toBe(true)
  action.resolve({ ok: true }); await pending
  expect(view.data.value?.requests).toBe(0); expect(view.busy.value).toBe(false)
 })
})
