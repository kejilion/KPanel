import { describe, expect, it, vi } from 'vitest'
import { TerminalStreamClient } from './terminalStream'

class FakeEventSource {
  static instances: FakeEventSource[] = []
  readyState = 1
  closed = false
  private listeners = new Map<string, Array<(event: MessageEvent<string>) => void>>()

  constructor(readonly url: string) {
    FakeEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: (event: MessageEvent<string>) => void): void {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener])
  }

  emit(type: string, data?: unknown): void {
    for (const listener of this.listeners.get(type) ?? []) {
      listener({ data: data === undefined ? '' : JSON.stringify(data) } as MessageEvent<string>)
    }
  }

  close(): void {
    this.closed = true
    this.readyState = 2
  }
}

function newClient(subscribe = vi.fn().mockResolvedValue({ accepted: true }), idleCloseMs = 0) {
  FakeEventSource.instances = []
  const client = new TerminalStreamClient({
    url: () => '/api/v1/terminal-stream',
    subscribe,
    createSource: (url) => new FakeEventSource(url) as unknown as EventSource,
  }, idleCloseMs)
  return { client, subscribe }
}

const flush = () => new Promise((resolve) => setTimeout(resolve, 20))

describe('TerminalStreamClient', () => {
  it('notifies subscribers when the native EventSource reconnects', () => {
    const { client } = newClient()
    const connected = vi.fn()
    const subscription = client.subscribe({ kind: 'job', job: 'app', id: 'job-a', offset: 0, inputOpen: true }, { connected })
    const source = FakeEventSource.instances[0]!
    source.emit('ready', { streamId: 'first' })
    source.emit('error')
    source.emit('ready', { streamId: 'second' })
    expect(connected).toHaveBeenCalledTimes(2)
    subscription!.close()
  })
  it('subscribes after ready, routes output and tracks offsets for resubscription', async () => {
    const { client, subscribe } = newClient()
    const outputs: string[] = []
    const subscription = client.subscribe({ kind: 'terminal', id: 's1', offset: 5 }, { output: (chunk) => outputs.push(chunk.data) })
    expect(subscription).not.toBeNull()
    const source = FakeEventSource.instances[0]!
    source.emit('ready', { streamId: 'stream-1' })
    await flush()
    expect(subscribe).toHaveBeenCalledWith({ streamId: 'stream-1', add: [{ kind: 'terminal', id: 's1', offset: 5 }], remove: [] })
    source.emit('output', { key: 'terminal:s1', output: { data: 'aGk=', offset: 5, nextOffset: 7, truncated: false, closed: false } })
    expect(outputs).toEqual(['aGk='])
    // A browser reconnect yields a new stream; the subscription resumes at 7.
    source.emit('ready', { streamId: 'stream-2' })
    await flush()
    expect(subscribe).toHaveBeenLastCalledWith({ streamId: 'stream-2', add: [{ kind: 'terminal', id: 's1', offset: 7 }], remove: [] })
    subscription!.close()
    // The idle stream lingers briefly before it closes.
    expect(source.closed).toBe(false)
    await flush()
    expect(source.closed).toBe(true)
  })

  it('reuses the idle stream when a paused terminal resumes within the linger', async () => {
    const { client, subscribe } = newClient(undefined, 1000)
    const first = client.subscribe({ kind: 'job', job: 'app', id: 'a', offset: 0, inputOpen: true }, {})
    const source = FakeEventSource.instances[0]!
    source.emit('ready', { streamId: 'stream' })
    await flush()
    first!.close()
    await flush()
    expect(subscribe).toHaveBeenLastCalledWith({ streamId: 'stream', add: [], remove: ['job:app:a'] })
    const resumed = client.subscribe({ kind: 'job', job: 'app', id: 'a', offset: 42, inputOpen: true }, {})
    await flush()
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(source.closed).toBe(false)
    expect(subscribe).toHaveBeenLastCalledWith({
      streamId: 'stream', add: [{ kind: 'job', job: 'app', id: 'a', offset: 42, inputOpen: true }], remove: [],
    })
    resumed!.close()
  })

  it('batches task subscriptions and removals', async () => {
    const { client, subscribe } = newClient()
    const first = client.subscribe({ kind: 'job', job: 'app', id: 'a', offset: 0, inputOpen: false }, {})
    client.subscribe({ kind: 'terminal', id: 't', offset: 0 }, {})
    FakeEventSource.instances[0]!.emit('ready', { streamId: 'stream' })
    await flush()
    expect(subscribe).toHaveBeenCalledTimes(1)
    expect(subscribe.mock.calls[0]![0].add).toHaveLength(2)
    first!.close()
    await flush()
    expect(subscribe).toHaveBeenLastCalledWith({ streamId: 'stream', add: [], remove: ['job:app:a'] })
  })

  it('falls back when the stream fails before becoming ready', () => {
    const { client } = newClient()
    const unavailable = vi.fn()
    client.subscribe({ kind: 'terminal', id: 's', offset: 0 }, { unavailable })
    const source = FakeEventSource.instances[0]!
    source.close()
    source.emit('error')
    expect(unavailable).toHaveBeenCalledTimes(1)
    expect(client.available).toBe(false)
    expect(client.subscribe({ kind: 'terminal', id: 'next', offset: 0 }, {})).toBeNull()
    client.reset()
    expect(client.available).toBe(true)
  })

  it('hands a rejected subscription back to polling', async () => {
    const subscribe = vi.fn().mockRejectedValue(new Error('limit'))
    const { client } = newClient(subscribe)
    const unavailable = vi.fn()
    client.subscribe({ kind: 'terminal', id: 's', offset: 0 }, { unavailable })
    FakeEventSource.instances[0]!.emit('ready', { streamId: 'stream' })
    await flush()
    expect(unavailable).toHaveBeenCalledTimes(1)
    // Any pumps the server did start for the rejected batch are released.
    await flush()
    expect(subscribe).toHaveBeenLastCalledWith({ streamId: 'stream', add: [], remove: ['terminal:s'] })
  })

  it('stops on auth expiry', () => {
    const { client } = newClient()
    const unavailable = vi.fn()
    client.subscribe({ kind: 'terminal', id: 's', offset: 0 }, { unavailable })
    FakeEventSource.instances[0]!.emit('auth.expired', { message: 'expired' })
    expect(unavailable).toHaveBeenCalled()
    expect(FakeEventSource.instances[0]!.closed).toBe(true)
  })
})
