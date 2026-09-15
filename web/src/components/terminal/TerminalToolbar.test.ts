import { createSSRApp } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { describe, expect, it } from 'vitest'
import TerminalToolbar from './TerminalToolbar.vue'

describe('TerminalToolbar', () => {
  it('replaces jump-to-top with the optional quick-command toggle', async () => {
    const html = await renderToString(createSSRApp(TerminalToolbar, {
      fullscreen: false,
      quickCommands: true,
      quickCommandsExpanded: false,
    }))

    expect(html).toContain('展开快捷命令')
    expect(html).not.toContain('回到顶部')
    expect(html.indexOf('铺满网页')).toBeGreaterThan(html.indexOf('展开快捷命令'))
  })

  it('exposes the collapsed state and omits the toggle from task terminals', async () => {
    const expanded = await renderToString(createSSRApp(TerminalToolbar, {
      fullscreen: false,
      quickCommands: true,
      quickCommandsExpanded: true,
    }))
    const taskTerminal = await renderToString(createSSRApp(TerminalToolbar, { fullscreen: false }))

    expect(expanded).toContain('收起快捷命令')
    expect(expanded).toContain('aria-expanded="true"')
    expect(taskTerminal).not.toContain('快捷命令')
  })

  it('changes fullscreen into the restore action', async () => {
    const html = await renderToString(createSSRApp(TerminalToolbar, { fullscreen: true }))

    expect(html).toContain('恢复窗口')
    expect(html).not.toContain('铺满网页')
  })
})
