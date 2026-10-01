import type { Component } from 'vue'

/**
 * Taskbar start menu model. The menu never owns a catalogue: system apps come
 * from `desktopApps`, apps/sites/shortcuts from the same desktop entry sources
 * (hidden ones included), and actions reuse existing desktop commands.
 */

export type DesktopStartMenuSection = 'system' | 'entries' | 'actions'

export const DESKTOP_START_MENU_SECTIONS: readonly DesktopStartMenuSection[] = ['system', 'entries', 'actions']

export interface DesktopStartMenuItem {
  /** Desktop stable key (`nav:`, `app:`, `site:`, `shortcut:`) or `action:<id>`. */
  key: string
  section: DesktopStartMenuSection
  label: string
  /** Short secondary text shown in list rows, such as the entry type. */
  detail?: string
  /** Searchable terms that are not displayed, such as route paths, URLs or English aliases. */
  keywords?: readonly string[]
  iconURL?: string
  icon?: Component
  gradient?: string
  /** An installed app or site the administrator removed from the desktop. */
  hidden?: boolean
}

export function normalizeStartMenuQuery(value: string): string {
  return value.normalize('NFKC').trim().toLocaleLowerCase().replace(/\s+/g, ' ')
}

function matchRank(item: DesktopStartMenuItem, query: string, tokens: readonly string[]): number | undefined {
  const label = normalizeStartMenuQuery(item.label)
  const terms = [item.detail, ...(item.keywords ?? [])]
    .filter((term): term is string => Boolean(term))
    .map(normalizeStartMenuQuery)
  const haystack = [label, ...terms]
  if (!tokens.every((token) => haystack.some((value) => value.includes(token)))) return undefined
  if (label.startsWith(query)) return 0
  if (label.includes(query)) return 1
  if (terms.some((term) => term.startsWith(query))) return 2
  return 3
}

/**
 * Empty queries show the launcher view (system apps and entries, no actions).
 * A query searches every section; sections keep their order and each section
 * lists label-prefix matches first, then other label matches, then term matches.
 */
export function searchDesktopStartMenu(
  items: readonly DesktopStartMenuItem[],
  query: string,
): DesktopStartMenuItem[] {
  const normalized = normalizeStartMenuQuery(query)
  const tokens = normalized.split(' ')
  return items
    .map((item, index) => ({
      item,
      index,
      rank: normalized ? matchRank(item, normalized, tokens) : item.section === 'actions' ? undefined : 0,
    }))
    .filter((match): match is { item: DesktopStartMenuItem; index: number; rank: number } => match.rank !== undefined)
    .sort((left, right) =>
      DESKTOP_START_MENU_SECTIONS.indexOf(left.item.section) - DESKTOP_START_MENU_SECTIONS.indexOf(right.item.section)
      || left.rank - right.rank
      || left.index - right.index)
    .map((match) => match.item)
}

/** Next active index for arrow keys; the launcher grid moves by rows vertically. */
export function moveStartMenuIndex(
  current: number,
  key: 'ArrowUp' | 'ArrowDown' | 'ArrowLeft' | 'ArrowRight' | 'Home' | 'End',
  total: number,
  grid: { count: number; columns: number } = { count: 0, columns: 1 },
): number {
  if (total <= 0) return -1
  const last = total - 1
  const index = Math.min(Math.max(current, 0), last)
  const columns = Math.max(1, grid.columns)
  const inGrid = index < grid.count
  switch (key) {
    case 'Home': return 0
    case 'End': return last
    case 'ArrowLeft': return Math.max(0, index - 1)
    case 'ArrowRight': return Math.min(last, index + 1)
    case 'ArrowDown':
      if (!inGrid) return Math.min(last, index + 1)
      if (index + columns < grid.count) return index + columns
      // Leave the grid for the first list row; a grid-only menu keeps its column.
      return grid.count <= last ? grid.count : index
    case 'ArrowUp':
      if (inGrid) return index - columns >= 0 ? index - columns : index
      if (index === grid.count && grid.count > 0) {
        // Return to the last grid row, aligned to its first column.
        return Math.floor((grid.count - 1) / columns) * columns
      }
      return Math.max(0, index - 1)
  }
}
