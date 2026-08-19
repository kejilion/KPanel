import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const read = (path: string) => readFileSync(new URL(path, import.meta.url), 'utf8')
const mainStyles = read('../styles/main.css')
const desktopStyles = read('../styles/desktop.css')
const view = (name: string) => read(`./${name}.vue`)

describe('phone portrait layout contract', () => {
  it('keeps the global phone chrome compact and preserves AI workspace width', () => {
    expect(mainStyles).toContain('@media (max-width: 480px)')
    expect(mainStyles).toMatch(/\.topbar__title span\s*\{[^}]*display:\s*none;/)
    expect(mainStyles).toMatch(/\.topbar \.language-selector__trigger\s*\{[^}]*width:\s*42px;/)
    expect(mainStyles).toMatch(/\.page-content\.page-content--ai\s*\{[^}]*padding:\s*0;/)
    expect(mainStyles).toMatch(/\.page-content\s*\{[^}]*overflow-x:\s*clip;/)
  })

  it('fits desktop mode inside a phone viewport and keeps the taskbar scrollable', () => {
    expect(desktopStyles).toMatch(/@media \(max-width: 420px\)[\s\S]*?\.desktop-window[^}]*width:\s*calc\(100vw - 12px\) !important;/)
    expect(desktopStyles).toContain('height: calc(100dvh - 76px) !important;')
    expect(desktopStyles).toMatch(/@media \(max-width: 760px\)[\s\S]*?\.desktop__taskbar-apps\s*\{[^}]*overflow-x:\s*auto;/)
  })

  it('stacks or scrolls dense market and cluster controls instead of squeezing them', () => {
    expect(view('AppsView')).toMatch(/@media \(max-width: 640px\)[\s\S]*?\.market-hero__actions\s*\{[^}]*grid-template-columns:\s*repeat\(2, minmax\(0, 1fr\)\);/)
    expect(view('AppsView')).toMatch(/\.market-segment,[\s\S]*?\.market-categories\s*\{[^}]*overflow-x:\s*auto;/)
    expect(view('ClusterView')).toMatch(/\.cluster-hero__actions\s*\{[^}]*grid-template-columns:\s*42px repeat\(3, minmax\(0, 1fr\)\);/)
    expect(view('ClusterView')).toMatch(/\.cluster-stats\s*\{[^}]*grid-template-columns:\s*repeat\(2, minmax\(0, 1fr\)\);/)
  })

  it('keeps operational pages usable at 360 to 430 pixels', () => {
    expect(view('DockerView')).toMatch(/\.workspace-card > header:not\(\.resource-section__header\)\s*\{[^}]*display:\s*grid;/)
    expect(view('FilesView')).toMatch(/@media \(max-width: 480px\)[\s\S]*?\.file-command-bar__actions,[\s\S]*?grid-template-columns:\s*repeat\(2, minmax\(0, 1fr\)\);/)
    expect(view('DiagnosticsView')).toMatch(/@media \(max-width: 680px\)[\s\S]*?\.diagnostic-command-panel\s*\{[^}]*transform: translateX\(-105%\);/)
    expect(view('DiagnosticsView')).toContain('class="diagnostic-mobile-selector"')
    expect(view('DiagnosticsView')).toContain('min-height: min(400px, 48dvh);')
    expect(view('TerminalView')).toContain('height:calc(100dvh - 94px)')
    expect(view('TerminalView')).toContain('class="terminal-stage__mobile-selector"')
    expect(view('MonitoringView')).toMatch(/@media \(max-width: 780px\)[\s\S]*?\.summary-grid\s*\{[^}]*grid-template-columns:\s*repeat\(2, minmax\(0, 1fr\)\);/)
    expect(view('EnvironmentView')).toContain('repeat(3, 40px)')
    expect(view('ProcessManagerView')).toMatch(/@media \(max-width: 480px\)[\s\S]*?\.process-toolbar__controls\s*\{[^}]*flex-wrap:\s*wrap;/)
  })
})
