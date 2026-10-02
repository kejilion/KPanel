import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const source = readFileSync(new URL('./GalleryViewer.vue', import.meta.url), 'utf8')

/** The standalone rule for a selector; grouped rules list it after a comma instead. */
function rule(selector: string): string {
  const start = source.lastIndexOf(`\n${selector} {`)
  expect(start, `${selector} should exist`).toBeGreaterThan(-1)
  return source.slice(start, source.indexOf('\n}', start))
}

describe('GalleryViewer layout contract', () => {
  it('gives the stage one definite cell so wide photos fit by height as well as width', () => {
    // An auto row let percentage max-height fall back to the natural height,
    // which cropped landscape photos in wide windows.
    expect(rule('.gallery-viewer__stage')).toMatch(/grid-template:\s*minmax\(0, 1fr\) \/ minmax\(0, 1fr\);/)
    const media = rule('.gallery-viewer__media')
    expect(media).toMatch(/max-width:\s*calc\(100% - 48px\);/)
    expect(media).toMatch(/max-height:\s*calc\(100% - 24px\);/)
    expect(media).toMatch(/object-fit:\s*contain;/)
  })

  it('lets the loading preview occupy the same box as the original', () => {
    const preview = rule('.gallery-viewer__media--preview')
    expect(preview).toMatch(/width:\s*calc\(100% - 48px\);/)
    expect(preview).toMatch(/height:\s*calc\(100% - 24px\);/)
  })
})
