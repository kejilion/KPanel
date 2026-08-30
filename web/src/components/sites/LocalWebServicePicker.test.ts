// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LocalWebServicePicker from './LocalWebServicePicker.vue'

const mocks = vi.hoisted(() => ({ portUsage: vi.fn() }))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {},
  api: { system: { portUsage: mocks.portUsage } },
}))

beforeEach(() => {
  vi.clearAllMocks()
  mocks.portUsage.mockResolvedValue({
    resourceVersion: 'a'.repeat(64),
    total: 4,
    truncated: false,
    observedAt: '2026-08-30T00:00:00Z',
    entries: [
      {
        protocol: 'tcp', state: 'LISTEN', localAddress: '127.0.0.1', localPort: '3000',
        peerAddress: '0.0.0.0', peerPort: '*', process: 'node', pid: 3010, raw: 'tcp LISTEN node',
      },
      {
        protocol: 'tcp', state: 'LISTEN', localAddress: '0.0.0.0', localPort: '3000',
        peerAddress: '0.0.0.0', peerPort: '*', process: 'node', pid: 3010, raw: 'tcp LISTEN node duplicate',
      },
      {
        protocol: 'tcp6', state: 'LISTEN', localAddress: '::1', localPort: '8080',
        peerAddress: '::', peerPort: '*', process: 'docker-proxy', pid: 8080, raw: 'tcp6 LISTEN docker-proxy',
        container: {
          id: 'container-1', name: 'kpanel-demo', image: 'demo-web:latest', containerPort: 8080,
        },
      },
      {
        protocol: 'udp', state: 'UNCONN', localAddress: '0.0.0.0', localPort: '53',
        peerAddress: '0.0.0.0', peerPort: '*', process: 'dns', pid: 53, raw: 'udp UNCONN dns',
      },
    ],
  })
})

describe('LocalWebServicePicker', () => {
  it('scans on demand and emits a generated local origin', async () => {
    const wrapper = mount(LocalWebServicePicker, {
      props: {
        existingSites: [{ type: 'proxy', upstream: 'http://127.0.0.1:8080' }],
      },
    })

    expect(mocks.portUsage).not.toHaveBeenCalled()
    await wrapper.find('.local-web-service-picker__scan').trigger('click')
    await flushPromises()

    expect(mocks.portUsage).toHaveBeenCalledWith(expect.any(AbortSignal))
    expect(wrapper.findAll('.local-web-service-picker__candidate')).toHaveLength(2)
    expect(wrapper.text()).toContain('3000')
    expect(wrapper.text()).toContain('node · PID 3010')
    expect(wrapper.findAll('.local-web-service-picker__container')).toHaveLength(1)
    expect(wrapper.text()).toContain('Docker 容器：kpanel-demo · 容器端口 8080')
    expect(wrapper.findAll('.local-web-service-picker__proxy-status')).toHaveLength(1)
    expect(wrapper.text()).toContain('已反代')
    expect(wrapper.findAll('.local-web-service-picker__candidate')[0]!.text()).toContain('8080')

    const nodeCandidate = wrapper
      .findAll('.local-web-service-picker__candidate')
      .find((candidate) => candidate.text().includes('3000'))
    await nodeCandidate!.trigger('click')

    expect(wrapper.emitted('select')).toEqual([['http://127.0.0.1:3000']])
    expect(wrapper.find('.local-web-service-picker__results').exists()).toBe(false)
  })

  it('keeps manual input as the fallback when the port adapter is unavailable', async () => {
    const wrapper = mount(LocalWebServicePicker, {
      props: { readable: false, unavailableReason: '需要更新 Agent' },
    })

    await flushPromises()
    expect(mocks.portUsage).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('需要更新 Agent')
    expect(wrapper.text()).toContain('仍可手动填写上游地址')
  })
})
