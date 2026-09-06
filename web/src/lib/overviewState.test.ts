import { describe, expect, it } from 'vitest'
import { mergeOverviewUpdate, pendingOverview } from './overviewState'

describe('overview background observations', () => {
  it('retains only completed observations and clearly marks refreshing without mutating inputs', () => {
    const previous = pendingOverview()
    previous.reads!.config = { state: 'ready', observedAt: 'old' }
    previous.reads!.bbrv3 = { state: 'ready', observedAt: 'old-bbr' }
    previous.management.dns.servers = ['1.1.1.1']
    previous.management.bbrv3.installed = true
    const incoming = pendingOverview()
    incoming.reads!.runtime = { state: 'ready' }
    incoming.management.swap.totalBytes = 2048
    const merged = mergeOverviewUpdate(previous, incoming)
    expect(merged.management.dns.servers).toEqual(['1.1.1.1'])
    expect(merged.management.bbrv3.installed).toBe(true)
    expect(merged.management.swap.totalBytes).toBe(2048)
    expect(merged.reads!.config).toEqual({ state: 'ready', observedAt: 'old', refreshing: true })
    expect(merged.reads!['ssh-defense']?.state).toBe('loading')
    expect(incoming.management.dns.servers).toEqual([])
    expect(previous.reads!.config?.refreshing).toBeUndefined()
  })

  it('never replaces a fresh success or failure with an old successful observation', () => {
    const previous = pendingOverview()
    previous.reads!.config = { state: 'ready', observedAt: 'old' }
    previous.management.dns.servers = ['old']
    const incoming = pendingOverview()
    incoming.reads!.config = { state: 'error' }
    expect(mergeOverviewUpdate(previous, incoming).reads!.config?.state).toBe('error')
    incoming.reads!.config = { state: 'ready', observedAt: 'new' }
    incoming.management.dns.servers = ['new']
    expect(mergeOverviewUpdate(previous, incoming).management.dns.servers).toEqual(['new'])
  })
})
