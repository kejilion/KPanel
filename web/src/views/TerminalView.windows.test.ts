// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetLocaleForTest } from '@/i18n'
import TerminalView from './TerminalView.vue'

const mocks=vi.hoisted(()=>({hosts:vi.fn(),open:vi.fn(),policy:vi.fn(),shellClose:vi.fn(),desktopClose:vi.fn()}))
vi.mock('vue-router',async(importOriginal)=>({...await importOriginal<typeof import('vue-router')>(),useRoute:()=>({path:'/terminal',query:{}})}))
vi.mock('@/lib/api',()=>({api:{cluster:{hosts:mocks.hosts},terminals:{open:mocks.open},desktops:{policy:mocks.policy}},ApiError:class extends Error{}}))
vi.mock('@/components/terminal/HostTerminal.vue',()=>({default:defineComponent({props:['sessionId','hostName'],setup(_,{expose}){expose({closeSession:mocks.shellClose,focusTerminal(){},scheduleResize(){},executeCommand(){return true}});return()=>null}})}))
vi.mock('@/components/terminal/HostDesktop.vue',()=>({default:defineComponent({props:['hostId','hostName','active'],setup(_,{expose}){expose({closeSession:mocks.desktopClose,focusTerminal(){},scheduleResize(){},executeCommand(){return false}});return()=>null}})}))
const windows={id:'win',name:'Windows 测试机',platform:'windows',kind:'light_node',terminalAvailable:true,desktopAvailable:true}
let wrapper:ReturnType<typeof mount>|undefined
function button(text:string):HTMLButtonElement {
  const found=Array.from(document.querySelectorAll<HTMLButtonElement>('button')).find(item=>item.textContent?.includes(text))
  if(!found)throw new Error(`missing button ${text}`)
  return found
}
async function openSelector(){button('Windows 测试机').click();await flushPromises()}
beforeEach(()=>{
  vi.clearAllMocks();resetLocaleForTest();localStorage.clear()
  mocks.hosts.mockResolvedValue({items:[windows,{id:'linux',name:'Linux 测试机',kind:'light_node',terminalAvailable:true}]})
  mocks.open.mockResolvedValue({sessionId:'shell',offset:0})
})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;document.body.innerHTML=''})
describe('Windows host connection choice',()=>{
  it('chooses before connecting, keeps CLI and RDP as independent sessions, and preserves Linux direct connect',async()=>{
    wrapper=mount(TerminalView,{attachTo:document.body});await flushPromises()
    await openSelector()
    expect(document.body.textContent).toContain('选择连接方式')
    expect(mocks.open).not.toHaveBeenCalled()
    button('命令行（PowerShell）').click();await flushPromises()
    expect(mocks.open).toHaveBeenCalledExactlyOnceWith('win',30,120)
    await openSelector();button('远程桌面（RDP）').click();await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(2)
    expect(wrapper.findAll('.terminal-tab').map(item=>item.text()).join(' ')).toContain('RDP')
    const desktopTab=wrapper.findAll('.terminal-tab').find(item=>item.text().includes('RDP'))!
    expect(desktopTab.text()).toContain('等待登录')
    await desktopTab.get('.terminal-tab__close').trigger('click');await flushPromises()
    expect(mocks.desktopClose).toHaveBeenCalledTimes(1)
    expect(mocks.shellClose).not.toHaveBeenCalled()
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(1)
    button('Linux 测试机').click();await flushPromises()
    expect(mocks.open).toHaveBeenLastCalledWith('linux',30,120)
  })

  it('shows the specific disabled desktop reason and supports center revocation',async()=>{
    mocks.hosts.mockResolvedValue({items:[{...windows,desktopAvailable:false,desktopUnavailableReason:'desktop_rdp_certificate_unavailable'}]})
    wrapper=mount(TerminalView,{attachTo:document.body});await flushPromises();await openSelector()
    expect(button('远程桌面（RDP）').disabled).toBe(true)
    expect(document.body.textContent).toContain('Windows 远程桌面证书不可用。')
    mocks.hosts.mockResolvedValue({items:[{...windows,desktopAvailable:false,desktopUnavailableReason:'desktop_disabled_by_center'}]})
    button('禁用此主机远程桌面').click();await flushPromises()
    expect(mocks.policy).toHaveBeenCalledWith('win',false)
    expect(document.body.textContent).toContain('中心已禁用此主机的远程桌面。')
  })
})
