import { describe, expect, it } from 'vitest'
import {
  formatNetworkTrafficCounter,
  networkTrafficCounterBytes,
} from './networkTraffic'

describe('network traffic presentation', () => {
  it('formats host receive/send counters independently', () => {
    const value = { receivedBytes: 17.5 * 1024 ** 3, sentBytes: 3 * 1024 ** 3 }

    expect(formatNetworkTrafficCounter(value, 'received')).toBe('17.5 GB')
    expect(formatNetworkTrafficCounter(value, 'sent')).toBe('3.0 GB')
  })

  it('normalizes the desktop widget field names', () => {
    expect(networkTrafficCounterBytes({ totalReceivedBytes: 1024 }, 'received')).toBe(1024)
    expect(networkTrafficCounterBytes({ totalTransmittedBytes: 2048 }, 'sent')).toBe(2048)
  })

  it('does not turn invalid counters into usage', () => {
    expect(networkTrafficCounterBytes({ receivedBytes: Number.NaN }, 'received')).toBeUndefined()
    expect(networkTrafficCounterBytes({ sentBytes: -1 }, 'sent')).toBeUndefined()
    expect(formatNetworkTrafficCounter({ receivedBytes: Number.NaN }, 'received')).toBe('—')
  })
})
