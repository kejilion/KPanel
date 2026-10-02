import type { ClusterHost } from '@/types/api'

/**
 * Whether a cluster host can be browsed in this panel's file views. Shared by
 * the file manager and the gallery so both offer the same hosts and say the
 * same thing about the rest. Labels are source phrases; callers translate them.
 */
export type FileHostAction = 'select' | 'manage' | 'open'

export interface FileHostStatus {
  action: FileHostAction
  label: string
}

export function fileHostStatus(host: ClusterHost): FileHostStatus {
  if (host.isLocal) return { action: 'select', label: '当前面板' }
  if (host.kind === 'light_node') {
    return host.fileManagementAvailable === true
      ? { action: 'select', label: '文件管理已就绪' }
      : { action: 'manage', label: '文件代理未就绪' }
  }
  if (['offline', 'auth_failed', 'tls_error', 'incompatible'].includes(host.state)) {
    return { action: 'manage', label: '主机连接异常' }
  }
  if (['pairing', 'revoking'].includes(host.state)) {
    return { action: 'manage', label: '主机状态处理中' }
  }
  if (host.kind === 'panel' && host.fileManagementAvailable === true) {
    return { action: 'select', label: '文件管理已就绪' }
  }
  if (host.mutualFileTransferAvailable) {
    return { action: 'open', label: '已配对 · 文件互传' }
  }
  if (host.fileTransferAvailable === true) {
    return { action: 'open', label: '已配对 · 仅支持接收' }
  }
  return { action: 'open', label: '打开远端文件管理' }
}
