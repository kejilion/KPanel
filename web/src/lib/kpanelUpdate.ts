import { api } from '@/lib/api'
import type { AppInstallJob, AppMarketInventory, AppMarketItem } from '@/types/api'

export const kpanelAppID = 'thirdparty-kpanel'
export const kpanelAppToken = 'kpanel'

export type KPanelUpdateState = 'available' | 'current' | 'unavailable'

export function kpanelUpdateHint(currentVersion?: string): string {
  const normalizedVersion = currentVersion?.trim().replace(/^v/i, '')
  if (!normalizedVersion) return '发现 KPanel 新版本，点击更新到最新版本'
  return `当前版本 v${normalizedVersion}，发现可用更新，点击更新到最新版本`
}

export function findKPanelApp(inventory: AppMarketInventory): AppMarketItem | undefined {
  return inventory.items.find((item) => item.id === kpanelAppID || item.token === kpanelAppToken)
}

export async function detectKPanelUpdate(signal?: AbortSignal): Promise<KPanelUpdateState> {
  const inventory = await api.apps.inventory(signal)
  const item = findKPanelApp(inventory)
  if (
    !item?.runtime.installed ||
    !item.runtime.resourceVersion ||
    !item.capabilities.check_update?.enabled
  ) {
    return 'unavailable'
  }
  const result = await api.apps.checkUpdate(item.id, item.runtime.resourceVersion)
  return result.status === 'fixed' ? 'unavailable' : result.status
}

export function isKPanelSelfUpdate(job: Pick<AppInstallJob, 'action' | 'appId'>): boolean {
  return job.action === 'update' && job.appId === kpanelAppID
}
