// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import DockerDeploymentEditor from './DockerDeploymentEditor.vue'

describe('DockerDeploymentEditor', () => {
  it('focuses and selects the exact diagnostic range', async () => {
    vi.useFakeTimers()
    const wrapper = mount(DockerDeploymentEditor, {
      attachTo: document.body,
      props: {
        modelValue: 'services:\n  web\n    image: nginx',
        diagnostics: [{
          code: 'yaml_syntax_error', message: 'YAML 语法错误', hint: '补上冒号。',
          from: 12, to: 15, line: 2, column: 3, endLine: 2, endColumn: 6,
        }],
      },
    })
    try {
      const textarea = wrapper.get('textarea').element
      expect(document.activeElement).not.toBe(textarea)
      expect(wrapper.get('.deployment-editor__error-line').classes()).toContain('is-pulsing')
      expect(wrapper.get('.deployment-editor__error-line').attributes('style')).toContain('top: 30.975px')
      expect(wrapper.findAll('.deployment-editor__gutter span')[1]?.classes()).toEqual(expect.arrayContaining([
        'has-diagnostic', 'is-diagnostic-line',
      ]))

      vi.advanceTimersByTime(1_600)
      await nextTick()
      expect(wrapper.get('.deployment-editor__error-line').classes()).not.toContain('is-pulsing')

      await wrapper.get('.deployment-diagnostics button').trigger('click')
      expect(document.activeElement).toBe(textarea)
      expect([textarea.selectionStart, textarea.selectionEnd]).toEqual([12, 15])
      expect(wrapper.text()).toContain('第 2 行 · 第 3 列')
      expect(wrapper.get('.deployment-editor__error-line').classes()).not.toContain('is-pulsing')
      expect(wrapper.findAll('.deployment-editor__gutter span')[1]?.classes()).toContain('has-diagnostic')
    } finally {
      wrapper.unmount()
      vi.useRealTimers()
    }
  })

  it('keeps every diagnostic line visibly marked', () => {
    const wrapper = mount(DockerDeploymentEditor, {
      props: {
        modelValue: 'services:\n  web\n    image nginx',
        diagnostics: [
          {
            code: 'missing_colon', message: '缺少冒号', from: 12, to: 15,
            line: 2, column: 3, endLine: 2, endColumn: 6,
          },
          {
            code: 'yaml_syntax_error', message: 'YAML 语法错误', from: 20, to: 25,
            line: 3, column: 5, endLine: 3, endColumn: 10,
          },
        ],
      },
    })
    expect(wrapper.findAll('.deployment-editor__error-line')).toHaveLength(2)
    expect(wrapper.findAll('.deployment-editor__gutter span').filter((item) => item.classes().includes('has-diagnostic'))).toHaveLength(2)
  })

  it('renders a line-number gutter for pasted content', () => {
    const wrapper = mount(DockerDeploymentEditor, { props: { modelValue: 'one\ntwo\nthree' } })
    expect(wrapper.findAll('.deployment-editor__gutter span').map((item) => item.text())).toEqual(['1', '2', '3'])
  })
})
