// UI preview only: the synthetic documents and edits stay in memory.
const base = { kind: 'file', editable: false, previewable: true, officeEditable: true, sizeBytes: 4096,
  mode: '-rw-r--r--', owner: 'demo', group: 'demo', modifiedAt: '2026-10-04T00:00:00Z' }
export const mockOfficeFiles = [
  { ...base, name: 'office-demo', path: '/office-demo', kind: 'directory', officeEditable: false, sizeBytes: 0 },
  ...['文档_UI模拟.docx', '表格_UI模拟.xlsx', '演示_UI模拟.pptx', '冲突_UI模拟.docx', '不支持_UI模拟.docx'].map(name => ({ ...base, name, path: `/office-demo/${name}` })),
].map((entry, index) => ({ ...entry, resourceVersion: `mock-office-${index}` }))
const text = (id, content) => ({ id, kind: 'text', text: content, editable: true })
const documents = new Map(mockOfficeFiles.filter(entry => entry.kind === 'file').map(entry => {
  const kind = entry.name.split('.').at(-1)
  let sections = [{ name: '', items: [
    { ...text('p1', 'KPanel 轻量 Office · UI 模拟数据'), bold: true, fontSize: 24 },
    text('p2', '点击这段文字，在右侧修改内容后保存。此预览不会修改服务器文件。'),
    { id: 'table', kind: 'table', text: '', editable: false, table: [[text('c1', '任务'), text('c2', '状态')], [text('c3', '自研预览与基础编辑'), text('c4', '候选体验')]] },
  ] }]
  if (kind === 'xlsx') sections = [{ name: '预算', rows: 8, columns: 4, items: [
    ...['项目', '数量', '单价', '总价'].map((value, index) => ({ ...text(`header-${index}`, value), kind: 'cell', row: 1, column: index + 1 })),
    ...['服务器', '2', '100', '200'].map((value, index) => ({ ...text(`value-${index}`, value), kind: 'cell', row: 2, column: index + 1,
      ...(index === 3 ? { formula: '=B2*C2', editable: false } : {}) })),
  ] }, { name: '备注', rows: 1, columns: 1, items: [{ ...text('note', '公式是缓存值，保存后请在 Excel 重算。'), kind: 'cell', row: 1, column: 1 }] }]
  if (kind === 'pptx') sections = [1, 2].map(page => ({ name: `${page}`, width: 960, height: 540, items: [
    { ...text(`title-${page}`, `KPanel · 幻灯片 ${page}`), x: 70, y: 70, width: 800, height: 100, bold: true, fontSize: 32 },
    { ...text(`body-${page}`, '文本框可以直接修改；复杂母版和图表暂不渲染。'), x: 70, y: 220, width: 800, height: 140, fontSize: 20 },
  ] }))
  return [entry.path, { entry, kind, contentVersion: 'a'.repeat(64), notes: ['basic_layout'], sections }]
}))

export async function handleMockOffice(request, response, url, { send, readJSON }) {
  if (url.pathname !== '/api/v1/files/content') return false
  const path = url.searchParams.get('path'), doc = documents.get(path)
  if (!doc) return false
  if (request.method === 'GET' && url.searchParams.get('mode') === 'office') {
    if (path.includes('不支持')) send(response, 422, { code: 'office_unsupported', title: '模拟不支持的文档结构' })
    else send(response, 200, doc)
    return true
  }
  if (request.method !== 'PUT') return false
  const input = await readJSON(request)
  if (path.includes('冲突') || input.expectedResourceVersion !== doc.entry.resourceVersion || input.expectedContentVersion !== doc.contentVersion) {
    send(response, 409, { code: 'file_conflict', title: '模拟保存冲突' }); return true
  }
  const items = doc.sections.flatMap(section => section.items.flatMap(item => [item, ...(item.table?.flat() ?? [])]))
  const targets = (input.officeEdits ?? []).map(edit => ({ edit, item: items.find(item => item.id === edit.id && item.editable) }))
  if (!targets.length || targets.some(target => !target.item || typeof target.edit.text !== 'string')) {
    send(response, 422, { code: 'office_edit_invalid', title: '模拟无效编辑' }); return true
  }
  for (const { item, edit } of targets) item.text = edit.text
  doc.entry.resourceVersion += '-saved'
  doc.contentVersion = doc.contentVersion === 'a'.repeat(64) ? 'b'.repeat(64) : 'a'.repeat(64)
  send(response, 200, { entry: doc.entry }); return true
}
