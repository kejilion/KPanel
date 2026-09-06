import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiSecurityState } from './api'
import type { SystemOverview } from '@/types/api'

function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } })
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}
const observedAt = '2026-09-07T00:00:00Z'
const runtime = { hostname: 'fast-host', os: 'Debian', uptimeSeconds: 60, collectedAt: observedAt,
  cpu: { cores: 2, usagePercent: 12 }, load: { one: 0, five: 0, fifteen: 0 },
  memory: { totalBytes: 1024, availableBytes: 512, usedBytes: 512, usagePercent: 50 },
  disks: [], network: { receivedBytes: 1, sentBytes: 2 } }

afterEach(() => { vi.unstubAllGlobals(); resetApiSecurityState() })

describe('independent overview reads', () => {
  it('emits metrics before slow health/config/probes and preserves out-of-order state and metric identity', async () => {
    const config = deferred<Response>()
    const defense = deferred<Response>()
    const health = deferred<Response>()
    const fetch = vi.fn((url: string) => {
      if (url.endsWith('/runtime')) return Promise.resolve(response(runtime))
      if (url.endsWith('/management/config')) return config.promise
      if (url.endsWith('/management/ssh-defense')) return defense.promise
      if (url.endsWith('/management/bbrv3')) return Promise.resolve(response({ state: { available: true, installed: true }, observedAt }))
      if (url.endsWith('/health')) return health.promise
      return Promise.resolve(response({}, 503))
    })
    vi.stubGlobal('fetch', fetch)
    const updates: SystemOverview[] = []
    const pending = api.overview.get(undefined, (value) => updates.push(value))
    await vi.waitFor(() => expect(updates.at(-1)?.reads?.bbrv3?.state).toBe('ready'))
    expect(updates[0]?.hostname).toBe('fast-host')
    expect(updates[0]?.reads?.config?.state).toBe('loading')
    expect(updates.at(-1)?.reads?.['ssh-defense']?.state).toBe('loading')
    defense.resolve(response({ state: { available: true, enabled: true }, observedAt }))
    await vi.waitFor(() => expect(updates.at(-1)?.management.ssh.defense.enabled).toBe(true))
    config.resolve(response({ state: { dns: { servers: ['1.1.1.1'] }, ssh: { ports: [22] }, bbrv3: {} }, observedAt }))
    health.resolve(response({ status: 'ok', version: '1.6.0' }))
    const final = await pending
    expect(final.management.ssh.defense.enabled).toBe(true)
    expect(final.management.bbrv3.installed).toBe(true)
    expect(final.management.dns.servers).toEqual(['1.1.1.1'])
    expect(final.reads?.config?.observedAt).toBe(observedAt)
    expect(final.reads?.capabilities?.state).toBe('error')
    expect(new Set(updates.map((item) => item.cpu)).size).toBe(1)
    expect(new Set(updates.map((item) => item.network)).size).toBe(1)
    expect(fetch.mock.calls.some(([url]) => url.endsWith('/system/summary'))).toBe(false)
  })

  it('keeps configuration usable after runtime failure and never reports failed probes as disabled', async () => {
    vi.stubGlobal('fetch', vi.fn((url: string) => Promise.resolve(
      url.endsWith('/management/config')
        ? response({ state: { timezone: 'UTC' }, observedAt })
        : url.endsWith('/management/bbrv3')
          ? response({ state: { available: false, reason: 'probe failed' }, observedAt })
          : response({}, 503),
    )))
    const final = await api.overview.get()
    expect(final.reads?.runtime?.state).toBe('error')
    expect(final.reads?.config?.state).toBe('ready')
    expect(final.management.timezone).toBe('UTC')
    expect(final.reads?.bbrv3?.state).toBe('error')
    expect(final.reads?.['ssh-defense']?.state).toBe('error')
  })

  it('never emits responses arriving after cancellation', async () => {
    const delayed = deferred<Response>()
    vi.stubGlobal('fetch', vi.fn((url: string) => url.endsWith('/runtime')
      ? Promise.resolve(response(runtime)) : url.endsWith('/management/config')
        ? delayed.promise : Promise.resolve(response({}, 503))))
    const controller = new AbortController()
    const update = vi.fn()
    const pending = api.overview.get(controller.signal, update)
    await vi.waitFor(() => expect(update).toHaveBeenCalled())
    controller.abort()
    const count = update.mock.calls.length
    delayed.resolve(response({ state: { timezone: 'stale' }, observedAt }))
    await pending
    expect(update).toHaveBeenCalledTimes(count)
  })
})
