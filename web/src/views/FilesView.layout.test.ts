import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const filesSource = readFileSync(new URL('./FilesView.vue', import.meta.url), 'utf8')
const desktopStyles = readFileSync(new URL('../styles/desktop.css', import.meta.url), 'utf8')

describe('files desktop window layout', () => {
  it('anchors the page grid when the in-window batch bar appears', () => {
    expect(filesSource).toContain('class="files-page"')
    expect(filesSource).toMatch(/<Transition name="batch-dock">[\s\S]*class="batch-bar"/)
    expect(desktopStyles).toMatch(
      /\.desktop-window__body > \.files-page\s*\{[^}]*align-content:\s*start;/,
    )
  })

  it('reserves a scrollable safe area behind the fixed phone batch bar', () => {
    expect(filesSource).toContain(":class=\"{ 'files-page--batch-active': selected.size > 0 }\"")
    expect(filesSource).toMatch(
      /@media \(max-width: 720px\)[\s\S]*?\.files-page--batch-active\s*\{[^}]*padding-bottom:\s*200px;/,
    )
    expect(filesSource).toMatch(
      /@media \(max-width: 480px\)[\s\S]*?\.files-page--batch-active\s*\{[^}]*padding-bottom:\s*126px;/,
    )
    expect(filesSource).toMatch(
      /\.batch-bar__actions\s*\{[^}]*display:\s*flex;[^}]*overflow-x:\s*auto;[^}]*scroll-snap-type:\s*x proximity;/,
    )
  })
})
