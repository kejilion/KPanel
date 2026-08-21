// A ProxyTransport (see @mercuryworkshop/proxy-transports' dist/types.d.ts)
// backed by KPanel's own /api/v1/browse/* endpoints instead of Scramjet-SSH's
// original SSH-tunnel relay. The egress is whichever host the caller picked
// (hostId — "local" or an already-paired cluster.HostKindPanel node); there
// is no per-session SSH credential to carry, only hostId.
//
// Two wire-format differences from the original ssh-tunnel-transport.js,
// both load-bearing:
//
// 1. KPanel's /api/v1/browse/fetch is a single buffered JSON round trip
//    (statusCode/headers/body, body base64), not a streamed HTTP response
//    with metadata packed into a response header — so request() has to
//    materialize the request body into base64 up front (no streaming
//    upload) and decode the JSON response body into bytes before handing it
//    back to the controller.
// 2. Unlike the SSH-based transport (which never supported target-site
//    WebSocket egress at all — see its connect() comment), this transport
//    DOES support it, via /api/v1/browse/ws-sessions' Open/Output/Input/Close
//    long-poll relay (see internal/agent/browse_ws.go and
//    internal/panel/browse_ws.go).
// The browse origin has its own credential; the panel's cookies are host-only
// and never reach this origin. See internal/panel/browse_origin.go.
function readCsrfCookie() {
  const match = document.cookie.match(/(?:^|; )kejilion_browse_csrf=([^;]*)/)
  return match ? decodeURIComponent(match[1]) : ''
}

function headersToRecord(headers) {
  // `headers` arrives as whatever @mercuryworkshop/scramjet-controller's
  // ScramjetHeaders.toRawHeaders() produces: an array of [name, value]
  // pairs. Duplicate names (e.g. repeated Accept) fold into one
  // comma-joined value, matching how net/http.Header round-trips through
  // Go's map[string][]string on our side.
  const record = {}
  for (const entry of headers || []) {
    const name = Array.isArray(entry) ? entry[0] : entry?.name
    const value = Array.isArray(entry) ? entry[1] : entry?.value
    if (!name) continue
    if (!record[name]) record[name] = []
    record[name].push(String(value ?? ''))
  }
  return record
}

function recordToHeaderPairs(record) {
  const pairs = []
  for (const [name, values] of Object.entries(record || {})) {
    for (const value of values || []) pairs.push([name, value])
  }
  return pairs
}

async function bodyToBase64(body) {
  if (body == null) return ''
  let bytes
  if (body instanceof ArrayBuffer) {
    bytes = new Uint8Array(body)
  } else if (ArrayBuffer.isView(body)) {
    bytes = new Uint8Array(body.buffer, body.byteOffset, body.byteLength)
  } else if (typeof body.getReader === 'function') {
    const chunks = []
    const reader = body.getReader()
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      chunks.push(value)
    }
    const total = chunks.reduce((sum, chunk) => sum + chunk.length, 0)
    bytes = new Uint8Array(total)
    let offset = 0
    for (const chunk of chunks) {
      bytes.set(chunk, offset)
      offset += chunk.length
    }
  } else if (typeof body === 'string') {
    bytes = new TextEncoder().encode(body)
  } else if (typeof body.arrayBuffer === 'function') {
    bytes = new Uint8Array(await body.arrayBuffer())
  } else {
    return ''
  }
  let binary = ''
  const chunkSize = 0x8000
  for (let i = 0; i < bytes.length; i += chunkSize) {
    binary += String.fromCharCode.apply(null, bytes.subarray(i, i + chunkSize))
  }
  return btoa(binary)
}

function base64ToBytes(b64) {
  if (!b64) return new Uint8Array(0)
  const binary = atob(b64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes
}

export class BrowseTransport {
  constructor({ hostId } = {}) {
    this.hostId = hostId
    this.ready = true
    this.wsSockets = new Map()
  }

  async init() {}

  async request(remote, method, body, headers, signal) {
    const requestBody = await bodyToBase64(body)
    const res = await fetch('/api/v1/browse/fetch', {
      method: 'POST',
      signal,
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
        Origin: location.origin,
        'X-CSRF-Token': readCsrfCookie(),
      },
      body: JSON.stringify({
        hostId: this.hostId,
        url: remote.toString(),
        method,
        headers: headersToRecord(headers),
        body: requestBody || undefined,
      }),
    })
    if (!res.ok) {
      throw new Error('browse fetch relay failed with status ' + res.status)
    }
    const meta = await res.json()
    const bytes = base64ToBytes(meta.body)
    return {
      body: bytes.buffer,
      headers: recordToHeaderPairs(meta.headers),
      status: meta.statusCode,
      statusText: '',
    }
  }

  connect(url, protocols, requestHeaders, onopen, onmessage, onclose, onerror) {
    let closed = false
    let sessionId = null
    let offset = 0
    let polling = false

    const csrfHeaders = () => ({
      'Content-Type': 'application/json',
      Origin: location.origin,
      'X-CSRF-Token': readCsrfCookie(),
    })

    const poll = async () => {
      if (closed || !sessionId || polling) return
      polling = true
      try {
        const res = await fetch(
          '/api/v1/browse/ws-sessions/' + encodeURIComponent(sessionId) + '/output?offset=' + offset + '&wait=1500',
          { credentials: 'same-origin' },
        )
        if (!res.ok) throw new Error('browse ws output failed with status ' + res.status)
        const data = await res.json()
        offset = data.nextOffset
        for (const message of data.messages || []) {
          const bytes = base64ToBytes(message.data)
          onmessage(message.type === 'binary' ? bytes.buffer : new TextDecoder().decode(bytes))
        }
        if (data.closed) {
          closed = true
          onclose(1000, data.closeReason || '')
          return
        }
      } catch (err) {
        if (!closed) {
          closed = true
          onerror(err.message || String(err))
        }
        return
      } finally {
        polling = false
      }
      if (!closed) queueMicrotask(poll)
    }

    ;(async () => {
      try {
        const openRes = await fetch('/api/v1/browse/ws-sessions', {
          method: 'POST',
          credentials: 'same-origin',
          headers: csrfHeaders(),
          body: JSON.stringify({ hostId: this.hostId, url: url.toString(), headers: headersToRecord(requestHeaders) }),
        })
        if (!openRes.ok) throw new Error('browse ws open failed with status ' + openRes.status)
        const opened = await openRes.json()
        if (closed) return
        sessionId = opened.sessionId
        onopen('', []);
        poll()
      } catch (err) {
        if (!closed) {
          closed = true
          onerror(err.message || String(err))
        }
      }
    })()

    const send = (data) => {
      if (closed || !sessionId) return
      const binary = !(typeof data === 'string')
      const payload = binary
        ? (() => {
            const bytes = data instanceof ArrayBuffer ? new Uint8Array(data) : new Uint8Array(data.buffer, data.byteOffset, data.byteLength)
            let str = ''
            for (const byte of bytes) str += String.fromCharCode(byte)
            return btoa(str)
          })()
        : btoa(unescape(encodeURIComponent(data)))
      fetch('/api/v1/browse/ws-sessions/' + encodeURIComponent(sessionId) + '/input', {
        method: 'POST',
        credentials: 'same-origin',
        headers: csrfHeaders(),
        body: JSON.stringify({ type: binary ? 'binary' : 'text', data: payload }),
      }).catch(() => {})
    }

    const close = () => {
      if (closed) return
      closed = true
      if (sessionId) {
        fetch('/api/v1/browse/ws-sessions/' + encodeURIComponent(sessionId) + '/close', {
          method: 'POST',
          credentials: 'same-origin',
          headers: csrfHeaders(),
          body: '{}',
        }).catch(() => {})
      }
    }

    return [send, close]
  }

  meta() {
    return {}
  }
}
