import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const workspace = readFileSync(new URL('./FilesSplitWorkspace.vue', import.meta.url), 'utf8')
const files = readFileSync(new URL('../../views/FilesView.vue', import.meta.url), 'utf8')
const media = readFileSync(new URL('./FileMediaMenu.vue', import.meta.url), 'utf8')

describe('split pane command bar', () => {
  it('applies the shared toolbar height to buttons inside child components', () => {
    expect(files).toMatch(/\.file-command-bar__actions\s+:deep\(\.button\)\s*\{[^}]*min-height:\s*40px;/)
  })

  it('wraps labelled actions instead of collapsing them into icon-only buttons', () => {
    expect(workspace).toMatch(/\.file-command-bar__actions\s*\{[^}]*flex-wrap:\s*wrap;/)
    expect(workspace).not.toMatch(/\.file-command-bar__actions\s*\{[^}]*flex-wrap:\s*nowrap;/)
    expect(workspace).not.toMatch(/\.file-command-bar__label\s*\{/)
    expect(workspace).not.toMatch(/\.button:not\(\.button--primary\)\s*\{[^}]*width:\s*40px;/)
  })

  it('keeps text on file actions and makes the pane close button a named icon', () => {
    for (const label of ['回收站', '分享管理', "{{ i18n.t('desktop.newItem') }}"]) {
      expect(files).toContain(`<span class="file-command-bar__label">${label}</span>`)
    }
    expect(files).toContain('<FileMediaMenu @gallery="openGallery" @music="openMusic" />')
    expect(media).toContain('<span class="file-command-bar__label">{{ phrase(\'媒体\') }}</span>')
    expect(media).toContain('<ChevronDown class="file-command-bar__label"')
    expect(files).toContain('<Upload :size="15" /> 上传文件')
    expect(files).toMatch(/file-command-bar__close"[^>]*aria-label="关闭此栏"[^>]*>\s*<X :size="15" \/>\s*<\/button>/)
    expect(files).toMatch(/\.file-command-bar__close\s*\{[^}]*width:\s*40px;/)
  })
})
