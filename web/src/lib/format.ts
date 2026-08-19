import { getLocale } from '@/i18n'

const numberFormatter = new Intl.NumberFormat('zh-CN', {
  maximumFractionDigits: 1,
})

const dateFormatter = new Intl.DateTimeFormat('zh-CN', {
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
})

export function formatPercent(value?: number): string {
  if (value === undefined || !Number.isFinite(value)) return '—'
  return `${numberFormatter.format(Math.max(0, value))}%`
}

export function formatBytes(value?: number, decimals = 1): string {
  if (value === undefined || !Number.isFinite(value)) return '—'
  if (value === 0) return '0 B'

  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const base = 1024
  const index = Math.min(
    Math.max(0, Math.floor(Math.log(Math.abs(value)) / Math.log(base))),
    units.length - 1,
  )
  const scaled = value / Math.pow(base, index)

  return `${scaled.toFixed(index === 0 ? 0 : decimals)} ${units[index]}`
}

export function formatRate(value?: number): string {
  const formatted = formatBytes(value)
  return formatted === '—' ? formatted : `${formatted}/s`
}

export function formatDuration(totalSeconds?: number): string {
  if (totalSeconds === undefined || !Number.isFinite(totalSeconds) || totalSeconds < 0) return '—'

  const locale = getLocale()
  const seconds = Math.floor(totalSeconds)
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)

  if (locale === 'en-US') {
    if (days > 0) return `${days}d ${hours}h`
    if (hours > 0) return `${hours}h ${minutes}m`
    if (minutes > 0) return `${minutes}m`
    return `${seconds}s`
  }
  const units = locale === 'zh-TW'
    ? { day: '天', hour: '小時', minute: '分鐘', second: '秒' }
    : { day: '天', hour: '小时', minute: '分钟', second: '秒' }
  if (days > 0) return `${days} ${units.day} ${hours} ${units.hour}`
  if (hours > 0) return `${hours} ${units.hour} ${minutes} ${units.minute}`
  if (minutes > 0) return `${minutes} ${units.minute}`
  return `${seconds} ${units.second}`
}

export function formatDateTime(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : dateFormatter.format(date)
}

export function formatHostDateTime(value?: string, timezone?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  try {
    return new Intl.DateTimeFormat(getLocale(), {
      timeZone: timezone || 'UTC',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
      timeZoneName: 'short',
    }).format(date)
  } catch {
    return `${dateFormatter.format(date)} ${timezone || 'UTC'}`
  }
}

export function relativeTime(value?: string, now = Date.now()): string {
  const locale = getLocale()
  if (!value) return locale === 'en-US' ? 'Never' : locale === 'zh-TW' ? '從未' : '从未'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return locale === 'en-US' ? 'Unknown' : '未知'

  const deltaSeconds = Math.round((date.getTime() - now) / 1000)
  const absolute = Math.abs(deltaSeconds)
  const formatter = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })

  if (absolute < 60) return formatter.format(deltaSeconds, 'second')
  if (absolute < 3600) return formatter.format(Math.round(deltaSeconds / 60), 'minute')
  if (absolute < 86400) return formatter.format(Math.round(deltaSeconds / 3600), 'hour')
  return formatter.format(Math.round(deltaSeconds / 86400), 'day')
}

export function clampPercent(value?: number): number {
  if (value === undefined || !Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, value))
}

export function shortId(value?: string, length = 12): string {
  if (!value) return '—'
  return value.length <= length ? value : value.slice(0, length)
}
