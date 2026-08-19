import { computed, readonly, ref, shallowRef } from 'vue'
import { zhCNMessages, type LocaleMessages, type MessageKey } from './messages/zh-CN'
import { zhTWMessages } from './messages/zh-TW'

export type SupportedLocale = 'zh-CN' | 'zh-TW' | 'en-US'
export type TranslationParams = Record<string, string | number>

export interface LocaleOption {
  id: SupportedLocale
  label: string
  shortLabel: string
}

const STORAGE_KEY = 'kejilion-panel-locale'
const DEFAULT_LOCALE: SupportedLocale = 'zh-CN'
const activeLocale = ref<SupportedLocale>(DEFAULT_LOCALE)
const activeMessages = shallowRef<LocaleMessages>(zhCNMessages)
let changeSequence = 0
let initialized = false

export const localeOptions: readonly LocaleOption[] = [
  { id: 'zh-CN', label: '简体中文', shortLabel: '中' },
  { id: 'zh-TW', label: '繁體中文', shortLabel: '繁' },
  { id: 'en-US', label: 'English', shortLabel: 'EN' },
]
const defaultLocaleOption = localeOptions[0]!

function isSupportedLocale(value: unknown): value is SupportedLocale {
  return value === 'zh-CN' || value === 'zh-TW' || value === 'en-US'
}

function resolveChineseLocale(language: string): Exclude<SupportedLocale, 'en-US'> {
  const normalized = language.trim().toLowerCase().replaceAll('_', '-')
  if (/^zh(?:-[a-z]{2,4})*-(?:tw|hk|mo)(?:-|$)/.test(normalized) || /(?:^|-)(?:hant|hks|hant-)/.test(normalized)) {
    return 'zh-TW'
  }
  return 'zh-CN'
}

export function resolveInitialLocale(
  stored: unknown,
  browserLanguages: readonly string[] = [],
): SupportedLocale {
  if (isSupportedLocale(stored)) return stored
  const primaryLanguage = browserLanguages.find((value) => value.trim().length > 0) || ''
  return primaryLanguage.toLowerCase().startsWith('zh') ? resolveChineseLocale(primaryLanguage) : 'en-US'
}

function readStoredLocale(): SupportedLocale {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY)
    return resolveInitialLocale(stored, navigator.languages?.length ? navigator.languages : [navigator.language])
  } catch {
    return resolveInitialLocale(undefined, navigator.languages?.length ? navigator.languages : [navigator.language])
  }
}

function persistLocale(locale: SupportedLocale): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, locale)
  } catch {
    // Storage may be unavailable in hardened/private browser contexts.
  }
}

function applyDocumentLocale(locale: SupportedLocale): void {
  document.documentElement.lang = locale
  document.documentElement.dir = 'ltr'
}

async function loadMessages(locale: SupportedLocale): Promise<LocaleMessages> {
  if (locale === 'zh-CN') return zhCNMessages
  if (locale === 'zh-TW') return zhTWMessages
  return (await import('./messages/en-US')).enUSMessages
}

export async function setLocale(locale: SupportedLocale, persist = true): Promise<boolean> {
  if (!isSupportedLocale(locale)) return false
  const sequence = ++changeSequence
  try {
    const messages = await loadMessages(locale)
    if (sequence !== changeSequence) return false
    activeMessages.value = messages
    activeLocale.value = locale
    applyDocumentLocale(locale)
    if (persist) persistLocale(locale)
    return true
  } catch {
    if (sequence !== changeSequence) return false
    activeMessages.value = zhCNMessages
    activeLocale.value = DEFAULT_LOCALE
    applyDocumentLocale(DEFAULT_LOCALE)
    return false
  }
}

export async function initializeI18n(): Promise<void> {
  if (initialized) return
  initialized = true
  await setLocale(readStoredLocale(), false)
  window.addEventListener('storage', (event) => {
    if (event.key !== STORAGE_KEY || !isSupportedLocale(event.newValue)) return
    void setLocale(event.newValue, false)
  })
}

export function t(key: MessageKey, params?: TranslationParams): string {
  const template = activeMessages.value[key] || zhCNMessages[key] || key
  if (!params) return template
  return template.replace(/\{([A-Za-z0-9_]+)\}/g, (token, name: string) => {
    const value = params[name]
    return value === undefined ? token : String(value)
  })
}

export function getLocale(): SupportedLocale {
  return activeLocale.value
}

export function useI18n() {
  return {
    locale: readonly(activeLocale),
    localeOption: computed(() => localeOptions.find((item) => item.id === activeLocale.value) || defaultLocaleOption),
    localeOptions,
    setLocale,
    t,
  }
}

export function resetLocaleForTest(): void {
  changeSequence += 1
  activeLocale.value = DEFAULT_LOCALE
  activeMessages.value = zhCNMessages
  initialized = false
}
