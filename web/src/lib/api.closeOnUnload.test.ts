// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './api'

afterEach(() => vi.unstubAllGlobals())

describe('api.terminals.closeOnUnload', () => {
  it('sends a keepalive close that the browser may finish after the page is gone', () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('{}'))
    vi.stubGlobal('fetch', fetchMock)
    api.terminals.closeOnUnload('abc/def')
    const [url, options] = fetchMock.mock.calls[0]! as [string, RequestInit]
    expect(url).toBe('/api/v1/terminal-sessions/abc%2Fdef/close')
    expect(options).toMatchObject({ method: 'POST', keepalive: true, credentials: 'same-origin', body: '{}' })
    expect((options.headers as Record<string, string>)['Content-Type']).toBe('application/json')
  })

  it('never throws, even when the request fails, because the page is already going away', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')))
    expect(() => api.terminals.closeOnUnload('abc')).not.toThrow()
    await Promise.resolve()
  })
})
