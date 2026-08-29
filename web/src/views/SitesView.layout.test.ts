import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const sitesSource = readFileSync(new URL('./SitesView.vue', import.meta.url), 'utf8')
const desktopStyles = readFileSync(new URL('../styles/desktop.css', import.meta.url), 'utf8')

describe('sites desktop window layout', () => {
  it('anchors website controls to the top instead of stretching grid rows', () => {
    expect(sitesSource).toContain('<div class="page sites-page">')
    expect(desktopStyles).toMatch(
      /\.desktop-window__body > \.sites-page\s*\{[^}]*align-content:\s*start;/,
    )
  })
})
