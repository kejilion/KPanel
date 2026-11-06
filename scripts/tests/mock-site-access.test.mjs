import test from 'node:test'
import assert from 'node:assert/strict'
import { handleMockSiteAccess, mockCreatedSiteAccess } from '../mock-site-access.mjs'

test('mock access rules allow the preview journey without returning passwords', async () => {
  const id = 'a'.repeat(32)
  let sent
  const send = (_response, status, body) => { sent = { status, body: structuredClone(body) } }
  const request = async (method, suffix, body = {}) => {
    await handleMockSiteAccess({ method }, {}, new URL(`http://localhost/api/v1/sites/${id}/access-passwords${suffix}`),
      async () => body, send)
    return sent
  }
  mockCreatedSiteAccess(id, { scope: 'server', username: 'alice', password: 'synthetic-secret' })
  let result = await request('GET', '')
  assert.equal(result.body.items[0].usernames[0], 'alice')
  assert.equal(JSON.stringify(result).includes('synthetic-secret'), false)
  const first = result.body.items[0]
  result = await request('POST', '/accounts', { action: 'add', ruleId: first.id, resourceVersion: first.resourceVersion, username: 'bob', password: 'another-secret' })
  assert.deepEqual(result.body.items[0].usernames, ['alice', 'bob'])
  result = await request('POST', '/rules', { action: 'delete', ruleId: first.id, resourceVersion: result.body.items[0].resourceVersion })
  assert.deepEqual(result.body.items, [])
})
