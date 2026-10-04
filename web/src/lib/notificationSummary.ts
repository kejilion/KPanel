import type { NotificationEvent } from '@/types/api'

export interface NotificationSummary {
  fields: { label: string; value: string }[]
  text: string
  occurredAt?: string
}

const metrics: Record<string, string[]> = {
  cpu: ['CPU 使用率', 'CPU usage'], memory: ['内存使用率', '記憶體使用率', 'Memory usage'],
  disk: ['磁盘使用率', '磁碟使用率', 'Disk usage'], traffic: ['网络吞吐', '網路吞吐量', 'Network throughput'],
  'traffic-total-received': ['累计接收', '累計接收', 'Cumulative received'],
  'traffic-total-sent': ['累计传送', '累計傳送', 'Cumulative sent'],
}
const logins: Record<string, string[]> = {
  ssh: ['SSH 登录：', 'SSH 登入：', 'SSH login: '],
  'panel-login': ['面板登录：', '面板登入：', 'Panel login: '],
}
const timestamp = /^(\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}:\d{2}) \(UTC([+-]\d{2}:\d{2})\)$/

function afterPrefix(line: string, prefixes: string[]): string | undefined {
  const prefix = prefixes.find(value => line.startsWith(value))
  return prefix === undefined ? undefined : line.slice(prefix.length).trim() || undefined
}

// Recognize only the existing service.go templates. Unknown formats keep their text.
export function summarizeNotification(event: NotificationEvent): NotificationSummary {
  const lines = event.message.split(/\r?\n/).map(line => line.trim()).filter(Boolean)
  const fallback = { fields: [], text: lines.join(' · ') }
  if (!/^(?:⚠️|✅|🔐|⏰) \[KPanel (?:集群(?:告警|通知)|叢集(?:警報|通知)|Cluster (?:Alert|Notice))\]$/.test(lines[0] || '') ||
    !['主机：', '主機：', 'Host: '].some(prefix => lines[1] === prefix + event.hostName)) return fallback
  const sentAt = afterPrefix(lines.at(-1) || '', ['时间：', '時間：', 'Time: ', '发送时间：', '傳送時間：', 'Sent: '])
  if (!sentAt || !timestamp.test(sentAt)) return fallback
  const body = lines.slice(2, -1)
  if (event.rule === 'server-expiry' && event.kind === 'info' && body.length === 2) {
    const date = afterPrefix(body[0]!, ['到期日期：', 'Expiry date: '])
    const days = afterPrefix(body[1]!, ['剩余天数：', '剩餘天數：', 'Days remaining: '])
    if (date && /^\d{4}-\d{2}-\d{2}$/.test(date) && days && /^(?:7|3|1|0)$/.test(days)) {
      return { fields: [{ label: '到期日期', value: date }, { label: '剩余天数', value: days }], text: '' }
    }
  }
  const metricNames = Object.hasOwn(metrics, event.rule) ? metrics[event.rule] : undefined
  if (metricNames && event.kind === 'alert' && body.length === 2) {
    const reached = afterPrefix(body[0]!, metricNames.flatMap(name => [`${name}达到 `, `${name}達到 `, `${name} reached `]))
    const threshold = afterPrefix(body[1]!, ['阈值：', '閾值：', 'Threshold: '])
    if (reached && threshold) return { fields: [{ label: '阈值', value: threshold }, { label: '到达值', value: reached }], text: '' }
  }
  if (metricNames && event.kind === 'recovery' && body.length === 1) {
    const current = afterPrefix(body[0]!, metricNames.flatMap(name => [
      `已恢复：${name} 当前 `, `已恢復：${name} 目前 `, `Recovered: ${name}, current `,
    ]))
    if (current) return { fields: [{ label: '当前值', value: current }], text: '' }
  }
  if (event.rule === 'availability' && body.length === 1) {
    const prefixes = event.kind === 'recovery'
      ? ['连接已恢复，当前状态：', '連線已恢復，目前狀態：', 'Connection recovered, current state: ']
      : event.kind === 'alert' ? ['主机暂时失联，当前状态：', '主機暫時失聯，目前狀態：', 'Host is temporarily unreachable, current state: '] : []
    const state = afterPrefix(body[0]!, prefixes)
    if (state) return { fields: [{ label: '状态', value: state }], text: '' }
  }
  const loginPrefixes = Object.hasOwn(logins, event.rule) ? logins[event.rule] : undefined
  if (loginPrefixes && event.kind === 'info' && body.length === 4) {
    const login = afterPrefix(body[0]!, loginPrefixes)?.match(timestamp)
    const user = afterPrefix(body[1]!, ['用户：', '使用者：', 'User: '])
    const source = afterPrefix(body[2]!, ['来源：', '來源：', 'Source: '])
    const method = afterPrefix(body[3]!, ['方式：', 'Method: '])
    if (login && user && source && method) {
      const occurredAt = `${login[1]}T${login[2]}${login[3]}`
      if (!Number.isNaN(Date.parse(occurredAt))) return {
        fields: [{ label: '用户', value: user }, { label: '来源', value: source }, { label: '方式', value: method }], text: '', occurredAt,
      }
    }
  }
  return fallback
}
