// @vitest-environment jsdom
import { shallowMount, flushPromises } from '@vue/test-utils'
import { beforeEach, afterEach, expect, it, vi } from 'vitest'
import FilesView from './FilesView.vue'
import { createWindowRouter, reactiveRouteFor, synchronizeWindowRoute } from '@/lib/desktopWindowRoute'
import { windowRouterKey, windowRouteKey } from '@/lib/desktopRouteKeys'
import { useFileClipboard, resetFileWindowTransferForTest } from '@/lib/fileWindowTransfer'
import { resetApiSecurityState } from '@/lib/api'
import { beginDesktopFileDrag } from '@/lib/desktopFileShortcuts'

vi.mock('@/stores/toast', () => ({ useToast: () => ({show: vi.fn(),success: vi.fn(),danger: vi.fn()}) }))
const hosts = ['local', 'a', 'b'].map(id => ({id,name:id,isLocal:id==='local',kind:'panel',state:'online',fileManagementAvailable:true,remoteNodeId:id.repeat(32)}))
const entry = {name:'test.txt',path:'/test.txt',kind:'file',editable:true,resourceVersion:'v1'}
let wrapper: ReturnType<typeof shallowMount> | undefined
let finishAction: (response:Response)=>void
const writes:string[]=[]
let transferStream: ReadableStream<Uint8Array> | undefined
beforeEach(()=>{
 window.scrollTo = vi.fn()
 resetApiSecurityState(); resetFileWindowTransferForTest(); writes.length=0
 transferStream = undefined
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>{
  const url=new URL(input,'http://localhost')
  if (url.pathname.endsWith('/transfers')) {
   writes.push(url.searchParams.get('hostId') || '')
   return new Response(transferStream, { headers: { 'content-type': 'application/x-ndjson' } })
  }
  if(url.pathname.endsWith('/actions')) {writes.push(url.searchParams.get('hostId')||''); return new Promise<Response>(resolve=>{finishAction=resolve})}
  let body:unknown={}
  if(url.pathname.endsWith('/cluster/hosts')) body={nodeId:'c'.repeat(32),items:hosts}
  if(url.pathname.endsWith('/files')) body={path:url.searchParams.get('path'),entries:[entry]}
  if(url.pathname.endsWith('/remote-downloads')) body={items:[]}
  return new Response(JSON.stringify(body),{headers:{'content-type':'application/json'}})
 }))
})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;vi.unstubAllGlobals();vi.useRealTimers()})

it('native window back must not display A copy completion inside B',async()=>{
 const router=createWindowRouter('/files?path=/&hostId=b')
 await router.isReady()
 await router.push('/files?path=/&hostId=a')
 wrapper=shallowMount(FilesView,{global:{provide:{[windowRouterKey as symbol]:router,[windowRouteKey as symbol]:reactiveRouteFor(router)}}})
 await flushPromises()
 const vm=wrapper.vm as unknown as Record<string,any>
 expect(vm.fileHostId).toBe('a')
 useFileClipboard().set('copy',[entry as any],'a')
 const pending=vm.pasteClipboard('/target-a')
 await flushPromises()
 expect(vm.pasteBusy).toBe(true)
 vm.handleFileHostSelection(hosts[2])
 expect(vm.fileHostId).toBe('a') // selector correctly refuses while busy
 router.back()
 await flushPromises()
 expect(router.currentRoute.value.query.hostId).toBe('b')
 expect(vm.fileHostId).toBe('b') // real router history bypasses selector
 expect(vm.fileTransferState).toBeUndefined()
 finishAction(new Response(JSON.stringify({action:'copy',succeeded:[{path:'/test.txt',destination:'/target-a/test.txt'}],failed:[]}),{headers:{'content-type':'application/json'}}))
 await pending
 await flushPromises()
 expect(writes).toEqual(['a']) // no backend write misrouting
 expect(vm.fileHostId).toBe('b')
 expect(vm.fileTransferState).toBeUndefined()
})

function response(action: string, status = 200): Response {
 return new Response(JSON.stringify(status === 200
  ? { action, succeeded: [{ path: entry.path, destination: '/done/test.txt' }], failed: [] }
  : { code: 'file_host_unavailable', detail: 'old operation failed' }),
 { status, headers: { 'content-type': 'application/json' } })
}

async function mountAtA() {
 const router = createWindowRouter('/files?path=/&hostId=b')
 await router.isReady()
 await router.push('/files?path=/&hostId=a')
 wrapper = shallowMount(FilesView, { global: { provide: {
  [windowRouterKey as symbol]: router,
  [windowRouteKey as symbol]: reactiveRouteFor(router),
 } } })
 await flushPromises()
 return { router, vm: wrapper.vm as unknown as Record<string, any> }
}

it.each([
 { returnToA: false, status: 200 }, { returnToA: false, status: 503 },
 { returnToA: true, status: 200 }, { returnToA: true, status: 503 },
])('late paste result does not own a new operation: %j', async ({ returnToA, status }) => {
 const { router, vm } = await mountAtA()
 vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
 useFileClipboard().set('copy', [entry as any], 'a')
 const older = vm.pasteClipboard('/old-target')
 await flushPromises()
 const finishOlder = finishAction
 router.back()
 await flushPromises()
 if (returnToA) {
  router.forward()
  await flushPromises()
 }
 const newHost = returnToA ? 'a' : 'b'
 expect(vm.fileHostId).toBe(newHost)
 useFileClipboard().set('copy', [entry as any], newHost)
 const newer = vm.pasteClipboard('/new-target')
 await flushPromises()
 const finishNewer = finishAction
 finishOlder(response('copy', status))
 await older
 expect(vm.fileTransferState).toMatchObject({ phase: 'running', target: '/new-target' })
 expect(vm.pasteBusy).toBe(true)
 await vi.advanceTimersByTimeAsync(2400)
 expect(vm.fileTransferState).toMatchObject({ phase: 'running', target: '/new-target' })
 finishNewer(response('copy'))
 await newer
 expect(vm.fileTransferState).toMatchObject({ phase: 'success', target: '/new-target' })
 expect(vm.pasteBusy).toBe(false)
 expect(writes).toEqual(['a', newHost])
})

it('window path synchronization updates the host and ignores an old internal move result', async () => {
 const { router, vm } = await mountAtA()
 const data = new Map<string, string>()
 const event = { preventDefault: vi.fn(), ctrlKey: false, altKey: false, dataTransfer: {
  get types() { return [...data.keys()] },
  setData: (type: string, value: string) => data.set(type, value),
  getData: (type: string) => data.get(type) || '',
 } }
 vm.startEntryDrag(event, entry)
 const moving = vm.transferInternalFileDrop(event, '/target-a')
 await flushPromises()
 const finishMove = finishAction
 synchronizeWindowRoute(router, '/files?path=/&hostId=b')
 await flushPromises()
 expect(vm.fileHostId).toBe('b')
 finishMove(response('move'))
 await moving
 expect(vm.fileTransferState).toBeUndefined()
 expect(writes).toEqual(['a'])
})

it.each([202, 503])('background submission result %s stays outside a new host operation', async status => {
 const { router, vm } = await mountAtA()
 let accept!: (response: Response) => void
 const original = fetch
 vi.stubGlobal('fetch', vi.fn((input: string, init?: RequestInit) => {
  const url = new URL(input, 'http://localhost')
  if (url.pathname.endsWith('/transfer-jobs') && init?.method === 'POST') {
   const body = JSON.parse(String(init.body)); writes.push(body.targetHostId)
   return new Promise<Response>(resolve => { accept = resolve })
  }
  if (url.pathname.endsWith('/transfer-jobs')) return Promise.resolve(new Response(JSON.stringify({items: []}), {headers: {'content-type':'application/json'}}))
  return original(input, init)
 }))
 const data = new Map<string, string>()
 const event = { dataTransfer: {
  get types() { return [...data.keys()] },
  setData: (type: string, value: string) => data.set(type, value),
  getData: (type: string) => data.get(type) || '',
 } } as unknown as DragEvent
 beginDesktopFileDrag(event, [entry as any], 'd'.repeat(32))
 const older = vm.transferCrossPanelFileDrop(event, '/old-target')
 await flushPromises()
 router.back(); await flushPromises()
 expect(vm.fileHostId).toBe('b')
 useFileClipboard().set('copy', [entry as any], 'b')
 const newer = vm.pasteClipboard('/new-target'); await flushPromises()
 accept(new Response(JSON.stringify(status === 202 ? { id: 'e'.repeat(32), sourceNodeId: 'd'.repeat(32), targetHostId:'a', targetDirectory:'/old-target',state:'queued',items:[],createdAt:'',updatedAt:'' } : {detail:'unavailable'}), {status, headers:{'content-type':'application/json'}}))
 await older
 expect(vm.fileTransferState).toMatchObject({ phase: 'running', target: '/new-target' })
 expect(vm.pasteBusy).toBe(true)
 finishAction(response('copy')); await newer
 expect(vm.fileTransferState).toMatchObject({ phase:'success', target:'/new-target' })
 expect(writes).toEqual(['a','b'])
})

it('route navigation waits for a pending rename to finish on its original host', async () => {
 const {router,vm}=await mountAtA()
 vm.openDialog('rename',entry); vm.dialogValue='renamed.txt'
 const pending=vm.submitDialog(); await flushPromises()
 expect(vm.dialogBusy).toBe(true)
 router.back(); await flushPromises()
 expect(vm.fileHostId).toBe('a')
 expect(router.currentRoute.value.query.hostId).toBe('a')
 finishAction(response('rename')); await pending; await flushPromises()
 expect(writes).toEqual(['a'])
 await router.push('/files?path=/&hostId=b'); await flushPromises()
 expect(vm.fileHostId).toBe('b')
})
