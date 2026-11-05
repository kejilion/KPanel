// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import SiteAccessPasswordCreate from './SiteAccessPasswordCreate.vue'

describe('SiteAccessPasswordCreate', () => {
  it('starts collapsed and enables password inputs only on explicit choice', async () => {
    const draft = { enabled: false, path: '/', username: '', password: '' }
    const wrapper = mount(SiteAccessPasswordCreate, {
      props: {
        modelValue: draft,
        writable: true,
        'onUpdate:modelValue': (value) => wrapper.setProps({ modelValue: value }),
      },
    })
    const toggle = wrapper.get('button')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('input[type=password]').exists()).toBe(false)
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    await wrapper.get('input[type=checkbox]').setValue(true)
    expect(wrapper.find('input[type=password]').exists()).toBe(true)
    await wrapper.get('input[type=password]').setValue('test-password')
    expect(wrapper.props('modelValue').password).toBe('test-password')
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(wrapper.props('modelValue').enabled).toBe(true)
  })

  it('allows disabling a draft when the chosen template becomes unsupported', async () => {
    const wrapper = mount(SiteAccessPasswordCreate, { props: { modelValue: { enabled: true, path: '/', username: 'alice', password: 'test' }, writable: false } })
    await wrapper.get('button').trigger('click')
    expect(wrapper.get('input[type=checkbox]').attributes('disabled')).toBeUndefined()
    await wrapper.get('input[type=checkbox]').setValue(false)
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toMatchObject({ enabled: false })
  })
})
