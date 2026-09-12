import { apiRequest } from '@/lib/api'

export type BackupModule = 'panel' | 'apps' | 'web' | 'docker'
export interface BackupRecord {
  id: string
  action: 'export' | 'import' | 'restore' | 'recover'
  status: string
  stage: string
  modules: BackupModule[]
  createdAt: string
  size: number
  targetRevision?: string
  errorCode?: string
  completedModules?: BackupModule[]
  manifest?: { version: string; createdAt: string; parts: Array<{ module: BackupModule; size: number }> }
}
export interface BackupInventory {
  revision: string
  panelBytes: number
  maxBytes: number
  hostAvailable: boolean
  host?: { revision: string; modules: Array<{ id: BackupModule; bytes: number; containers: number; requires: BackupModule[]; issue?: string }> }
}
export const backups = {
  list: () => apiRequest<{ items: BackupRecord[]; maxBytes: number }>('/backups'),
  preview: (id: string) => apiRequest<BackupRecord>(`/backups/${id}/preview`, { method: 'POST', body: {} }),
  inventory: () => apiRequest<BackupInventory>('/backups/inventory'),
  export: (modules: BackupModule[], password: string, agentRevision?: string) => apiRequest<BackupRecord>('/backups/export', { method: 'POST', body: { modules, password, agentRevision: agentRevision || '' } }),
  import: (file: File, password: string) => {
    const body = new FormData()
    body.append('password', password)
    body.append('file', file)
    return apiRequest<BackupRecord>('/backups/import', { method: 'POST', body })
  },
  restore: (source: BackupRecord, modules: BackupModule[]) => apiRequest<BackupRecord>(`/backups/${source.id}/restore`, { method: 'POST', body: { modules, revision: source.targetRevision } }),
  recover: (id: string) => apiRequest<BackupRecord>(`/backups/${id}/recover`, { method: 'POST', body: {} }),
  delete: (id: string) => apiRequest<void>(`/backups/${id}`, { method: 'DELETE' }),
  download: (id: string) => `/api/v1/backups/${id}/download`,
}
