// UI Mock only: lifecycle state and task recovery, without Docker or Chromium.
let installed = false, running = false, revision = 1, job, kind = 'chromium'
const resourceVersion = () => `sha256:${String(revision).padStart(64, '0')}`
export async function mockBrowserComponent(request, response, url, send, readJSON) {
  if (url.pathname === '/api/v1/browser/status') {
    send(response, 200, { ready: false, reason: installed ? 'browser_unavailable' : 'browser_unconfigured', maxTabs: 8 }); return true
  }
  if (url.pathname === '/api/v1/browser/component' && request.method === 'GET') {
    if (job?.state === 'queued' && Date.now() - job.started > 800) {
      installed = job.action !== 'uninstall'; running = installed && job.action !== 'stop'; revision++
      if (job.action === 'install') kind = job.kind || kind
      job = { ...job, state: 'succeeded', stage: 'complete' }
    }
    send(response, 200, { supported: true, torSupported: true, kind, installed, running, healthy: running, resourceVersion: resourceVersion(), image: 'UI Mock', job }); return true
  }
  if (url.pathname === '/api/v1/browser/component/actions' && request.method === 'POST') {
    const input = await readJSON(request)
    if (input.expectedResourceVersion !== resourceVersion() || job?.state === 'queued') { send(response, 409, { code: 'browser_component_changed', title: 'Refresh component status' }); return true }
    if (!['install', 'start', 'stop', 'restart', 'uninstall'].includes(input.action)) { send(response, 422, { code: 'browser_component_invalid' }); return true }
    if (input.kind && (!['chromium', 'tor'].includes(input.kind) || input.action !== 'install')) { send(response, 422, { code: 'browser_component_invalid' }); return true }
    if (installed && input.action === 'install' && input.kind && input.kind !== kind) { send(response, 409, { code: 'browser_component_changed' }); return true }
    job = { id: `mock-browser-${Date.now()}`, action: input.action, kind: input.kind, state: 'queued', stage: 'queued', started: Date.now() }
    send(response, 202, job); return true
  }
  return false
}
