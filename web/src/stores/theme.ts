import { computed, ref } from 'vue'
import {
  DEFAULT_THEME_COLORS,
  THEME_TOKEN_NAMES,
  deriveThemeTokens,
  normalizeThemeColors,
  parseStoredThemeColors,
  serializeThemeColors,
  type ThemeColorIntent,
} from '@/theme/colors'

export type ThemePreference = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'kejilion-panel-theme'
const COLOR_STORAGE_KEY = 'kejilion-panel-colors'
const THEME_TRANSITION_MS = 460

const preference = ref<ThemePreference>('system')
const colors = ref<ThemeColorIntent>({ ...DEFAULT_THEME_COLORS })
const customColors = ref(false)
const systemDark = ref(false)
let mediaQuery: MediaQueryList | undefined
let storageListenerAttached = false
let transitionTimer: number | undefined

function isThemePreference(value: string | null): value is ThemePreference {
  return value === 'light' || value === 'dark' || value === 'system'
}

function resolvedTheme(): 'light' | 'dark' {
  if (preference.value !== 'system') return preference.value
  return systemDark.value ? 'dark' : 'light'
}

function beginThemeTransition(): void {
  if (typeof window === 'undefined' || typeof document === 'undefined') return
  const root = document.documentElement
  if (transitionTimer !== undefined) window.clearTimeout(transitionTimer)
  root.classList.add('desktop-theme-transitioning')
  transitionTimer = window.setTimeout(() => {
    root.classList.remove('desktop-theme-transitioning')
    transitionTimer = undefined
  }, THEME_TRANSITION_MS)
}

function clearAppliedColorTokens(): void {
  if (typeof document === 'undefined') return
  for (const token of THEME_TOKEN_NAMES) document.documentElement.style.removeProperty(token)
}

function applyTheme(animate = false): void {
  if (typeof document === 'undefined') return
  if (animate) beginThemeTransition()
  const mode = resolvedTheme()
  const root = document.documentElement
  root.dataset.theme = mode
  root.style.colorScheme = mode
  clearAppliedColorTokens()
  if (customColors.value) {
    const tokens = deriveThemeTokens(colors.value, mode)
    for (const token of THEME_TOKEN_NAMES) root.style.setProperty(token, tokens[token])
  }
  try {
    const style = getComputedStyle(root)
    const tokens = Object.fromEntries(THEME_TOKEN_NAMES
      .filter(token => token.startsWith('--desktop-wallpaper-') || token.startsWith('--desktop-aurora-'))
      .map(token => [token, style.getPropertyValue(token).trim()]))
    window.sessionStorage.setItem('kpanel:desktop-backdrop:v1', JSON.stringify({
      theme: mode, colors: readPreference(COLOR_STORAGE_KEY), tokens,
    }))
  } catch { /* Optional per-tab paint cache must not affect theme selection. */ }
}

function readPreference(key: string): string | null {
  if (typeof window === 'undefined') return null
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

function persistPreference(key: string, value: string): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(key, value)
  } catch {
    // Browser privacy modes can disable storage without disabling theming.
  }
}

function removePreference(key: string): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.removeItem(key)
  } catch {
    // Keep the in-memory preference usable when storage is unavailable.
  }
}

function restoreColors(): void {
  const raw = readPreference(COLOR_STORAGE_KEY)
  const stored = parseStoredThemeColors(raw)
  if (stored) {
    colors.value = stored
    customColors.value = true
  } else {
    colors.value = { ...DEFAULT_THEME_COLORS }
    customColors.value = false
    if (raw !== null) removePreference(COLOR_STORAGE_KEY)
  }
}

function resetColorsInMemory(): void {
  colors.value = { ...DEFAULT_THEME_COLORS }
  customColors.value = false
}

function handleStorageChange(event: StorageEvent): void {
  if (event.key === null) {
    preference.value = 'system'
    resetColorsInMemory()
    applyTheme(true)
    return
  }
  if (event.key === STORAGE_KEY) {
    preference.value = isThemePreference(event.newValue) ? event.newValue : 'system'
    applyTheme(true)
    return
  }
  if (event.key === COLOR_STORAGE_KEY) {
    const next = parseStoredThemeColors(event.newValue)
    if (next) {
      colors.value = next
      customColors.value = true
    } else {
      resetColorsInMemory()
    }
    applyTheme(true)
    return
  }
}

function handleSystemThemeChange(event: MediaQueryListEvent): void {
  systemDark.value = event.matches
  applyTheme(true)
}

export function initializeTheme(): void {
  mediaQuery?.removeEventListener('change', handleSystemThemeChange)
  mediaQuery = undefined
  const storedTheme = readPreference(STORAGE_KEY)
  preference.value = isThemePreference(storedTheme) ? storedTheme : 'system'
  restoreColors()

  systemDark.value = false
  if (typeof window !== 'undefined') {
    if (typeof window.matchMedia === 'function') {
      mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
      systemDark.value = mediaQuery.matches
      mediaQuery.addEventListener('change', handleSystemThemeChange)
    }
    if (!storageListenerAttached) {
      window.addEventListener('storage', handleStorageChange)
      storageListenerAttached = true
    }
  }
  applyTheme(false)
}

export function resetThemeForTest(): void {
  mediaQuery?.removeEventListener('change', handleSystemThemeChange)
  mediaQuery = undefined
  if (typeof window !== 'undefined' && storageListenerAttached) window.removeEventListener('storage', handleStorageChange)
  storageListenerAttached = false
  if (typeof window !== 'undefined' && transitionTimer !== undefined) window.clearTimeout(transitionTimer)
  transitionTimer = undefined
  preference.value = 'system'
  resetColorsInMemory()
  systemDark.value = false
  if (typeof document === 'undefined') return
  delete document.documentElement.dataset.theme
  document.documentElement.classList.remove('desktop-theme-transitioning')
  document.documentElement.style.removeProperty('color-scheme')
  clearAppliedColorTokens()
}

export function useTheme() {
  const setTheme = (value: ThemePreference) => {
    preference.value = value
    persistPreference(STORAGE_KEY, value)
    applyTheme(true)
  }

  const setColors = (value: ThemeColorIntent) => {
    const next = normalizeThemeColors(value)
    colors.value = next
    customColors.value = true
    persistPreference(COLOR_STORAGE_KEY, serializeThemeColors(next))
    applyTheme(true)
  }

  const resetColors = () => {
    resetColorsInMemory()
    removePreference(COLOR_STORAGE_KEY)
    applyTheme(true)
  }

  return {
    preference: computed(() => preference.value),
    resolved: computed(resolvedTheme),
    colors: computed(() => colors.value),
    isCustom: computed(() => customColors.value),
    setTheme,
    setColors,
    resetColors,
  }
}

export type { ThemeColorIntent } from '@/theme/colors'
