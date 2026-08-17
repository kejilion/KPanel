// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import DockerDeploymentEditor from './DockerDeploymentEditor.vue'

describe('DockerDeploymentEditor', () => {
  it('focuses and selects the exact diagnostic range', async () => {
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
    await wrapper.get('.deployment-diagnostics button').trigger('click')
    const textarea = wrapper.get('textarea').element
    expect(document.activeElement).toBe(textarea)
    expect([textarea.selectionStart, textarea.selectionEnd]).toEqual([12, 15])
    expect(wrapper.text()).toContain('第 2 行 · 第 3 列')
    wrapper.unmount()
  })

  it('renders a line-number gutter for pasted content', () => {
    const wrapper = mount(DockerDeploymentEditor, { props: { modelValue: 'one\ntwo\nthree' } })
    expect(wrapper.findAll('.deployment-editor__gutter span').map((item) => item.text())).toEqual(['1', '2', '3'])
  })
})
