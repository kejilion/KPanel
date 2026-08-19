import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const styles = readFileSync(new URL('../../styles/desktop.css', import.meta.url), 'utf8')
const windowSource = readFileSync(new URL('./DesktopWindow.vue', import.meta.url), 'utf8')
const desktopViewSource = readFileSync(new URL('./DesktopView.vue', import.meta.url), 'utf8')

describe('desktop visual and interaction contract', () => {
  it('keeps desktop chrome, windows, fullscreen views and teleports in a stable layer order', () => {
    expect(styles).toMatch(/\.desktop\s*\{[^}]*z-index:\s*1000;/)
    expect(styles).toContain('z-index: 1200;')
    expect(styles).toContain('z-index: 2800 !important;')
    expect(styles).toContain('z-index: 5000 !important;')
    expect(styles).toContain('z-index: 5200 !important;')
  })

  it('removes the root scrollbar gutter while desktop mode owns the viewport', () => {
    expect(styles).toMatch(/html\.desktop-mode-open\s*\{[^}]*scrollbar-gutter:\s*auto;[^}]*scrollbar-width:\s*none;/)
    expect(styles).toContain('html.desktop-mode-open::-webkit-scrollbar')
    expect(styles).toMatch(/\.desktop\s*\{[^}]*width:\s*100vw;[^}]*height:\s*100vh;[^}]*height:\s*100dvh;/)
  })

  it('lets every supported in-window fullscreen surface escape window clipping', () => {
    expect(styles).toContain('.desktop-window:has(.terminal-stage.is-fullscreen)')
    expect(styles).toContain('.desktop-window:has(.interactive-terminal.is-fullscreen)')
    expect(styles).toContain('.desktop-window__body:has(.interactive-terminal.is-fullscreen)')
  })

  it('keeps mobile CSS geometry aligned with the TypeScript work area', () => {
    expect(styles).toContain('min-width: min(320px, calc(100vw - 48px));')
    expect(styles).toContain('min-height: min(220px, calc(100vh - 88px));')
    expect(styles).toContain('min-height: min(220px, calc(100dvh - 88px));')
    expect(styles).toMatch(/@media \(max-width: 760px\), \(hover: none\) and \(pointer: coarse\)/)
    expect(styles).toMatch(/\.desktop-window__action--maximize,\s*\.desktop-window__resize\s*\{\s*display:\s*none;/)
    expect(styles).toContain('env(safe-area-inset-bottom)')
  })

  it('supports direct touch dragging and native window-content scrolling without changing mouse semantics', () => {
    expect(styles).toMatch(/\.desktop__icon\s*\{[^}]*touch-action:\s*none;/)
    expect(styles).toMatch(/@media \(max-width: 760px\) \{[\s\S]*?\.desktop__icon\s*\{[^}]*touch-action:\s*pan-y;/)
    expect(styles).toMatch(/\.desktop-window__body\s*\{[^}]*touch-action:\s*pan-x pan-y;/)
    expect(windowSource).toContain('if (isCompactLayout()) return')
    expect(windowSource).toContain("height: 'calc(100dvh - 82px)'")
  })

  it('positions icon slots independently and disables slot motion while dragging', () => {
    expect(styles).toMatch(/\.desktop__icons\s*\{[^}]*overflow-x:\s*hidden;[^}]*overflow-y:\s*auto;/)
    expect(styles).toMatch(/\.desktop__icons-scroll-space\s*\{[^}]*pointer-events:\s*none;/)
    expect(styles).toMatch(/\.desktop__icon-slot\s*\{[^}]*position:\s*absolute;[^}]*transition:\s*left [^;]+, top [^;]+;/)
    expect(styles).toMatch(/\.desktop__icon-slot--dragging\s*\{[^}]*transition:\s*none;/)
  })

  it('gives shortcut fields a unified focusable control surface', () => {
    expect(styles).toMatch(/\.desktop-shortcut-form__control\s*\{[^}]*border:\s*1px solid var\(--border\);[^}]*border-radius:\s*12px;/)
    expect(styles).toMatch(/\.desktop-shortcut-form__control:focus-within\s*\{[^}]*border-color:[^;]*var\(--brand\)/)
    expect(styles).toMatch(/\.desktop-shortcut-form__control :is\(input, textarea\)\s*\{[^}]*background:\s*transparent;[^}]*border:\s*0;/)
    expect(styles).toMatch(/\.desktop-shortcut-form__icon-actions label:focus-within\s*\{[^}]*outline:/)
    expect(styles).toMatch(/@media \(max-width: 760px\) \{[\s\S]*?\.desktop-shortcut-form__control :is\(input, textarea\)\s*\{[^}]*font-size:\s*16px;/)
  })

  it('uses Windows-style desktop selection, controls and bottom taskbar', () => {
    expect(styles).toContain('.desktop__icon--selected')
    expect(styles).toMatch(/\.desktop__selection-box\s*\{[^}]*z-index:\s*11;[^}]*background:\s*color-mix\(in srgb, var\(--brand\) 28%, transparent\);[^}]*border:\s*2px solid/)
    expect(styles).toContain(":root:not([data-theme='dark']) .desktop__selection-box")
    expect(styles).toContain('.desktop-window__action--close:hover')
    expect(styles).toContain('grid-template-columns: minmax(150px, 1fr) auto minmax(150px, 1fr);')
    expect(styles).toMatch(/\.desktop__taskbar-brand \.brand__mark\s*\{[^}]*width:\s*36px;[^}]*height:\s*36px;/)
    expect(styles).toContain('.desktop__taskbar-brand > span,')
    expect(windowSource.indexOf('desktop-window__action--minimize')).toBeLessThan(
      windowSource.indexOf('desktop-window__action--close'),
    )
  })

  it('keeps external-drop feedback above icons, below global dialogs, and motion-safe', () => {
    expect(styles).toMatch(/\.desktop__file-drop\s*\{[^}]*z-index:\s*12;[^}]*pointer-events:\s*none;/)
    expect(styles).toMatch(/\.desktop-transfer\s*\{[^}]*z-index:\s*24;[^}]*bottom:\s*78px;/)
    expect(styles).toMatch(/\.desktop__drop-pulse\s*\{[^}]*position:\s*fixed;[^}]*pointer-events:\s*none;/)
    expect(styles).toMatch(/@media \(max-width: 760px\) \{[\s\S]*?\.desktop-transfer\s*\{[^}]*width:\s*calc\(100vw - 16px\);/)
    expect(styles).toMatch(/@media \(prefers-reduced-motion: reduce\) \{[\s\S]*?animation-duration:\s*\.01ms !important;/)
  })

  it('keeps the snap preview lightweight and below interactive desktop chrome', () => {
    expect(styles).toMatch(/\.desktop-window-snap-preview\s*\{[^}]*z-index:\s*90;[^}]*pointer-events:\s*none;/)
    const previewRule = styles.match(/\.desktop-window-snap-preview\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(previewRule).not.toContain('backdrop-filter')
    expect(windowSource).toContain('<Teleport v-if="snapTarget" to=".desktop">')
  })

  it('keeps desktop icons and labels crisp in both color themes', () => {
    expect(styles).toMatch(/\.desktop__icon-glyph--dynamic::before\s*\{[^}]*display:\s*none;/)
    expect(styles).toMatch(/\.desktop__icon-img\s*\{[^}]*background:\s*#f7f9f8;/)
    expect(styles).toMatch(/\.desktop__external-confirm-icon-image\s*\{[^}]*background:\s*#f7f9f8;/)
    expect(styles).toMatch(/\.modal-panel--compact:has\(\.desktop__external-confirm\) \.modal-panel__header\s*\{[^}]*align-items:\s*center;[^}]*padding-block:\s*12px;/)
    expect(styles).toMatch(/\.modal-panel--compact:has\(\.desktop__external-confirm\) \.modal-panel__header h2\s*\{[^}]*margin-bottom:\s*0;/)
    expect(styles).toMatch(/\.desktop__icon-label\s*\{[^}]*height:\s*18px;[^}]*padding:\s*0 6px;[^}]*font-size:\s*11px;[^}]*line-height:\s*16px;/)
    expect(styles).toMatch(/:root:not\(\[data-theme='dark'\]\) \.desktop__icon-label\s*\{[^}]*text-shadow:\s*none;/)
    expect(styles).toMatch(/:root:not\(\[data-theme='dark'\]\) \.desktop__icon--selected \.desktop__icon-label\s*\{[^}]*color:\s*#17312c;[^}]*background:\s*color-mix\(in srgb, var\(--brand\) 38%, #fff\);/)
  })

  it('preserves wallpaper depth in light mode without changing the dark treatment', () => {
    expect(styles).toContain('linear-gradient(145deg, rgb(226 242 239 / 8%), rgb(190 218 224 / 3%))')
    expect(styles).toMatch(/:root\[data-theme='dark'\] \.desktop__wallpaper::after\s*\{/)
    expect(styles).toMatch(/:root\[data-theme='dark'\] \.desktop__aurora\s*\{[^}]*opacity:\s*\.2;/)
  })

  it('crossfades wallpaper layers and respects reduced motion', () => {
    expect(styles).toMatch(/\.desktop-wallpaper-fade-enter-active,\s*\.desktop-wallpaper-fade-leave-active\s*\{[^}]*opacity \.5s cubic-bezier\(\.22, 1, \.36, 1\),[^}]*transform \.65s cubic-bezier\(\.22, 1, \.36, 1\);/)
    expect(styles).toMatch(/\.desktop-wallpaper-fade-enter-from\s*\{[^}]*opacity:\s*0;[^}]*transform:\s*scale\(1\.012\);/)
    expect(styles).toMatch(/@media \(prefers-reduced-motion: reduce\)\s*\{[\s\S]*?\.desktop-wallpaper-fade-enter-active,[\s\S]*?transition:\s*none;/)
  })

  it('crossfades full desktop theme changes with a motion-safe fallback', () => {
    expect(desktopViewSource).toContain("root.classList.add('desktop-theme-transitioning')")
    expect(desktopViewSource).toContain('const duration = motionDuration(420)')
    expect(styles).toMatch(/:root\.desktop-theme-transitioning \.desktop[\s\S]*?transition-duration:\s*\.42s;[\s\S]*?cubic-bezier\(\.22, 1, \.36, 1\);/)
    expect(styles).toMatch(/\.desktop__wallpaper-image::before,\s*\.desktop__wallpaper-image::after\s*\{[^}]*transition:\s*opacity \.42s cubic-bezier\(\.22, 1, \.36, 1\);/)
    expect(styles).toMatch(/:root\[data-theme='dark'\] \.desktop__wallpaper-image::after\s*\{[^}]*opacity:\s*1;/)
  })

  it('keeps all three monitor rows evenly spaced while compacting cumulative traffic', () => {
    expect(styles).toContain('grid-template-rows: repeat(3, 37px);')
    expect(styles).toContain('row-gap: 14px;')
    expect(styles).toMatch(/\.desktop-monitor__network\s*\{[^}]*gap:\s*0;/)
    expect(styles).toMatch(/\.desktop-monitor__metric--network \.desktop-monitor__row\s*\{[^}]*gap:\s*1px;/)
    expect(styles).toMatch(/\.desktop-monitor__network-total\s*\{[^}]*padding-top:\s*1px;/)
  })

  it('does not reserve a light outer scrollbar gutter around script terminals', () => {
    expect(styles).toMatch(/\.desktop-window__body:has\(> \.app-script-page\)\s*\{[^}]*overflow:\s*hidden;[^}]*background:\s*var\(--terminal-shell-background\);[^}]*scrollbar-gutter:\s*auto;/)
  })

  it('adapts the application market grid to the desktop window width', () => {
    expect(styles).toMatch(/@container desktop-window \(max-width: 980px\)[\s\S]*?\.desktop-window__body \.app-grid\s*\{[^}]*grid-template-columns:\s*repeat\(2, minmax\(0, 1fr\)\) !important;/)
    expect(styles).toMatch(/@container desktop-window \(max-width: 680px\)[\s\S]*?\.desktop-window__body \.app-grid\s*\{[^}]*grid-template-columns:\s*minmax\(0, 1fr\) !important;/)
  })

  it('keeps focused, minimized and closing window states keyboard-safe', () => {
    expect(windowSource).toContain('tabindex="-1"')
    expect(windowSource).toContain(':inert="windowState.minimized || closing || undefined"')
    expect(windowSource).toContain(':aria-hidden="windowState.minimized || closing"')
    expect(windowSource).toContain('element.focus({ preventScroll: true })')
  })
})
