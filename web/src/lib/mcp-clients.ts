// Client formats are deliberately separate: Claude Code requires a transport
// type, VS Code uses `servers`, and Codex reads TOML rather than mcpServers JSON.
export const mcpClients = [
  { id: 'cursor', name: 'Cursor', location: '~/.cursor/mcp.json' },
  { id: 'claude-code', name: 'Claude Code', location: '.mcp.json（仅限私人目录）' },
  { id: 'codex', name: 'Codex', location: '~/.codex/config.toml' },
  { id: 'vscode', name: 'VS Code / GitHub Copilot', location: 'MCP: Open User Configuration' },
  { id: 'windsurf', name: 'Windsurf / Cascade', location: '~/.codeium/windsurf/mcp_config.json' },
  { id: 'stdio', name: 'stdio / Claude Desktop', location: 'mcpServers' },
  { id: 'generic', name: '通用 HTTP 客户端', location: 'mcpServers' },
] as const
export type MCPClientKind = typeof mcpClients[number]['id']

export function mcpClientConfig(client: MCPClientKind, endpoint: string, token: string): string {
  const url = new URL(endpoint)
  const loopback = ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)
  if ((url.protocol !== 'https:' && !(url.protocol === 'http:' && loopback)) || url.pathname !== '/mcp' || url.username || url.password || url.search || url.hash || !token || /[\r\n]/.test(token)) throw new Error('invalid_mcp_connection')
  if (client === 'stdio') return JSON.stringify({ mcpServers: { kpanel: { command: 'kpanel-mcp', args: ['--url', endpoint], env: { KPANEL_MCP_TOKEN: token } } } }, null, 2)
  const headers = { Authorization: `Bearer ${token}` }
  if (client === 'codex') {
    // JSON basic strings are also valid TOML basic strings for these values.
    return `[mcp_servers.kpanel]\nurl = ${JSON.stringify(endpoint)}\nhttp_headers = { Authorization = ${JSON.stringify(headers.Authorization)} }\n`
  }
  if (client === 'vscode') return JSON.stringify({ servers: { kpanel: { type: 'http', url: endpoint, headers } } }, null, 2)
  if (client === 'claude-code') return JSON.stringify({ mcpServers: { kpanel: { type: 'http', url: endpoint, headers } } }, null, 2)
  if (client === 'windsurf') return JSON.stringify({ mcpServers: { kpanel: { serverUrl: endpoint, headers } } }, null, 2)
  return JSON.stringify({ mcpServers: { kpanel: { url: endpoint, headers } } }, null, 2)
}
