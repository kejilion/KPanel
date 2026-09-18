// @vitest-environment jsdom
import { Buffer } from 'node:buffer'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import BatchTerminalPanel from './BatchTerminalPanel.vue'
import type { ClusterHost } from '@/types/api'

const mocks = vi.hoisted(() => ({
  open: vi.fn(),
  input: vi.fn(),
  output: vi.fn(),
  close: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {
    constructor(readonly message: string, readonly status = 0, readonly code = 'request_failed') {
      super(message)
    }
  },
  api: { terminals: mocks },
}))

function host(id: string, name: string, osId: string): ClusterHost {
  return {
    id,
    isLocal: id === 'local',
    name,
    origin: id === 'local' ? '' : `https://${id}.example.test`,
    transportSecurity: 'tls',
    remoteNodeId: id,
    federationProtocol: 'v2',
    scope: 'cluster.summary.read cluster.terminal.open',
    terminalAvailable: true,
    mutualFileTransferAvailable: false,
    state: 'online',
    consecutiveFailures: 0,
    polling: false,
    resourceVersion: `${id}-rv`,
    createdAt: '2026-09-15T00:00:00Z',
    updatedAt: '2026-09-15T00:00:00Z',
    lastSnapshot: {
      telemetry: {
        agentVersion: '1.0.0', agentProtocolVersion: 'v1', hostname: id, os: osId,
        osId, uptimeSeconds: 60, load: { one: 0, five: 0, fifteen: 0 },
        cpu: { cores: 2, usagePercent: 1 },
        memory: { totalBytes: 1024, availableBytes: 512, usedBytes: 512, usagePercent: 50 },
        disk: { totalBytes: 1024, usedBytes: 128, usagePercent: 12.5 },
        network: { receivedBytes: 0, sentBytes: 0, tcpConnections: 0, udpConnections: 0 },
        publicNetwork: {}, collectedAt: '2026-09-15T00:00:00Z',
      },
      receivedAt: '2026-09-15T00:00:00Z', latencyMilliseconds: 2,
      receiveBytesPerSecond: 0, transmitBytesPerSecond: 0,
    },
  }
}

function decodeInput(value: string): string {
  return Buffer.from(value, 'base64').toString('utf8')
}

describe('BatchTerminalPanel', () => {
  beforeEach(() => {
    mocks.open.mockReset()
    mocks.input.mockReset().mockResolvedValue({ accepted: true })
    mocks.output.mockReset()
    mocks.close.mockReset().mockResolvedValue({ closed: true })
  })

  it('sends the same custom command as terminal input and reports each host result', async () => {
    const hosts = [host('local', '本机', 'debian'), host('edge', '边缘节点', 'ubuntu')]
    mocks.open.mockImplementation(async (hostID: string) => ({ sessionId: `session-${hostID}`, offset: 0 }))
    mocks.output.mockImplementation(async (sessionID: string) => ({
      data: Buffer.from(sessionID === 'session-local' ? '\u001b[32mlocal ok\u001b[0m\r\n' : 'remote failed\r\n').toString('base64'),
      offset: 0,
      nextOffset: 16,
      truncated: false,
      exitedAt: '2026-09-15T00:00:02Z',
      exitError: sessionID === 'session-edge' ? 'exit status 2' : undefined,
      closed: false,
    }))
    const wrapper = mount(BatchTerminalPanel, { props: { hosts, sessionCapacity: 2 } })

    await wrapper.get('textarea').setValue('uname -a')
    await wrapper.get('button.button--primary').trigger('click')
    await flushPromises()

    expect(mocks.open).toHaveBeenCalledTimes(2)
    expect(mocks.input).toHaveBeenCalledTimes(2)
    expect(mocks.input.mock.calls.map((call) => decodeInput(call[1]))).toEqual([
      'uname -a\r',
      'uname -a\r',
    ])
    expect(mocks.input.mock.calls.every((call) => !decodeInput(call[1]).includes('/bin/sh -c'))).toBe(true)
    expect(wrapper.text()).toContain('执行成功')
    expect(wrapper.text()).toContain('执行失败')
    expect(wrapper.html()).not.toContain('\u001b[32m')
    const summaries = wrapper.findAll('.batch-result__summary')
    expect(summaries).toHaveLength(2)
    const details = wrapper.findAll('.batch-result__detail')
    expect(summaries.every((summary) => summary.attributes('aria-expanded') === 'false')).toBe(true)
    expect(details.every((detail) => (detail.element as HTMLElement).style.display === 'none')).toBe(true)

    await summaries[0]!.trigger('click')
    expect(summaries[0]!.attributes('aria-expanded')).toBe('true')
    expect(((details[0]!.element) as HTMLElement).style.display).toBe('')
    expect(((details[1]!.element) as HTMLElement).style.display).toBe('none')

    await summaries[0]!.trigger('click')
    expect(summaries[0]!.attributes('aria-expanded')).toBe('false')
    expect(((details[0]!.element) as HTMLElement).style.display).toBe('none')

    await summaries[1]!.trigger('click')
    expect(((details[1]!.element) as HTMLElement).style.display).toBe('')

    expect(wrapper.get('.batch-status__toggle').text()).toBe('全部展开')
    await wrapper.get('.batch-status__toggle').trigger('click')
    expect(summaries.every((summary) => summary.attributes('aria-expanded') === 'true')).toBe(true)
    expect(details.every((detail) => (detail.element as HTMLElement).style.display === '')).toBe(true)
    expect(wrapper.get('.batch-status__toggle').text()).toBe('全部收起')

    await wrapper.get('.batch-status__toggle').trigger('click')
    expect(summaries.every((summary) => summary.attributes('aria-expanded') === 'false')).toBe(true)
    expect(details.every((detail) => (detail.element as HTMLElement).style.display === 'none')).toBe(true)
    expect(wrapper.get('.batch-status__toggle').text()).toBe('全部展开')
    wrapper.unmount()
  })

  it('closes an active batch terminal when the panel unmounts', async () => {
    const target = host('local', '本机', 'debian')
    mocks.open.mockResolvedValue({ sessionId: 'session-local', offset: 0 })
    mocks.output.mockImplementation((_sessionID: string, _offset: number, signal: AbortSignal) => new Promise((_, reject) => {
      signal.addEventListener('abort', () => reject(new Error('aborted')), { once: true })
    }))
    const wrapper = mount(BatchTerminalPanel, { props: { hosts: [target], sessionCapacity: 1 } })
    await wrapper.get('textarea').setValue('sleep 60')
    await wrapper.get('button.button--primary').trigger('click')
    await flushPromises()

    wrapper.unmount()
    await flushPromises()
    expect(mocks.close).toHaveBeenCalledWith('session-local')
  })

  it('retries transient output polling failures and keeps running long tasks', async () => {
    const target = host('local', '本机', 'debian')
    mocks.open.mockResolvedValue({ sessionId: 'session-local', offset: 0 })
    let polls = 0
    mocks.output.mockImplementation(async (sessionID: string) => {
      polls += 1
      if (polls <= 2) throw new Error('transient network blip')
      return {
        data: Buffer.from('eventually done\r\n').toString('base64'),
        offset: 0,
        nextOffset: 14,
        truncated: false,
        exitedAt: '2026-09-15T00:00:09Z',
        exitError: undefined,
        closed: true,
        sessionID,
      }
    })
    const wrapper = mount(BatchTerminalPanel, { props: { hosts: [target], sessionCapacity: 1 } })
    await wrapper.get('textarea').setValue('sleep infinity')
    await wrapper.get('button.button--primary').trigger('click')
    await flushPromises()

    for (let waited = 0; waited < 40 && mocks.output.mock.calls.length < 3; waited += 1) {
      await new Promise((resolve) => setTimeout(resolve, 100))
    }
    await flushPromises()

    expect(mocks.output).toHaveBeenCalledTimes(3)
    expect(wrapper.text()).toContain('执行成功')
    wrapper.unmount()
  })

  it('scrolls an expanded output block to the latest content', async () => {
    const target = host('local', '本机', 'debian')
    const longOutput = `${'line\n'.repeat(120)}tail-visible`
    mocks.open.mockResolvedValue({ sessionId: 'session-local', offset: 0 })
    mocks.output.mockResolvedValue({
      data: Buffer.from(longOutput).toString('base64'),
      offset: 0,
      nextOffset: longOutput.length,
      truncated: false,
      exitedAt: '2026-09-15T00:00:05Z',
      exitError: undefined,
      closed: true,
    })
    const wrapper = mount(BatchTerminalPanel, { props: { hosts: [target], sessionCapacity: 1 } })
    await wrapper.get('textarea').setValue('cat big.log')
    await wrapper.get('button.button--primary').trigger('click')
    await flushPromises()

    await wrapper.get('.batch-result__summary').trigger('click')
    await flushPromises()
    const pre = wrapper.get('.batch-result__detail pre')
    expect((pre.element as HTMLPreElement).scrollTop).toBe((pre.element as HTMLPreElement).scrollHeight - pre.element.clientHeight)

    // Collapse and re-expand must land at the latest content again.
    await wrapper.get('.batch-result__summary').trigger('click')
    await wrapper.get('.batch-result__summary').trigger('click')
    await flushPromises()
    expect((pre.element as HTMLPreElement).scrollTop).toBe((pre.element as HTMLPreElement).scrollHeight - pre.element.clientHeight)
    wrapper.unmount()
  })

  it('completes the host when a menu swallows the trailing exit but the prompt returns', async () => {
    vi.useFakeTimers()
    try {
      const target = host('local', '本机', 'debian')
      mocks.open.mockResolvedValue({ sessionId: 'session-local', offset: 0 })
      const fullOutput = '正在系统更新...\nroot@debian:~# '
      let delivered = 0
      let pollsAtStable = 0
      mocks.output.mockImplementation(async () => {
        if (delivered) {
          pollsAtStable += 1
          return { data: '', offset: fullOutput.length, nextOffset: fullOutput.length, truncated: false, exitedAt: '', exitError: '', closed: false }
        }
        delivered = 1
        return { data: Buffer.from(fullOutput).toString('base64'), offset: 0, nextOffset: fullOutput.length, truncated: false, exitedAt: '', exitError: '', closed: false }
      })
      const wrapper = mount(BatchTerminalPanel, { props: { hosts: [target], sessionCapacity: 1 } })
      await wrapper.get('textarea').setValue('k 更新')
      await wrapper.get('button.button--primary').trigger('click')
// The stability window reads the wall clock; advance it alongside the timers.
      const start = Date.now()
      for (let step = 0; step < 40; step += 1) {
        await vi.advanceTimersByTimeAsync(400)
        vi.setSystemTime(start + (step + 1) * 400)
      }

      expect(wrapper.text()).toContain('执行成功')
      expect(mocks.close).toHaveBeenCalledWith('session-local')
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it.each(['# ', '$ '])('completes the host on a bare-shell prompt %j without PS1', async (marker) => {
    vi.useFakeTimers()
    try {
      const target = host('local', '本机', 'alpine')
      mocks.open.mockResolvedValue({ sessionId: 'session-local', offset: 0 })
      const fullOutput = `done\n${marker}`
      let delivered = 0
      mocks.output.mockImplementation(async () => {
        if (delivered) return { data: '', offset: fullOutput.length, nextOffset: fullOutput.length, truncated: false, exitedAt: '', exitError: '', closed: false }
        delivered = 1
        return { data: Buffer.from(fullOutput).toString('base64'), offset: 0, nextOffset: fullOutput.length, truncated: false, exitedAt: '', exitError: '', closed: false }
      })
      const wrapper = mount(BatchTerminalPanel, { props: { hosts: [target], sessionCapacity: 1 } })
      await wrapper.get('textarea').setValue('true')
      await wrapper.get('button.button--primary').trigger('click')
      const start = Date.now()
      for (let step = 0; step < 40; step += 1) {
        await vi.advanceTimersByTimeAsync(400)
        vi.setSystemTime(start + (step + 1) * 400)
      }

      expect(mocks.input.mock.calls.some(([, payload]) => decodeInput(payload) === 'exit\r')).toBe(true)
      expect(wrapper.text()).toContain('执行成功')
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('keeps running when output merely ends with a dollar sign that is not a prompt', async () => {
    vi.useFakeTimers()
    try {
      const target = host('local', '本机', 'debian')
      mocks.open.mockResolvedValue({ sessionId: 'session-local', offset: 0 })
      const staleTail = 'calculating... total $ '
      let delivered = 0
      mocks.output.mockImplementation(async () => {
        if (delivered) return { data: '', offset: staleTail.length, nextOffset: staleTail.length, truncated: false, exitedAt: '', exitError: '', closed: false }
        delivered = 1
        return { data: Buffer.from(staleTail).toString('base64'), offset: 0, nextOffset: staleTail.length, truncated: false, exitedAt: '', exitError: '', closed: false }
      })
      const wrapper = mount(BatchTerminalPanel, { props: { hosts: [target], sessionCapacity: 1 } })
      await wrapper.get('textarea').setValue('slow-thing')
      await wrapper.get('button.button--primary').trigger('click')
const start = Date.now()
      for (let step = 0; step < 25; step += 1) {
        await vi.advanceTimersByTimeAsync(400)
        vi.setSystemTime(start + (step + 1) * 400)
      }

      expect(wrapper.text()).toContain('命令执行中')
      expect(mocks.close).not.toHaveBeenCalledWith('session-local')
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('stops the batch run manually and closes open sessions', async () => {
    const target = host('local', '本机', 'debian')
    mocks.open.mockResolvedValue({ sessionId: 'session-local', offset: 0 })
    mocks.output.mockImplementation((_sessionID: string, _offset: number, signal: AbortSignal) => new Promise((_, reject) => {
      signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')), { once: true })
    }))
    const wrapper = mount(BatchTerminalPanel, { props: { hosts: [target], sessionCapacity: 1 } })
    await wrapper.get('textarea').setValue('sleep 3600')
    await wrapper.get('button.button--primary').trigger('click')
    await flushPromises()

    const stopButton = wrapper.get('.batch-command footer .button--danger')
    expect(stopButton.text()).toContain('终止执行')
    await stopButton.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('已手动终止')
    expect(mocks.close).toHaveBeenCalledWith('session-local')
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).disabled).toBe(false)
    wrapper.unmount()
  })
})
