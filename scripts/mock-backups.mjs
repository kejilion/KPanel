// Local preview fixtures only. These endpoints never read or restore host data.
const modules = ['panel', 'apps', 'web', 'docker']
const maxBytes = 50 * 2 ** 30
let sequence = 10
const records = new Map()
function create(action, selected, status = 'queued') {
  const record = { id: (++sequence).toString(16).padStart(32, '0'), action, modules: selected, status, stage: status, createdAt: new Date().toISOString(), size: 0, targetRevision: 'mock-revision' }
  records.set(record.id, record)
  return record
}
create('import', ['panel', 'web'], 'ready')
Object.assign(create('restore', ['docker'], 'failed'), { errorCode: 'cleanup_pending', completedModules: ['docker'] })
function advance(record, status) {
  setTimeout(() => Object.assign(record, { status, stage: status, size: record.action === 'export' ? 24576 : 0, ...(record.action === 'restore' ? { completedModules: record.modules } : {}) }), 800).unref()
  return record
}
export async function mockBackups(request, response, url, send, readJSON) {
  const prefix = '/api/v1/backups'
  if (url.pathname !== prefix && !url.pathname.startsWith(prefix + '/')) return false
  const path = url.pathname.slice(prefix.length)
  const fail = (status, title) => send(response, status, { code: 'mock_backup_error', title })
  try {
    if (path === '' && request.method === 'GET') send(response, 200, { items: [...records.values()].reverse(), maxBytes })
    else if (path === '/inventory' && request.method === 'GET') send(response, 200, { revision: 'mock-revision', panelBytes: 24576, maxBytes, hostAvailable: true, host: { revision: 'mock-revision', modules: [{ id: 'apps', bytes: 3 * 2 ** 30, containers: 2, requires: ['docker'] }, { id: 'web', bytes: 512 * 2 ** 20, containers: 4, requires: [] }, { id: 'docker', bytes: 128 * 2 ** 20, containers: 2, requires: [] }] } })
    else if (path === '/export' && request.method === 'POST') {
      const input = await readJSON(request)
      if (!input.modules?.length || input.modules.some(m => !modules.includes(m)) || Buffer.byteLength(input.password || '') < 10) fail(400, '请选择备份内容并填写密码。')
      else send(response, 202, advance(create('export', input.modules), 'completed'))
    } else if (path === '/import' && request.method === 'POST') {
      // Consume bounded fixture uploads; cryptography is verified by Go integration tests.
      let size = 0
      for await (const chunk of request) { size += chunk.length; if (size > 2 * 2 ** 20) throw new Error('Mock preview accepts fixture files up to 2 MiB') }
      send(response, 202, advance(create('import', ['panel', 'apps', 'web', 'docker']), 'ready'))
    } else {
      const [, id, action] = path.split('/')
      const record = records.get(id)
      if (!record) fail(404, '备份记录不存在。')
      else if (request.method === 'GET' && !action) send(response, 200, record)
      else if (request.method === 'POST' && action === 'preview') send(response, 200, record)
      else if (request.method === 'POST' && action === 'restore') {
        const input = await readJSON(request)
        if (record.status !== 'ready' || input.revision !== record.targetRevision || !input.modules?.length || input.modules.some(m => !record.modules.includes(m))) fail(409, '请重新检查所选内容。')
        else send(response, 202, advance(create('restore', input.modules), 'completed'))
      } else if (request.method === 'POST' && action === 'recover') {
        Object.assign(record, { status: 'completed', stage: 'completed', errorCode: '' })
        send(response, 202, advance(create('recover', record.modules), 'completed'))
      } else if (request.method === 'DELETE' && !action) { records.delete(id); response.writeHead(204); response.end() }
      else fail(409, 'Mock 预览不生成真实备份文件。')
    }
  } catch (error) { fail(400, error.message) }
  return true
}
