import { apiRequest } from '@/lib/api'

export function backupMessage(reason: unknown): string {
  const codes: Record<string, string> = {
    backup_settings_changed: '备份设置已变更，请刷新后重试',
    invalid_backup_settings: '请检查远程存储、时间和备份密码',
    backup_remote_limit: '远程存储或文件数量超过限制，请使用独立的备份目录',
    backup_remote_unavailable: '远程连接失败，请检查地址、凭据和读写权限',
  }
  const code = reason && typeof reason === 'object' && 'code' in reason ? String(reason.code) : ''
  return codes[code] || (reason instanceof Error ? reason.message : '操作未完成，请刷新后重试。')
}

export type BackupModule = 'panel' | 'apps' | 'web' | 'docker'
export interface BackupRootRef {
  path: string
  module: BackupModule
}
export interface BackupRecord {
  automatic?: boolean
  localReady?: boolean
  remote?: { storageId: string; storageName: string; key: string; status: string }
  id: string
  action: 'export' | 'import' | 'restore' | 'recover'
  status: string
  stage: string
  modules: BackupModule[]
  roots?: BackupRootRef[]
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
  settings: () => apiRequest<BackupSettings>('/backups/settings'),
  saveStorage: (revision: string, storage: BackupStorage) => apiRequest<BackupSettings>('/backups/storage', { method: 'PUT', body: { revision, storage } }),
  deleteStorage: (revision: string, id: string) => apiRequest<BackupSettings>(`/backups/storage/${id}`, { method: 'DELETE', body: { revision } }),
  testStorage: (id: string) => apiRequest<{ ok: boolean }>(`/backups/storage/${id}/test`, { method: 'POST', body: {} }),
  files: (id: string) => apiRequest<{ items: RemoteBackupFile[] }>(`/backups/storage/${id}/files`),
  saveSchedule: (revision: string, schedule: BackupSchedule) => apiRequest<BackupSettings>('/backups/schedule', { method: 'PUT', body: { revision, schedule } }),
  runSchedule: () => apiRequest<BackupRecord>('/backups/schedule/run', { method: 'POST', body: {} }),
  upload: (id: string, storageId: string) => apiRequest<BackupRecord>(`/backups/${id}/upload`, { method: 'POST', body: { storageId } }),
  remoteImport: (storageId: string, key: string, password: string) => apiRequest<BackupRecord>('/backups/remote-import', { method: 'POST', body: { storageId, key, password } }),
  list: () => apiRequest<{ items: BackupRecord[]; maxBytes: number }>('/backups'),
  preview: (id: string) => apiRequest<BackupRecord>(`/backups/${id}/preview`, { method: 'POST', body: {} }),
  inventory: () => apiRequest<BackupInventory>('/backups/inventory'),
  export: (modules: BackupModule[], password: string, agentRevision?: string, storageId = '') => apiRequest<BackupRecord>('/backups/export', { method: 'POST', body: { modules, password, storageId, agentRevision: agentRevision || '' } }),
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

export interface BackupStorage {
  id: string; name: string; kind: 's3' | 'webdav'; endpoint: string; prefix: string
  bucket?: string; region?: string; pathStyle?: boolean; accessKey?: string; username?: string; secret?: string; hasSecret?: boolean
}
export interface BackupSchedule {
  enabled: boolean; modules: BackupModule[]; storageId: string; frequency: 'daily' | 'weekly' | 'monthly'
  hour: number; minute: number; weekday: number; day: number; timezone: string; keep: number
  password?: string; hasPassword?: boolean; nextRun?: string; lastRun?: string; lastError?: string
}
export interface BackupScheduleHealth {
  state: 'disabled' | 'idle' | 'running' | 'healthy' | 'warning' | 'failed' | 'missed' | 'unavailable'
  lastSuccessAt?: string; lastRecordId?: string; errorCode?: string
}
export interface BackupSettings { revision: string; storages: BackupStorage[]; schedule: BackupSchedule; health?: BackupScheduleHealth }
export interface RemoteBackupFile { key: string; size: number; modified: string }
