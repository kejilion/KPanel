import { describe, expect, it } from 'vitest'
import type { NotificationEvent } from '@/types/api'
import { summarizeNotification } from './notificationSummary'

const when = '2026-09-28 20:00:00 (UTC+08:00)'
function event(rule: string, kind: NotificationEvent['kind'], body: string, locale = 'zh-CN'): NotificationEvent {
  const title = locale === 'en-US' ? `KPanel Cluster ${kind === 'alert' ? 'Alert' : 'Notice'}`
    : locale === 'zh-TW' ? `KPanel 叢集${kind === 'alert' ? '警報' : '通知'}` : `KPanel 集群${kind === 'alert' ? '告警' : '通知'}`
  const host = locale === 'en-US' ? 'Host: node' : locale === 'zh-TW' ? '主機：node' : '主机：node'
  const time = locale === 'en-US' ? 'Time: ' : locale === 'zh-TW' ? '時間：' : '时间：'
  return { id: '1', hostId: 'local', hostName: 'node', isLocal: true, rule, kind,
    message: `${kind === 'alert' ? '⚠️' : '✅'} [${title}]\n\n${host}\n${body}\n\n${time}${when}`,
    createdAt: '2026-09-28T12:00:00Z', delivery: 'local_only', attempts: 0 }
}

describe('notification summaries', () => {
  it.each([
    ['zh-CN', '到期日期：2026-10-05\n剩余天数：7'],
    ['zh-TW', '到期日期：2026-10-05\n剩餘天數：7'],
    ['en-US', 'Expiry date: 2026-10-05\nDays remaining: 7'],
  ])('summarizes %s server expiry reminders', (locale, body) => {
    const reminder = event('server-expiry', 'info', body, locale)
    reminder.message = reminder.message.replace('✅', '⏰')
    expect(summarizeNotification(reminder)).toEqual({ fields: [
      { label: '到期日期', value: '2026-10-05' }, { label: '剩余天数', value: '7' },
    ], text: '' })
  })
  it.each([
    ['cpu', 'CPU 使用率', '100.0%', '90.0%'], ['memory', '内存使用率', '92.6%', '90.0%'],
    ['disk', '磁盘使用率', '91.7%', '90.0%'], ['traffic', '网络吞吐', '160.0 MiB/s', '100.0 MiB/s'],
    ['traffic-total-received', '累计接收', '108.6 GB', '100.0 GB'], ['traffic-total-sent', '累计传送', '102.4 GB', '100.0 GB'],
  ])('extracts %s values with their original units', (rule, label, reached, threshold) => {
    expect(summarizeNotification(event(rule, 'alert', `${label}达到 ${reached}\n阈值：${threshold}`)).fields)
      .toEqual([{ label: '阈值', value: threshold }, { label: '到达值', value: reached }])
  })
  it.each([
    ['zh-CN', 'CPU 使用率达到 100.0%\n阈值：90.0%', '已恢复：CPU 使用率 当前 0.0%'],
    ['zh-TW', 'CPU 使用率達到 100.0%\n閾值：90.0%', '已恢復：CPU 使用率 目前 0.0%'],
    ['en-US', 'CPU usage reached 100.0%\nThreshold: 90.0%', 'Recovered: CPU usage, current 0.0%'],
  ])('recognizes stored %s messages independently of the UI language', (locale, alert, recovery) => {
    expect(summarizeNotification(event('cpu', 'alert', alert, locale)).fields).toHaveLength(2)
    expect(summarizeNotification(event('cpu', 'recovery', recovery, locale)).fields).toEqual([{ label: '当前值', value: '0.0%' }])
  })
  it.each([
    ['alert', '主机暂时失联，当前状态：授权失败', '授权失败'],
    ['recovery', '连接已恢复，当前状态：在线', '在线'],
  ] as const)('keeps the actual connection state for %s events', (kind, body, state) => {
    expect(summarizeNotification(event('availability', kind, body)).fields).toEqual([{ label: '状态', value: state }])
  })
  it('uses SSH occurrence time and preserves user, source and method', () => {
    const ssh = event('ssh', 'info', 'SSH 登录：2026-09-28 19:59:00 (UTC+08:00)\n用户：deploy\n来源：2001:db8::24\n方式：publickey')
    ssh.message = ssh.message.replace('✅', '🔐').replace('时间：', '发送时间：')
    expect(summarizeNotification(ssh)).toEqual({ fields: [
      { label: '用户', value: 'deploy' }, { label: '来源', value: '2001:db8::24' }, { label: '方式', value: 'publickey' },
    ], text: '', occurredAt: '2026-09-28T19:59:00+08:00' })
  })
  it.each([
    ['zh-CN', '面板登录：2026-09-28 19:59:30 (UTC+08:00)\n用户：admin\n来源：203.0.113.9\n方式：密码 + 两步验证', '密码 + 两步验证'],
    ['zh-TW', '面板登入：2026-09-28 19:59:30 (UTC+08:00)\n使用者：admin\n來源：203.0.113.9\n方式：通行金鑰', '通行金鑰'],
    ['en-US', 'Panel login: 2026-09-28 19:59:30 (UTC+08:00)\nUser: admin\nSource: 203.0.113.9\nMethod: Passkey + two-factor', 'Passkey + two-factor'],
  ])('summarizes %s panel login notices like SSH logins', (locale, body, method) => {
    const login = event('panel-login', 'info', body, locale)
    login.message = login.message.replace('✅', '🔐')
    expect(summarizeNotification(login)).toEqual({ fields: [
      { label: '用户', value: 'admin' }, { label: '来源', value: '203.0.113.9' }, { label: '方式', value: method },
    ], text: '', occurredAt: '2026-09-28T19:59:30+08:00' })
  })
  it('does not read an SSH body as a panel login', () => {
    const mixed = event('panel-login', 'info', 'SSH 登录：2026-09-28 19:59:00 (UTC+08:00)\n用户：deploy\n来源：2001:db8::24\n方式：publickey')
    mixed.message = mixed.message.replace('✅', '🔐')
    expect(summarizeNotification(mixed).fields).toEqual([])
  })
  it.each([
    'CPU 使用率达到 95.0%',
    'CPU 使用率达到 95.0%\n阈值：90.0%\n额外原因：采样不足',
    '<img src=x onerror=alert(1)>',
  ])('preserves unrecognized or incomplete bodies instead of inventing fields', body => {
    const original = event('cpu', 'alert', body)
    const summary = summarizeNotification(original)
    expect(summary.fields).toEqual([])
    expect(summary.text).toContain(body.replace(/\n/g, ' · '))
    expect(original.message).toContain(body)
  })
  it('does not strip metadata from an unknown message or mismatched host', () => {
    const mismatch = event('cpu', 'alert', 'CPU 使用率达到 95.0%\n阈值：90.0%')
    mismatch.hostName = 'another node'
    expect(summarizeNotification(mismatch).fields).toEqual([])
    expect(summarizeNotification({ ...mismatch, message: 'Custom alert\nHost: node\nThreshold: 90%' }).text)
      .toBe('Custom alert · Host: node · Threshold: 90%')
  })
  it.each(['constructor', '__proto__', 'certificate-expiry'])('falls back safely for unknown rule %s', rule => {
    expect(summarizeNotification(event(rule, 'alert', '证书即将到期')).text).toContain('证书即将到期')
  })
})
