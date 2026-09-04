import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const filesSource = readFileSync(new URL('./FilesView.vue', import.meta.url), 'utf8')
const desktopStyles = readFileSync(new URL('../styles/desktop.css', import.meta.url), 'utf8')

describe('files desktop window layout', () => {
  it('anchors the page grid when the in-window batch bar appears', () => {
    expect(filesSource).toContain('<section ref="filesPage" class="files-page"')
    expect(filesSource).toMatch(/<Transition name="batch-dock">[\s\S]*class="batch-bar"/)
    expect(desktopStyles).toMatch(
      /\.desktop-window__body > \.files-page\s*\{[^}]*align-content:\s*start;/,
    )
  })
})
