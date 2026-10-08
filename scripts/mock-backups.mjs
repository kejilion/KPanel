// Local preview fixtures only. These endpoints never read or restore host data.
const modules = ['panel', 'apps', 'web', 'docker']
const maxBytes = 50 * 2 ** 30
let sequence = 10
const records = new Map()
const storageId = 'f'.repeat(32)
let settings = { revision: 'mock-settings-1', storages: [{ id: storageId, name: 'Demo WebDAV', kind: 'webdav', endpoint: 'https://nas.example/dav', prefix: 'kpanel', username: 'backup', hasSecret: true }], schedule: { enabled: false, modules: ['panel'], storageId: '', frequency: 'daily', hour: 3, minute: 0, weekday: 0, day: 1, timezone: 'Asia/Shanghai', keep: 7, hasPassword: false } }
function publicSettings() {
  const automatic = [...records.values()].filter(r => r.automatic && r.action === 'export' && r.modules.join() === settings.schedule.modules.join() && (r.remote?.storageId || '') === settings.schedule.storageId)
  const latest = automatic.at(-1)
  const success = automatic.filter(r => r.status === 'completed' && r.size > 0 && (!r.remote || r.remote.status === 'completed')).at(-1)
  const state = !settings.schedule.enabled ? 'disabled' : settings.schedule.lastError === 'schedule_missed' ? 'missed' : !latest ? 'idle' : ['queued', 'running'].includes(latest.status) ? 'running' : latest.status === 'failed' ? 'failed' : latest.errorCode ? 'warning' : 'healthy'
  return { ...settings, health: { state, lastSuccessAt: success?.updatedAt, lastRecordId: latest?.id, errorCode: settings.schedule.lastError || latest?.errorCode } }
}
function updated() { settings.revision = `mock-settings-${++sequence}`; return publicSettings() }
function remote(record, id) { if (id) record.remote = { storageId: id, storageName: settings.storages.find(s => s.id === id)?.name || 'Remote', key: `kpanel-${record.id}.kpb`, status: 'completed' }; return record }
function create(action, selected, status = 'queued') {
  const record = { id: (++sequence).toString(16).padStart(32, '0'), action, modules: selected, status, stage: status, createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(), size: 0, targetRevision: 'mock-revision' }
  records.set(record.id, record)
  return record
}
create('import', ['panel', 'web'], 'ready')
records.get([...records.keys()][0]).roots = [{ path: '/home/web', module: 'web' }]
Object.assign(create('restore', ['docker'], 'failed'), { errorCode: 'cleanup_pending', completedModules: ['docker'] })
if (process.env.KPANEL_MOCK_BACKUP_HEALTH === '1') {
  const previous = new Date(Date.now() - 86400000).toISOString()
  Object.assign(create('export', ['panel'], 'completed'), { automatic: true, localReady: true, size: 24576, createdAt: previous, updatedAt: previous })
  settings.schedule = { ...settings.schedule, enabled: true, hasPassword: true, lastError: 'schedule_missed', nextRun: new Date(Date.now() + 86400000).toISOString() }
}
function advance(record, status) {
  setTimeout(() => {
    Object.assign(record, { status, stage: status, updatedAt: new Date().toISOString(), localReady: record.action === 'export', size: record.action === 'export' ? 24576 : 0, ...(record.action === 'restore' ? { completedModules: record.modules } : {}) })
    if (record.automatic) settings.schedule.lastError = ''
  }, 800).unref()
  return record
}
export async function mockBackups(request, response, url, send, readJSON) {
  const prefix = '/api/v1/backups'
  if (url.pathname !== prefix && !url.pathname.startsWith(prefix + '/')) return false
  const path = url.pathname.slice(prefix.length)
  const fail = (status, title) => send(response, status, { code: 'mock_backup_error', title })
  try {
    if (path === '' && request.method === 'GET') send(response, 200, { items: [...records.values()].reverse(), maxBytes })
    else if (path === '/settings' && request.method === 'GET') send(response, 200, publicSettings())
    else if (path === '/storage' && request.method === 'PUT') {
      const input = await readJSON(request)
      if (input.revision !== settings.revision) fail(409, '备份设置已变更，请刷新后重试')
      else { const storage = { ...input.storage, id: input.storage.id || (++sequence).toString(16).padStart(32, '0'), hasSecret: true }; delete storage.secret; settings.storages = [...settings.storages.filter(s => s.id !== storage.id), storage]; send(response, 200, updated()) }
    } else if (/^\/storage\/[^/]+\/test$/.test(path) && request.method === 'POST') {
      const storage = settings.storages.find(s => s.id === path.split('/')[2])
      if (!storage || storage.endpoint.includes('fail.example')) fail(502, '远程连接失败，请检查地址、凭据和读写权限')
      else send(response, 200, { ok: true })
    } else if (/^\/storage\/[^/]+\/files$/.test(path) && request.method === 'GET') send(response, 200, { items: [{ key: 'kpanel-demo.kpb', size: 24576, modified: new Date().toISOString() }] })
    else if (/^\/storage\/[^/]+$/.test(path) && request.method === 'DELETE') {
      const input = await readJSON(request); const id = path.split('/')[2]
      if (input.revision !== settings.revision || settings.schedule.storageId === id) fail(409, '备份设置已变更，请刷新后重试')
      else { settings.storages = settings.storages.filter(s => s.id !== id); send(response, 200, updated()) }
    } else if (path === '/schedule' && request.method === 'PUT') {
      const input = await readJSON(request)
      if (input.revision !== settings.revision) fail(409, '备份设置已变更，请刷新后重试')
      else { settings.schedule = { ...input.schedule, hasPassword: !!input.schedule.password || settings.schedule.hasPassword, lastError: '', nextRun: new Date(Date.now() + 86400000).toISOString() }; delete settings.schedule.password; send(response, 200, updated()) }
    } else if (path === '/schedule/run' && request.method === 'POST') {
      const record = remote(create('export', settings.schedule.modules), settings.schedule.storageId); record.automatic = true; send(response, 202, advance(record, 'completed'))
    } else if (path === '/remote-import' && request.method === 'POST') {
      const input = await readJSON(request)
      if (Buffer.byteLength(input.password || '') < 10) fail(400, '请检查远程存储、时间和备份密码')
      else send(response, 202, advance(create('import', ['panel']), 'ready'))
    }
    else if (path === '/inventory' && request.method === 'GET') send(response, 200, { revision: 'mock-revision', panelBytes: 24576, maxBytes, hostAvailable: true, host: { revision: 'mock-revision', modules: [{ id: 'apps', bytes: 3 * 2 ** 30, containers: 2, requires: ['docker'] }, { id: 'web', bytes: 512 * 2 ** 20, containers: 4, requires: [] }, { id: 'docker', bytes: 128 * 2 ** 20, containers: 2, requires: [] }] } })
    else if (path === '/export' && request.method === 'POST') {
      const input = await readJSON(request)
      if (!input.modules?.length || input.modules.some(m => !modules.includes(m)) || Buffer.byteLength(input.password || '') < 10) fail(400, '请选择备份内容并填写密码。')
      else send(response, 202, advance(remote(create('export', input.modules), input.storageId), 'completed'))
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
      else if (request.method === 'POST' && action === 'upload') { const input = await readJSON(request); remote(record, input.storageId); send(response, 202, advance(record, 'completed')) }
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
