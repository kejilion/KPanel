// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

describe('window identity snapshots', () => {
  beforeEach(() => { window.sessionStorage.clear(); vi.resetModules() })
  afterEach(() => { Reflect.deleteProperty(navigator, 'locks'); vi.unstubAllGlobals(); vi.restoreAllMocks() })

  it('does not restore copied sessionStorage when an opener still owns the tab lease', async () => {
    const held = new Set<string>()
    const request = (name: string, _options: unknown, callback: (lock: Lock | null) => unknown) => {
      if (held.has(name)) return Promise.resolve(callback(null))
      held.add(name)
      return Promise.resolve(callback({ name, mode: 'exclusive' } as Lock)).finally(() => held.delete(name))
    }
    Object.defineProperty(navigator, 'locks', { configurable: true, value: { request } })
    const original = await import('./windowSessionIdentities')
    const firstKey = await original.windowIdentityKey('terminal', 'owner', '1')
    original.writeWindowIdentities(firstKey, ['a'.repeat(64)])
    window.dispatchEvent(new PageTransitionEvent('pagehide', { persisted: true }))
    window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true }))

    // A second module instance models a new document with copied storage and
    // an independent in-memory lease, while the first document is still live.
    vi.resetModules()
    const duplicate = await import('./windowSessionIdentities')
    const secondKey = await duplicate.windowIdentityKey('terminal', 'owner', '1')
    expect(secondKey).not.toBe(firstKey)
    expect(duplicate.readWindowIdentities(secondKey, /^[a-f0-9]{64}$/, 16)).toEqual([])
    expect(original.readWindowIdentities(firstKey, /^[a-f0-9]{64}$/, 16)).toEqual(['a'.repeat(64)])
    original.resetWindowIdentityForTest()
    duplicate.resetWindowIdentityForTest()
  })

  it('does not use a potentially copied snapshot when tab leases are unavailable', async () => {
    Reflect.deleteProperty(navigator, 'locks')
    vi.stubGlobal('SharedWorker', undefined)
    const identities = await import('./windowSessionIdentities')
    expect(await identities.windowIdentityKey('terminal', 'owner', '1')).toBe('')
    identities.resetWindowIdentityForTest()
  })

  it('uses a shared-worker lease to isolate copied tabs without Web Locks', async () => {
    Reflect.deleteProperty(navigator, 'locks')
    const owners = new Map<string, object>()
    class FakeSharedWorker {
      onerror: (() => void) | null = null
      port: { onmessage: ((event: { data: { status: string } }) => void) | null; postMessage: (value: { op: string; token: string }) => void; start: () => void; close: () => void }
      constructor() {
        const identity = {}
        const port = {
          onmessage: null as ((event: { data: { status: string } }) => void) | null,
          postMessage: (value: { op: string; token: string }) => {
            if (value.op === 'release') { if (owners.get(value.token) === identity) owners.delete(value.token); return }
            const owner = owners.get(value.token)
            if (!owner) owners.set(value.token, identity)
            queueMicrotask(() => port.onmessage?.({ data: { status: owner && owner !== identity ? 'busy' : 'acquired' } }))
          },
          start: () => undefined,
          close: () => undefined,
        }
        this.port = port
      }
    }
    vi.stubGlobal('SharedWorker', FakeSharedWorker)
    const original = await import('./windowSessionIdentities')
    const firstKey = await original.windowIdentityKey('app', 'owner', '1', 'app')
    original.writeWindowIdentities(firstKey, ['job-a'])
    vi.resetModules()
    const duplicate = await import('./windowSessionIdentities')
    const secondKey = await duplicate.windowIdentityKey('app', 'owner', '1', 'app')
    expect(secondKey).not.toBe(firstKey)
    expect(duplicate.readWindowIdentities(secondKey, /^[a-z-]+$/, 16)).toEqual([])
    expect(original.readWindowIdentities(firstKey, /^[a-z-]+$/, 16)).toEqual(['job-a'])
    original.resetWindowIdentityForTest()
    duplicate.resetWindowIdentityForTest()
  })
})
