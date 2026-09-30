// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ClusterShareThemes from './ClusterShareThemes.vue'

const mocks = vi.hoisted(() => ({ shareThemes: vi.fn(), installShareTheme: vi.fn(), selectShareTheme: vi.fn(), deleteShareTheme: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { cluster: mocks } }))
const wrappers: ReturnType<typeof mount>[] = []
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); vi.resetAllMocks() })
const pack = { id: 'minimal', name: { 'zh-CN': '<script>主题</script>' }, description: { 'zh-CN': '一句话介绍' }, author: { name: 'KPanel' }, theme: { brand: '#112233', neutral: '#ffffff', signature: '#00aa66' }, sizeBytes: 1024, version: '1.0.0', installed: false, resourceVersion: 'pack-version' }
describe('share theme settings', () => {
  it('downloads without selecting, applies explicitly, and deletes active theme', async () => {
    let installed = false, selected = ''
    mocks.shareThemes.mockImplementation(async () => ({ resourceVersion: 'list-version', selected, packs: [{ ...pack, installed, installedVersion: installed ? '1.0.0' : null }] }))
    mocks.installShareTheme.mockImplementation(async () => { installed = true })
    mocks.selectShareTheme.mockImplementation(async id => { selected = id })
    mocks.deleteShareTheme.mockImplementation(async () => { installed = false; selected = '' })
    const wrapper = mount(ClusterShareThemes); wrappers.push(wrapper); await flushPromises()
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.text()).toContain('一句话介绍'); expect(wrapper.text()).toContain('官方')
    expect(wrapper.get('.theme-card__swatch[style]').attributes('style')).toContain('--swatch-brand: #112233')
    const click = async (text: string) => { await wrapper.findAll('button').find(button => button.text() === text)!.trigger('click'); await flushPromises() }
    await click('下载'); expect(mocks.installShareTheme).toHaveBeenCalledWith('minimal', 'pack-version'); expect(mocks.selectShareTheme).not.toHaveBeenCalled()
    await click('应用'); expect(mocks.selectShareTheme).toHaveBeenCalledWith('minimal', 'list-version')
    await click('删除'); expect(mocks.deleteShareTheme).toHaveBeenCalledWith('minimal', 'pack-version'); expect(wrapper.text()).toMatch(/默认样式\s*内置/); expect(wrapper.text()).toContain('使用中')
  })
  it('keeps a retry after a failed catalog request', async () => {
    mocks.shareThemes.mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ selected: '', resourceVersion: 'v', packs: [] })
    const wrapper = mount(ClusterShareThemes); wrappers.push(wrapper); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('读取失败')
    await wrapper.findAll('button').find(button => button.text() === '重试')!.trigger('click'); await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('暂无可下载主题')
  })
})
