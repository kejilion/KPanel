// In-memory UI preview data only. Passwords are never retained or echoed.
import { randomBytes } from 'node:crypto'

const rulesBySite = new Map()
let revision = 0
const nextVersion = () => `sha256:${(++revision).toString(16).padStart(64, '0')}`
const makeRule = (input) => ({
  id: randomBytes(16).toString('hex'), scope: input.scope, path: input.path || '/', realm: 'Restricted',
  usernames: [input.username], accountsReadable: true, resourceVersion: nextVersion(),
})

export function mockCreatedSiteAccess(siteId, input) {
  if (input?.username && input?.password) rulesBySite.set(siteId, [makeRule(input)])
}

export async function handleMockSiteAccess(request, response, url, readJSON, send) {
  const match = url.pathname.match(/^\/api\/v1\/sites\/([a-f0-9]{32})\/access-passwords(?:\/(rules|accounts))?$/)
  if (!match) return false
  const [, siteId, action] = match
  const rules = rulesBySite.get(siteId) || []
  if (request.method === 'GET' && !action) {
    send(response, 200, { items: rules })
    return true
  }
  if (request.method !== 'POST' || !action) {
    send(response, 405, { title: '请求方法不允许' })
    return true
  }
  const input = await readJSON(request)
  if (action === 'rules' && input.action === 'add' && input.username && input.password) {
    rules.push(makeRule(input))
    rulesBySite.set(siteId, rules)
    send(response, 200, { items: rules })
    return true
  }
  const index = rules.findIndex((rule) => rule.id === input.ruleId && rule.resourceVersion === input.resourceVersion)
  if (index < 0) {
    send(response, 409, { title: '规则已发生变化' })
    return true
  }
  const rule = rules[index]
  if (action === 'rules' && input.action === 'delete') {
    rules.splice(index, 1)
  } else if (action === 'accounts' && input.action === 'add' && input.username && input.password) {
    rule.usernames.push(input.username)
    rule.resourceVersion = nextVersion()
  } else if (action === 'accounts' && input.action === 'update' && input.username && input.password && rule.usernames.includes(input.previousUsername)) {
    rule.usernames = rule.usernames.map((name) => name === input.previousUsername ? input.username : name)
    rule.resourceVersion = nextVersion()
  } else if (action === 'accounts' && input.action === 'delete' && rule.usernames.includes(input.username)) {
    rule.usernames = rule.usernames.filter((name) => name !== input.username)
    rule.resourceVersion = nextVersion()
  } else {
    send(response, 422, { title: '模拟操作无效' })
    return true
  }
  send(response, 200, { items: rules })
  return true
}
