export function parseClusterHostPrice(price: string | undefined) {
  // A single amount only: offers, ranges and unknown cycles are not inferred.
  const match = price?.normalize('NFKC').trim().match(/^(?:([a-z]{3}|US\$|HK\$|[¥$€£])\s*)?((?:\d{1,3}(?:,\d{3})+|\d+)(?:\.\d+)?)\s*([a-z]{3}|元|美元|欧元|歐元|英镑|英鎊)?(?:\s*(?:\/|每|per\s+)\s*(\d+)?\s*(月|个月|個月|季|季度|半年|年|mo|month|months|quarter|quarters|yr|year|years))?$/i)
  if (!match || (match[1] && match[3])) return undefined
  const currencies: Record<string, string> = {
    '¥': 'CNY', RMB: 'CNY', 元: 'CNY', '$': 'USD', 'US$': 'USD', 美元: 'USD',
    'HK$': 'HKD', '€': 'EUR', 欧元: 'EUR', 歐元: 'EUR', '£': 'GBP', 英镑: 'GBP', 英鎊: 'GBP',
  }
  const unit = (match[1] || match[3] || '').toUpperCase()
  const currency = currencies[unit] || unit
  const months: Record<string, number> = {
    月: 1, 个月: 1, 個月: 1, mo: 1, month: 1, months: 1, 季: 3, 季度: 3, quarter: 3, quarters: 3,
    半年: 6, 年: 12, yr: 12, year: 12, years: 12,
  }
  const period = match[5]?.toLowerCase()
  const count = Number(match[4] || 1)
  if (!Number.isFinite(count) || count <= 0) return undefined
  const decimal = match[2]!.replaceAll(',', '')
  const amount = Number(decimal)
  const cycleMonths = period ? months[period]! * count : undefined
  const monthlyAmount = cycleMonths ? amount / cycleMonths : amount
  if (!Number.isFinite(monthlyAmount)) return undefined
  return {
    amount, currency, cycleMonths, monthlyAmount,
    monthlyFraction: {
      numerator: BigInt(decimal.replace('.', '')),
      denominator: 10n ** BigInt(decimal.split('.')[1]?.length || 0)
        * (period ? BigInt(months[period]!) * BigInt(match[4] || 1) : 1n),
    },
  }
}
