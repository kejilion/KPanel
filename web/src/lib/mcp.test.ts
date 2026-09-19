import { afterEach, describe, expect, it, vi } from 'vitest'
import { mcpAccess } from './mcp'

afterEach(() => vi.unstubAllGlobals())
describe('MCP connection test', () => {
  it('uses only the fixed endpoint and bearer header, excludes session cookies and validates tool success', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ result: { structuredContent: { permission: 'inspect' } } }) })
    vi.stubGlobal('fetch', fetchMock)
    await mcpAccess.test('credential')
    expect(fetchMock).toHaveBeenCalledWith('/mcp', expect.objectContaining({ credentials: 'omit', cache: 'no-store', headers: expect.objectContaining({ Authorization: 'Bearer credential' }) }))
    fetchMock.mockResolvedValue({ ok: true, json: async () => ({ result: { isError: true } }) })
    await expect(mcpAccess.test('credential')).rejects.toThrow('mcp_connection_failed')
    fetchMock.mockResolvedValue({ ok: false })
    await expect(mcpAccess.test('credential')).rejects.toThrow('mcp_connection_failed')
  })
})
