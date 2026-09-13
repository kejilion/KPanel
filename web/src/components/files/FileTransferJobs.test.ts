// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import FileTransferJobs from './FileTransferJobs.vue'
import { createTransferJob, transferJobs, transferJobsError } from '@/lib/fileTransferJobs'
import type { ClusterHost, FileEntry, FileTransferJob, FileTransferJobInput } from '@/types/api'

const mocks=vi.hoisted(()=>({stop:vi.fn()}))
vi.mock('@/lib/fileTransferJobs', async()=>{
  const {ref}=await import('vue')
  return {
    transferJobs:ref([]), transferJobsError:ref(''), transferJobsLoading:ref(false),
    createTransferJob:vi.fn(),changeTransferJob:vi.fn(),refreshTransferJobs:vi.fn(),
    newTransferJobID:()=> 'd'.repeat(32),observeTransferJobs:()=>mocks.stop,
    transferJobActive:(state:string)=>['queued','running','connecting','transferring','committing'].includes(state),
  }
})
const hosts=[{id:'local',name:'Local',isLocal:true,remoteNodeId:'c'.repeat(32)},{id:'a'.repeat(32),name:'Host A',isLocal:false,remoteNodeId:'a'.repeat(32),fileManagementAvailable:true},{id:'b'.repeat(32),name:'Host B',isLocal:false,remoteNodeId:'b'.repeat(32),fileManagementAvailable:true}] as ClusterHost[]
const source={path:'/one.txt',name:'one.txt',kind:'file',resourceVersion:'v1'} as FileEntry
let view:ReturnType<typeof mount<typeof FileTransferJobs>>|undefined
async function open(){
 view=mount(FileTransferJobs,{props:{hosts,sourceNodeId:'a'.repeat(32),localNodeId:'c'.repeat(32),hostId:'a'.repeat(32),path:'/'},global:{stubs:{ModalDialog:{props:['open','closeDisabled'],emits:['close'],template:'<div v-if="open" class="dialog" :data-locked="closeDisabled"><slot /></div>'}}}})
 await flushPromises();return view
}
beforeEach(()=>{vi.resetAllMocks();transferJobs.value=[];transferJobsError.value=''})
afterEach(()=>{view?.unmount();view=undefined})

it('freezes source identity and disables duplicate submissions while the window host changes',async()=>{
 const wrapper=await open();wrapper.vm.openCopy([source]);await flushPromises()
 expect(wrapper.findAll('option').map(option=>option.text())).toEqual(['Local','Host B'])
 await wrapper.get('select').setValue('b'.repeat(32));await wrapper.get('input').setValue('/destination')
 let accept!:(value:FileTransferJob)=>void
 vi.mocked(createTransferJob).mockImplementation(()=>new Promise(resolve=>{accept=resolve}))
 await wrapper.get('form').trigger('submit');await flushPromises()
 expect(wrapper.get('.dialog').attributes('data-locked')).toBe('true')
 await wrapper.setProps({sourceNodeId:'b'.repeat(32),hostId:'b'.repeat(32)})
 await wrapper.get('form').trigger('submit');expect(createTransferJob).toHaveBeenCalledTimes(1)
 const input=vi.mocked(createTransferJob).mock.calls[0]![0]
 expect(input).toMatchObject({sourceNodeId:'a'.repeat(32),targetHostId:'b'.repeat(32),targetDirectory:'/destination',items:[{path:'/one.txt',resourceVersion:'v1'}]})
 accept({...input,state:'queued',items:[],createdAt:'',updatedAt:''});await flushPromises()
 expect(wrapper.find('form').exists()).toBe(false)
})

it('shows uncertain results without offering an unsafe retry, and retries only recoverable items',async()=>{
 const wrapper=await open()
 const original:FileTransferJob={id:'e'.repeat(32),sourceNodeId:'a'.repeat(32),targetHostId:'b'.repeat(32),targetDirectory:'/target',state:'partial',createdAt:'',updatedAt:'',items:[
  {path:'/done',resourceVersion:'v1',state:'complete',loadedBytes:0,totalBytes:0,retryable:false,entry:{...source,path:'/target/done'}},
  {path:'/uncertain',resourceVersion:'v2',state:'interrupted',loadedBytes:0,totalBytes:0,retryable:false},
  {path:'/waiting',resourceVersion:'v3',state:'cancelled',loadedBytes:0,totalBytes:0,retryable:true},
 ]}
 transferJobs.value=[original];await flushPromises()
 expect(wrapper.text()).toContain('结果不确定的项目请先检查目标目录')
 const retry=wrapper.findAll('button').find(button=>button.text()==='重试可恢复项目')!
 vi.mocked(createTransferJob).mockImplementation(async(input:FileTransferJobInput)=>({...input,state:'queued',items:[],createdAt:'',updatedAt:''}))
 await retry.trigger('click');await flushPromises()
 expect(createTransferJob).toHaveBeenCalledWith(expect.objectContaining({retryOf:original.id,items:[{path:'/waiting',resourceVersion:'v3'}]}))
 transferJobs.value=[{...original,items:original.items.slice(0,2)}];await flushPromises()
 expect(wrapper.findAll('button').some(button=>button.text()==='重试可恢复项目')).toBe(false)
})

it('preserves the request ID after an unconfirmed receipt and rejects invalid destinations',async()=>{
 const wrapper=await open();wrapper.vm.openCopy([source]);await flushPromises()
 await wrapper.get('input').setValue('/a/../b');await wrapper.get('form').trigger('submit')
 expect(createTransferJob).not.toHaveBeenCalled()
 await wrapper.get('input').setValue('/target')
 vi.mocked(createTransferJob).mockRejectedValue(new Error('unconfirmed'))
 await wrapper.get('form').trigger('submit');await flushPromises()
 await wrapper.get('form').trigger('submit');await flushPromises()
 expect(vi.mocked(createTransferJob).mock.calls.map(call=>call[0].id)).toEqual(['d'.repeat(32),'d'.repeat(32)])
 expect(wrapper.get('[role=alert]').text()).toBe('unconfirmed')
})
