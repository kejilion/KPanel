import { afterEach, describe, expect, it, vi } from 'vitest'
import { enUSMessages } from './messages/en-US'
import { zhCNMessages } from './messages/zh-CN'
import { zhTWMessages } from './messages/zh-TW'
import {
  resetLocaleForTest,
  initializeI18n,
  resolveInitialLocale,
  setLocale,
  t,
} from './index'
import { localizeError } from './errors'

afterEach(() => {
  resetLocaleForTest()
  vi.unstubAllGlobals()
})

describe('locale selection', () => {
  it('uses a persisted user choice before the browser language', () => {
    expect(resolveInitialLocale('zh-CN', ['en-US'])).toBe('zh-CN')
    expect(resolveInitialLocale('zh-TW', ['en-US'])).toBe('zh-TW')
    expect(resolveInitialLocale('en-US', ['zh-Hans-CN'])).toBe('en-US')
  })

  it('distinguishes Traditional Chinese browser languages from Simplified Chinese', () => {
    expect(resolveInitialLocale(undefined, ['zh-TW', 'en-US'])).toBe('zh-TW')
    expect(resolveInitialLocale(undefined, ['zh-HK', 'en-US'])).toBe('zh-TW')
    expect(resolveInitialLocale(undefined, ['zh-Hant-TW', 'en-US'])).toBe('zh-TW')
    expect(resolveInitialLocale(undefined, ['zh-CN', 'en-US'])).toBe('zh-CN')
    expect(resolveInitialLocale(undefined, ['ja-JP'])).toBe('en-US')
    expect(resolveInitialLocale(undefined, [])).toBe('en-US')
  })

  it('initializes from the browser only when no user choice is stored', async () => {
    const documentElement = { lang: '', dir: '' }
    const addEventListener = vi.fn()
    vi.stubGlobal('document', { documentElement })
    vi.stubGlobal('navigator', { languages: ['de-DE'], language: 'de-DE' })
    vi.stubGlobal('window', {
      addEventListener,
      localStorage: { getItem: vi.fn().mockReturnValue(null), setItem: vi.fn() },
    })

    await initializeI18n()

    expect(t('route.overview')).toBe('Overview')
    expect(t('route.systemCenter')).toBe('System center')
    expect(documentElement.lang).toBe('en-US')
    expect(addEventListener).toHaveBeenCalledWith('storage', expect.any(Function))
  })

  it('keeps all locale resources structurally identical', () => {
    expect(Object.keys(enUSMessages).sort()).toEqual(Object.keys(zhCNMessages).sort())
    expect(Object.keys(enUSMessages).sort()).toEqual(Object.keys(zhTWMessages).sort())
  })
})

describe('translations', () => {
  it('loads English on demand and updates the document language', async () => {
    const documentElement = { lang: '', dir: '' }
    const setItem = vi.fn()
    vi.stubGlobal('document', { documentElement })
    vi.stubGlobal('window', { localStorage: { setItem } })

    await expect(setLocale('en-US')).resolves.toBe(true)

    expect(t('route.overview')).toBe('Overview')
    expect(documentElement).toMatchObject({ lang: 'en-US', dir: 'ltr' })
    expect(setItem).toHaveBeenCalledWith('kejilion-panel-locale', 'en-US')
  })

  it('loads Traditional Chinese and persists the selected locale', async () => {
    const documentElement = { lang: '', dir: '' }
    const setItem = vi.fn()
    vi.stubGlobal('document', { documentElement })
    vi.stubGlobal('window', { localStorage: { setItem } })

    await expect(setLocale('zh-TW')).resolves.toBe(true)

    expect(t('route.overview')).toBe('概覽')
    expect(t('common.language')).toBe('介面語言')
    expect(documentElement).toMatchObject({ lang: 'zh-TW', dir: 'ltr' })
    expect(setItem).toHaveBeenCalledWith('kejilion-panel-locale', 'zh-TW')
  })

  it('keeps the most recent choice during rapid language switching', async () => {
    const documentElement = { lang: '', dir: '' }
    vi.stubGlobal('document', { documentElement })
    vi.stubGlobal('window', { localStorage: { setItem: vi.fn() } })

    await Promise.all([setLocale('en-US'), setLocale('zh-CN')])

    expect(t('route.overview')).toBe('概览')
    expect(t('route.systemCenter')).toBe('系统中心')
    expect(documentElement.lang).toBe('zh-CN')
  })

  it('localizes stable API codes and preserves an unknown server detail', async () => {
    const documentElement = { lang: '', dir: '' }
    vi.stubGlobal('document', { documentElement })
    vi.stubGlobal('window', { localStorage: { setItem: vi.fn() } })
    await setLocale('en-US')

    expect(localizeError({ code: 'resource_version_changed', message: 'raw detail' }))
      .toBe('The resource changed. Refresh and try again.')
    expect(localizeError({ code: 'http_502', status: 502, message: '请求失败（HTTP 502）' }))
      .toBe('Request failed (HTTP 502).')
    expect(localizeError({ code: 'custom_error', message: 'operator detail' }))
      .toBe('operator detail')
    expect(localizeError({ code: 'invalid_bootstrap_token', message: 'raw detail' }))
      .toBe('The bootstrap credential is invalid. Copy it again and retry.')
    expect(localizeError({ code: 'second_factor_unavailable', message: 'raw detail' }))
      .toBe('The authenticator is temporarily unavailable. Use a recovery code or check the TOTP encryption key.')
  })
})
