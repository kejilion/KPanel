// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import LightNodeHealth from './LightNodeHealth.vue'
import type { LightNodeHealth as Health } from '@/types/api'

afterEach(() => vi.useRealTimers())
it('expires independently when the parent keeps an old snapshot after API failures', async () => {
  vi.useFakeTimers(); vi.setSystemTime(new Date('2026-09-06T00:00:00Z'))
  const service = { loadState: 'loaded', activeState: 'active', subState: 'running', unitFileState: 'enabled' }
  const health: Health = { observedAt: new Date().toISOString(), runtimeVersion: '1.4.1', update: { state: 'current', checkedAt: Date.now() / 1000, finishedAt: Date.now() / 1000 }, services: { timer: service, telemetry: service, terminal: service, file: service, sshLogin: service } }
  const wrapper = mount(LightNodeHealth, { props: { health } })
  expect(wrapper.text()).toContain('已是最新版本')
  await vi.advanceTimersByTimeAsync(105_000)
  expect(wrapper.text()).toContain('观测已过期')
  expect(wrapper.text()).not.toContain('已是最新版本')
  await wrapper.setProps({ health: undefined })
  expect(wrapper.text()).toContain('未上报不代表正常')
  expect(wrapper.text()).not.toContain('正在运行')
  wrapper.unmount()
  expect(vi.getTimerCount()).toBe(0)
})
