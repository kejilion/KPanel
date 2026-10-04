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

  it('draws a clipped band and shows its range while few series carry one', async () => {
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(elementBox)
    const banded = (label: string): TrendSeries => ({
      label,
      color: 'blue',
      // Twelve 5-minute points; the sixth spikes to 900 ms.
      points: Array.from({ length: 12 }, (_, index) => ({
        at: new Date(Date.UTC(2026, 7, 5, 0, index * 5)).toISOString(),
        value: 20 + (index % 3), low: 18, high: index === 5 ? 900 : 40,
      })),
    })
    const wrapper = mount(TrendChart, {
      props: { series: [banded('电信 · 北京')], formatter: (value: number) => `${value} ms` },
    })
    const band = wrapper.get('.trend-chart__band')
    expect(band.attributes('d')).toMatch(/^M.+Z$/)
    expect(band.attributes('clip-path')).toMatch(/^url\(#trend-chart-plot-\d+\)$/)
    // The 900 ms peak is above the 90th percentile ceiling, so the axis stays
    // near the median line and the band is clipped.
    const ticks = wrapper.findAll('.trend-chart__tick').map((tick) => Number.parseFloat(tick.text()))
    expect(Math.max(...ticks)).toBeLessThan(900)

    wrapper.get('svg').element.dispatchEvent(new MouseEvent('pointermove', { bubbles: true, clientX: 360, clientY: 80 }))
    await wrapper.vm.$nextTick()
    expect(wrapper.get('.trend-chart__tooltip').text()).toContain('22 ms18 ms–900 ms')

    await wrapper.setProps({ series: ['甲', '乙', '丙', '丁'].map(banded) })
    expect(wrapper.findAll('.trend-chart__band')).toHaveLength(0)
    expect(wrapper.findAll('.trend-chart__line')).toHaveLength(4)
    wrapper.unmount()
  })

  it('draws no band for plain series', () => {
    const wrapper = mount(TrendChart, {
      props: { series: [{ label: 'CPU', color: 'red', points: [
        { at: '2026-08-05T00:00:00Z', value: 20 },
        { at: '2026-08-05T00:05:00Z', value: 30 },
      ] }] },
    })
    expect(wrapper.find('.trend-chart__band').exists()).toBe(false)
    expect(wrapper.find('clipPath').exists()).toBe(false)
    wrapper.unmount()
  })

  it('can hide a duplicate persistent legend without disabling the chart', () => {
    const wrapper = mount(TrendChart, {
      props: {
        showLegend: false,
        series: [{
          label: '电信 · 北京',
          color: 'blue',
          points: [{ at: '2026-08-05T00:00:00Z', value: 20 }],
        }],
      },
    })

    expect(wrapper.find('.trend-chart__legend').exists()).toBe(false)
    expect(wrapper.find('.trend-chart__canvas').exists()).toBe(true)
    wrapper.unmount()
  })

  it('uses stable ids, line styles and container highlighting', async () => {
    const wrapper = mount(TrendChart, {
      props: {
        highlightGroup: 'container-a',
        series: [
          {
            id: 'container-a:rx', group: 'container-a', label: 'same name · 接收', color: 'blue',
            points: [{ at: '2026-08-05T00:00:00Z', value: 10 }],
          },
          {
            id: 'container-b:tx', group: 'container-b', label: 'same name · 发送', color: 'orange', dash: 'dashed',
            points: [{ at: '2026-08-05T00:00:00Z', value: 20 }],
          },
        ],
      },
    })
    const paths = wrapper.findAll('.trend-chart__line')
    expect(paths).toHaveLength(2)
    expect(paths[0]!.classes()).not.toContain('is-muted')
    expect(paths[1]!.classes()).toContain('is-muted')
    expect(paths[1]!.attributes('stroke-dasharray')).toBe('7 5')
    wrapper.unmount()
  })

  it('breaks a line across missing buckets and suppresses stale hover values', async () => {
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(elementBox)
    const wrapper = mount(TrendChart, {
      props: {
        series: [{
          id: 'container:cpu', label: '容器 · CPU', color: 'blue',
          maxGapMilliseconds: 90_000,
          maxPointDistanceMilliseconds: 30_000,
          points: [
            { at: '2026-08-05T00:00:00Z', value: 10 },
            { at: '2026-08-05T00:03:00Z', value: 20 },
          ],
        }],
      },
    })
    expect(wrapper.get('.trend-chart__line').attributes('d')?.match(/M/g)).toHaveLength(2)

    wrapper.get('svg').element.dispatchEvent(new MouseEvent('pointermove', {
      bubbles: true,
      clientX: 360,
      clientY: 80,
    }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.trend-chart__tooltip').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps the five-container detailed chart bounded to ten SVG paths', () => {
    const start = Date.parse('2026-08-01T00:00:00Z')
    const series: TrendSeries[] = Array.from({ length: 10 }, (_, seriesIndex) => ({
      id: `container-${Math.floor(seriesIndex / 2)}:${seriesIndex % 2}`,
      group: `container-${Math.floor(seriesIndex / 2)}`,
      label: `容器 ${Math.floor(seriesIndex / 2) + 1} · ${seriesIndex % 2 ? '发送' : '接收'}`,
      color: `hsl(${seriesIndex * 36} 70% 50%)`,
      dash: seriesIndex % 2 ? 'dashed' : 'solid',
      points: Array.from({ length: 720 }, (_, pointIndex) => ({
        at: new Date(start + pointIndex * 60_000).toISOString(),
        value: seriesIndex * 100 + pointIndex,
      })),
    }))
    const wrapper = mount(TrendChart, { props: { series, showLegend: false } })
    expect(wrapper.findAll('.trend-chart__line')).toHaveLength(10)
    expect(wrapper.findAll('.trend-chart__line[stroke-dasharray="7 5"]')).toHaveLength(5)
    expect(wrapper.find('.trend-chart__legend').exists()).toBe(false)
    wrapper.unmount()
  })

  it('emits one normalized time range after a horizontal drag', async () => {
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(elementBox)
    const wrapper = mount(TrendChart, {
      props: {
        series: [{
          label: 'CPU', color: 'blue', points: [
            { at: '2026-08-05T00:00:00Z', value: 10 },
            { at: '2026-08-05T01:00:00Z', value: 20 },
          ],
        }],
      },
    })
    const svg = wrapper.get('svg')
    svg.element.dispatchEvent(new MouseEvent('pointerdown', {
      bubbles: true, clientX: 600, clientY: 80, button: 0,
    }))
    svg.element.dispatchEvent(new MouseEvent('pointerup', {
      bubbles: true, clientX: 200, clientY: 80, button: 0,
    }))
    await wrapper.vm.$nextTick()

    const selection = wrapper.emitted('selectRange')?.[0]?.[0] as { start: string; end: string }
    expect(Date.parse(selection.start)).toBeLessThan(Date.parse(selection.end))
    expect(Date.parse(selection.start)).toBeGreaterThan(Date.parse('2026-08-05T00:00:00Z'))
    expect(Date.parse(selection.end)).toBeLessThan(Date.parse('2026-08-05T01:00:00Z'))
    wrapper.unmount()
  })

  it('ignores a short accidental drag', async () => {
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(elementBox)
    const wrapper = mount(TrendChart, {
      props: {
        series: [{
          label: 'CPU', color: 'blue', points: [
            { at: '2026-08-05T00:00:00Z', value: 10 },
            { at: '2026-08-05T01:00:00Z', value: 20 },
          ],
        }],
      },
    })
    const svg = wrapper.get('svg')
    svg.element.dispatchEvent(new MouseEvent('pointerdown', {
      bubbles: true, clientX: 300, clientY: 80, button: 0,
    }))
    svg.element.dispatchEvent(new MouseEvent('pointerup', {
      bubbles: true, clientX: 306, clientY: 80, button: 0,
    }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('selectRange')).toBeUndefined()
    wrapper.unmount()
  })
})
