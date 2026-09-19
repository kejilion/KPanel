import { describe, expect, it } from 'vitest'
import { mcpClientConfig } from './mcp-clients'

describe('client configuration compatibility', () => {
  it('uses the client-specific root and required transport discriminator', () => {
    const endpoint = 'https://panel.example/mcp'
    const claude = JSON.parse(mcpClientConfig('claude-code', endpoint, 'secret'))
    expect(claude.mcpServers.kpanel).toEqual({ type: 'http', url: endpoint, headers: { Authorization: 'Bearer secret' } })
    const vscode = JSON.parse(mcpClientConfig('vscode', endpoint, 'secret'))
    expect(vscode.mcpServers).toBeUndefined()
    expect(vscode.servers.kpanel.type).toBe('http')
    expect(JSON.parse(mcpClientConfig('windsurf', endpoint, 'secret')).mcpServers.kpanel.serverUrl).toBe(endpoint)
    const bridge = JSON.parse(mcpClientConfig('stdio', endpoint, 'secret')).mcpServers.kpanel
    expect(bridge).toEqual({ command: 'kpanel-mcp', args: ['--url', endpoint], env: { KPANEL_MCP_TOKEN: 'secret' } })
    expect(mcpClientConfig('codex', endpoint, 'secret')).toBe('[mcp_servers.kpanel]\nurl = "https://panel.example/mcp"\nhttp_headers = { Authorization = "Bearer secret" }\n')
  })
  it('refuses plaintext remote connections and credentials embedded in an endpoint', () => {
    for (const endpoint of ['http://panel.example/mcp', 'https://user:password@panel.example/mcp', 'https://panel.example/mcp?token=secret', 'https://panel.example/api/terminal']) {
      expect(() => mcpClientConfig('generic', endpoint, 'secret')).toThrow('invalid_mcp_connection')
    }
    expect(() => mcpClientConfig('cursor', 'http://127.0.0.1:8080/mcp', 'secret')).not.toThrow()
    expect(() => mcpClientConfig('cursor', 'https://panel.example/mcp', 'secret\nheader')).toThrow()
  })
})
