import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const terminalSource = readFileSync(new URL('./TerminalView.vue', import.meta.url), 'utf8')
const hostTerminalSource = readFileSync(new URL('../components/terminal/HostTerminal.vue', import.meta.url), 'utf8')
const desktopStyles = readFileSync(new URL('../styles/desktop.css', import.meta.url), 'utf8')

describe('multi-host terminal workspace layout', () => {
  it('uses the shared persisted cluster host order', () => {
    expect(terminalSource).toContain("import {\n  readClusterHostOrder,\n  sortClusterHosts,\n  subscribeClusterHostOrder,\n} from '@/lib/clusterHostOrder'")
    expect(terminalSource).toContain(
      'return sortClusterHosts(inventory.value?.items || [], readClusterHostOrder())',
    )
  })

  it('opens the available local host after the first successful inventory load', () => {
    expect(terminalSource).toContain('let initialHostLoad = true')
    expect(terminalSource).toContain('if (initialHostLoad) {')
    expect(terminalSource).toContain('const localHost = inventory.value.items.find((host) => host.isLocal && host.terminalAvailable)')
    expect(terminalSource).toContain('if (localHost) await openHost(localHost)')
  })

  it('keeps a large connection inventory in its own scroll region', () => {
    expect(terminalSource).toContain('class="terminal-connections__list"')
    expect(terminalSource).toMatch(
      /\.terminal-connections\s*\{[^}]*display:grid;[^}]*min-height:0;[^}]*overflow:hidden;/,
    )
    expect(terminalSource).toMatch(
      /\.terminal-connections__list\s*\{[^}]*min-height:0;[^}]*overflow-y:auto;/,
    )
    expect(terminalSource).toMatch(
      /\.terminal-connections__list\s*\{[^}]*--scrollbar-size:8px;[^}]*--scrollbar-track:var\(--terminal-shell-panel,#111a1d\);[^}]*--scrollbar-thumb:var\(--terminal-shell-scrollbar,#35474a\);[^}]*scrollbar-color:var\(--scrollbar-thumb\) var\(--scrollbar-track\);[^}]*scrollbar-width:thin;/,
    )
    expect(terminalSource).toContain('v-if="!loading && !hosts.length" class="terminal-connections__empty"')
    expect(terminalSource).not.toContain('v-if="loading" class="terminal-connections__empty"')
    expect(terminalSource).not.toContain("t('terminal.loadingHosts')")
  })

  it('uses each cluster host operating system identity in both selector layouts', () => {
    expect(terminalSource).toContain("import OperatingSystemIcon from '@/components/overview/OperatingSystemIcon.vue'")
    expect(terminalSource).toContain("import { detectOperatingSystemIdentity } from '@/lib/operatingSystem'")
    expect(terminalSource).toContain('detectOperatingSystemIdentity(host.lastSnapshot?.telemetry)')
    expect(terminalSource.match(/:distro="hostOperatingSystemIdentity\(host\)\.key"/g)).toHaveLength(2)
    expect(terminalSource.match(/:label="hostOperatingSystemIdentity\(host\)\.label"/g)).toHaveLength(2)
    expect(terminalSource).not.toContain('<Server v-else')
  })

  it('keeps connection metadata and status at the visual language minimums', () => {
    expect(terminalSource).toMatch(/\.terminal-host small\s*\{[^}]*font-size:13px;/)
    expect(terminalSource).toMatch(/\.terminal-host em\s*\{[^}]*font-size:12px;/)
  })

  it('uses core messages for runtime host states and session prompts', () => {
    expect(terminalSource).toContain("t('terminal.hostState.local')")
    expect(terminalSource).toContain("t('terminal.hostCount'")
    expect(terminalSource).toContain("t('terminal.closeSessionsConfirm'")
    expect(terminalSource).not.toContain('关闭窗口将断开')
  })

  it('collapses the host selector into a persistent narrow rail', () => {
    expect(terminalSource).toContain("'is-connections-collapsed': connectionsCollapsed")
    expect(terminalSource).toContain('aria-controls="terminal-connection-selector"')
    expect(terminalSource).toContain("'terminal.expandConnections'")
    expect(terminalSource).toContain("'terminal.collapseConnections'")
    expect(terminalSource).toContain('terminal-connections__toggle terminal-connections__refresh')
    expect(terminalSource).toContain('class="terminal-connections__rail"')
    expect(terminalSource).toContain('class="terminal-host-rail"')
    expect(terminalSource).toContain(':title="`${host.name} · ${hostStateLabel(host)}`"')
    expect(terminalSource).toMatch(
      /class="terminal-host-rail__os"[\s\S]*?:show-tooltip="false"/,
    )
    expect(terminalSource).toMatch(
      /\.terminal-workspace\.is-connections-collapsed\s*\{[^}]*grid-template-columns:52px minmax\(0,1fr\);/,
    )
    expect(terminalSource).toMatch(
      /\.terminal-workspace\.is-connections-collapsed \.terminal-connections__heading,\.terminal-workspace\.is-connections-collapsed \.terminal-connections__refresh\s*\{\s*display:none;/,
    )
  })

  it('reserves the remaining stage height for the terminal and composer', () => {
    expect(terminalSource).toMatch(
      /\.terminal-stage\s*\{[^}]*grid-template-rows:auto minmax\(0,1fr\);[^}]*min-height:0;[^}]*overflow:hidden;/,
    )
    expect(terminalSource).toMatch(
      /@media \(max-width: 900px\)[\s\S]*?\.terminal-stage\s*\{[^}]*grid-template-rows:auto minmax\(0,1fr\);/,
    )
    expect(terminalSource).toMatch(
      /\.terminal-stage\.is-fullscreen\s*\{[^}]*grid-template-rows:auto minmax\(0,1fr\);/,
    )
  })

  it('uses one continuous terminal surface without nested frame gutters', () => {
    expect(terminalSource).toMatch(
      /\.terminal-stage\s*\{[^}]*padding:0;[^}]*background:var\(--terminal-shell-background/,
    )
    expect(terminalSource).toMatch(
      /\.terminal-tabs-bar\s*\{[^}]*border:0;[^}]*border-bottom:1px solid var\(--terminal-shell-border/,
    )
    expect(terminalSource).toMatch(
      /\.terminal-stage :deep\(\.host-terminal\)\s*\{[^}]*border:0;[^}]*border-radius:0;[^}]*box-shadow:none;/,
    )
  })

  it('keeps the host selector dark and uses a lighter internal border in light mode', () => {
    expect(terminalSource).toMatch(
      /\.terminal-connections\s*\{[^}]*border-right:1px solid var\(--terminal-shell-border,#29383a\);[^}]*background:var\(--terminal-shell-panel,#111a1d\);/,
    )
    expect(terminalSource).toContain(":global(:root:not([data-theme='dark'])) .terminal-workspace { --terminal-shell-border:rgb(255 255 255 / 18%); }")
  })

  it('uses an overlay drawer for host selection on mobile', () => {
    expect(terminalSource).toContain("'is-connections-drawer-open': mobileConnectionsOpen")
    expect(terminalSource).toContain('class="terminal-connections-overlay"')
    expect(terminalSource).toContain('aria-label="打开主机选择"')
    expect(terminalSource).toContain('aria-label="关闭主机选择"')
    expect(terminalSource).toContain('mobileConnectionsOpen.value = false')
    expect(terminalSource).toMatch(
      /@media \(max-width: 900px\)[\s\S]*?\.terminal-connections\s*\{[^}]*position:absolute;[^}]*transform:translateX\(-105%\);/,
    )
    expect(terminalSource).toMatch(
      /\.terminal-workspace\.is-connections-drawer-open \.terminal-connections\s*\{[^}]*transform:translateX\(0\);/,
    )
  })

  it('keeps the mobile host terminal in two rows so the composer cannot overflow', () => {
    expect(hostTerminalSource).toMatch(
      /\.host-terminal\s*\{[^}]*grid-template-rows:minmax\(0,1fr\) auto;[^}]*min-height:0;[^}]*overflow:hidden;/,
    )
    expect(hostTerminalSource).not.toMatch(
      /@media \(max-width: 760px\)[\s\S]*?\.host-terminal\s*\{[^}]*grid-template-rows:auto minmax\(0,1fr\) auto;/,
    )
  })

  it('keeps the outer screen edge-to-edge and uses fit-aware xterm content padding', () => {
    expect(hostTerminalSource).toMatch(
      /\.host-terminal__screen\s*\{[^}]*padding:0;/,
    )
    expect(hostTerminalSource).toMatch(
      /\.host-terminal__screen :deep\(\.xterm\)\s*\{[^}]*box-sizing:border-box;[^}]*height:100%;[^}]*padding:6px 8px 4px;[^}]*touch-action:none;/,
    )
    expect(hostTerminalSource).toMatch(
      /\.host-terminal__screen :deep\(\.xterm-viewport\)\s*\{[^}]*background:var\(--terminal-shell-background,#0b1214\);/,
    )
  })

  it('fits the terminal workspace to the desktop window instead of scrolling the outer page', () => {
    expect(desktopStyles).toMatch(
      /\.desktop-window__body:has\(> \.terminal-page\),[\s\S]*?overflow:\s*hidden;[\s\S]*?scrollbar-gutter:\s*auto;/,
    )
    expect(desktopStyles).toMatch(
      /\.desktop-window__body \.terminal-page\s*\{[^}]*height:\s*100% !important;[^}]*min-height:\s*0 !important;[^}]*overflow:\s*hidden;/,
    )
    expect(desktopStyles).toMatch(
      /\.desktop-window__body > \.terminal-page > \.terminal-workspace\s*\{[^}]*height:\s*auto !important;[^}]*min-height:\s*0 !important;[^}]*flex:\s*1 1 0;/,
    )
  })

  it('lets the terminal workspace use the full desktop window body', () => {
    expect(desktopStyles).toMatch(
      /\.desktop-window__body > \.terminal-page\s*\{[^}]*padding:\s*0;/,
    )
    expect(desktopStyles).toMatch(
      /\.desktop-window__body > \.terminal-page > \.terminal-workspace\s*\{[^}]*border:\s*0;[^}]*border-radius:\s*0;[^}]*box-shadow:\s*none;/,
    )
  })

  it('turns the host selector into an overlay drawer in a narrow desktop window', () => {
    expect(desktopStyles).toMatch(
      /@container desktop-window \(max-width: 820px\)[\s\S]*?\.desktop-window__body \.terminal-connections\s*\{[^}]*position:\s*absolute;[^}]*transform:\s*translateX\(-105%\);/,
    )
    expect(desktopStyles).toMatch(
      /\.desktop-window__body \.terminal-workspace\.is-connections-drawer-open \.terminal-connections\s*\{[^}]*transform:\s*translateX\(0\);/,
    )
    expect(desktopStyles).toMatch(
      /\.desktop-window__body \.terminal-stage__mobile-selector\s*\{[^}]*display:\s*flex !important;/,
    )
    expect(terminalSource).toMatch(
      /<button\s+v-if="!sessions\.length"\s+class="terminal-stage__mobile-selector"[\s\S]*?aria-label="打开主机选择"/,
    )
    expect(terminalSource).not.toContain('<div v-if="!sessions.length" class="terminal-stage__mobile-selector">')
    expect(terminalSource).toContain('class="terminal-tabs-bar__connections"')
    expect(desktopStyles).toMatch(
      /\.desktop-window__body \.terminal-tabs-bar__connections\s*\{[^}]*display:\s*grid !important;/,
    )
    expect(desktopStyles).toMatch(
      /\.desktop-window__body \.terminal-page \.terminal-workspace\.is-connections-drawer-open \.terminal-connections > header\s*\{[^}]*min-height:\s*42px;[^}]*justify-content:\s*flex-end;[^}]*padding:\s*6px 8px 0;/,
    )
    expect(desktopStyles).toMatch(
      /\.desktop-window__body \.terminal-page \.terminal-workspace\.is-connections-drawer-open \.terminal-connections__heading,[\s\S]*?\.desktop-window__body \.terminal-page \.terminal-workspace\.is-connections-drawer-open \.terminal-connections__refresh\s*\{[^}]*display:\s*none;/,
    )
    expect(terminalSource).toContain('<X :size="17" />')
  })

  it('does not reserve an inline host-list row in mobile desktop mode', () => {
    expect(desktopStyles).toMatch(
      /@media \(max-width: 900px\)[\s\S]*?\.desktop-window__body \.terminal-workspace,[\s\S]*?\.desktop-window__body \.terminal-workspace\.is-connections-collapsed\s*\{[^}]*grid-template-columns:\s*minmax\(0, 1fr\) !important;[^}]*grid-template-rows:\s*minmax\(0, 1fr\) !important;/,
    )
    expect(desktopStyles).toMatch(
      /@media \(max-width: 900px\)[\s\S]*?\.desktop-window__body \.terminal-connections\s*\{[^}]*border-right:\s*1px solid var\(--border\);[^}]*border-bottom:\s*0;/,
    )
  })

  it('contains wheel scrolling inside the host terminal viewport', () => {
    expect(hostTerminalSource).toContain('@wheel="containTerminalWheel"')
    expect(hostTerminalSource).toMatch(
      /\.host-terminal__screen :deep\(\.xterm-viewport\)\s*\{[^}]*overflow-y:scroll !important;[^}]*overscroll-behavior:contain;/,
    )
    expect(hostTerminalSource).toMatch(
      /\.host-terminal__screen :deep\(\.xterm-scrollable-element\)\s*\{[^}]*overscroll-behavior:contain;/,
    )
  })

  it('maps mobile vertical swipes to terminal scrollback without moving the desktop window', () => {
    expect(hostTerminalSource).toContain('@touchstart="terminalTouchScroll.start"')
    expect(hostTerminalSource).toContain('@touchmove="terminalTouchScroll.move"')
    expect(hostTerminalSource).toContain('@touchend="terminalTouchScroll.end"')
    expect(hostTerminalSource).toMatch(
      /\.host-terminal__screen :deep\(\.xterm\)\s*\{[^}]*touch-action:none;/,
    )
  })

  it('focuses the shell cursor by default and leaves the composer user-activated', () => {
    expect(hostTerminalSource).toContain('window.requestAnimationFrame(focusTerminal)')
    expect(hostTerminalSource).not.toContain('composerInput')
    expect(terminalSource).toContain('focusActiveTerminal()')
  })

  it('uses the terminal clipboard menu instead of the browser context menu', () => {
    expect(hostTerminalSource).toContain('@contextmenu="clipboardMenu?.open($event)"')
    expect(hostTerminalSource).toContain('@paste.capture="clipboardMenu?.handlePaste($event)"')
    expect(hostTerminalSource).toContain('terminal.attachCustomKeyEventHandler')
  })

  it('keeps session tabs and terminal actions in one dark toolbar row', () => {
    expect(terminalSource).toContain('class="terminal-tab__status"')
    expect(terminalSource).toContain('@state-change="item.state = $event"')
    expect(terminalSource).toContain('class="terminal-tabs-bar"')
    expect(terminalSource).toContain('<TerminalToolbar')
    expect(terminalSource).toMatch(
      /\.terminal-tabs-bar\s*\{[^}]*display:flex;[^}]*background:var\(--terminal-shell-panel/,
    )
  })

  it('centers the empty state consistently across window sizes', () => {
    expect(terminalSource).toMatch(
      /\.terminal-stage\s*\{[^}]*grid-template-columns:minmax\(0,1fr\);[^}]*grid-template-rows:auto minmax\(0,1fr\);/,
    )
    expect(terminalSource).toMatch(
      /\.terminal-stage__mobile-selector\s*\{[^}]*width:100%;[^}]*grid-row:1;[^}]*grid-column:1;[^}]*color:var\(--terminal-shell-text/,
    )
    expect(terminalSource).toMatch(
      /\.terminal-empty\s*\{[^}]*grid-row:2;[^}]*grid-column:1;[^}]*place-content:center;[^}]*padding:32px;/,
    )
    expect(terminalSource).not.toContain('.terminal-stage.is-fullscreen .terminal-empty')
    expect(terminalSource).not.toContain('padding:32px 32px clamp(72px,12vh,120px)')
  })

  it('keeps the terminal edge-to-edge at the smallest desktop window width', () => {
    expect(desktopStyles).toMatch(
      /@container desktop-window \(max-width: 580px\)[\s\S]*?\.desktop-window__body > \.terminal-page\s*\{[^}]*padding:\s*0;/,
    )
  })

  it('fills the whole terminal stage so tabs remain switchable', () => {
    expect(terminalSource).not.toContain('terminal-fullscreen-toggle')
    expect(terminalSource).toContain("'is-fullscreen': workspaceFullscreen")
    expect(terminalSource).toContain('@click="selectSession(item.id)"')
    expect(terminalSource).toMatch(
      /\.terminal-stage\.is-fullscreen\s*\{[^}]*position:fixed;[^}]*inset:0;[^}]*height:100dvh;[^}]*grid-template-rows:auto minmax\(0,1fr\);/,
    )
    expect(hostTerminalSource).toContain('defineExpose({ focusTerminal, scrollToTop, scheduleResize })')
  })
})
