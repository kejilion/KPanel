// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import DesktopStartMenu from './DesktopStartMenu.vue'
import type { DesktopStartMenuItem } from '@/lib/desktopStartMenu'

const items: DesktopStartMenuItem[] = [
  { key: 'nav:/overview', section: 'system', label: '概览', keywords: ['overview'], iconURL: '/desktop-icons/overview.webp', gradient: 'linear-gradient(#000, #111)' },
  { key: 'nav:/docker', section: 'system', label: '容器', keywords: ['docker'], gradient: 'linear-gradient(#000, #111)' },
  { key: 'nav:/files', section: 'system', label: '文件管理', keywords: ['files'], gradient: 'linear-gradient(#000, #111)' },
  { key: 'app:nginx', section: 'entries', label: 'Nginx', detail: '应用', keywords: ['nginx'] },
  { key: 'site:blog', section: 'entries', label: 'blog.example.com', detail: '网站', hidden: true },
  { key: 'action:wallpaper', section: 'actions', label: '更换壁纸和主题', keywords: ['wallpaper'] },
]

describe('DesktopStartMenu', () => {
  let wrapper: VueWrapper

  afterEach(() => {
    wrapper?.unmount()
    document.body.innerHTML = ''
  })

  async function mountMenu(props: Partial<InstanceType<typeof DesktopStartMenu>['$props']> = {}) {
    wrapper = mount(DesktopStartMenu, {
      attachTo: document.body,
      props: { open: true, items, entriesState: 'ready', userName: 'admin', ...props },
    })
    await nextTick()
    await nextTick()
    return wrapper
  }

  function search() {
    return wrapper.get<HTMLInputElement>('input[role="combobox"]')
  }

  async function press(key: string, init: KeyboardEventInit = {}) {
    await search().trigger('keydown', { key, ...init })
    await nextTick()
  }

  function activeKey(): string | undefined {
    const id = search().attributes('aria-activedescendant')
    return id ? wrapper.get(`#${id}`).attributes('data-start-menu-key') : undefined
  }

  it('focuses search, shows system tiles and entry rows, and keeps actions for search only', async () => {
    await mountMenu()
    expect(document.activeElement).toBe(search().element)
    expect(wrapper.get('[role="dialog"]').attributes('aria-label')).toBe('开始菜单')
    expect(wrapper.findAll('.desktop-start-menu__option--tile')).toHaveLength(3)
    const rows = wrapper.findAll('.desktop-start-menu__option:not(.desktop-start-menu__option--tile)')
    expect(rows.map((row) => row.attributes('data-start-menu-key'))).toEqual(['app:nginx', 'site:blog'])
    expect(rows[1]!.text()).toContain('桌面已隐藏')
    expect(wrapper.text()).not.toContain('更换壁纸和主题')
    expect(activeKey()).toBe('nav:/overview')
  })

  it('filters across sections, reports no matches, and opens the highlighted result with Enter', async () => {
    await mountMenu()
    await search().setValue('wall')
    expect(wrapper.findAll('[role="option"]').map((option) => option.attributes('data-start-menu-key')))
      .toEqual(['action:wallpaper'])
    await press('Enter')
    expect(wrapper.emitted('select')?.[0]?.[0]).toMatchObject({ key: 'action:wallpaper' })

    await search().setValue('missing')
    expect(wrapper.findAll('[role="option"]')).toHaveLength(0)
    expect(wrapper.get('[role="status"]').text()).toBe('没有找到与“missing”匹配的内容')
    expect(search().attributes('aria-activedescendant')).toBeUndefined()
    await press('Enter')
    expect(wrapper.emitted('select')).toHaveLength(1)
  })

  it('moves the highlight with arrows and keeps horizontal keys for text while searching', async () => {
    await mountMenu()
    await press('ArrowRight')
    expect(activeKey()).toBe('nav:/docker')
    await press('ArrowDown')
    expect(activeKey()).toBe('app:nginx')
    await press('ArrowDown')
    expect(activeKey()).toBe('site:blog')
    await press('ArrowUp')
    await press('ArrowUp')
    expect(activeKey()).toBe('nav:/overview')

    await search().setValue('n')
    const event = new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true })
    search().element.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(false)
  })

  it('selects with the pointer without moving focus out of the search field', async () => {
    await mountMenu()
    const row = wrapper.get('[data-start-menu-key="app:nginx"]')
    await row.trigger('pointermove')
    expect(activeKey()).toBe('app:nginx')
    const press = new MouseEvent('mousedown', { bubbles: true, cancelable: true })
    row.element.dispatchEvent(press)
    expect(press.defaultPrevented).toBe(true)
    await row.trigger('click')
    expect(wrapper.emitted('select')?.[0]?.[0]).toMatchObject({ key: 'app:nginx' })
  })

  it('clears the query on the first Escape and closes with focus restore on the second', async () => {
    await mountMenu()
    await search().setValue('docker')
    await press('Escape')
    expect(search().element.value).toBe('')
    expect(wrapper.emitted('close')).toBeUndefined()
    await press('Escape')
    expect(wrapper.emitted('close')).toEqual([[true]])
  })

  it('ignores the Enter that commits an IME composition', async () => {
    await mountMenu()
    await press('Enter', { isComposing: true })
    await press('Enter', { keyCode: 229 })
    expect(wrapper.emitted('select')).toBeUndefined()
    await press('Enter')
    expect(wrapper.emitted('select')?.[0]?.[0]).toMatchObject({ key: 'nav:/overview' })
  })

  it('scrolls the highlight into view for keyboard moves only, and resets the list on a new query', async () => {
    await mountMenu()
    const scrollIntoView = vi.fn()
    Element.prototype.scrollIntoView = scrollIntoView
    try {
      await wrapper.get('[data-start-menu-key="site:blog"]').trigger('pointermove')
      await nextTick()
      expect(scrollIntoView).not.toHaveBeenCalled()
      await press('ArrowUp')
      await nextTick()
      expect(scrollIntoView).toHaveBeenCalledTimes(1)
      expect(scrollIntoView.mock.contexts[0]).toBe(wrapper.get('[data-start-menu-key="app:nginx"]').element)
    } finally {
      delete (Element.prototype as Partial<Element>).scrollIntoView
    }
    const body = wrapper.get('.desktop-start-menu__body').element
    body.scrollTop = 120
    await search().setValue('docker')
    expect(body.scrollTop).toBe(0)
  })

  it('lets Escape close an open language list without closing the menu', async () => {
    await mountMenu()
    await wrapper.get('.language-selector__trigger').trigger('click')
    expect(wrapper.find('.language-selector__menu').exists()).toBe(true)
    const event = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
    let reachedDocument = false
    const listener = () => { reachedDocument = true }
    document.addEventListener('keydown', listener)
    try {
      wrapper.get('.language-selector__trigger').element.dispatchEvent(event)
      await nextTick()
    } finally {
      document.removeEventListener('keydown', listener)
    }
    expect(reachedDocument).toBe(true)
    expect(event.defaultPrevented).toBe(true)
    expect(wrapper.find('.language-selector__menu').exists()).toBe(false)
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('closes without restoring focus when focus moves outside, but not to its opener', async () => {
    const opener = document.createElement('button')
    const outside = document.createElement('button')
    document.body.append(opener, outside)
    await mountMenu({ opener })
    await wrapper.get('section').trigger('focusout', { relatedTarget: opener })
    await wrapper.get('section').trigger('focusout', { relatedTarget: null })
    expect(wrapper.emitted('close')).toBeUndefined()
    await wrapper.get('section').trigger('focusout', { relatedTarget: outside })
    expect(wrapper.emitted('close')).toEqual([[false]])
  })

  it.each([
    ['loading', '正在读取已安装的应用和网站…'],
    ['unavailable', '已安装的应用和网站暂时无法读取。'],
  ] as const)('explains the %s entry state', async (entriesState, message) => {
    await mountMenu({ entriesState, items: items.filter((item) => item.section !== 'entries') })
    expect(wrapper.get('[role="status"]').text()).toContain(message)
    if (entriesState === 'unavailable') {
      await wrapper.get('.desktop-start-menu__retry').trigger('click')
      expect(wrapper.emitted('retry')).toHaveLength(1)
    }
  })

  it('invites setup when no apps, sites or shortcuts exist', async () => {
    await mountMenu({ items: items.filter((item) => item.section !== 'entries') })
    expect(wrapper.get('[role="status"]').text()).toBe('还没有已安装的应用、网站或快捷方式。')
  })

  it('offers sign-out, theme and classic mode in the footer', async () => {
    await mountMenu({ dark: true })
    const user = wrapper.get('.desktop-start-menu__user')
    expect(user.text()).toContain('admin')
    expect(user.text()).toContain('退出登录')
    await user.trigger('click')
    await wrapper.get('[aria-label="切换到浅色主题"]').trigger('click')
    await wrapper.get('[aria-label="切换回经典模式"]').trigger('click')
    expect(wrapper.emitted('sign-out')).toHaveLength(1)
    expect(wrapper.emitted('theme')).toHaveLength(1)
    expect(wrapper.emitted('classic')).toHaveLength(1)
    expect(wrapper.find('.language-selector').exists()).toBe(true)

    await wrapper.setProps({ signingOut: true })
    expect(wrapper.get('.desktop-start-menu__user').attributes('disabled')).toBeDefined()
  })

  it('renders nothing while closed and resets the query when reopened', async () => {
    await mountMenu()
    await search().setValue('docker')
    await wrapper.setProps({ open: false })
    await nextTick()
    expect(wrapper.find('section').exists()).toBe(false)
    await wrapper.setProps({ open: true })
    await nextTick()
    await nextTick()
    expect(search().element.value).toBe('')
    expect(document.activeElement).toBe(search().element)
  })
})
