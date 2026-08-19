import { onUnmounted, ref, watch, type WatchStopHandle } from 'vue'
import { useI18n, type SupportedLocale } from '@/i18n'

export type PhraseEntry = readonly [source: string, translation: string]
export type PhraseCatalog = readonly PhraseEntry[]
export type PhraseCatalogLoader = (locale: SupportedLocale) => Promise<PhraseCatalog>

interface RegisteredCatalog {
  id: symbol
  entries: PhraseCatalog
}

interface PatternTranslation {
  expression: RegExp
  translation: string
}

const catalogs: RegisteredCatalog[] = []
const originalText = new WeakMap<Text, string>()
const translatedText = new WeakMap<Text, string>()
const originalAttributes = new WeakMap<Element, Map<string, string>>()
const translatedAttributes = new WeakMap<Element, Map<string, string>>()
const translatedAttributeNames = ['placeholder', 'title', 'aria-label'] as const
const ignoredSelector = [
  '[data-i18n-ignore]',
  '.xterm',
  '.terminal-output',
  '.terminal-screen',
  '.code-editor-surface',
  '.cm-editor',
  'pre',
  'code',
].join(',')

let rootElement: HTMLElement | null = null
let observer: MutationObserver | null = null
let enabled = false
let refreshQueued = false
let exactTranslations = new Map<string, string>()
let patternTranslations: PatternTranslation[] = []
export const phraseCatalogVersion = ref(0)

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function compilePattern(source: string, translation: string): PatternTranslation | null {
  if (!source.includes('{0}') && !/\{\d+\}/.test(source)) return null
  let cursor = 0
  let expression = '^'
  const tokenPattern = /\{(\d+)\}/g
  let match: RegExpExecArray | null
  while ((match = tokenPattern.exec(source))) {
    expression += escapeRegExp(source.slice(cursor, match.index))
    expression += '([\\s\\S]*?)'
    cursor = match.index + match[0].length
  }
  expression += `${escapeRegExp(source.slice(cursor))}$`
  return { expression: new RegExp(expression), translation }
}

function rebuildIndex(): void {
  const exact = new Map<string, string>()
  const patterns: PatternTranslation[] = []
  for (const catalog of catalogs) {
    for (const [source, translation] of catalog.entries) {
      if (!source || !translation || source === translation) continue
      const compiled = compilePattern(source, translation)
      if (compiled) patterns.push(compiled)
      else exact.set(source, translation)
    }
  }
  exactTranslations = exact
  patternTranslations = patterns.sort((left, right) => right.expression.source.length - left.expression.source.length)
  phraseCatalogVersion.value += 1
}

function interpolatePattern(template: string, captures: readonly string[]): string {
  return template.replace(/\{(\d+)\}/g, (token, rawIndex: string) => {
    const value = captures[Number(rawIndex)]
    return value === undefined ? token : value
  })
}

export function translatePhrase(value: string): string {
  if (!value || !/[\u3400-\u9fff]/.test(value)) return value
  const leading = value.match(/^\s*/)?.[0] || ''
  const trailing = value.match(/\s*$/)?.[0] || ''
  const content = value.slice(leading.length, value.length - trailing.length)
  const exact = exactTranslations.get(content)
  if (exact) return `${leading}${exact}${trailing}`
  for (const pattern of patternTranslations) {
    const match = pattern.expression.exec(content)
    if (match) return `${leading}${interpolatePattern(pattern.translation, match.slice(1))}${trailing}`
  }
  return value
}

function isIgnored(node: Node): boolean {
  const element = node instanceof Element ? node : node.parentElement
  return Boolean(element?.closest(ignoredSelector))
}

function translateTextNode(node: Text): void {
  if (isIgnored(node)) return
  const current = node.data
  const previousTranslation = translatedText.get(node)
  const source = previousTranslation === current ? originalText.get(node) || current : current
  const next = translatePhrase(source)
  originalText.set(node, source)
  translatedText.set(node, next)
  if (next !== current) node.data = next
}

function translateElementAttributes(element: Element): void {
  if (isIgnored(element)) return
  let originals = originalAttributes.get(element)
  let translations = translatedAttributes.get(element)
  if (!originals) {
    originals = new Map<string, string>()
    originalAttributes.set(element, originals)
  }
  if (!translations) {
    translations = new Map<string, string>()
    translatedAttributes.set(element, translations)
  }
  for (const name of translatedAttributeNames) {
    const current = element.getAttribute(name)
    if (current === null) continue
    const previousTranslation = translations.get(name)
    const source = previousTranslation === current ? originals.get(name) || current : current
    const next = translatePhrase(source)
    originals.set(name, source)
    translations.set(name, next)
    if (next !== current) element.setAttribute(name, next)
  }
}

function walkAndTranslate(root: Node): void {
  if (root instanceof Text) {
    translateTextNode(root)
    return
  }
  if (!(root instanceof Element) || isIgnored(root)) return
  translateElementAttributes(root)
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT)
  let node: Node | null
  while ((node = walker.nextNode())) {
    if (node instanceof Text) translateTextNode(node)
    else if (node instanceof Element) translateElementAttributes(node)
  }
}

function restoreChinese(root: Node): void {
  const restoreNode = (node: Node): void => {
    if (node instanceof Text) {
      const source = originalText.get(node)
      const translated = translatedText.get(node)
      if (source !== undefined && translated === node.data) node.data = source
      return
    }
    if (!(node instanceof Element)) return
    const originals = originalAttributes.get(node)
    const translations = translatedAttributes.get(node)
    if (!originals || !translations) return
    for (const [name, source] of originals) {
      if (node.getAttribute(name) === translations.get(name)) node.setAttribute(name, source)
    }
  }
  restoreNode(root)
  if (!(root instanceof Element)) return
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT)
  let node: Node | null
  while ((node = walker.nextNode())) restoreNode(node)
}

function scheduleRefresh(): void {
  if (!enabled || !rootElement || refreshQueued) return
  refreshQueued = true
  queueMicrotask(() => {
    refreshQueued = false
    if (enabled && rootElement) walkAndTranslate(rootElement)
  })
}

function startObserver(): void {
  if (!rootElement || observer) return
  observer = new MutationObserver((mutations) => {
    for (const mutation of mutations) {
      if (mutation.type === 'characterData') {
        translateTextNode(mutation.target as Text)
        continue
      }
      if (mutation.type === 'attributes' && mutation.target instanceof Element) {
        translateElementAttributes(mutation.target)
        continue
      }
      for (const node of mutation.addedNodes) walkAndTranslate(node)
    }
  })
  observer.observe(rootElement, {
    subtree: true,
    childList: true,
    characterData: true,
    attributes: true,
    attributeFilter: [...translatedAttributeNames],
  })
}

function stopObserver(): void {
  observer?.disconnect()
  observer = null
}

export function installPhraseLocalization(root: HTMLElement): WatchStopHandle {
  rootElement = root
  const { locale } = useI18n()
  return watch(
    locale,
    (nextLocale) => {
      enabled = nextLocale !== 'zh-CN'
      if (enabled) {
        startObserver()
        scheduleRefresh()
      } else {
        stopObserver()
        restoreChinese(root)
      }
    },
    { immediate: true },
  )
}

export function registerPhraseCatalog(entries: PhraseCatalog): () => void {
  const id = Symbol('phrase-catalog')
  catalogs.push({ id, entries })
  rebuildIndex()
  scheduleRefresh()
  return () => {
    const index = catalogs.findIndex((catalog) => catalog.id === id)
    if (index < 0) return
    catalogs.splice(index, 1)
    rebuildIndex()
  }
}

export function usePhraseCatalog(loader: PhraseCatalogLoader): void {
  const { locale } = useI18n()
  let active = true
  const loadedEntries = new Map<SupportedLocale, PhraseCatalog>()
  let sequence = 0
  let disposeCatalog: (() => void) | null = null

  const unregister = (): void => {
    disposeCatalog?.()
    disposeCatalog = null
  }

  const stop = watch(
    locale,
    async (nextLocale) => {
      const request = ++sequence
      if (nextLocale !== 'en-US') {
        if (nextLocale !== 'zh-TW') {
          unregister()
          return
        }
      }
      unregister()
      const cachedEntries = loadedEntries.get(nextLocale)
      if (cachedEntries) {
        disposeCatalog = registerPhraseCatalog(cachedEntries)
        return
      }
      const entries = await loader(nextLocale)
      if (!active || request !== sequence) return
      loadedEntries.set(nextLocale, entries)
      disposeCatalog = registerPhraseCatalog(entries)
    },
    { immediate: true },
  )

  onUnmounted(() => {
    active = false
    sequence += 1
    stop()
    unregister()
  })
}

export function resetPhraseLocalizationForTest(): void {
  stopObserver()
  catalogs.splice(0, catalogs.length)
  rebuildIndex()
  rootElement = null
  enabled = false
  refreshQueued = false
  phraseCatalogVersion.value += 1
}
