import { describe, expect, it } from 'vitest'
import {
  formatNetworkTrafficCounter,
  formatTotalNetworkTraffic,
  networkTrafficCounterBytes,
  totalNetworkTrafficBytes,
} from './networkTraffic'

describe('network traffic presentation', () => {
  it('uses the host receive/send counters as one cumulative total', () => {
    const value = { receivedBytes: 17.5 * 1024 ** 3, sentBytes: 3 * 1024 ** 3 }

    expect(totalNetworkTrafficBytes(value)).toBe(20.5 * 1024 ** 3)
    expect(formatTotalNetworkTraffic(value)).toBe('20.5 GB')
    expect(formatNetworkTrafficCounter(value, 'received')).toBe('17.5 GB')
    expect(formatNetworkTrafficCounter(value, 'sent')).toBe('3.0 GB')
  })

  it('normalizes the desktop widget field names and preserves public totals', () => {
    expect(networkTrafficCounterBytes({ totalReceivedBytes: 1024 }, 'received')).toBe(1024)
    expect(networkTrafficCounterBytes({ totalTransmittedBytes: 2048 }, 'sent')).toBe(2048)
    expect(totalNetworkTrafficBytes({ totalBytes: 4096 })).toBe(4096)
    expect(formatTotalNetworkTraffic({ totalBytes: 4096 })).toBe('4.0 KB')
  })

  it('does not turn invalid counters into usage', () => {
    expect(totalNetworkTrafficBytes({ receivedBytes: Number.NaN, sentBytes: -1 })).toBeUndefined()
    expect(formatTotalNetworkTraffic({ receivedBytes: Number.NaN, sentBytes: -1 })).toBe('—')
  })
})
