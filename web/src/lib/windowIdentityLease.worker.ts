// Same-origin fallback for browsers that do not expose Web Locks on HTTP.
// Only uncommitted reservations expire: a suspended tab may still own sessions.
type Lease = { port: WeakRef<MessagePort>; committed: boolean; timer?: ReturnType<typeof setTimeout> }
const owners = new Map<string, Lease>()
const releasedPorts = new FinalizationRegistry<{ token: string; lease: Lease }>(({ token, lease }) => {
  if (owners.get(token) === lease) {
    if (lease.timer) clearTimeout(lease.timer)
    owners.delete(token)
  }
})
const scope = globalThis as unknown as { onconnect: ((event: MessageEvent) => void) | null }

export function attachLeasePort(port: MessagePort): void {
  port.addEventListener('close', () => {
    for (const [token, lease] of owners) if (lease.port.deref() === port) {
      if (lease.timer) clearTimeout(lease.timer)
      releasedPorts.unregister(lease)
      owners.delete(token)
    }
  })
  port.onmessage = message => {
    const data: unknown = message.data
    if (!data || typeof data !== 'object' || !('token' in data) || !('op' in data)) return
    const { token, op } = data as { token: unknown; op: unknown }
    if (typeof token !== 'string' || !/^[a-f0-9]{32}$/.test(token)) return
    if (op === 'claim') {
      let existing = owners.get(token)
      if (existing && !existing.port.deref()) {
        if (existing.timer) clearTimeout(existing.timer)
        releasedPorts.unregister(existing)
        owners.delete(token)
        existing = undefined
      }
      if (existing && existing.port.deref() !== port) port.postMessage({ status: 'busy' })
      else {
        if (!existing) {
          const lease: Lease = { port: new WeakRef(port), committed: false }
          releasedPorts.register(port, { token, lease }, lease)
          lease.timer = setTimeout(() => {
            if (owners.get(token) === lease && !lease.committed) {
              releasedPorts.unregister(lease)
              owners.delete(token)
            }
          }, 5000)
          owners.set(token, lease)
        }
        port.postMessage({ status: existing?.committed ? 'acquired' : 'offered' })
      }
    } else if (op === 'commit') {
      const lease = owners.get(token)
      if (!lease || lease.port.deref() !== port) port.postMessage({ status: 'expired' })
      else {
        lease.committed = true
        if (lease.timer) clearTimeout(lease.timer)
        lease.timer = undefined
        port.postMessage({ status: 'acquired' })
      }
    } else if (op === 'release') {
      const lease = owners.get(token)
      if (lease?.port.deref() === port) {
        if (lease.timer) clearTimeout(lease.timer)
        releasedPorts.unregister(lease)
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
