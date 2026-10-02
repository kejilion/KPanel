import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const workspace = readFileSync(new URL('./FilesSplitWorkspace.vue', import.meta.url), 'utf8')
const files = readFileSync(new URL('../../views/FilesView.vue', import.meta.url), 'utf8')

describe('split pane command bar', () => {
  it('keeps the actions on one row by shrinking secondary buttons to icon squares', () => {
    expect(workspace).toMatch(/\.file-command-bar__actions\s*\{\s*flex-wrap:\s*nowrap;\s*\}/)
    expect(workspace).toMatch(/\.button:not\(\.button--primary\)\s*\{[^}]*width:\s*40px;[^}]*flex:\s*0 0 40px;/)
    // The label stays in the DOM for assistive technology; only its pixels are removed.
    expect(workspace).toMatch(/\.file-command-bar__label\s*\{[^}]*position:\s*absolute;[^}]*clip-path:\s*inset\(50%\);/)
    expect(workspace).not.toMatch(/\.file-command-bar__label\s*\{[^}]*display:\s*none/)
  })

  it('wraps the label of every secondary action and leaves the primary upload labelled', () => {
    for (const label of ['图库', '回收站', '分享管理', '新建目录', '关闭此栏']) {
      expect(files).toContain(`<span class="file-command-bar__label">${label}</span>`)
    }
    expect(files).toContain('<Upload :size="15" /> 上传文件')
  })
})
