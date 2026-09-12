import type { Site } from '../types/api'

export function parseSiteAddress(value: string): { host: string; port?: number } | undefined {
  if (value !== value.trim() || value.length > 259) return undefined
  const parts = value.toLowerCase().split(':')
  if (parts.length > 2) return undefined
  const host = parts[0] || ''
  let port: number | undefined
  if (parts.length === 2) {
    if (!/^\d{1,5}$/.test(parts[1] || '')) return undefined
    port = Number(parts[1])
    if (port < 1 || port > 65535) return undefined
  }
  if (/^[0-9.]+$/.test(host)) {
    const octets = host.split('.')
    if (!port || octets.length !== 4 || !octets.every(v => /^(0|[1-9]\d{0,2})$/.test(v) && Number(v) <= 255)) return undefined
  } else if (!/^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(host)) {
    return undefined
  }
  return { host, port }
}

export function sitePublicURL(site: Site): string {
  const candidates = (site.accessUrls || []).filter(raw => {
    try {
      const url = new URL(raw)
      return ['http:', 'https:'].includes(url.protocol) && !url.username && !url.password &&
        url.hostname.replace(/^\[|\]$/g, '') === site.primaryDomain && url.pathname === '/' && !url.search && !url.hash
    } catch { return false }
  })
  const observed = candidates.find(url => url.startsWith('https://')) || candidates[0]
  if (observed) return new URL(observed).origin
  const certificateStatus = site.certificate?.status
  const protocol = certificateStatus && !['missing', 'unknown'].includes(certificateStatus) ? 'https' : 'http'
  const host = site.primaryDomain.includes(':') ? `[${site.primaryDomain}]` : site.primaryDomain
  return `${protocol}://${host}`
}
