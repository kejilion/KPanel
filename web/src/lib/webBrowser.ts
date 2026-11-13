import type { InjectionKey, Ref } from 'vue'
import type { DesktopEntry } from './desktopEntries'

export const BROWSER_MAX_TABS = 8

export type BrowserComponentAction = 'install' | 'start' | 'stop' | 'restart' | 'uninstall'
export type BrowserComponentKind = 'chromium' | 'tor'
export interface BrowserComponentJob { id: string; action: BrowserComponentAction; state: string; stage: string; message?: string }
export interface BrowserComponent { supported: boolean; reason?: string; kind?: BrowserComponentKind; torSupported?: boolean; installed: boolean; running: boolean; healthy: boolean; image: string; resourceVersion: string; job?: BrowserComponentJob }

export interface BrowserTab { id: string; url: string; title: string; loading: boolean; back: boolean; forward: boolean }
export interface BrowserEvent { type: string; tab?: string; active?: string; tabs?: BrowserTab[]; image?: string; width?: number; height?: number; code?: string; text?: string }
export interface BrowserLink { id: string; name: string; url: string; iconURL?: string }
export function browserStartLinks(entries: DesktopEntry[]): BrowserLink[] {
  return entries.flatMap(entry => entry.launch === 'external' && entry.url && browserInput(entry.url)
    ? [{ id: entry.key, name: entry.name, url: entry.url, iconURL: entry.iconURL }] : [])
}
export const browserLinksKey: InjectionKey<Readonly<Ref<BrowserLink[]>>> = Symbol('browser-links')
export interface BrowserLaunch { url: string; request: string }
export const browserLaunchKey: InjectionKey<Readonly<Ref<BrowserLaunch | undefined>>> = Symbol('browser-launch')

// Wire limits are UTF-8 bytes, while textarea maxlength counts UTF-16 units.
export function browserTextChunks(value: string): string[] {
  const chunks: string[] = []
  let text = '', bytes = 0, encodedBytes = 0
  const encoder = new TextEncoder()
  for (const char of value.slice(0, 8192)) {
    const size = encoder.encode(char).length
    const escapedSize = encoder.encode(JSON.stringify(char).slice(1, -1)).length
    if (bytes + size > 8192 || encodedBytes + escapedSize > 16 * 1024 - 128) {
      chunks.push(text); text = ''; bytes = 0; encodedBytes = 0
    }
    text += char; bytes += size; encodedBytes += escapedSize
  }
  if (text) chunks.push(text)
  return chunks
}

export function browserInput(raw: string): string | undefined {
  const value = raw.trim()
  if (!value || value.length > 4096) return undefined
  if (/^[a-z][a-z\d+.-]*:\/\//i.test(value) && !/^https?:\/\//i.test(value)) return undefined
  const isURL = /^https?:\/\//i.test(value) || (!/\s/.test(value) && /[.:[\]]|^localhost(?:[/:]|$)/.test(value))
  if (!isURL) {
    if (/^[a-z][a-z\d+.-]*:/i.test(value)) return undefined
    return `https://www.bing.com/search?q=${encodeURIComponent(value)}`
  }
  try {
    const url = new URL(/^https?:\/\//i.test(value) ? value : `https://${value}`)
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) return undefined
    return url.href
  } catch { return undefined }
}

export function browserCoordinates(x: number, y: number, rect: Pick<DOMRect, 'left' | 'top' | 'width' | 'height'>, width: number, height: number) {
  if (rect.width <= 0 || rect.height <= 0) return undefined
  return { x: Math.max(0, Math.min(width, (x - rect.left) * width / rect.width)), y: Math.max(0, Math.min(height, (y - rect.top) * height / rect.height)) }
}

export function browserEvent(raw: unknown): BrowserEvent | undefined {
  if (typeof raw !== 'string' || raw.length > 2 * 1024 * 1024) return undefined
  try {
    const event: BrowserEvent = JSON.parse(raw)
    if (!event || typeof event !== 'object') return undefined
    if (event.type === 'state' && Array.isArray(event.tabs) && event.tabs.length <= BROWSER_MAX_TABS && typeof event.active === 'string'
      && event.tabs.every(tab => typeof tab.id === 'string' && tab.id.length <= 64 && typeof tab.url === 'string' && tab.url.length <= 4096 && typeof tab.title === 'string' && tab.title.length <= 160)) return event
    if (event.type === 'frame' && typeof event.tab === 'string' && typeof event.image === 'string' && /^[A-Za-z0-9+/=]+$/.test(event.image)
      && Number.isInteger(event.width) && Number.isInteger(event.height) && event.width! >= 1 && event.width! <= 1600 && event.height! >= 1 && event.height! <= 1000) return event
    if (['error', 'dialog'].includes(event.type) && typeof event.code === 'string' && event.code.length <= 80 && (!event.text || typeof event.text === 'string' && event.text.length <= 2048)) return event
  } catch { /* Malformed service output is not executable page content. */ }
  return undefined
}
