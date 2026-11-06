import type { MonitoringCheckKind } from '@/types/api'

function validHost(value: string): boolean {
  if (!value || /[\s/@\\?#\[\]]/.test(value)) return false
  if (value.includes(':')) {
    const [ip, zone, ...extra] = value.split('%')
    if (extra.length || zone === '') return false
    try { return Boolean(new URL(`http://[${ip}]/`).hostname) } catch { return false }
  }
  if (value.includes('%')) return false
  // Reject noncanonical numeric hosts rather than letting URL reinterpret them
  // as short/octal IPv4 addresses which the Agent does not accept.
  if (/^[\d.]+$/.test(value)) {
    const octets = value.split('.')
    return octets.length === 4 && octets.every((part) => /^(0|[1-9]\d{0,2})$/.test(part) && Number(part) <= 255)
  }
  try {
    const host = new URL(`http://${value}/`).hostname.replace(/\.$/, '')
    return !/^[\d.]+$/.test(host) && host.length <= 253 && host.split('.').every((label) =>
      /^[a-z0-9_](?:[a-z0-9_-]{0,61}[a-z0-9_])?$/i.test(label))
  } catch { return false }
}

export function validMonitoringCheckTarget(kind: MonitoringCheckKind, target: string): boolean {
  if (target !== target.trim() || target.length > 2048 || /[\u0000-\u001f\u007f]/.test(target)) return false
  if (kind === 'ping') return validHost(target)
  if (kind === 'tcp') {
    const match = target.match(/^(?:\[([^\]]+)\]|([^:\s]+)):(\d{1,5})$/)
    return Boolean(match && (!match[1] || match[1].includes(':')) && validHost(match[1] || match[2] || '') && Number(match[3]) >= 1 && Number(match[3]) <= 65535)
  }
  try {
    const authority = target.match(/^https?:\/\/([^/?#]*)/)?.[1] || ''
    let host = authority.startsWith('[')
      ? authority.slice(1, authority.indexOf(']')) : authority.replace(/:\d+$/, '')
    let parseTarget = target
    if (authority.startsWith('[') && host.includes('%')) {
      if (!host.includes('%25')) return false
      const encodedHost = host
      host = decodeURIComponent(host)
      // WHATWG URL rejects scope IDs; validate the host/scope separately and
      // use its parser for the rest of the URL. The Agent receives the original.
      parseTarget = target.replace(`[${encodedHost}]`, `[${host.split('%')[0]}]`)
    }
    const url = new URL(parseTarget)
    return /^https?:\/\//.test(target) && !target.includes('\\') && !url.username && !url.password &&
      !authority.endsWith(':') && validHost(host) && (!url.port || Number(url.port) >= 1)
  } catch { return false }
}
