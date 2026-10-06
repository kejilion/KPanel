import { describe, expect, it } from 'vitest'
import {
  desktopPagerGrid,
  desktopPagerOrder,
  desktopPagerPageForScroll,
  layoutDesktopPager,
} from '@/lib/desktopPagerLayout'

describe('desktop pager layout', () => {
  it('sizes a phone grid from the measured work area', () => {
    expect(desktopPagerGrid({ width: 359, height: 704 })).toMatchObject({ columns: 4, rows: 6, pageCapacity: 24 })
    expect(desktopPagerGrid({ width: 655, height: 281 })).toMatchObject({ columns: 6, rows: 2, pageCapacity: 12 })
    // 200% zoom on a phone leaves too little room for three readable columns.
    expect(desktopPagerGrid({ width: 179, height: 298 })).toMatchObject({ columns: 2, rows: 2 })
    expect(desktopPagerGrid({ width: 0, height: 0 })).toMatchObject({ columns: 1, rows: 1, pageCapacity: 1 })
  })

  it('fills each page row-major and continues on the next page to the right', () => {
    const keys = Array.from({ length: 10 }, (_, index) => `nav:${index}`)
    const layout = layoutDesktopPager(keys, { width: 300, height: 216 })
    expect(layout.grid).toMatchObject({ columns: 3, rows: 2, cellWidth: 100, cellHeight: 108 })
    expect(layout.pageCount).toBe(2)
    expect(layout.placements.map(({ page, row, column }) => [page, row, column])).toEqual([
      [0, 0, 0], [0, 0, 1], [0, 0, 2],
      [0, 1, 0], [0, 1, 1], [0, 1, 2],
      [1, 0, 0], [1, 0, 1], [1, 0, 2],
      [1, 1, 0],
    ])
    // Icons are centered in their cells; later pages are offset by whole page widths.
    expect(layout.placements[0]).toMatchObject({ left: 9, top: 9.5 })
    expect(layout.placements[6]).toMatchObject({ left: 309, top: 9.5 })
    expect(layoutDesktopPager([], { width: 300, height: 216 }).pageCount).toBe(1)
  })

  it('keeps the saved wide column order and appends unsaved keys in catalog order', () => {
    const order = desktopPagerOrder(
      ['nav:a', 'nav:b', 'app:new', 'nav:c', 'group:g', 'nav:d', 'nav:a'],
      {
        'nav:a': { x: 0.5, y: 0 },
        'nav:b': { x: 0, y: 1.4 },
        'nav:c': { x: 0.005, y: 0.2 },
        'group:g': { x: 0, y: 0.6 },
        'nav:d': { x: 0.5, y: Number.NaN },
      },
    )
    // nav:c sits in the same visual column as x=0 even though its saved x drifted slightly.
    expect(order).toEqual(['nav:c', 'group:g', 'nav:b', 'nav:a', 'app:new', 'nav:d'])
  })

  it('maps scroll offsets to the nearest existing page', () => {
    expect(desktopPagerPageForScroll(0, 360, 3)).toBe(0)
    expect(desktopPagerPageForScroll(170, 360, 3)).toBe(0)
    expect(desktopPagerPageForScroll(190, 360, 3)).toBe(1)
    expect(desktopPagerPageForScroll(5000, 360, 3)).toBe(2)
    expect(desktopPagerPageForScroll(-40, 360, 3)).toBe(0)
    expect(desktopPagerPageForScroll(400, 0, 3)).toBe(0)
    expect(desktopPagerPageForScroll(400, 360, 1)).toBe(0)
  })
})
