// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

describe('window identity snapshots', () => {
  beforeEach(() => { window.sessionStorage.clear(); vi.resetModules() })
  afterEach(() => { Reflect.deleteProperty(navigator, 'locks'); vi.restoreAllMocks() })

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
    const identities = await import('./windowSessionIdentities')
    expect(await identities.windowIdentityKey('terminal', 'owner', '1')).toBe('')
    identities.resetWindowIdentityForTest()
  })
})
