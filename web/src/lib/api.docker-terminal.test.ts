import { afterEach, expect, it, vi } from 'vitest'
import { api, resetApiSecurityState } from './api'

afterEach(() => { vi.unstubAllGlobals(); resetApiSecurityState() })

it('sends a structured container target and leaves existing host opens unchanged', async () => {
  const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(async () => new Response(JSON.stringify({ sessionId: 'terminal', offset: 0 }), { headers: { 'content-type': 'application/json' } }))
  vi.stubGlobal('fetch', fetch)
  await api.terminals.open('local', 30, 120, { containerId: 'a'.repeat(64), resourceVersion: 'version-a' })
  expect(fetch.mock.calls[0]![0]).toBe('/api/v1/terminal-sessions')
  expect(JSON.parse(fetch.mock.calls[0]![1]!.body as string)).toEqual({ hostId: 'local', rows: 30, columns: 120, containerId: 'a'.repeat(64), resourceVersion: 'version-a' })
  await api.terminals.open('edge', 24, 80)
  expect(JSON.parse(fetch.mock.calls[1]![1]!.body as string)).toEqual({ hostId: 'edge', rows: 24, columns: 80 })
})
