import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiSecurityState } from './api'

afterEach(() => { vi.unstubAllGlobals(); resetApiSecurityState() })

describe('Passkey API authentication', () => {
  it('uses the login response CSRF token for subsequent authenticated writes', async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ user: { id: 'admin', username: 'admin' }, csrfToken: 'passkey-csrf' }), { headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ reauthenticate: true }), { headers: { 'content-type': 'application/json' } }))
    vi.stubGlobal('fetch', fetch)
    const credential = { id: 'AQ', rawId: 'AQ', type: 'public-key', clientExtensionResults: {}, response: { clientDataJSON: 'Ag', signature: 'Aw' } }
    const status = await api.auth.passkeys.loginFinish({ ceremonyId: 'ceremony', credential, totpCode: '123456' })
    expect(status).toMatchObject({ authenticated: true, setupRequired: false, user: { username: 'admin' } })
    await api.auth.passkeys.delete({ id: 'AQ', password: 'password', totpCode: '123456' })
    expect(fetch.mock.calls[0]![0]).toBe('/api/v1/auth/passkeys/login/finish')
    expect(JSON.parse(fetch.mock.calls[0]![1].body)).toEqual({ ceremonyId: 'ceremony', credential, totpCode: '123456' })
    expect(fetch.mock.calls[1]![1].headers.get('X-CSRF-Token')).toBe('passkey-csrf')
    expect(fetch.mock.calls[1]![1].credentials).toBe('same-origin')
  })
})
