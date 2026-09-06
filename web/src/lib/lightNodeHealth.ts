import type { LightNodeHealth, LightNodeServiceHealth } from '@/types/api'

export function lightHealthFresh(health: LightNodeHealth | undefined, now: number): boolean {
  const observed = Date.parse(health?.observedAt || '')
  return Number.isFinite(observed) && observed <= now + 60_000 && now - observed <= 90_000
}

export function lightUpdateLabel(health: LightNodeHealth | undefined, now: number): string {
  if (!health) return '尚未上报'
  if (!lightHealthFresh(health, now)) return '观测已过期'
  const update = health.update
  if (update.state === 'running' && now / 1000 - (update.checkedAt || 0) > 1800) return '更新已中断'
  if (update.checkedAt && now / 1000 - update.checkedAt > 3 * 3600) return '检查记录已过期'
  const labels: Record<string, string> = {
    missing: '暂无检查记录', invalid: '检查记录无效', unavailable: '检查记录不可读',
    running: '正在检查或更新', interrupted: '更新已中断', current: '已是最新版本',
    updated: '更新成功', degraded: '辅助服务异常', failed: '更新失败', rolled_back: '更新失败，已回滚',
  }
  return labels[update.state] || '状态未知'
}

export function lightServiceLabel(service: LightNodeServiceHealth | undefined, fresh: boolean, timer = false): string {
  if (!fresh) return '观测已过期'
  if (!service || service.loadState === 'unknown') return '状态未知'
  if (service.loadState === 'not-found') return '未安装'
  if (service.loadState === 'masked' || service.unitFileState.startsWith('masked')) return '已屏蔽'
  if (service.loadState !== 'loaded' || service.activeState === 'failed') return '运行失败'
  if (timer && service.unitFileState === 'disabled') return '自动检查未启用'
  if (service.activeState === 'inactive') return '未运行'
  if (service.activeState === 'activating' || service.activeState === 'deactivating' || service.activeState === 'reloading') return '状态切换中'
  if (timer && service.unitFileState !== 'enabled') return '启用状态待确认'
  if (service.activeState === 'active' && service.subState === (timer ? 'waiting' : 'running')) return timer ? '等待自动检查' : '正在运行'
  return '状态未知'
}
