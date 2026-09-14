import { api } from '@/lib/api'
import type {
  AppInstallJob,
  AppMarketInventory,
  AppMarketItem,
  AutomaticUpdateStatus,
  KPanelReleaseInfo,
} from '@/types/api'

export const kpanelAppID = 'thirdparty-kpanel'
export const kpanelAppToken = 'kpanel'
export const kpanelUpdateSettingsSection = 'version-updates'
export const kpanelUpdateSettingsPath = `/settings?section=${kpanelUpdateSettingsSection}`
export const kpanelAppUpdatePath = `/apps?app=${kpanelAppToken}&action=update`
export const kpanelReleasesURL = 'https://github.com/kejilion/KPanel/releases'

const kpanelVersionPattern = /^\d+\.\d+\.\d+(?:-rc\.[1-9]\d*)?$/

export type KPanelUpdateState = 'available' | 'current' | 'unavailable'

export function isKPanelUpdateSettingsIntent(value: unknown): boolean {
  return value === kpanelUpdateSettingsSection
}

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

export function normalizedKPanelVersion(value?: string): string {
  return value?.trim().replace(/^v/i, '') || ''
}

export function officialKPanelReleaseURL(version?: string): string {
  const normalized = normalizedKPanelVersion(version)
  return kpanelVersionPattern.test(normalized)
    ? `${kpanelReleasesURL}/tag/v${normalized}`
    : kpanelReleasesURL
}

export function releaseFromAutomaticUpdate(status: AutomaticUpdateStatus): KPanelReleaseInfo | undefined {
  if (!status.candidateVersion || !status.candidateImageDigest) return undefined
  return {
    channel: status.channel,
    version: normalizedKPanelVersion(status.candidateVersion),
    imageDigest: status.candidateImageDigest,
    releaseUrl: status.candidateReleaseUrl,
    publishedAt: status.candidatePublishedAt,
    notes: status.candidateNotes,
    upgradeNotes: status.candidateUpgradeNotes,
    cached: true,
    stale: false,
  }
}

export function releaseMatchesTarget(
  release: KPanelReleaseInfo,
  targetVersion?: string,
  targetDigest?: string,
): boolean {
  const version = normalizedKPanelVersion(targetVersion)
  return (!version || release.version === version)
    && (!targetDigest || release.imageDigest === targetDigest)
}
