// Same-origin fallback for browsers that do not expose Web Locks on HTTP.
// Never expire a live claim on a timer: a suspended tab may still own sessions.
const owners = new Map<string, MessagePort>()
const scope = globalThis as unknown as { onconnect: ((event: MessageEvent) => void) | null }

scope.onconnect = event => {
  const port = event.ports[0]
  if (!port) return
  port.onmessage = message => {
    const data: unknown = message.data
    if (!data || typeof data !== 'object' || !('token' in data) || !('op' in data)) return
    const { token, op } = data as { token: unknown; op: unknown }
    if (typeof token !== 'string' || !/^[a-f0-9]{32}$/.test(token)) return
    if (op === 'claim') {
      const owner = owners.get(token)
      if (owner && owner !== port) port.postMessage({ status: 'busy' })
      else {
        owners.set(token, port)
        port.postMessage({ status: 'acquired' })
      }
    } else if (op === 'release' && owners.get(token) === port) {
      owners.delete(token)
    }
  }
  port.start()
}

export {}
