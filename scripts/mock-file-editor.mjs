// UI preview fixtures only. Contents and writes remain in this process's memory.
const contents = new Map([
  [
    '/editor-demo/nginx.conf',
    '# KPanel · 模拟数据 / Preview only\nserver {\n    listen 80;\n    server_name example.com;\n\n    location / {\n        root /var/www/html;\n        index index.html;\n    }\n}\n',
  ],
  [
    '/editor-demo/settings.json',
    '{\n  "name": "KPanel",\n  "theme": "system",\n  "port": 8080,\n  "features": {\n    "files": true,\n    "terminal": true\n  }\n}\n',
  ],
  [
    '/editor-demo/README.md',
    '# KPanel 文件编辑器\n\n这是模拟数据，不会修改服务器文件。\n\n- 点击左侧文件，在标签间切换\n- 修改内容后点击保存\n- 切换标签保留光标、滚动和撤销记录\n',
  ],
  ['/editor-demo/conflict.conf', '# 模拟保存冲突：修改并保存后显示错误，草稿保留\nport=8080\n'],
  ['/editor-demo/unavailable.txt', ''],
  [
    '/editor-demo/config/long-configuration-name-for-responsive-preview.yaml',
    Array.from({ length: 800 }, (_, i) => `setting_${i + 1}: value_${i + 1}`).join('\n'),
  ],
])
export const mockEditorFiles = [
  ...['/editor-demo', '/editor-demo/config', '/editor-demo/empty'].map((path) => ({
    path,
    kind: 'directory',
    editable: false,
    sizeBytes: 0,
  })),
  ...[...contents].map(([path, content]) => ({
    path,
    kind: 'file',
    editable: true,
    sizeBytes: Buffer.byteLength(content),
    mime: 'text/plain',
  })),
].map((entry, index) => ({
  ...entry,
  name: entry.path.split('/').at(-1),
  mode: entry.kind === 'file' ? '-rw-r--r--' : 'drwxr-xr-x',
  owner: 'demo',
  group: 'demo',
  modifiedAt: '2026-09-23T00:00:00Z',
  resourceVersion: `mock-editor-${index}`,
  previewable: true,
}))

export async function handleMockEditor(request, response, url, { send, readJSON }) {
  if (url.pathname !== '/api/v1/files/content') return false
  const path = url.searchParams.get('path')
  const entry = mockEditorFiles.find((item) => item.path === path && item.kind === 'file')
  if (!entry) return false
  if (request.method === 'GET') {
    if (path.endsWith('/unavailable.txt'))
      send(response, 404, { title: '模拟文件暂时不可用，请重试', code: 'not_found' })
    else {
      response.writeHead(200, { 'content-type': 'text/plain; charset=utf-8' })
      response.end(contents.get(path))
    }
    return true
  }
  if (request.method === 'PUT') {
    const input = await readJSON(request)
    if (path.endsWith('/conflict.conf') || input.expectedResourceVersion !== entry.resourceVersion)
      send(response, 409, {
        title: '文件已被外部修改，请保留草稿并重新打开文件核对',
        code: 'file_conflict',
      })
    else {
      contents.set(path, input.content)
      entry.resourceVersion += '-saved'
      entry.sizeBytes = Buffer.byteLength(input.content)
      send(response, 200, { entry })
    }
    return true
  }
  return false
}
