// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import type { CinemaMetadata } from '@/lib/cinemaMetadata'
import Summary from './CinemaMetadataSummary.vue'
const film: CinemaMetadata = { provider: 'tmdb', id: 27205, kind: 'movie', title: '盗梦空间', year: '2010', rating: 8.4, genres: ['科幻', '悬疑'], cast: ['演员一', '演员二'], overview: '剧情简介。'.repeat(60), url: 'https://www.themoviedb.org/movie/27205' }
it('presents real metadata with expandable synopsis, cast, genres and a trusted source', async () => {
  const view = mount(Summary, { props: { metadata: film } })
  expect(view.text()).toContain('2010'); expect(view.text()).toContain('8.4'); expect(view.text()).toContain('科幻'); expect(view.text()).toContain('演员一 · 演员二')
  expect(view.find('a').attributes('href')).toBe('https://www.themoviedb.org/movie/27205')
  expect(view.find('button').attributes('aria-expanded')).toBe('false'); await view.find('button').trigger('click'); expect(view.find('button').attributes('aria-expanded')).toBe('true'); expect(view.find('.cinema-synopsis__clamped').exists()).toBe(false)
  view.unmount()
})
it('omits absent or invalid fields instead of manufacturing ratings and empty separators', () => {
  for (const rating of [undefined, 0, 12, NaN]) {
    const view = mount(Summary, { props: { metadata: { ...film, overview: '', year: undefined, rating, genres: [], cast: [] } } })
    expect(view.find('.cinema-synopsis__tags').exists()).toBe(false); expect(view.find('dl').exists()).toBe(false); expect(view.find('button').exists()).toBe(false); view.unmount()
  }
})
