import { describe, expect, it } from 'vitest'
import type { OfficeDocument } from '@/types/api'
import {
  cellName, columnName, documentTextStyle, indexOfficeDocument, isNumericText, OFFICE_PAGE_ITEMS, parseCellReference,
  singleLineText, slideBoxStyle, slideFontRatio, slideMetrics, textAlign, thumbnailBars,
} from './officeDocument'

describe('office document helpers', () => {
  it('converts between column numbers and A1 references within Excel bounds', () => {
    expect([1, 26, 27, 702, 703, 16384].map(columnName)).toEqual(['A', 'Z', 'AA', 'ZZ', 'AAA', 'XFD'])
    expect(cellName(12, 2)).toBe('B12')
    expect(parseCellReference(' b12 ')).toEqual({ row: 12, column: 2 })
    expect(parseCellReference('XFD1048576')).toEqual({ row: 1048576, column: 16384 })
    expect(parseCellReference('XFE1')).toBeUndefined()
    expect(parseCellReference('A0')).toBeUndefined()
    expect(parseCellReference('12')).toBeUndefined()
  })
  it('detects numeric cell text for right alignment', () => {
    expect(['2', '-3.5', '1,234.50', '45%', '.5'].every(isNumericText)).toBe(true)
    expect(['', '2026-10-09', '1.2.3', 'abc', '%'].some(isNumericText)).toBe(false)
  })
  it('flattens line breaks and tabs that Word and slide edits reject', () => {
    expect(singleLineText('a\r\nb\nc\td\re')).toBe('a b c d e')
  })
  it('indexes editable targets with readable locations and Word pages', () => {
    const doc: OfficeDocument = { entry: {} as OfficeDocument['entry'], kind: 'docx', contentVersion: '', notes: [], sections: [{ name: '', items: [
      ...Array.from({ length: OFFICE_PAGE_ITEMS }, (_, i) => ({ id: `p${i}`, kind: 'text' as const, text: '', editable: true })),
      { id: 'img', kind: 'image', text: '', editable: false },
      { id: 'tbl', kind: 'table', text: '', editable: false, table: [[{ id: 'c11', kind: 'text', text: '', editable: true }, { id: 'c12', kind: 'text', text: '', editable: true }]] },
    ] }] }
    const index = indexOfficeDocument(doc)
    expect(index.get('p0')?.label).toEqual({ kind: 'paragraph', n: 1 })
    expect(index.get('c12')).toMatchObject({ page: 1, label: { kind: 'tableCell', table: 1, row: 1, column: 2 } })
    expect(index.has('img')).toBe(false)
    expect(index.has('tbl')).toBe(false)
    const sheet = indexOfficeDocument({ ...doc, kind: 'xlsx', sections: [{ name: '', items: [{ id: 'x', kind: 'cell', text: '', editable: true, row: 3, column: 28 }] }] })
    expect(sheet.get('x')?.label).toEqual({ kind: 'cell', sheet: '1', cell: 'AB3' })
    const deck = indexOfficeDocument({ ...doc, kind: 'pptx', sections: [{ name: '1', items: [] }, { name: '2', items: [{ id: 's', kind: 'text', text: '', editable: true }] }] })
    expect(deck.get('s')).toMatchObject({ section: 1, label: { kind: 'slideText', slide: 2, n: 1 } })
  })
  it('maps Office alignment and keeps Word text readable', () => {
    expect(['ctr', 'r', 'both', 'start', undefined].map(textAlign)).toEqual(['center', 'right', 'justify', 'left', 'left'])
    expect(documentTextStyle({ kind: 'text', text: '', editable: true, fontSize: 24, bold: true })).toMatchObject({ fontSize: '32px', fontWeight: 700 })
    expect(documentTextStyle({ kind: 'text', text: '', editable: true, fontSize: 8 }).fontSize).toBe('14px')
  })
  it('scales slide geometry from EMU and keeps boxes on the slide', () => {
    const metrics = slideMetrics({ name: '', items: [], width: 12192000, height: 6858000 })
    expect(metrics.points).toBe(960)
    expect(slideMetrics({ name: '', items: [] })).toEqual({ width: 960, height: 540, points: 960 })
    const item = { kind: 'text' as const, text: 'Hello', editable: true, x: 6096000, y: 0, width: 9144000, height: 685800, fontSize: 24 }
    expect(slideBoxStyle(item, metrics)).toMatchObject({ left: '50%', top: '0%', width: '50%', height: '10%' })
    expect(slideFontRatio(item, metrics)).toBe(0.025)
  })
  it('draws thumbnail text as up to three wireframe bars', () => {
    const metrics = slideMetrics()
    const item = { kind: 'text' as const, text: '', editable: true, width: 960, fontSize: 20 }
    expect(thumbnailBars('  ', item, metrics)).toEqual([])
    expect(thumbnailBars('Hi', item, metrics)).toEqual([14])
    expect(thumbnailBars('长'.repeat(60), item, metrics)).toEqual([100, 25])
    expect(thumbnailBars('长'.repeat(500), item, metrics)).toEqual([100, 100, 100])
  })
})
