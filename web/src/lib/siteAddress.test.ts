import { describe, expect, it } from 'vitest'
import { parseSiteAddress, sitePublicURL } from './siteAddress'
import type { Site } from '../types/api'

describe('site address', () => {
  it('keeps domains on the legacy path and selects HTTP only with an explicit valid port', () => {
    expect(parseSiteAddress('Example.COM')).toEqual({ host: 'example.com', port: undefined })
    expect(parseSiteAddress('example.com:08443')).toEqual({ host: 'example.com', port: 8443 })
    expect(parseSiteAddress('192.168.1.10:8080')).toEqual({ host: '192.168.1.10', port: 8080 })
    for (const input of ['192.168.1.10', '999.1.1.1:80', 'x.com:0', 'x.com:65536', 'x.com:', 'x.com:+80', 'https://x.com:80', 'x.com:80/path', 'x.com:80\n', 'x.com:000080']) {
      expect(parseSiteAddress(input), input).toBeUndefined()
    }
  })
  it('uses real listeners even when HTTP has a leftover certificate and handles old API responses', () => {
    const site = { primaryDomain: 'example.com', certificate: { status: 'valid' }, accessUrls: ['http://example.com:8443'] } as Site
    expect(sitePublicURL(site)).toBe('http://example.com:8443')
    expect(sitePublicURL({ ...site, accessUrls: ['http://example.com:80', 'https://example.com:9443'] })).toBe('https://example.com:9443')
    expect(sitePublicURL({ ...site, accessUrls: undefined })).toBe('https://example.com')
    expect(sitePublicURL({ ...site, accessUrls: ['javascript:alert(1)', 'https://other.com:8443'] })).toBe('https://example.com')
    expect(sitePublicURL({ ...site, primaryDomain: '2001:db8::1', accessUrls: ['http://[2001:db8::1]:8080'] })).toBe('http://[2001:db8::1]:8080')
  })
})
