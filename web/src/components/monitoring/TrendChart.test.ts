// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import TrendChart, { type TrendSeries } from './TrendChart.vue'

function elementBox(): DOMRect {
  return {
    bottom: 210,
    height: 210,
    left: 0,
    right: 720,
    top: 0,
    width: 720,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  }
}

afterEach(() => vi.restoreAllMocks())

describe('TrendChart', () => {
  it('shows all nine routes in a compact hover tooltip', async () => {
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(elementBox)
    const series: TrendSeries[] = Array.from({ length: 9 }, (_, index) => ({
      label: `线路 ${index + 1}`,
      color: `hsl(${index * 40} 70% 50%)`,
      points: [
        { at: '2026-08-05T00:00:00Z', value: 20 + index },
        { at: '2026-08-05T00:05:00Z', value: 30 + index },
      ],
    }))
    const wrapper = mount(TrendChart, {
      props: { series, formatter: (value: number) => `${value} ms` },
    })

    wrapper.get('svg').element.dispatchEvent(new MouseEvent('pointermove', {
      bubbles: true,
      clientX: 360,
      clientY: 80,
    }))
    await wrapper.vm.$nextTick()

    const tooltip = wrapper.get('.trend-chart__tooltip')
    expect(tooltip.classes()).toContain('is-dense')
    expect(tooltip.findAll('span')).toHaveLength(9)
    expect(tooltip.text()).toContain('线路 1')
    expect(tooltip.text()).toContain('线路 9')
    wrapper.unmount()
  })
})
