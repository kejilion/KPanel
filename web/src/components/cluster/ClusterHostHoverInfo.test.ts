// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ClusterHostRegionInfo from './ClusterHostRegionInfo.vue'
import ClusterHostSystemInfo from './ClusterHostSystemInfo.vue'
import type { ClusterTelemetry } from '@/types/api'

function telemetry(overrides: Partial<ClusterTelemetry> = {}): ClusterTelemetry {
  return {
    agentVersion: '1.25.1',
    agentProtocolVersion: 'v1',
    hostname: 'hk-01',
    os: 'Debian GNU/Linux 13 (trixie)',
    osId: 'debian',
    kernel: '6.12.48+deb13-arm64',
    architecture: 'arm64',
    uptimeSeconds: 60,
    load: { one: 0, five: 0, fifteen: 0 },
    cpu: { model: 'Neoverse-N1', cores: 4, usagePercent: 1 },
    memory: { totalBytes: 1, availableBytes: 1, usedBytes: 0, usagePercent: 0 },
    disk: { totalBytes: 1, usedBytes: 0, usagePercent: 0 },
    network: { receivedBytes: 0, sentBytes: 0, tcpConnections: 0, udpConnections: 0 },
    publicNetwork: {},
    collectedAt: '2026-10-10T00:00:00Z',
    ...overrides,
  }
}

function card(): HTMLElement | null {
  return document.body.querySelector('.cluster-hover-info__card')
}

afterEach(() => {
  document.body.innerHTML = ''
  vi.useRealTimers()
})

describe('ClusterHostSystemInfo', () => {
  it('uses the overview operating-system mapping for the icon and keeps the details in the accessible name', () => {
    const known = mount(ClusterHostSystemInfo, { props: { telemetry: telemetry({ os: 'AlmaLinux 9.6 (Sage Margay)', osId: 'almalinux', osLike: ['rhel', 'centos', 'fedora'] }) } })
    expect(known.find('.os-identity__mark').attributes('title')).toBeUndefined()
    expect(known.get('button').attributes('aria-label')).toBe('系统 · AlmaLinux 9.6 (Sage Margay) · arm64 · 6.12.48+deb13-arm64 · Neoverse-N1 · 4 核')

    const unknown = mount(ClusterHostSystemInfo, { props: { telemetry: telemetry({ os: 'Vendor Linux 1', osId: 'vendorlinux', osLike: ['ubuntu', 'debian'] }) } })
    expect(unknown.get('button').attributes('aria-label')).toContain('Vendor Linux 1')
  })

  it('opens the system card on focus with architecture, kernel and processor, and closes on Escape', async () => {
    const wrapper = mount(ClusterHostSystemInfo, { attachTo: document.body, props: { telemetry: telemetry() } })
    const trigger = wrapper.get('button')
    expect(card()).toBeNull()

    await trigger.trigger('focus')
    await nextTick()
    expect(card()?.getAttribute('role')).toBe('tooltip')
    expect(trigger.attributes('aria-describedby')).toBe(card()?.id)
    expect(card()?.textContent).toContain('Debian GNU/Linux 13 (trixie)')
    expect([...card()!.querySelectorAll('dt')].map((node) => node.textContent)).toEqual(['架构', '内核', '处理器'])
    expect(card()?.textContent).toContain('Neoverse-N1 · 4 核')

    await trigger.trigger('keydown', { key: 'Escape' })
    expect(card()).toBeNull()
    wrapper.unmount()
  })

  it('follows a focused trigger that scrolls into view and closes once it leaves the viewport', async () => {
    const wrapper = mount(ClusterHostSystemInfo, { attachTo: document.body, props: { telemetry: telemetry() } })
    const trigger = wrapper.get('button')
    await trigger.trigger('focus')
    await nextTick()
    document.dispatchEvent(new Event('scroll'))
    await nextTick()
    expect(card()).not.toBeNull()

    vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue({ top: window.innerHeight + 40, bottom: window.innerHeight + 80, left: 0, right: 24, width: 24, height: 40, x: 0, y: window.innerHeight + 40, toJSON: () => ({}) })
    document.dispatchEvent(new Event('scroll'))
    await nextTick()
    expect(card()).toBeNull()
    wrapper.unmount()
  })

  it('names a host without telemetry instead of showing an empty card', async () => {
    const wrapper = mount(ClusterHostSystemInfo, { attachTo: document.body })
    await wrapper.get('button').trigger('click')
    await nextTick()
    expect(card()?.textContent).toContain('系统信息未获取')
    expect(card()?.querySelector('dl')).toBeNull()
    wrapper.unmount()
  })
})

describe('ClusterHostRegionInfo', () => {
  it('shows location, ASN and operator on hover and hides them again on leave', async () => {
    vi.useFakeTimers()
    const wrapper = mount(ClusterHostRegionInfo, {
      attachTo: document.body,
      props: { location: { country: 'HK', countryCode: 'HK', region: 'Hong Kong', city: 'Hong Kong', isp: 'AS152194 CTG Server Limited' } },
    })
    const trigger = wrapper.get('button')
    expect(trigger.attributes('aria-label')).toBe('地区 · HK · Hong Kong · AS152194 CTG Server Limited')

    await trigger.trigger('pointerenter', { pointerType: 'mouse' })
    expect(card()).toBeNull()
    vi.advanceTimersByTime(150)
    await nextTick()
    await nextTick()
    expect(card()?.querySelector('strong')?.textContent).toBe('HK · Hong Kong')
    expect([...card()!.querySelectorAll('dd')].map((node) => node.textContent)).toEqual(['AS152194', 'CTG Server Limited'])

    await trigger.trigger('pointerleave', { pointerType: 'mouse' })
    vi.advanceTimersByTime(100)
    await nextTick()
    expect(card()).not.toBeNull()
    vi.advanceTimersByTime(100)
    await nextTick()
    expect(card()).toBeNull()
    wrapper.unmount()
  })

  it('stays open while the pointer rests on the card and closes after it leaves', async () => {
    vi.useFakeTimers()
    const wrapper = mount(ClusterHostRegionInfo, { attachTo: document.body, props: { location: { country: 'SG', countryCode: 'SG', isp: 'AS31898 Oracle Corporation' } } })
    const trigger = wrapper.get('button')
    await trigger.trigger('pointerenter', { pointerType: 'mouse' })
    vi.advanceTimersByTime(150)
    await nextTick()
    await trigger.trigger('pointerleave', { pointerType: 'mouse' })
    card()!.dispatchEvent(new PointerEvent('pointerenter', { pointerType: 'mouse' }))
    vi.advanceTimersByTime(400)
    await nextTick()
    expect(card()).not.toBeNull()

    // Pressing inside the card, for example to select the ASN, keeps it open.
    card()!.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
    await trigger.trigger('blur')
    await nextTick()
    expect(card()).not.toBeNull()

    // Focus moving to another control still closes it, even with the pointer on the card.
    const other = document.createElement('button')
    document.body.append(other)
    trigger.element.dispatchEvent(new FocusEvent('blur', { relatedTarget: other }))
    await nextTick()
    expect(card()).toBeNull()
    await trigger.trigger('focus')
    await nextTick()
    card()!.dispatchEvent(new PointerEvent('pointerenter', { pointerType: 'mouse' }))

    card()!.dispatchEvent(new PointerEvent('pointerleave', { pointerType: 'mouse' }))
    vi.advanceTimersByTime(200)
    await nextTick()
    expect(card()).toBeNull()
    wrapper.unmount()
  })

  it('keeps a single card open so a covered row never stacks a second one', async () => {
    const first = mount(ClusterHostSystemInfo, { attachTo: document.body, props: { telemetry: telemetry() } })
    const second = mount(ClusterHostRegionInfo, { attachTo: document.body, props: { location: { country: 'SG', countryCode: 'SG', isp: 'AS31898 Oracle Corporation' } } })
    await first.get('button').trigger('click')
    await nextTick()
    expect(document.body.querySelectorAll('.cluster-hover-info__card')).toHaveLength(1)
    expect(card()?.textContent).toContain('Debian')

    await second.get('button').trigger('click')
    await nextTick()
    expect(document.body.querySelectorAll('.cluster-hover-info__card')).toHaveLength(1)
    expect(card()?.textContent).toContain('Oracle Corporation')
    first.unmount()
    second.unmount()
  })

  it('keeps a tapped card open until the next outside press', async () => {
    const wrapper = mount(ClusterHostRegionInfo, { attachTo: document.body, props: { location: { country: 'US', city: 'San Jose', isp: 'Oracle Corporation' } } })
    const trigger = wrapper.get('button')
    await trigger.trigger('click')
    await nextTick()
    await trigger.trigger('pointerleave', { pointerType: 'touch' })
    expect(card()?.textContent).toContain('Oracle Corporation')
    expect(card()?.querySelectorAll('dt')).toHaveLength(1)

    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await nextTick()
    expect(card()).toBeNull()
    wrapper.unmount()
  })

  it('falls back to a globe mark and an explicit unknown location', async () => {
    const wrapper = mount(ClusterHostRegionInfo, { attachTo: document.body, props: { location: {} } })
    expect(wrapper.find('.cluster-host-region__unknown').exists()).toBe(true)
    await wrapper.get('button').trigger('focus')
    await nextTick()
    expect(card()?.textContent).toContain('位置未获取')
    expect(card()?.textContent).toContain('运营商未知')
    wrapper.unmount()
  })
})
