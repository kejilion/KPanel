import { apiRequest } from '@/lib/api'

export interface MCPClient {
  id: string
  name: string
  hosts: Array<{ id: string; identity: string }>
  createdAt: string
  expiresAt: string
}
export interface MCPSettings {
  access: { enabled: boolean; available: boolean; resourceVersion: string; clients: MCPClient[] }
  endpoint: string
  transportReady: boolean
  maxClients: number
  permission: 'inspect'
}
export const mcpAccess = {
  get: () => apiRequest<MCPSettings>('/settings/mcp'),
  enable: (enabled: boolean, expectedResourceVersion: string) => apiRequest<MCPSettings>('/settings/mcp', { method: 'PUT', body: { enabled, expectedResourceVersion } }),
  create: (input: { name: string; hostIds: string[]; expiresInDays: number; expectedResourceVersion: string }) => apiRequest<{ client: MCPClient; token: string; settings: MCPSettings }>('/settings/mcp/clients', { method: 'POST', body: input }),
  revoke: (id: string, expectedResourceVersion: string) => apiRequest<MCPSettings>(`/settings/mcp/clients/${encodeURIComponent(id)}`, { method: 'DELETE', body: { expectedResourceVersion } }),
  async test(token: string): Promise<void> {
    const response = await fetch('/mcp', {
      method: 'POST', credentials: 'omit', cache: 'no-store', signal: AbortSignal.timeout(12000),
      headers: { 'Content-Type': 'application/json', Accept: 'application/json, text/event-stream', Authorization: `Bearer ${token}`, 'MCP-Protocol-Version': '2025-11-25' },
      body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name: 'kpanel_info', arguments: {} } }),
    })
    if (!response.ok) throw new Error('mcp_connection_failed')
    const body = await response.json() as { result?: { isError?: boolean; structuredContent?: { permission?: string } } }
    if (body.result?.isError || body.result?.structuredContent?.permission !== 'inspect') throw new Error('mcp_connection_failed')
  },
}
