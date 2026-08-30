// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { resetLocaleForTest, setLocale } from '@/i18n'
import { clampPercent, formatBytes, formatDateTime, formatDuration, formatHostDateTime, formatPercent, shortId } from './format'

afterEach(() => {
  resetLocaleForTest()
})

describe('format helpers', () => {
  it('formats binary byte values without losing units', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(0.5)).toBe('1 B')
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(1024 ** 3 * 1.5)).toBe('1.5 GB')
    expect(formatBytes(undefined)).toBe('—')
  })

  it('formats stable human durations', () => {
    expect(formatDuration(42)).toBe('42 秒')
    expect(formatDuration(3660)).toBe('1 小时 1 分钟')
    expect(formatDuration(90000)).toBe('1 天 1 小时')
  })

  it('formats observations in the host timezone', () => {
    expect(formatHostDateTime('2026-07-26T00:00:00Z', 'Asia/Shanghai')).toContain('08:00:00')
    expect(formatHostDateTime('invalid', 'Asia/Shanghai')).toBe('—')
  })

  it('follows the active locale for shared date formatting', async () => {
    await setLocale('en-US', false)
    const english = formatDateTime('2026-07-26T00:00:00Z')
    await setLocale('zh-TW', false)
    const traditional = formatDateTime('2026-07-26T00:00:00Z')

    expect(english).toBe(new Intl.DateTimeFormat('en-US', dateOptions).format(sampleDate))
    expect(traditional).toBe(new Intl.DateTimeFormat('zh-TW', dateOptions).format(sampleDate))
    expect(english).not.toBe(traditional)
  })

  it('guards percentages and resource identifiers', () => {
    expect(clampPercent(-5)).toBe(0)
    expect(clampPercent(120)).toBe(100)
    expect(formatPercent(72.25)).toBe('72.3%')
    expect(shortId('1234567890abcdef')).toBe('1234567890ab')
  })
})

const sampleDate = new Date('2026-07-26T00:00:00Z')
const dateOptions: Intl.DateTimeFormatOptions = {
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
}
