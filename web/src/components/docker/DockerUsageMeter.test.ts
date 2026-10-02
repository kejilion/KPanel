// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import DockerUsageMeter from './DockerUsageMeter.vue'

function meter(props: Partial<InstanceType<typeof DockerUsageMeter>['$props']> = {}) {
  return mount(DockerUsageMeter, { props: { percent: 10, value: '10%', label: 'CPU', ...props } })
}

describe('DockerUsageMeter', () => {
  it('prints the figure and exposes it as a meter', () => {
    const wrapper = meter({ percent: 42.4, value: '42.4%', detail: '256 MB / 1 GB', label: '内存占用' })
    const track = wrapper.get('[role="meter"]')
    expect(wrapper.text()).toContain('42.4%')
    expect(wrapper.text()).toContain('256 MB / 1 GB')
    expect(track.attributes('aria-label')).toBe('内存占用')
    expect(track.attributes('aria-valuenow')).toBe('42')
    expect(track.attributes('aria-valuetext')).toBe('42.4%, 256 MB / 1 GB')
    expect(wrapper.get('i').attributes('style')).toContain('width: 42.4%')
  })

  it('keeps the bar inside the track while still showing a multi-core CPU figure', () => {
    const wrapper = meter({ percent: 230, value: '230%' })
    expect(wrapper.text()).toContain('230%')
    expect(wrapper.get('i').attributes('style')).toContain('width: 100%')
    expect(wrapper.get('[role="meter"]').attributes('aria-valuenow')).toBe('100')
  })

  it.each([
    [0, 'normal'],
    [69.9, 'normal'],
    [70, 'warning'],
    [89.9, 'warning'],
    [90, 'danger'],
    [140, 'danger'],
  ])('shows %s%% at the %s level', (percent, level) => {
    expect(meter({ percent }).classes()).toContain(`usage-meter--${level}`)
  })

  it('dims the meter when its last refresh failed', () => {
    expect(meter({ stale: true }).classes()).toContain('is-stale')
    expect(meter().classes()).not.toContain('is-stale')
  })
})
