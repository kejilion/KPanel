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
      'uname -a\rexit\r',
      'uname -a\rexit\r',
    ])
    expect(mocks.input.mock.calls.every((call) => !decodeInput(call[1]).includes('/bin/sh -c'))).toBe(true)
    expect(wrapper.text()).toContain('local ok')
    expect(wrapper.text()).toContain('remote failed')
    expect(wrapper.text()).toContain('执行成功')
    expect(wrapper.text()).toContain('执行失败')
    expect(wrapper.text()).toContain('exit status 2')
    expect(wrapper.html()).not.toContain('\u001b[32m')
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
})
