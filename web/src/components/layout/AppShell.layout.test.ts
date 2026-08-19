import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const styles = readFileSync(new URL('../../styles/main.css', import.meta.url), 'utf8')
const appShellSource = readFileSync(new URL('./AppShell.vue', import.meta.url), 'utf8')
const sitesSource = readFileSync(new URL('../../views/SitesView.vue', import.meta.url), 'utf8')
const jobsSource = readFileSync(new URL('../../views/JobsView.vue', import.meta.url), 'utf8')

describe('responsive application shell comfort', () => {
  it('keeps the mobile navigation usable on short screens', () => {
    expect(styles).toMatch(
      /\.sidebar__nav\s*\{[^}]*min-height:\s*0;[^}]*flex:\s*1 1 auto;[^}]*overflow-x:\s*hidden;[^}]*overflow-y:\s*auto;[^}]*overscroll-behavior:\s*contain;/,
    )
    expect(styles).toMatch(/@media \(max-width: 920px\)[\s\S]*?\.sidebar\s*\{[^}]*height:\s*100dvh;/)
  })

  it('uses the available phone width without reserving a desktop scrollbar gutter', () => {
    expect(styles).toMatch(/@media \(max-width: 920px\)[\s\S]*?html\s*\{[^}]*scrollbar-gutter:\s*auto;/)
    expect(styles).toContain('env(safe-area-inset-bottom)')
  })

  it('keeps monitoring cards compact without forcing a single long column', () => {
    expect(styles).toMatch(
      /@media \(min-width: 360px\) and \(max-width: 680px\)[\s\S]*?\.metric-grid,[\s\S]*?grid-template-columns:\s*repeat\(2, minmax\(0, 1fr\)\);/,
    )
    expect(styles).toMatch(/\.metric-grid > :last-child:nth-child\(odd\)[\s\S]*?grid-column:\s*1 \/ -1;/)
  })

  it('keeps the first table column visible during horizontal scrolling', () => {
    expect(styles).toMatch(
      /@media \(max-width: 680px\)[\s\S]*?\.data-table th:first-child,[\s\S]*?\.data-table td:first-child\s*\{[^}]*position:\s*sticky;[^}]*left:\s*0;/,
    )
  })

  it('raises authentication content and balances bottom-sheet actions on phones', () => {
    expect(styles).toMatch(
      /@media \(max-width: 680px\)[\s\S]*?\.auth-layout__form\s*\{[^}]*place-items:\s*start center;[^}]*padding:\s*clamp\(96px, 15vh, 132px\)/,
    )
    expect(styles).toMatch(/\.modal-panel__footer \.button\s*\{[^}]*flex:\s*1 1 0;/)
  })

  it('keeps mobile search and filters in a compact two-row toolbar', () => {
    expect(sitesSource).toContain('class="toolbar-card toolbar-card--search-tabs"')
    expect(jobsSource).toContain('class="toolbar-card toolbar-card--search-tabs"')
    expect(styles).toMatch(
      /\.toolbar-card--search-tabs\s*\{[^}]*display:\s*grid;[^}]*grid-template-columns:\s*minmax\(0, 1fr\) auto;/,
    )
  })

  it('avoids persistent blur and broad transitions in the shared shell', () => {
    expect(styles).not.toMatch(/\.topbar\s*\{[^}]*backdrop-filter:/)
    expect(styles).not.toContain('transition: all')
    expect(styles).toContain('touch-action: manipulation')
  })

  it('unmounts and deactivates the classic shell while desktop mode is open', () => {
    expect(appShellSource).toContain("const desktopActive = computed(() => desktop.mode.value === 'desktop')")
    expect(appShellSource.match(/v-show="!desktopActive"/g)).toHaveLength(2)
    expect(appShellSource).toContain(':inert="desktopActive ? true : undefined"')
    expect(appShellSource).toContain(':aria-hidden="desktopActive ? \'true\' : undefined"')
    expect(appShellSource).toContain('<RouterView v-if="!desktopActive" />')
    expect(appShellSource).toContain('@click="enterDesktopSafely"')
    expect(appShellSource).toContain('desktopCloseGuardCoordinator.checkAll()')
    expect(appShellSource).not.toContain('documentFullscreen')
  })

  it('keeps section navigation active on nested workspace routes', () => {
    expect(appShellSource).toContain(
      "'router-link-active': route.path === item.to || route.path.startsWith(`${item.to}/`)",
    )
  })

  it('falls back to the classic shell when the lazy desktop chunk cannot load', () => {
    expect(appShellSource).toContain('loadingComponent: DesktopLoadingView')
    expect(appShellSource).toContain('delay: 0')
    expect(appShellSource).toContain("class: 'desktop'")
    expect(appShellSource).toContain("class: 'desktop__wallpaper'")
    expect(appShellSource).toContain('onError(_error, retry, fail, attempts)')
    expect(appShellSource).toContain('desktop.enterClassic()')
    expect(appShellSource).toContain("toast.danger(i18n.t('nav.loadFailedTitle'), i18n.t('nav.loadFailedMessage'))")
  })

  it('makes the desktop entry prominent and clears its one-time notice after use', () => {
    expect(appShellSource).toContain('class="icon-button desktop-entry-button"')
    expect(appShellSource).toContain("'desktop-entry-button--unseen': !desktopEntrySeen")
    expect(appShellSource).toContain('markDesktopEntrySeen()')
    expect(appShellSource).toContain("'kpanel:desktop-entry-notice:v2'")
    expect(styles).toContain('.desktop-entry-button__notice')
    expect(styles).toContain('@keyframes desktop-entry-attention')
    expect(styles).toMatch(/\.topbar \.desktop-entry-button\s*\{[^}]*align-items:\s*center;[^}]*justify-content:\s*center;/)
  })
})
