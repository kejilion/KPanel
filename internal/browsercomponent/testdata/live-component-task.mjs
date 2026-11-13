import { execFileSync } from 'node:child_process'
import { createHash, randomBytes } from 'node:crypto'
import { writeFileSync, readFileSync } from 'node:fs'
const [action, directory] = process.argv.slice(2)
const ids = execFileSync('docker', ['container', 'ls', '-aq', '--filter', 'name=^/kpanel-browser$'], { encoding: 'utf8' }).trim()
let identity = 'missing'
if (ids) {
  const [value] = JSON.parse(execFileSync('docker', ['inspect', ids], { encoding: 'utf8' }))
  const kind = value.Config?.Labels?.['io.kpanel.browser.kind'] === 'tor' ? 'tor' : 'chromium'
  identity = `${value.Id}:${value.Image}:${value.State.Status}:${value.RestartCount}:${kind}`
}
const version = `sha256:${createHash('sha256').update(identity).digest('hex')}`
if (action === 'result') {
  const result = JSON.parse(readFileSync(`${directory}/task.json`, 'utf8'))
  if (result.state !== 'succeeded') throw Error(JSON.stringify(result))
  console.log(`${result.action}=succeeded`)
} else {
  const id = randomBytes(16).toString('hex')
  writeFileSync(`${directory}/task.json`, JSON.stringify({ id, action, expectedResourceVersion: version, state: 'queued', stage: 'queued', startedAt: new Date().toISOString() }), { mode: 0o600 })
  console.log(id)
}
