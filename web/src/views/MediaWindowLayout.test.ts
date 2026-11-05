// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import { ref } from 'vue'
import { desktopWindowVisibleKey } from '@/lib/desktopRouteKeys'
import cinemaSource from './CinemaView.vue?raw'

vi.mock('/desktop-icons/music-kpanel-flat-v1.webp', () => ({ default: '' }))
vi.mock('/desktop-icons/cinema-kpanel-flat-v1.webp', () => ({ default: '' }))
vi.mock('vue-router', async importOriginal => ({
  ...await importOriginal<typeof import('vue-router')>(),
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))
vi.mock('@/lib/api', () => ({ api: { cluster: { hosts: vi.fn(async () => ({ items: [] })) } } }))
vi.mock('@/lib/music', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/music')>(),
  readMusicFolder: vi.fn(async () => ({ entries: [], truncated: false })),
}))
vi.mock('@/lib/cinema', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/cinema')>(),
  readCinemaFolder: vi.fn(async () => ({ films: [], folders: [], images: new Map() })),
}))
import MusicView from './MusicView.vue'
import CinemaView from './CinemaView.vue'

const wrappers: ReturnType<typeof shallowMount>[] = []
afterEach(() => wrappers.splice(0).forEach(wrapper => wrapper.unmount()))

describe('media views in desktop windows', () => {
  it.each([['music', MusicView], ['cinema', CinemaView]] as const)('keeps the %s title accessible without the repeated brand header', (_name, component) => {
    const wrapper = shallowMount(component, { global: { provide: { [desktopWindowVisibleKey as symbol]: ref(true) } } })
    wrappers.push(wrapper)
    expect(wrapper.find('h1').classes()).toContain('sr-only')
    expect(wrapper.find('.music-header, .cinema-heading').exists()).toBe(false)
  })
  it.each([['music', MusicView], ['cinema', CinemaView]] as const)('retains the %s heading on a classic page', (_name, component) => {
    const wrapper = shallowMount(component)
    wrappers.push(wrapper)
    expect(wrapper.find('.music-header h1, .cinema-heading h1').exists()).toBe(true)
  })
  it('keeps file management accessible in a desktop cinema window', () => {
    const wrapper = shallowMount(CinemaView, { global: { provide: { [desktopWindowVisibleKey as symbol]: ref(true) } } })
    wrappers.push(wrapper)
    expect(wrapper.findAll('.cinema-source button').some(button => button.text().includes('文件管理'))).toBe(true)
  })
  it('does not stretch cinema sections to fill a tall page', () => {
    const cinemaRule = cinemaSource.match(/\.cinema-page\s*\{[^}]+\}/)?.[0]
    expect(cinemaRule).toBeTruthy()
    const style = document.createElement('style')
    style.textContent = `.page { display: grid; } ${cinemaRule}`
    document.head.appendChild(style)
    const page = document.createElement('main')
    page.className = 'page cinema-page'
    document.body.appendChild(page)
    expect(getComputedStyle(page).display).toBe('block')
    page.remove(); style.remove()
  })
})
