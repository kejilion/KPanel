import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const source = readFileSync(new URL('./GalleryView.vue', import.meta.url), 'utf8')

/** The standalone rule for a selector, without grouped or responsive variants. */
function rule(selector: string): string {
  const start = source.lastIndexOf(`\n${selector} {`)
  expect(start, `${selector} should exist`).toBeGreaterThan(-1)
  return source.slice(start, source.indexOf('\n}', start))
}

describe('GalleryView layout contract', () => {
  it('lets the cover actions menu drop below the cover instead of being clipped by it', () => {
    // The menu hangs under the "更多操作" button, outside the cover's box, so the
    // cover itself must not clip; the photo layer rounds and clips on its own.
    expect(rule('.gallery-hero')).not.toMatch(/overflow:\s*hidden/)
    expect(rule('.gallery-hero')).toMatch(/z-index:\s*2;/)
    expect(rule('.gallery-hero__backdrop')).toMatch(/overflow:\s*hidden;/)
    expect(rule('.gallery-hero__backdrop')).toMatch(/border-radius:\s*inherit;/)
  })
})
