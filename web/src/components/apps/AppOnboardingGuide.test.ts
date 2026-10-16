// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'

const copyText = vi.fn(async (_value: string) => true)
vi.mock('@/lib/clipboard', () => ({ copyText }))
vi.mock('@/lib/api', () => ({ ApiError: Error, api: { apps: {} } }))

const { default: AppOnboardingGuide } = await import('./AppOnboardingGuide.vue')

const wrappers: VueWrapper[] = []

afterEach(async () => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  await nextTick()
  document.body.innerHTML = ''
  copyText.mockClear()
})

function mountGuide(): VueWrapper {
  const wrapper = mount(AppOnboardingGuide, { attachTo: document.body, props: { open: true, localApps: [] } })
  wrappers.push(wrapper)
  return wrapper
}

function button(label: string): HTMLButtonElement {
  const found = [...document.querySelectorAll<HTMLButtonElement>('button')]
    .find((item) => item.textContent?.includes(label))
  if (!found) throw new Error(`missing button ${label}`)
  return found
}

describe('AppOnboardingGuide', () => {
  it('starts with the official submission path', () => {
    mountGuide()
    const text = document.body.textContent || ''
    expect(text).toContain('向 kejilion/apps 提交 Pull Request')
    expect(document.querySelector('a[href="https://github.com/kejilion/apps"]')).not.toBeNull()
    expect(document.querySelector('a[href="https://dev.kejilion.sh/"]')).not.toBeNull()
  })

  it('switches to the in-place local app manager with the root warning', async () => {
    mountGuide()
    button('仅本机使用').click()
    await nextTick()
    const text = document.body.textContent || ''
    expect(text).toContain('本机的本地应用')
    expect(text).toContain('还没有本地应用')
    expect(text).toContain('会以 root 身份执行')
    expect(document.querySelector('a[href="https://github.com/kejilion/apps"]')).toBeNull()
    expect(button('新建本地应用')).toBeTruthy()
  })

  it('says so when the clipboard is unavailable', async () => {
    copyText.mockResolvedValueOnce(false)
    mountGuide()
    button('复制模板').click()
    await nextTick()
    await nextTick()
    expect(button('复制失败，请手动选择')).toBeTruthy()
  })

  it('copies a template that registers through docker_app_plus', async () => {
    mountGuide()
    button('复制模板').click()
    await nextTick()
    const template = copyText.mock.calls[0]?.[0] ?? ''
    expect(template).toContain('local app_id="myapp"')
    expect(template).toContain('local docker_name="myapp"')
    expect(template).toContain('${gh_proxy}')
    expect(template.trim().endsWith('docker_app_plus')).toBe(true)
  })
})
