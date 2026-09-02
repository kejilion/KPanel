import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const read = (path: string) => readFileSync(new URL(path, import.meta.url), 'utf8')
const desktopStyles = read('../styles/desktop.css')
const view = (name: string) => read(`./${name}.vue`)

// Regression contract for the "narrow desktop window wastes vertical space at
// the top of the page" defect.
//
// A desktop window declares `container-name: desktop-window` on its body, so a
// 380px-wide window is a container-query context inside a wide viewport. Every
// `@media (max-width: ...)` phone rule in a view is therefore inert there, and
// the three hero regions were laid out by desktop rules they no longer fitted:
// the counters stacked into four rows, ClusterView's refresh button was pushed
// off the left edge, and DockerView's identity block collapsed to 14px wide
// underneath its own status badge.
describe('narrow desktop-window hero contract', () => {
  it('lays the Apps and Cluster heroes out as wrapping flex rows', () => {
    // A wrapping flex row needs no width threshold: the actions drop to their
    // own row exactly when they stop fitting, in a window or a phone alike.
    // The previous `max-content minmax(0, 1fr)` grid could not do this, and
    // `justify-self: end` sent the overflow off the left edge.
    for (const [name, hero] of [['AppsView', 'market-hero'], ['ClusterView', 'cluster-hero']] as const) {
      expect(view(name)).toMatch(new RegExp(`\\.${hero}\\s*\\{[^}]*display:\\s*flex;[^}]*flex-wrap:\\s*wrap;`))
      expect(view(name)).toMatch(new RegExp(`\\.${hero}__actions\\s*\\{[^}]*flex:\\s*0 1 auto;[^}]*flex-wrap:\\s*wrap;`))
      expect(view(name)).not.toMatch(new RegExp(`\\.${hero}__actions\\s*\\{[^}]*justify-self:\\s*end;`))
    }
  })

  it('keeps the counter summaries two-up instead of stacking them in one column', () => {
    // These selectors must stay out of the single-column `:is(...)` group that
    // collapses card grids: four "number + label" cells in one column only
    // trades a too-wide hero for a very tall one.
    expect(desktopStyles).toMatch(
      /\.desktop-window__body :is\(\.market-stats, \.cluster-stats\)\s*\{[^}]*grid-template-columns:\s*repeat\(2, minmax\(0, 1fr\)\) !important;/,
    )
    expect(desktopStyles).not.toMatch(
      /:is\([^)]*\.(?:market|cluster)-stats[^)]*\)\s*\{\s*grid-template-columns:\s*minmax\(0, 1fr\) !important;/,
    )
  })

  it('moves the counter separators onto the wrapped row', () => {
    // Two-up wrapping puts cells 3-4 on a second row, so cell 3 must drop the
    // vertical rule on its left and both must rule off the new row — the same
    // correction the phone rules make.
    expect(desktopStyles).toMatch(
      /:is\(\.market-stats, \.cluster-stats\) > :nth-child\(3\)\s*\{[^}]*border-top:\s*1px solid var\(--border\);[^}]*border-left:\s*0;/,
    )
    expect(desktopStyles).toMatch(
      /:is\(\.market-stats, \.cluster-stats\) > :nth-child\(4\)\s*\{[^}]*border-top:\s*1px solid var\(--border\);/,
    )
  })

  it('mirrors the Docker command-center phone layout against the window container', () => {
    // The identity keeps a flex basis so it wraps rather than being squeezed
    // under the status badge, and the counters take the short third row so the
    // identity and actions can share the tall one.
    expect(view('DockerView')).toMatch(
      /\.docker-command-center__identity\s*\{[^}]*flex:\s*1 1 160px;/,
    )
    expect(view('DockerView')).toMatch(
      /@container desktop-window \(max-width: 560px\)[\s\S]*?\.docker-command-center__stats\s*\{[^}]*order:\s*3;[^}]*flex:\s*1 1 100%;/,
    )
    expect(view('DockerView')).toMatch(
      /@container desktop-window \(max-width: 560px\)[\s\S]*?\.docker-command-center__stat:first-child\s*\{[^}]*border-left:\s*0;/,
    )
  })

  it('keeps every hero label at or above the 12px floor', () => {
    // docs/ui-visual-language.md §2.1 sets 12px as the absolute lower bound for
    // semantic text, and §2.2 forbids regaining compactness by shrinking fonts.
    expect(view('ClusterView')).toMatch(/\.cluster-stats span\s*\{[^}]*font-size:\s*12px;/)
    expect(view('DockerView')).toMatch(/\.docker-command-center__identity small\s*\{[^}]*font-size:\s*\.75rem;/)
    expect(view('DockerView')).toMatch(/\.docker-command-center__stat small\s*\{[^}]*font-size:\s*\.75rem;/)
    expect(view('DockerView')).toMatch(/\.resource-section__heading small\s*\{[^}]*font-size:\s*\.75rem;/)
  })
})

// The same container-query gap one region lower down: the Docker resource
// section's header and empty state were also laid out by desktop rules inside a
// window a third of the viewport's width.
describe('narrow desktop-window resource section contract', () => {
  it('pairs the header column switch with a content-sized heading basis', () => {
    // `flex-direction: column` reinterprets the view's `flex: 1 1 190px` on the
    // heading as a 190px *height* basis, inflating a 38px title to 190px. The
    // phone rules pair the same column switch with `flex: 0 0 auto`; this block
    // had copied only half of that pair.
    expect(desktopStyles).toMatch(
      /@container desktop-window \(max-width: 820px\)[\s\S]*?\.desktop-window__body \.resource-section__header\s*\{[^}]*flex-direction:\s*column;/,
    )
    // The `__header >` step is load-bearing: a two-class selector only ties
    // with the view's scoped rules, which then win on source order.
    expect(desktopStyles).toMatch(
      /\.desktop-window__body \.resource-section__header > \.resource-section__heading,\s*\.desktop-window__body \.resource-section__header > \.resource-section__controls\s*\{[^}]*flex:\s*0 0 auto;/,
    )
  })

  it('lays the resource-section buttons out two-up instead of stacking them', () => {
    // The blanket `.card-actions { flex-direction: column }` in the same
    // container block stacks two buttons that need ~218px of a 316px track,
    // doubling the header height for nothing. The phone rules lay them out
    // two-up; this override matches them.
    expect(desktopStyles).toMatch(
      /@container desktop-window \(max-width: 580px\)[\s\S]*?\.desktop-window__body \.resource-section__controls > \.card-actions\s*\{[^}]*display:\s*grid;[^}]*grid-template-columns:\s*repeat\(2, minmax\(0, 1fr\)\);/,
    )
    // Without `flex-direction: row` the blanket column rule still applies to
    // the grid container, and without `min-width: 0` the buttons refuse to
    // shrink below their content width and overflow the track.
    expect(desktopStyles).toMatch(
      /\.desktop-window__body \.resource-section__controls > \.card-actions\s*\{[^}]*flex-direction:\s*row;/,
    )
    expect(desktopStyles).toMatch(
      /\.desktop-window__body \.resource-section__controls > \.card-actions > \*\s*\{[^}]*min-width:\s*0;/,
    )
  })

  it('compacts the empty state in a narrow window like the phone rule does', () => {
    // `.error-state` already had a narrow-window rule; `.empty-state` kept its
    // 250px/38px desktop framing. main.css compacts it to 220px/28px 16px on a
    // phone, which a window-width container can never reach on its own.
    expect(desktopStyles).toMatch(
      /@container desktop-window \(max-width: 580px\)[\s\S]*?\.desktop-window__body \.empty-state\s*\{[^}]*min-height:\s*220px;[^}]*padding:\s*28px 16px;/,
    )
  })
})
