import { describe, expect, it } from 'vitest'
import { estimateRemainingValue, formatClusterMoney, summarizeRemainingValue } from './clusterRemainingValue'

const now = new Date(2026, 0, 1, 12)
describe('cluster remaining prepaid value estimates', () => {
  it('prorates annual, monthly and multi-year prices and caps advance renewal at one cycle', () => {
    expect(estimateRemainingValue({ price: '¥365/年', expiresOn: '2026-07-02' }, now)).toMatchObject({ remaining: 182, remainingDays: 182, currency: 'CNY', cycleDays: 365 })
    expect(estimateRemainingValue({ price: '$30/month', expiresOn: '2026-01-16' }, now)).toMatchObject({ remaining: 15, cycleDays: 30 })
    expect(estimateRemainingValue({ price: 'HK$730/2年', expiresOn: '2026-01-11' }, now)).toMatchObject({ remaining: 10, cycleDays: 730 })
    expect(estimateRemainingValue({ price: 'EUR 545/18 months', expiresOn: '2026-01-11' }, now)).toMatchObject({ remaining: 10, cycleDays: 545 })
    expect(estimateRemainingValue({ price: '$5/month', expiresOn: '9999-12-31' }, now)).toMatchObject({ remaining: 5 })
  })

  it('uses whole local calendar dates, including leap days and expiry day zero', () => {
    expect(estimateRemainingValue({ price: '$30/月', expiresOn: '2024-03-01' }, new Date(2024, 1, 28, 23, 59))).toMatchObject({ remaining: 2, remainingDays: 2 })
    expect(estimateRemainingValue({ price: '$30/月', expiresOn: '2026-01-01' }, now)).toMatchObject({ status: 'expired', remaining: 0 })
    expect(estimateRemainingValue({ price: '$30/月', expiresOn: '2025-12-31' }, now)).toMatchObject({ status: 'expired', remaining: 0 })
    expect(estimateRemainingValue({ price: '$30/月', expiresOn: '2026-03-09' }, new Date(2026, 2, 7, 23))).toMatchObject({ remainingDays: 2 })
  })

  it.each(['$5', '5/月', '首年$1，续费$10', '$1–5/月', '$-5/月', '$5/0月', '$5/1001年', '$1000000000001/年', '<b>$5/月</b>'])('excludes ambiguous or unsupported price %s without inventing zero', price => {
    expect(estimateRemainingValue({ price, expiresOn: '2026-01-16' }, now)).toEqual({ status: 'unknownPrice' })
  })

  it('distinguishes missing information from an explicit free price and rejects impossible dates', () => {
    expect(estimateRemainingValue(undefined, now)).toEqual({ status: 'missingPrice' })
    expect(estimateRemainingValue({ price: '$5/月' }, now)).toEqual({ status: 'missingExpiry' })
    for (const expiresOn of ['2026-02-29', '2026-04-31', '0000-01-01', '01/02/2026', 'invalid']) {
      expect(estimateRemainingValue({ price: '$5/月', expiresOn }, now)).toEqual({ status: 'invalidExpiry' })
    }
    expect(estimateRemainingValue({ price: '$0/月', expiresOn: '2026-01-16' }, now)).toMatchObject({ status: 'active', remaining: 0 })
    expect(estimateRemainingValue({ price: '$5/月', expiresOn: '2026-01-16' }, new Date(NaN))).toEqual({ status: 'invalidExpiry' })
  })

  it('groups currencies, includes expired zeroes, and ignores metadata for removed hosts', () => {
    const hosts = ['cny', 'usd', 'expired', 'missing'].map(id => ({ id, name: id }))
    const summary = summarizeRemainingValue(hosts, {
      cny: { price: '¥30/月', expiresOn: '2026-01-16' },
      usd: { price: '$30/month', expiresOn: '2026-01-11' },
      expired: { price: '$5/月', expiresOn: '2025-01-01' },
      removed: { price: '$100/月', expiresOn: '2027-01-01' },
    }, now)
    expect(summary).toMatchObject({ included: 3, excluded: 1, expired: 1, groups: [
      { currency: 'CNY', remaining: 15, price: 30, monthlyCost: 30, count: 1 },
      { currency: 'USD', remaining: 10, price: 35, monthlyCost: 35, count: 2 },
    ] })
    expect(summary.rows.map(row => row.host.id)).toEqual(hosts.map(host => host.id))
    expect(summarizeRemainingValue([], {}, now).groups).toEqual([])
    expect(formatClusterMoney(15, 'CNY', 'zh-CN')).toBe('¥15.00')
  })
})
