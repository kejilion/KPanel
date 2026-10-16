// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'

vi.mock('@/lib/clipboard', () => ({ copyText: vi.fn(async () => true) }))
vi.mock('@/lib/api', () => ({ ApiError: Error, api: { apps: {} } }))

const { default: AppOnboardingGuide } = await import('./AppOnboardingGuide.vue')

const wrappers: VueWrapper[] = []

afterEach(async () => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  await nextTick()
  document.body.innerHTML = ''
})

describe('AppOnboardingGuide', () => {
  it('opens straight into local app management with the add, verify, submit flow', () => {
    wrappers.push(mount(AppOnboardingGuide, { attachTo: document.body, props: { open: true, localApps: [] } }))
    const text = document.body.textContent || ''
    expect([...document.querySelectorAll('.onboarding-flow strong')].map((item) => item.textContent))
      .toEqual(['添加配置', '安装验证', '提交官方'])
    expect(text).toContain('本机的本地应用')
    expect(text).toContain('会以 root 身份执行')
    expect(document.querySelector('[aria-pressed]')).toBeNull()
    expect(document.querySelector('a[href="https://dev.kejilion.sh/"]')).not.toBeNull()
  })
})
