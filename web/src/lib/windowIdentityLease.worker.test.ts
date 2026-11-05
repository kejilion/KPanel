// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { attachLeasePort } from './windowIdentityLease.worker'

class TestPort extends EventTarget {
  onmessage: ((event: MessageEvent) => void) | null = null
  messages: unknown[] = []
  start(): void { /* no-op */ }
  postMessage(value: unknown): void { this.messages.push(value) }
  receive(op: string, token: string): void { this.onmessage?.({ data: { op, token } } as MessageEvent) }
}

describe('window identity shared-worker leases', () => {
  it('releases a disconnected port without releasing another port’s claim', () => {
    const first = new TestPort(), second = new TestPort(), token = 'a'.repeat(32)
    attachLeasePort(first as unknown as MessagePort)
    attachLeasePort(second as unknown as MessagePort)
    first.receive('claim', token)
    first.receive('commit', token)
    second.receive('claim', token)
    expect(first.messages).toEqual([{ status: 'offered' }, { status: 'acquired' }])
    expect(second.messages).toEqual([{ status: 'busy' }])
    second.receive('release', token)
    second.receive('claim', token)
    expect(second.messages.at(-1)).toEqual({ status: 'busy' })
    first.dispatchEvent(new Event('close'))
    second.receive('claim', token)
    second.receive('commit', token)
    expect(second.messages.at(-1)).toEqual({ status: 'acquired' })
    second.dispatchEvent(new Event('close'))
  })
  it('expires only an uncommitted reservation after a lost acknowledgement', () => {
    vi.useFakeTimers()
    try {
      const first = new TestPort(), second = new TestPort(), token = 'b'.repeat(32)
      attachLeasePort(first as unknown as MessagePort)
      attachLeasePort(second as unknown as MessagePort)
      first.receive('claim', token)
      vi.advanceTimersByTime(5000)
      second.receive('claim', token)
      expect(second.messages.at(-1)).toEqual({ status: 'offered' })
      second.receive('commit', token)
      vi.advanceTimersByTime(6000)
      first.receive('claim', token)
      expect(first.messages.at(-1)).toEqual({ status: 'busy' })
      second.dispatchEvent(new Event('close'))
    } finally { vi.useRealTimers() }
  })
})
