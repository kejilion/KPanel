import { apiRequest } from '@/lib/api'

export interface MCPClient {
  id: string
  name: string
  hosts: Array<{ id: string; identity: string }>
  createdAt: string
  expiresAt: string
  policy?: MCPPolicy
}
export interface MCPPolicy { domains: string[]; operations: string[]; operationVersions: Record<string, string>; write?: boolean; autoApprove?: boolean; fileRoots?: string[] }
export const mcpDomains = [{ id: 'system', name: '系统与日志' }, { id: 'docker', name: 'Docker 与 Compose' }, { id: 'sites', name: '网站与 Web 环境' }, { id: 'apps', name: '应用' }, { id: 'diagnostics', name: '诊断' }, { id: 'backups', name: '备份与恢复' }, { id: 'files', name: '文件' }]
export interface MCPClusterGrants {
  grants: { available: boolean; resourceVersion: string; items: Array<{ revision: string; controllerId: string; policy: { operationVersions: Record<string, string>; write: boolean; fileRoots?: string[] }; createdAt: string; expiresAt: string }> }
  controllers: Array<{ id: string; name: string; fingerprint: string }>
}
export interface MCPOAuthRequest { id: string; clientName: string; redirectUri: string; expiresAt: string }
export interface MCPOperation {
  operationId: string; digest: string; hostId: string; clientId?: string; tool: string
  state: 'pending' | 'approved' | 'rejected' | 'executing' | 'submitted' | 'succeeded' | 'failed' | 'unknown' | 'expired'
  createdAt: string; expiresAt: string; updatedAt: string; arguments?: Record<string, unknown>; result?: unknown; error?: string
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
  create: (input: { name: string; hostIds: string[]; expiresInDays: number; expectedResourceVersion: string; domains?: string[]; write?: boolean; autoApprove?: boolean; fileRoots?: string[] }) => apiRequest<{ client: MCPClient; token: string; settings: MCPSettings }>('/settings/mcp/clients', { method: 'POST', body: input }),
  revoke: (id: string, expectedResourceVersion: string) => apiRequest<MCPSettings>(`/settings/mcp/clients/${encodeURIComponent(id)}`, { method: 'DELETE', body: { expectedResourceVersion } }),
  operations: () => apiRequest<{ items: MCPOperation[] }>('/settings/mcp/operations'),
  operation: (id: string) => apiRequest<MCPOperation>(`/settings/mcp/operations/${encodeURIComponent(id)}`),
  oauthRequest: (id: string) => apiRequest<MCPOAuthRequest>(`/settings/mcp/oauth/${encodeURIComponent(id)}`),
  oauthConsent: (id: string, input: { approve: boolean; hostIds: string[]; domains: string[]; write: boolean; fileRoots: string[]; expiresInDays: number; expectedResourceVersion: string }) => apiRequest<{ redirectUrl: string }>(`/settings/mcp/oauth/${encodeURIComponent(id)}`, { method: 'POST', body: input }),
  clusterGrants: () => apiRequest<MCPClusterGrants>('/settings/mcp/cluster-grants'),
  grantCluster: (input: { controllerId: string; expectedResourceVersion: string; domains: string[]; write: boolean; fileRoots: string[]; expiresInDays: number }) => apiRequest<MCPClusterGrants>('/settings/mcp/cluster-grants', { method: 'POST', body: input }),
  revokeCluster: (controllerId: string, expectedResourceVersion: string) => apiRequest<MCPClusterGrants>('/settings/mcp/cluster-grants', { method: 'DELETE', body: { controllerId, expectedResourceVersion } }),
  decide: (operationId: string, digest: string, approve: boolean) => apiRequest<MCPOperation>(`/settings/mcp/operations/${encodeURIComponent(operationId)}`, { method: 'POST', body: { digest, approve } }),
  async test(token: string): Promise<void> {
    const response = await fetch('/mcp', {
      method: 'POST', credentials: 'omit', cache: 'no-store', signal: AbortSignal.timeout(12000),
      headers: { 'Content-Type': 'application/json', Accept: 'application/json, text/event-stream', Authorization: `Bearer ${token}`, 'MCP-Protocol-Version': '2025-11-25' },
      body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name: 'kpanel_info', arguments: {} } }),
    })
    if (!response.ok) throw new Error('mcp_connection_failed')
    const body = await response.json() as { result?: { isError?: boolean; structuredContent?: { permission?: string } } }
    if (body.result?.isError || !['inspect', 'managed'].includes(body.result?.structuredContent?.permission || '')) throw new Error('mcp_connection_failed')
  },
}
