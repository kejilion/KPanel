// Same-origin fallback for browsers that do not expose Web Locks on HTTP.
// Only uncommitted reservations expire: a suspended tab may still own sessions.
type Lease = { port: MessagePort; committed: boolean; timer?: ReturnType<typeof setTimeout> }
const owners = new Map<string, Lease>()
const scope = globalThis as unknown as { onconnect: ((event: MessageEvent) => void) | null }

export function attachLeasePort(port: MessagePort): void {
  port.addEventListener('close', () => {
    for (const [token, lease] of owners) if (lease.port === port) {
      if (lease.timer) clearTimeout(lease.timer)
      owners.delete(token)
    }
  })
  port.onmessage = message => {
    const data: unknown = message.data
    if (!data || typeof data !== 'object' || !('token' in data) || !('op' in data)) return
    const { token, op } = data as { token: unknown; op: unknown }
    if (typeof token !== 'string' || !/^[a-f0-9]{32}$/.test(token)) return
    if (op === 'claim') {
      const existing = owners.get(token)
      if (existing && existing.port !== port) port.postMessage({ status: 'busy' })
      else {
        if (!existing) {
          const lease: Lease = { port, committed: false }
          lease.timer = setTimeout(() => { if (owners.get(token) === lease && !lease.committed) owners.delete(token) }, 5000)
          owners.set(token, lease)
        }
        port.postMessage({ status: existing?.committed ? 'acquired' : 'offered' })
      }
    } else if (op === 'commit') {
      const lease = owners.get(token)
      if (!lease || lease.port !== port) port.postMessage({ status: 'expired' })
      else {
        lease.committed = true
        if (lease.timer) clearTimeout(lease.timer)
        lease.timer = undefined
        port.postMessage({ status: 'acquired' })
      }
    } else if (op === 'release') {
      const lease = owners.get(token)
      if (lease?.port === port) {
        if (lease.timer) clearTimeout(lease.timer)
        owners.delete(token)
      }
      port.postMessage({ status: 'released' })
    }
  }
  port.start()
}

scope.onconnect = event => {
  const port = event.ports[0]
  if (port) attachLeasePort(port)
}
