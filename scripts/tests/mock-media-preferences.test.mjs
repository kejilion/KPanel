import { test } from 'node:test'
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { createServer } from 'node:net'
import { once } from 'node:events'
import { fileURLToPath } from 'node:url'

test('mock preferences preserve media locations and playback checkpoints together', async () => {
  const reservation = createServer().listen(0, '127.0.0.1')
  await once(reservation, 'listening')
  const port = reservation.address().port
  await new Promise((resolve, reject) => reservation.close(error => error ? reject(error) : resolve()))
  const child = spawn(process.execPath, [fileURLToPath(new URL('../mock-app-market-api.mjs', import.meta.url))], {
    env: { ...process.env, KPANEL_MOCK_API_PORT: String(port) }, stdio: ['ignore', 'pipe', 'pipe'],
  })
  let startupTimer
  try {
    await Promise.race([
      once(child.stdout, 'data'),
      once(child, 'exit').then(([code]) => { throw new Error(`mock exited before ready: ${code}`) }),
      new Promise((_, reject) => { startupTimer = setTimeout(() => reject(new Error('mock startup timed out')), 10000) }),
    ])
    clearTimeout(startupTimer)
    const url = `http://127.0.0.1:${port}/api/v1/settings/preferences`
    const initial = await (await fetch(url)).json()
    const musicLocation = { path: '/music', hostId: '' }
    const musicPlayback = { path: '/music/a.mp3', hostId: '', position: 42, playing: true, repeat: 'off', shuffle: false }
    const update = await fetch(url, { method: 'PUT', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ expectedResourceVersion: initial.resourceVersion, musicLocation, musicPlayback }) })
    assert.equal(update.status, 200)
    const saved = await update.json()
    assert.deepEqual(saved.musicLocation, musicLocation)
    assert.deepEqual(saved.musicPlayback, musicPlayback)
    const invalid = await fetch(url, { method: 'PUT', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ expectedResourceVersion: saved.resourceVersion, musicPlayback: { ...musicPlayback, path: '/../bad' } }) })
    assert.equal(invalid.status, 422)
    assert.deepEqual((await (await fetch(url)).json()).musicPlayback, musicPlayback)
  } finally {
    clearTimeout(startupTimer)
    child.kill()
  }
})
