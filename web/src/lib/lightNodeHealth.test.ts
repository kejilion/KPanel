import { describe, expect, it } from 'vitest'
import { lightHealthFresh, lightServiceLabel, lightUpdateLabel } from './lightNodeHealth'
import type { LightNodeHealth, LightNodeServiceHealth } from '@/types/api'

const now = Date.parse('2026-09-06T00:00:00Z')
const service: LightNodeServiceHealth = { loadState: 'loaded', activeState: 'active', subState: 'running', unitFileState: 'enabled' }
const health: LightNodeHealth = { observedAt: new Date(now).toISOString(), runtimeVersion: '1.4.1', update: { state: 'current', checkedAt: now / 1000 - 4500, finishedAt: now / 1000 - 4400 }, services: { timer: { ...service, subState: 'waiting' }, telemetry: service, terminal: service, file: service, sshLogin: service } }

describe('light node health truth', () => {
  it('separates an hourly update check from a fresh telemetry observation', () => {
    expect(lightUpdateLabel(health, now)).toBe('已是最新版本')
    expect(lightHealthFresh(health, now + 91_000)).toBe(false)
    expect(lightUpdateLabel(health, now + 91_000)).toBe('观测已过期')
    expect(lightUpdateLabel(undefined, now)).toBe('尚未上报')
  })
  it('does not turn missing, invalid, expired or abandoned checks into success', () => {
    for (const [state, want] of [['missing', '暂无检查记录'], ['invalid', '检查记录无效'], ['unavailable', '检查记录不可读'], ['rolled_back', '更新失败，已回滚'], ['degraded', '辅助服务异常']] as const) {
      expect(lightUpdateLabel({ ...health, update: { state } }, now)).toBe(want)
    }
    expect(lightUpdateLabel({ ...health, update: { state: 'running', checkedAt: now / 1000 - 1801 } }, now)).toBe('更新已中断')
    expect(lightUpdateLabel({ ...health, update: { state: 'current', checkedAt: now / 1000 - 10801 } }, now)).toBe('检查记录已过期')
  })
  it('separates failed queries, missing units and disabled or masked active timers', () => {
    expect(lightServiceLabel(undefined, true)).toBe('状态未知')
    expect(lightServiceLabel({ ...service, loadState: 'not-found' }, true)).toBe('未安装')
    expect(lightServiceLabel({ ...service, unitFileState: 'disabled' }, true, true)).toBe('自动检查未启用')
    expect(lightServiceLabel({ ...service, unitFileState: 'masked' }, true, true)).toBe('已屏蔽')
    expect(lightServiceLabel(health.services.timer, true, true)).toBe('等待自动检查')
    expect(lightServiceLabel(service, false)).toBe('观测已过期')
  })
})
