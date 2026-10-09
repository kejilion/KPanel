import type { CSSProperties } from 'vue'
import type { OfficeDocument, OfficeItem, OfficeSection } from '@/types/api'

// Rendering windows keep huge documents bounded: Word body is paged, sheets
// show a sliding 40 x 12 window and slides render one at a time.
export const OFFICE_PAGE_ITEMS = 80
export const OFFICE_SHEET_ROWS = 40
export const OFFICE_SHEET_COLUMNS = 12
export const OFFICE_MAX_EDITS = 256
export const OFFICE_MAX_EDIT_BYTES = 65536
const MAX_SHEET_ROWS = 1_048_576
const MAX_SHEET_COLUMNS = 16_384
const EMU_PER_POINT = 12700

export type OfficeLocationLabel =
  | { kind: 'paragraph'; n: number }
  | { kind: 'tableCell'; table: number; row: number; column: number }
  | { kind: 'cell'; sheet: string; cell: string }
  | { kind: 'slideText'; slide: number; n: number }

export interface OfficeLocation {
  item: OfficeItem
  section: number
  /** Word body page (OFFICE_PAGE_ITEMS items per page); 0 for sheets and slides. */
  page: number
  label: OfficeLocationLabel
}

export function columnName(column: number): string {
  let name = ''
  for (let n = column; n > 0; n = Math.floor((n - 1) / 26)) name = String.fromCharCode(65 + ((n - 1) % 26)) + name
  return name
}

export function cellName(row: number, column: number): string {
  return `${columnName(column)}${row}`
}

/** Parses an A1-style reference such as "b12"; returns undefined outside Excel bounds. */
export function parseCellReference(value: string): { row: number; column: number } | undefined {
  const match = /^\s*([A-Za-z]{1,3})\s*(\d{1,7})\s*$/.exec(value)
  if (!match) return undefined
  let column = 0
  for (const letter of match[1]!.toUpperCase()) column = column * 26 + letter.charCodeAt(0) - 64
  const row = Number(match[2])
  if (row < 1 || row > MAX_SHEET_ROWS || column < 1 || column > MAX_SHEET_COLUMNS) return undefined
  return { row, column }
}

/** Numbers, percentages and thousands-separated values align right like spreadsheet apps. */
export function isNumericText(text: string): boolean {
  return /^[-+]?(?:\d{1,3}(?:,\d{3})+|\d+)?(?:\.\d+)?%?$/.test(text.trim()) && /\d/.test(text)
}

/** Word and PowerPoint edits replace existing single-line text runs only. */
export function singleLineText(text: string): string {
  return text.replace(/\r\n|[\r\n\t]/g, ' ')
}

export function indexOfficeDocument(doc: OfficeDocument): Map<string, OfficeLocation> {
  const index = new Map<string, OfficeLocation>()
  doc.sections.forEach((section, sectionIndex) => {
    let paragraphs = 0, tables = 0, texts = 0
    section.items.forEach((item, itemIndex) => {
      const page = doc.kind === 'docx' ? Math.floor(itemIndex / OFFICE_PAGE_ITEMS) : 0
      if (item.kind === 'table') {
        tables++
        item.table?.forEach((cells, row) => cells.forEach((cell, column) => {
          if (cell.id) index.set(cell.id, { item: cell, section: sectionIndex, page,
            label: { kind: 'tableCell', table: tables, row: row + 1, column: column + 1 } })
        }))
        return
      }
      if (item.kind === 'image') return
      if (doc.kind === 'xlsx') {
        if (item.id) index.set(item.id, { item, section: sectionIndex, page,
          label: { kind: 'cell', sheet: section.name || String(sectionIndex + 1), cell: cellName(item.row ?? 1, item.column ?? 1) } })
        return
      }
      if (doc.kind === 'pptx') {
        texts++
        if (item.id) index.set(item.id, { item, section: sectionIndex, page, label: { kind: 'slideText', slide: sectionIndex + 1, n: texts } })
        return
      }
      paragraphs++
      if (item.id) index.set(item.id, { item, section: sectionIndex, page, label: { kind: 'paragraph', n: paragraphs } })
    })
  })
  return index
}

export function textAlign(align?: string): 'left' | 'center' | 'right' | 'justify' {
  switch (align) {
    case 'center': case 'ctr': return 'center'
    case 'right': case 'r': case 'end': return 'right'
    case 'both': case 'just': case 'justify': case 'distribute': case 'dist': return 'justify'
    default: return 'left'
  }
}

/** Word sizes arrive in points; render them in CSS px while keeping body text readable. */
export function documentTextStyle(item: OfficeItem): CSSProperties {
  const px = item.fontSize ? (item.fontSize * 4) / 3 : 15
  return {
    fontWeight: item.bold ? 700 : undefined,
    fontStyle: item.italic ? 'italic' : undefined,
    fontSize: `${Math.round(Math.max(14, Math.min(64, px)) * 10) / 10}px`,
    textAlign: textAlign(item.align),
  }
}

export interface SlideMetrics { width: number; height: number; points: number }

/** Slide geometry is EMU in real files (12700 per point); small mock values are already points. */
export function slideMetrics(section?: OfficeSection): SlideMetrics {
  const width = section?.width || 960, height = section?.height || 540
  return { width, height, points: width > 20_000 ? width / EMU_PER_POINT : width }
}

function percent(value: number, total: number): string {
  return `${Math.round(Math.max(0, Math.min(100, (value / total) * 100)) * 1000) / 1000}%`
}

export function slideBoxStyle(item: OfficeItem, metrics: SlideMetrics): CSSProperties {
  const x = item.x ?? 0, y = item.y ?? 0
  return {
    left: percent(x, metrics.width),
    top: percent(y, metrics.height),
    width: percent(Math.min(item.width ?? metrics.width, metrics.width - x), metrics.width),
    height: percent(Math.min(item.height ?? metrics.height, metrics.height - y), metrics.height),
  }
}

/** Text size as a fraction of slide width; CSS multiplies it by the slide's container width. */
export function slideFontRatio(item: OfficeItem, metrics: SlideMetrics): number {
  return Math.round(((item.fontSize || 18) / metrics.points) * 100000) / 100000
}

/**
 * Thumbnails draw text as wireframe bars instead of unreadable miniature text.
 * Returns bar widths (percent of the text box) for up to three wrapped lines.
 */
export function thumbnailBars(text: string, item: OfficeItem, metrics: SlideMetrics): number[] {
  const trimmed = text.trim()
  if (!trimmed) return []
  let units = 0
  for (const char of trimmed) units += /[⺀-鿿가-힯豈-﫿＀-￯]/.test(char) ? 1 : 0.55
  const boxPoints = Math.max(1, ((item.width ?? metrics.width) / metrics.width) * metrics.points)
  const lineFraction = (units * (item.fontSize || 18)) / boxPoints
  const lines = Math.max(1, Math.min(3, Math.ceil(lineFraction)))
  return Array.from({ length: lines }, (_, index) => index < lines - 1 ? 100
    : Math.max(14, Math.min(100, Math.round((lineFraction - (lines - 1)) * 100))))
}

const imagePattern = /^data:image\/(?:png|jpeg|gif);base64,[A-Za-z0-9+/=]+$/
export function officeImageSource(item: OfficeItem): string | undefined {
  return imagePattern.test(item.image ?? '') ? item.image : undefined
}
