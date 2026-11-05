// A newly opened or duplicated browser tab may receive a copy of its opener's
// sessionStorage. A per-tab exclusive lease prevents a copied snapshot from
// ever being used to close another tab's sessions.
const tabTokenKey = 'kpanel:window-tab-token'
let tabLease: Promise<string> | undefined
let releaseLease: (() => void) | undefined
let generation = 0

function randomToken(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), value => value.toString(16).padStart(2, '0')).join('')
}

function tryWebLease(token: string, attemptGeneration: number): Promise<'busy' | string> {
  return new Promise(resolve => {
    let settled = false
    const finish = (value: string) => {
      if (settled) return
      settled = true
      window.clearTimeout(timer)
      resolve(value)
    }
    const timer = window.setTimeout(() => finish(''), 300)
    try {
      void navigator.locks.request(`kpanel:window-tab:${token}`, { ifAvailable: true }, lock => {
        if (!lock) { finish('busy'); return }
        if (settled || generation !== attemptGeneration) { finish(''); return }
        return new Promise<void>(release => {
          releaseLease = release
          finish(token)
        })
      }).catch(() => finish(''))
    } catch { finish('') }
  })
}

function trySharedLease(token: string, attemptGeneration: number): Promise<'busy' | string> {
  return new Promise(resolve => {
    let settled = false
    let worker: SharedWorker | undefined
    const finish = (value: string) => {
      if (settled) return
      settled = true
      window.clearTimeout(timer)
      if (value !== token) worker?.port.close()
      resolve(value)
    }
    const timer = window.setTimeout(() => finish(''), 750)
    try {
      worker = new SharedWorker(new URL('./windowIdentityLease.worker.ts', import.meta.url), { type: 'module', name: 'kpanel-window-identity-v1' })
      const port = worker.port
      worker.onerror = () => finish('')
      port.onmessage = event => {
        if (event.data?.status === 'busy') { finish('busy'); return }
        if (event.data?.status !== 'acquired' || settled || generation !== attemptGeneration) { finish(''); return }
        releaseLease = () => { port.postMessage({ op: 'release', token }); port.close() }
        finish(token)
      }
      port.start()
      port.postMessage({ op: 'claim', token })
    } catch { finish('') }
  })
}

async function acquireTabLease(attemptGeneration: number): Promise<string> {
  const locks = (navigator as unknown as { locks?: LockManager }).locks
  const claim = locks ? tryWebLease : typeof SharedWorker === 'function' ? trySharedLease : undefined
  if (!claim) return ''
  try {
    let token = window.sessionStorage.getItem(tabTokenKey) || ''
    if (!/^[a-f0-9]{32}$/.test(token)) {
      token = randomToken()
      window.sessionStorage.setItem(tabTokenKey, token)
    }
    let acquired = await claim(token, attemptGeneration)
    if (acquired === 'busy' && generation === attemptGeneration) {
      token = randomToken()
      window.sessionStorage.setItem(tabTokenKey, token)
      acquired = await claim(token, attemptGeneration)
    }
    return acquired === 'busy' ? '' : acquired
  } catch { return '' }
}

function resetTabLease(): void {
  generation += 1
  releaseLease?.()
  releaseLease = undefined
  tabLease = undefined
}
window.addEventListener('pagehide', event => { if (!event.persisted) resetTabLease() })

// Window snapshots contain only opaque identities, never output or commands.
// On browsers without an exclusive tab lease, skip persistence rather than
// risk attributing an opener's copied sessionStorage to a new tab.
export async function windowIdentityKey(kind: string, owner: string, windowId: string | undefined, target = ''): Promise<string> {
  if (!owner || !windowId) return ''
  const expectedGeneration = generation
  const lease = tabLease ||= acquireTabLease(expectedGeneration)
  const token = await lease
  if (!token && tabLease === lease) tabLease = undefined
  if (generation !== expectedGeneration) return ''
  if (!token) return ''
  return ['kpanel:window-identities', token, kind, owner, windowId, target].map(encodeURIComponent).join(':')
}

/** Release the in-memory test lease between isolated browser fixtures. */
export function resetWindowIdentityForTest(): void { resetTabLease() }

export function readWindowIdentities(key: string, pattern: RegExp, limit: number): string[] {
  if (!key) return []
  try {
    const value: unknown = JSON.parse(window.sessionStorage.getItem(key) || '[]')
    if (!Array.isArray(value)) return []
    return [...new Set(value.filter((id): id is string => typeof id === 'string' && pattern.test(id)))].slice(0, limit)
  } catch { return [] }
}

export function writeWindowIdentities(key: string, ids: string[]): void {
  if (!key) return
  try { window.sessionStorage.setItem(key, JSON.stringify(ids)) } catch { /* best effort */ }
}
