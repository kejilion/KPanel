import type { HostSwitcherAction, HostSwitcherStatus, HostSwitcherTone } from '@/lib/hostSwitcher'
import type { ClusterHost } from '@/types/api'

/**
 * Whether a cluster host can be browsed in this panel's file views. Shared by
 * the file manager and the gallery so both offer the same hosts and say the
 * same thing about the rest. Labels are source phrases; callers translate them.
 */
export type FileHostAction = HostSwitcherAction

export interface FileHostStatus extends HostSwitcherStatus {
  action: FileHostAction
  tone: HostSwitcherTone
}

/** Display the local file source consistently while retaining remote names. */
export function fileHostLabel(hostId: string | undefined, hosts: readonly Pick<ClusterHost, 'id' | 'name' | 'isLocal'>[], localLabel: string): string {
  if (!hostId) return localLabel
  const host = hosts.find(item => item.id === hostId)
  return host?.isLocal ? localLabel : host?.name || hostId
}

export function fileHostStatus(host: ClusterHost): FileHostStatus {
  if (host.isLocal) return { action: 'select', tone: 'online', label: '当前面板' }
  if (host.remoteFilesDisabled) return { action: 'manage', disabled: true, tone: 'neutral', label: '当前中心已禁用远程文件管理' }
  if (host.kind === 'light_node') {
    return host.fileManagementAvailable === true
      ? { action: 'select', tone: 'online', label: '文件管理已就绪' }
      : { action: 'manage', tone: 'warning', label: '文件代理未就绪' }
  }
  // Summary failures do not revoke a file grant or prove its relay is down.
  // Allow the authenticated file request to report its own connection result.
  if (['auth_failed', 'tls_error', 'incompatible'].includes(host.state)) {
    return { action: 'manage', tone: 'offline', label: '主机连接异常' }
  }
  if (['pairing', 'revoking'].includes(host.state)) {
    return { action: 'manage', tone: 'warning', label: '主机状态处理中' }
  }
  if (host.kind === 'panel' && host.fileManagementAvailable === true) {
    if (host.state === 'offline') {
      return { action: 'select', tone: 'warning', label: '监控离线 · 可尝试连接' }
    }
    return { action: 'select', tone: 'online', label: '文件管理已就绪' }
  }
  if (host.mutualFileTransferAvailable) {
    return { action: 'open', tone: 'neutral', label: '已配对 · 文件互传' }
  }
  if (host.fileTransferAvailable === true) {
    return { action: 'open', tone: 'neutral', label: '已配对 · 仅支持接收' }
  }
  return { action: 'open', tone: 'neutral', label: '打开远端文件管理' }
}
