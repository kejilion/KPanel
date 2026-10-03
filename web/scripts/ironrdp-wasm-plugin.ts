import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import type { Plugin } from 'vite'

// Upstream embeds WASM as a data URL. Extract the immutable package payload to
// a same-origin asset so connect-src does not have to permit data: or a CDN.
export function ironRdpWasm(): Plugin {
  const require = createRequire(import.meta.url)
  const entry = require.resolve('@devolutions/iron-remote-desktop-rdp')
  const source = readFileSync(entry, 'utf8')
  const match = source.match(/"data:application\/wasm;base64,([A-Za-z0-9+/=]+)"/)
  if (!match?.[1]) throw new Error('IronRDP WASM asset layout changed; review the dependency before building')
  const bytes = Buffer.from(match[1], 'base64')
  if (!bytes.subarray(0, 8).equals(Buffer.from([0, 97, 115, 109, 1, 0, 0, 0]))) throw new Error('Invalid IronRDP WASM header')
  let development = false
  return {
    name: 'kpanel-ironrdp-wasm',
    enforce: 'pre',
    configResolved(config) { development = config.command === 'serve' },
    configureServer(server) {
      server.middlewares.use('/@kpanel/ironrdp.wasm', (_request, response) => {
        response.setHeader('Content-Type', 'application/wasm')
        response.setHeader('Cache-Control', 'no-store')
        response.end(bytes)
      })
    },
    transform(code, id) {
      if (id.replaceAll('\\', '/') !== entry.replaceAll('\\', '/')) return
      const url = development ? JSON.stringify('/@kpanel/ironrdp.wasm')
        : `import.meta.ROLLUP_FILE_URL_${this.emitFile({ type: 'asset', name: 'ironrdp.wasm', source: bytes })}`
      return { code: code.replace(match[0], url), map: null }
    },
  }
}
