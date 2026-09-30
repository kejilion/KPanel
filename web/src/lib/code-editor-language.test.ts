import { describe, expect, it } from 'vitest'
import {
  CODE_HIGHLIGHT_MAX_BYTES,
  detectCodeLanguage,
  loadCodeLanguage,
} from '@/lib/code-editor-language'

describe('code editor language loading', () => {
  it('detects common file names and MIME fallbacks', () => {
    expect(detectCodeLanguage('app.tsx')).toMatchObject({ id: 'tsx', label: 'TSX' })
    expect(detectCodeLanguage('Dockerfile.production')).toMatchObject({
      id: 'dockerfile',
      label: 'Dockerfile',
    })
    expect(detectCodeLanguage('nginx.conf')).toMatchObject({ id: 'nginx', label: 'Nginx' })
    expect(detectCodeLanguage('script', 'text/x-shellscript')).toMatchObject({
      id: 'shell',
      label: 'Shell',
    })
    expect(detectCodeLanguage('Containerfile')).toMatchObject({ id: 'dockerfile' })
    expect(detectCodeLanguage('.env.production')).toMatchObject({ id: 'properties' })
    expect(detectCodeLanguage('.bashrc')).toMatchObject({ id: 'shell' })
    expect(detectCodeLanguage('CMakeLists.txt')).toMatchObject({ id: 'cmake' })
  })

  it('keeps the inner language for backup and template copies', () => {
    expect(detectCodeLanguage('nginx.conf.bak')).toMatchObject({ id: 'nginx' })
    expect(detectCodeLanguage('nginx.conf.default')).toMatchObject({ id: 'nginx' })
    expect(detectCodeLanguage('nginx.yml')).toMatchObject({ id: 'yaml' })
    expect(detectCodeLanguage('config.yml.example')).toMatchObject({ id: 'yaml' })
    expect(detectCodeLanguage('Cargo.toml.orig')).toMatchObject({ id: 'toml' })
    expect(detectCodeLanguage('dump.bak')).toBeUndefined()
  })

  it('uses plain text without loading a parser for unsupported files', async () => {
    await expect(loadCodeLanguage('README.unknown', 'text/plain', 1024)).resolves.toMatchObject({
      id: 'plain-text',
      highlighted: false,
      reason: 'unsupported',
    })
  })

  it('disables syntax parsing above the large-file threshold', async () => {
    await expect(
      loadCodeLanguage('large.js', 'text/javascript', CODE_HIGHLIGHT_MAX_BYTES + 1),
    ).resolves.toMatchObject({
      id: 'javascript',
      highlighted: false,
      reason: 'large-file',
    })
  })

  it.each([
    ['app.js', 'javascript'],
    ['page.html', 'html'],
    ['theme.css', 'css'],
    ['settings.json', 'json'],
    ['README.md', 'markdown'],
    ['worker.py', 'python'],
    ['compose.yaml', 'yaml'],
    ['index.php', 'php'],
    ['schema.sql', 'sql'],
    ['main.go', 'go'],
    ['feed.xml', 'xml'],
    ['deploy.sh', 'shell'],
    ['nginx.conf', 'nginx'],
    ['Dockerfile', 'dockerfile'],
    ['service.ini', 'properties'],
    ['main.c', 'c'],
    ['widget.hpp', 'cpp'],
    ['App.java', 'java'],
    ['Program.cs', 'csharp'],
    ['Main.kt', 'kotlin'],
    ['lib.rs', 'rust'],
    ['Cargo.toml', 'toml'],
    ['app.rb', 'ruby'],
    ['init.lua', 'lua'],
    ['View.swift', 'swift'],
    ['theme.scss', 'scss'],
    ['theme.less', 'less'],
    ['fix.patch', 'diff'],
    ['deploy.ps1', 'powershell'],
    ['api.proto', 'protobuf'],
    ['build.gradle', 'groovy'],
    ['nginx.service', 'properties'],
    ['template.j2', 'jinja2'],
  ])('loads syntax support for %s', async (fileName, languageId) => {
    const result = await loadCodeLanguage(fileName, '', 1024)
    expect(result).toMatchObject({ id: languageId, highlighted: true })
    expect(result.extension).toBeDefined()
  })
})
