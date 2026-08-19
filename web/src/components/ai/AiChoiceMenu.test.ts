// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import AiChoiceMenu, { type AiChoiceOption } from './AiChoiceMenu.vue'

const options:AiChoiceOption[]=[
  {value:'manual',label:'手动审批',description:'写操作逐次确认'},
  {value:'auto',label:'安全自动审批',description:'常规写操作自动执行'},
]

describe('AiChoiceMenu',()=>{
  it('opens a themed listbox and emits the selected value',async()=>{
    const wrapper=mount(AiChoiceMenu,{props:{modelValue:'manual',options,label:'权限模式',variant:'access'}})
    await wrapper.get('.ai-choice__trigger').trigger('click')
    expect(wrapper.get('[role="listbox"]').attributes('aria-label')).toBe('权限模式')
    expect(wrapper.get('[data-value="manual"]').attributes('aria-selected')).toBe('true')
    await wrapper.get('[data-value="auto"]').trigger('click')
    expect(wrapper.emitted('change')).toEqual([['auto']])
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
  })

  it('supports keyboard open and escape focus restoration',async()=>{
    const wrapper=mount(AiChoiceMenu,{attachTo:document.body,props:{modelValue:'manual',options,label:'权限模式'}})
    const trigger=wrapper.get<HTMLButtonElement>('.ai-choice__trigger')
    await trigger.trigger('keydown',{key:'ArrowDown'})
    expect(wrapper.find('[role="listbox"]').exists()).toBe(true)
    await wrapper.get('[role="listbox"]').trigger('keydown',{key:'Escape'})
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    expect(document.activeElement).toBe(trigger.element)
    wrapper.unmount()
  })

  it('filters larger model lists without changing their groups',async()=>{
    const models=Array.from({length:10},(_,index)=>({value:`m${index}`,label:index===9?'Claude Opus':'Model '+index,description:`provider/model-${index}`,group:index<5?'Primary':'Secondary'}))
    const wrapper=mount(AiChoiceMenu,{props:{modelValue:'m0',options:models,label:'选择模型',searchable:true}})
    await wrapper.get('.ai-choice__trigger').trigger('click')
    await wrapper.get('input[aria-label="搜索模型"]').setValue('Claude')
    expect(wrapper.findAll('.ai-choice__option')).toHaveLength(1)
    expect(wrapper.text()).toContain('Secondary')
    expect(wrapper.text()).toContain('Claude Opus')
  })

  it('anchors the mobile menu above the trigger for viewport centering',async()=>{
    const originalWidth=window.innerWidth;const originalHeight=window.innerHeight
    Object.defineProperty(window,'innerWidth',{configurable:true,value:390});Object.defineProperty(window,'innerHeight',{configurable:true,value:844})
    const wrapper=mount(AiChoiceMenu,{attachTo:document.body,props:{modelValue:'manual',options,label:'思考强度',variant:'thinking'}})
    vi.spyOn(wrapper.get('.ai-choice__trigger').element,'getBoundingClientRect').mockReturnValue({top:700} as DOMRect)
    await wrapper.get('.ai-choice__trigger').trigger('click');await nextTick()
    expect(wrapper.get('.ai-choice__menu').attributes('style')).toContain('--ai-choice-menu-bottom: 152px')
    wrapper.unmount();Object.defineProperty(window,'innerWidth',{configurable:true,value:originalWidth});Object.defineProperty(window,'innerHeight',{configurable:true,value:originalHeight})
  })
})
