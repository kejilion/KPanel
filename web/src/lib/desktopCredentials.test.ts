import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiSecurityState } from './api'

function json(body: unknown) {
  return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } })
}
afterEach(() => { vi.unstubAllGlobals(); resetApiSecurityState() })

describe('desktop credential API contract', () => {
  it('uses CSRF-protected same-origin POSTs with no-store and keeps secrets out of URLs', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(json({ required: false }))
      .mockResolvedValueOnce(json({ user: { id: 'owner' }, csrfToken: 'test-csrf' }))
      .mockImplementation(async () => json({ saved: true }))
    vi.stubGlobal('fetch', fetchMock)
    await api.auth.status()
    const controller = new AbortController()
    await api.desktops.credentialStatus('windows-a', controller.signal)
    await api.desktops.saveCredentials('windows-a', { username: 'alice', domain: 'EXAMPLE', password: 'test-only-password' })
    await api.desktops.open('windows-a', true)
    await api.desktops.open('windows-a')
    await api.desktops.clearCredentials('windows-a')
    const calls = fetchMock.mock.calls.slice(2) as [string, RequestInit][]
    expect(calls.map(([url]) => url)).toEqual([
      '/api/v1/desktop-sessions/credentials/status', '/api/v1/desktop-sessions/credentials/save',
      '/api/v1/desktop-sessions', '/api/v1/desktop-sessions', '/api/v1/desktop-sessions/credentials/clear',
    ])
    for (const [url, init] of calls) {
      expect(init).toMatchObject({ method: 'POST', credentials: 'same-origin', cache: 'no-store' })
      expect(new Headers(init.headers).get('X-CSRF-Token')).toBe('test-csrf')
      expect(url).not.toContain('test-only-password')
    }
    expect(calls[0]![1].signal).toBe(controller.signal)
    expect(JSON.parse(String(calls[1]![1].body))).toEqual({ hostId: 'windows-a', username: 'alice', domain: 'EXAMPLE', password: 'test-only-password' })
    expect(JSON.parse(String(calls[2]![1].body))).toEqual({ hostId: 'windows-a', useSavedCredentials: true })
    expect(JSON.parse(String(calls[3]![1].body))).toEqual({ hostId: 'windows-a', useSavedCredentials: false })
    expect(JSON.parse(String(calls[4]![1].body))).toEqual({ hostId: 'windows-a' })
  })
})
