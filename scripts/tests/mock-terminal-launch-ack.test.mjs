import { test } from 'node:test'
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { createServer } from 'node:net'
import { once } from 'node:events'
import { fileURLToPath } from 'node:url'

test('mock launch acknowledgement is tied to an existing terminal session', async () => {
  const reservation = createServer().listen(0, '127.0.0.1')
  await once(reservation, 'listening')
  const port = reservation.address().port
  await new Promise((resolve, reject) => reservation.close(error => error ? reject(error) : resolve()))
  const child = spawn(process.execPath, [fileURLToPath(new URL('../mock-app-market-api.mjs', import.meta.url))], {
    env: { ...process.env, KPANEL_MOCK_API_PORT: String(port) }, stdio: ['ignore', 'pipe', 'pipe'],
  })
  let timer
  try {
    await Promise.race([
      once(child.stdout, 'data'),
      once(child, 'exit').then(([code]) => { throw new Error(`mock exited before ready: ${code}`) }),
      new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('mock startup timed out')), 10000) }),
    ])
    clearTimeout(timer)
    const base = `http://127.0.0.1:${port}/api/v1/terminal-sessions`
    const post = (path, body = {}) => fetch(path, { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify(body) })
    const created = await post(base, { hostId: 'local' })
    assert.equal(created.status, 201)
    const { sessionId } = await created.json()
    const ack = await post(`${base}/${sessionId}/launch-ack`)
    assert.equal(ack.status, 200)
    assert.deepEqual(await ack.json(), { acknowledged: true })
    const unknown = await post(`${base}/missing/launch-ack`)
    assert.equal(unknown.status, 404)
  } finally {
    clearTimeout(timer)
    child.kill()
  }
})
