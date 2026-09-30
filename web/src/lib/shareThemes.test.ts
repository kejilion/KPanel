import { describe, expect, it } from 'vitest'
import { shareThemeModel, shareThemeModelV2, shareThemeProtocol, shareThemeURL } from './shareThemes'
import type { PublicClusterShareSnapshot } from '@/types/api'
import { useI18n } from '@/i18n'
import { regionCenters } from '@/components/cluster/globeData'

export function themeSnapshot(): PublicClusterShareSnapshot {
  return { title: 'Public fleet', generatedAt: '2026-01-01T12:00:00Z', total: 1, online: 1, attention: 0,
    theme: { id: 'minimal', name: { 'zh-CN': '简约看板' }, fileBase: `/api/v1/cluster/share-themes/minimal/files/${'b'.repeat(32)}/` },
    items: [{ id: 'public-one', name: '<script>bad</script>', state: 'online', collectedAt: '2026-01-01',
      cpu: { cores: 2, usagePercent: 20 }, memory: { totalBytes: 100, usedBytes: 30, usagePercent: 30 }, disk: { totalBytes: 100, usedBytes: 50, usagePercent: 50 },
      network: { receivedBytes: 1e12, sentBytes: 1e12, receiveBytesPerSecond: 0, transmitBytesPerSecond: 0 },
      location: { country: 'Test' }, load: { one: 0, five: 0, fifteen: 0 },
      trafficPeriod: { available: true, partial: false, estimated: false, receivedBytes: 29 * 1024 ** 3, sentBytes: 68 * 1024 ** 3, startedAt: '2026-01-01', endsAt: '2026-02-01' },
      trafficMonthlyQuotaGiB: 100, trafficCalculation: 'max', price: '$30/月', expiresOn: '2026-01-16' }] }
}
describe('share theme protocol', () => {
  it('omits unconfigured value text while preserving configured zero', () => {
    const snapshot = themeSnapshot()
    const now = new Date(2026, 0, 20, 12)
    expect(shareThemeModel(snapshot, 'en-US', now).hosts[0]?.remaining).toBe('$0.00')
    expect(shareThemeModel(snapshot, 'en-US', now).value.groups).toEqual([{ currency: 'USD', text: '$0.00' }])
    delete snapshot.items[0]!.expiresOn
    const model = shareThemeModel(snapshot, 'en-US', now)
    expect(model.hosts[0]?.remaining).toBe('')
    expect(model.value.groups).toEqual([])
  })
  it('accepts only the exact local capability URL', () => {
    const theme = themeSnapshot().theme!
    expect(shareThemeURL(theme)).toBe(`${theme.fileBase}index.html`)
    for (const fileBase of ['https://evil.test/', '//evil.test/', `${theme.fileBase}?token=x`, `${theme.fileBase}../`, theme.fileBase.replace('minimal', 'other')]) {
      expect(shareThemeURL({ ...theme, fileBase })).toBeUndefined()
    }
  })
  it('reuses monthly accounting and value estimates without spreading future API fields', () => {
    const snapshot = themeSnapshot()
    Object.assign(snapshot, { token: 'secret-token', csrf: 'secret-csrf' })
    Object.assign(snapshot.items[0]!, { address: 'secret-address', reminderEnabled: true })
    const model = shareThemeModel(snapshot, 'en-US', new Date(2026, 0, 1, 12))
    expect(model.hosts[0]?.traffic).toMatchObject({ monthly: true, percent: '68%', tone: 'normal' })
    expect(model.value.groups).toEqual([{ currency: 'USD', text: '$15.00' }])
    expect(JSON.stringify(model)).not.toContain('secret-')
    expect(JSON.stringify(model)).not.toContain('reminderEnabled')
    snapshot.items[0]!.trafficPeriod!.available = false
    const waiting = shareThemeModel(snapshot, 'en-US')
    expect(waiting.hosts[0]?.traffic.percent).toBe('')
    expect(waiting.hosts[0]?.traffic.received).toBe('—')
  })
  it('preserves core incomplete/estimated/waiting and accounting explanations', () => {
    const snapshot = themeSnapshot(), { t } = useI18n()
    snapshot.items[0]!.trafficPeriod!.partial = true
    snapshot.items[0]!.trafficPeriod!.estimated = true
    const traffic = shareThemeModel(snapshot, 'zh-CN', new Date(), t).hosts[0]!.traffic
    expect(traffic).toMatchObject({ partial: true, estimated: true })
    expect(traffic.hint).toContain('统计不完整')
    expect(traffic.hint).toContain('按时间比例估算')
    expect(traffic.hint).toContain('收发取较大值')
    expect(traffic).not.toHaveProperty('startedAt')
    expect(traffic).not.toHaveProperty('endsAt')
    expect(traffic.hint).not.toContain('2026-01-01')
    expect(traffic.hint).not.toContain('2026-02-01')
    snapshot.items[0]!.trafficPeriod!.available = false
    expect(shareThemeModel(snapshot, 'zh-CN', new Date(), t).hosts[0]!.traffic.hint).toContain('等待有效采样')
    delete snapshot.items[0]!.trafficPeriod
    snapshot.items[0]!.trafficResetDay = 1
    expect(shareThemeModel(snapshot, 'zh-CN', new Date(), t).hosts[0]!.traffic.monthly).toBe(true)
    expect(shareThemeModel(snapshot, 'zh-CN', new Date(), t).hosts[0]!.traffic.hint).toContain('等待有效采样')
  })
  it('protocol 2 adds raw numbers and localized vocabulary without leaking private fields', () => {
    const snapshot = themeSnapshot(), { t } = useI18n()
    Object.assign(snapshot, { token: 'secret-token' }); Object.assign(snapshot.items[0]!, { address: 'secret-address' })
    const model = shareThemeModelV2(snapshot, 'zh-CN', new Date(2026, 0, 1, 12), t)
    const host = model.hosts[0]!
    expect(model.counts).toEqual({ total: 1, online: 1, attention: 0, offline: 0 })
    expect(host).toMatchObject({ stateLabel: '在线', cores: 2, collected: true, cpu: { text: '20%', ratio: 0.2 }, memory: { ratio: 0.3, usedBytes: 30, totalBytes: 100 } })
    expect(host.traffic).toMatchObject({ monthly: true, percent: '68%', ratio: expect.closeTo(0.68, 2), quotaGiB: 100 })
    expect(host.location.text).toBe('Test')
    expect(host.location).toMatchObject({ latitude: null, longitude: null })
    snapshot.items[0]!.location.countryCode = 'jp'
    expect(shareThemeModelV2(snapshot, 'zh-CN').hosts[0]!.location).toMatchObject({ countryCode: 'JP', latitude: null })
    expect(shareThemeModelV2(snapshot, 'zh-CN', new Date(), undefined, regionCenters).hosts[0]!.location).toMatchObject({ countryCode: 'JP', latitude: regionCenters.JP![0], longitude: regionCenters.JP![1] })
    expect(model.value.groups[0]).toMatchObject({ currency: 'USD', amount: expect.any(Number) })
    expect(JSON.stringify(model)).not.toContain('secret-')
    // Unknown metrics stay unknown instead of turning into zero.
    delete snapshot.items[0]!.collectedAt
    const pending = shareThemeModelV2(snapshot, 'en-US').hosts[0]!
    expect(pending).toMatchObject({ collected: false, cores: null, cpu: { ratio: null }, load: null, uptimeSeconds: null })
    expect(pending.network.down.bytesPerSecond).toBeNull()
  })
  it('falls back to protocol 1 unless a package explicitly declares protocol 2', () => {
    expect(shareThemeProtocol(2)).toBe(2)
    for (const value of [undefined, 1, '2', 3, null]) expect(shareThemeProtocol(value)).toBe(1)
  })
})
