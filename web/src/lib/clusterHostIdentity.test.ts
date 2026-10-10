import { describe, expect, it } from 'vitest'
import { formatCapacityPair, hostLocationLabel, splitAutonomousSystem, usageTone } from './clusterHostIdentity'

describe('splitAutonomousSystem', () => {
  it('separates the ASN from the operator name', () => {
    expect(splitAutonomousSystem('AS31898 Oracle Corporation')).toEqual({ asn: 'AS31898', organization: 'Oracle Corporation' })
    expect(splitAutonomousSystem('as152194 CTG Server Limited')).toEqual({ asn: 'AS152194', organization: 'CTG Server Limited' })
    expect(splitAutonomousSystem('AS13335')).toEqual({ asn: 'AS13335', organization: undefined })
  })

  it('keeps an operator without an ASN prefix intact and ignores empty values', () => {
    expect(splitAutonomousSystem('Hetzner Online GmbH')).toEqual({ organization: 'Hetzner Online GmbH' })
    expect(splitAutonomousSystem('ASPIRE Networks')).toEqual({ organization: 'ASPIRE Networks' })
    expect(splitAutonomousSystem('  ')).toEqual({})
    expect(splitAutonomousSystem()).toEqual({})
  })
})

describe('hostLocationLabel', () => {
  it('joins the known parts and drops a repeated city-state name', () => {
    expect(hostLocationLabel({ country: 'HK', region: 'Hong Kong', city: 'Hong Kong' })).toBe('HK · Hong Kong')
    expect(hostLocationLabel({ country: 'US', region: 'California', city: 'San Jose' })).toBe('US · California · San Jose')
    expect(hostLocationLabel({ city: 'Melbourne' })).toBe('Melbourne')
    expect(hostLocationLabel()).toBe('')
  })
})

describe('usageTone', () => {
  it('raises the meter tone at 75% and 90%', () => {
    expect([0, 74.9, 75, 89.9, 90, 100].map(usageTone)).toEqual(['normal', 'normal', 'warning', 'warning', 'danger', 'danger'])
    expect(usageTone(Number.NaN)).toBe('normal')
    expect(usageTone()).toBe('normal')
  })
})

describe('formatCapacityPair', () => {
  it('prints the shared unit once and keeps both units when they differ', () => {
    expect(formatCapacityPair(41.3 * 1024 ** 3, 146.5 * 1024 ** 3)).toBe('41.3 / 146.5 GB')
    expect(formatCapacityPair(512 * 1024 ** 2, 2 * 1024 ** 3)).toBe('512.0 MB / 2.0 GB')
    expect(formatCapacityPair(undefined, 2 * 1024 ** 3)).toBe('— / 2.0 GB')
  })
})
