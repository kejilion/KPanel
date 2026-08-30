// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetLocaleForTest, setLocale } from '@/i18n'
import AiView from './AiView.vue'

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  messages: vi.fn(),
  providers: vi.fn(),
  models: vi.fn(),
  sessions: vi.fn(),
  update: vi.fn(),
  send: vi.fn(),
  cancel: vi.fn(),
}))

vi.mock('@/lib/aiApi', () => ({
  runEventURL: (id: string) => `/api/v1/ai/runs/${id}/events`,
  aiApi: {
    providers: { list: mocks.providers },
    models: mocks.models,
    sessions: {
      list: mocks.sessions,
      messages: mocks.messages,
      create: mocks.create, update: mocks.update, remove: vi.fn(), send: mocks.send,
    },
    runs: { get: vi.fn(), decision: vi.fn(), cancel: mocks.cancel, retry: vi.fn(), propose: vi.fn() },
    evolution: { memories: vi.fn(), procedures: vi.fn(), proposals: vi.fn() },
  },
}))

class MockEventSource {
  static urls: string[] = []
  static instances: MockEventSource[] = []
  listeners = new Map<string,((event:{data:string})=>void)[]>()
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  constructor(url: string) { MockEventSource.urls.push(url);MockEventSource.instances.push(this) }
  addEventListener(name:string,handler:(event:{data:string})=>void) { this.listeners.set(name,[...(this.listeners.get(name)||[]),handler]) }
  emit(name:string,value:unknown){for(const handler of this.listeners.get(name)||[])handler({data:JSON.stringify(value)})}
  close() {}
}

beforeEach(() => {
  vi.clearAllMocks()
  MockEventSource.urls = []
  MockEventSource.instances = []
  vi.stubGlobal('EventSource', MockEventSource)
  vi.stubGlobal('requestAnimationFrame',(callback:FrameRequestCallback)=>{callback(0);return 1})
  vi.stubGlobal('cancelAnimationFrame',vi.fn())
  Element.prototype.scrollIntoView = vi.fn()
  mocks.providers.mockResolvedValue([
    { id: 'p1', name: 'Primary', enabled: true },
    { id: 'p2', name: 'Secondary', enabled: true },
  ])
  mocks.models.mockResolvedValue([
    { id: 'm1', providerId: 'p1', modelId: 'mock', displayName: 'Mock', contextWindow: 8192, enabled: true, isDefault: true, toolCalling: true, vision: true, reasoning: true },
    { id: 'm2', providerId: 'p2', modelId: 'next', displayName: 'Next', contextWindow: 32768, enabled: true, isDefault: false, toolCalling: true, vision: false, reasoning: false },
  ])
  mocks.sessions.mockResolvedValue([{
    id: 's1', title: 'Running', providerId: 'p1', modelId: 'm1', providerName: 'Mock', modelName: 'Mock',
    approvalMode: 'manual', thinkingLevel:'medium', pinned: false, archived: false, modelAvailable: true, running: true, activeRunId: 'run-active',
    createdAt: '2026-08-04T00:00:00Z', updatedAt: '2026-08-04T00:00:00Z', lastMessageAt: '2026-08-04T00:00:00Z',
  }])
  mocks.messages.mockResolvedValue({ items: [], nextCursor: '' })
  mocks.update.mockImplementation(async (_id, body) => ({
    id: 's1', title: 'Running', providerId: body.providerId || 'p1', modelId: body.modelId || 'm1',
    providerName: body.providerId === 'p2' ? 'Secondary' : 'Primary', modelName: body.modelId === 'm2' ? 'Next' : 'Mock',
    approvalMode: body.approvalMode || 'manual', thinkingLevel:body.thinkingLevel||'medium', modelAvailable: true, pinned: false, archived: false, running: true, activeRunId: 'run-active',
    createdAt: '2026-08-04T00:00:00Z', updatedAt: '2026-08-04T00:00:00Z', lastMessageAt: '2026-08-04T00:00:00Z',
  }))
})

afterEach(() => {
  resetLocaleForTest()
  vi.unstubAllGlobals()
})

function makeRouter(path='/ai/s/s1') {
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/ai', component: AiView },
    { path: '/ai/s/:sessionId', component: AiView },
  ] })
  return router.push(path).then(()=>router.isReady()).then(()=>router)
}

describe('AI workspace reconnect', () => {
  it('reopens SSE for the active run after a route reload', async () => {
    const router = await makeRouter()
    const wrapper = mount(AiView, { global: { plugins: [router] } })
    await flushPromises()
    expect(mocks.messages).toHaveBeenCalledWith('s1')
    expect(MockEventSource.urls).toContain('/api/v1/ai/runs/run-active/events')
    wrapper.unmount()
  })

  it('creates a conversation immediately with the default model', async () => {
    const router = await makeRouter()
    mocks.create.mockResolvedValue({ id: 's2', title: '新会话', providerId: 'p1', modelId: 'm1', providerName: 'Primary', modelName: 'Mock', modelAvailable: true, pinned: false, archived: false, running: false, createdAt: '', updatedAt: '', lastMessageAt: '' })
    const wrapper = mount(AiView, { global: { plugins: [router] } })
    await flushPromises()
    await wrapper.get('.ai-new-chat').trigger('click')
    await flushPromises()
    expect(mocks.create).toHaveBeenCalledWith('p1','m1')
    expect(router.currentRoute.value.fullPath).toBe('/ai/s/s2')
    wrapper.unmount()
  })

  it('switches provider and model during an active run for the next turn', async () => {
    const router = await makeRouter()
    const wrapper = mount(AiView, { global: { plugins: [router] } })
    await flushPromises()
    MockEventSource.instances[0]?.emit('run.snapshot',{run:{id:'run-active',sessionId:'s1',providerId:'p1',providerName:'Primary',modelId:'m1',modelName:'Mock',status:'running',step:1,usage:{inputTokens:0,outputTokens:0,totalTokens:0},createdAt:'',updatedAt:''},toolCalls:[],messages:[]})
    await flushPromises()
    const picker=wrapper.get('.ai-choice--model .ai-choice__trigger')
    expect((picker.element as HTMLButtonElement).disabled).toBe(false)
    await picker.trigger('click')
    await wrapper.get('.ai-choice--model [data-value="m2"]').trigger('click')
    await flushPromises()
    expect(mocks.update).toHaveBeenCalledWith('s1',{providerId:'p2',modelId:'m2'})
    expect(wrapper.text()).toContain('下一轮')
    wrapper.unmount()
  })

  it('switches approval mode for the next run', async () => {
	const router = await makeRouter()
	const wrapper = mount(AiView, { global: { plugins: [router] } })
	await flushPromises()
	MockEventSource.instances[0]?.emit('run.snapshot',{run:{id:'run-active',sessionId:'s1',providerId:'p1',providerName:'Primary',modelId:'m1',modelName:'Mock',approvalMode:'manual',status:'running',step:1,usage:{inputTokens:0,outputTokens:0,totalTokens:0},createdAt:'',updatedAt:''},toolCalls:[],messages:[]})
	await wrapper.get('.ai-choice--access .ai-choice__icon').trigger('click')
	await wrapper.get('.ai-choice--access [data-value="auto"]').trigger('click')
	await flushPromises()
	expect(mocks.update).toHaveBeenCalledWith('s1',{approvalMode:'auto'})
	expect(wrapper.text()).toContain('下一轮')
	wrapper.unmount()
  })

  it('opens the thinking menu when its icon is tapped', async () => {
    const router = await makeRouter()
    const wrapper = mount(AiView, { global: { plugins: [router] } })
    await flushPromises()
    expect(wrapper.find('select[aria-label="思考强度"]').exists()).toBe(false)
    await wrapper.get('.ai-choice--thinking .ai-choice__icon').trigger('click')
    await wrapper.get('.ai-choice--thinking [data-value="high"]').trigger('click')
    await flushPromises()
    expect(mocks.update).toHaveBeenCalledWith('s1',{thinkingLevel:'high'})
    wrapper.unmount()
  })

  it('uses the empty composer action to stop an active run', async () => {
    const router = await makeRouter()
    const wrapper = mount(AiView, { global: { plugins: [router] } })
    await flushPromises()
    MockEventSource.instances[0]?.emit('run.snapshot',{run:{id:'run-active',sessionId:'s1',providerId:'p1',providerName:'Primary',modelId:'m1',modelName:'Mock',approvalMode:'manual',thinkingLevel:'medium',status:'running',step:1,usage:{inputTokens:0,outputTokens:0,totalTokens:0},createdAt:'',updatedAt:''},toolCalls:[],messages:[]})
    await flushPromises()
    const action=wrapper.get('.ai-composer-submit')
    expect(action.attributes('aria-label')).toBe('停止运行')
    expect((action.element as HTMLButtonElement).disabled).toBe(false)
    await action.trigger('click')
    await flushPromises()
    expect(mocks.cancel).toHaveBeenCalledWith('run-active')
    wrapper.unmount()
  })

  it('adds composer input to the active run without cancelling it', async () => {
    mocks.send.mockResolvedValue({runId:'run-active'})
    const router = await makeRouter()
    const wrapper = mount(AiView, { global: { plugins: [router] } })
    await flushPromises()
    MockEventSource.instances[0]?.emit('run.snapshot',{run:{id:'run-active',sessionId:'s1',providerId:'p1',providerName:'Primary',modelId:'m1',modelName:'Mock',approvalMode:'manual',thinkingLevel:'medium',status:'running',step:1,usage:{inputTokens:0,outputTokens:0,totalTokens:0},createdAt:'',updatedAt:''},toolCalls:[],messages:[]})
    await wrapper.get('.ai-composer textarea').setValue('补充检查最新错误日志')
    const action=wrapper.get('.ai-composer-submit')
    expect(action.attributes('aria-label')).toBe('发送')
    await action.trigger('click')
    await flushPromises()
    expect(mocks.send).toHaveBeenCalledWith('s1','补充检查最新错误日志',[])
    expect(mocks.cancel).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('loads archived conversations from the sidebar filter', async () => {
    const router = await makeRouter()
    const wrapper = mount(AiView, { global: { plugins: [router] } })
    await flushPromises()
    await wrapper.get('button[aria-label="查看已归档会话"]').trigger('click')
    await flushPromises()
    expect(mocks.sessions).toHaveBeenCalledWith('',true)
    wrapper.unmount()
  })

  it('never renders internal tool messages as user conversation text', async () => {
    const router = await makeRouter()
    const wrapper = mount(AiView, { global: { plugins: [router] } })
    await flushPromises()
    MockEventSource.instances[0]?.emit('run.snapshot',{
      run:{id:'run-active',sessionId:'s1',providerId:'p1',providerName:'Primary',modelId:'m1',modelName:'Mock',status:'running',step:1,usage:{inputTokens:0,outputTokens:0,totalTokens:0},createdAt:'',updatedAt:''},
      toolCalls:[],
      messages:[
        {id:'user',sessionId:'s1',role:'user',content:'查询 CPU',createdAt:''},
        {id:'tool',sessionId:'s1',role:'tool',toolCallId:'call-1',content:'<tool_result>secret raw data</tool_result>',createdAt:''},
        {id:'legacy',sessionId:'s1',role:'user',toolCallId:'call-2',content:'legacy raw data',createdAt:''},
      ],
    })
    await flushPromises()
    expect(wrapper.text()).toContain('查询 CPU')
    expect(wrapper.text()).not.toContain('secret raw data')
    expect(wrapper.text()).not.toContain('legacy raw data')
    wrapper.unmount()
  })

  it('renders stable process cards before the answer from the same run', async () => {
    const router = await makeRouter()
    const wrapper = mount(AiView, { global: { plugins: [router] } })
    await flushPromises()
    const stream=MockEventSource.instances[0]
    const run={id:'run-active',sessionId:'s1',providerId:'p1',providerName:'Primary',modelId:'m1',modelName:'Mock',status:'running',step:2,usage:{inputTokens:0,outputTokens:0,totalTokens:0},createdAt:'',updatedAt:''}
    const first={id:'call-1',runId:'run-active',sessionId:'s1',name:'host_system_summary',status:'completed',requiresApproval:false,createdAt:'',updatedAt:''}
    const second={id:'call-2',runId:'run-active',sessionId:'s1',name:'host_system_processes',status:'completed',requiresApproval:false,createdAt:'',updatedAt:''}
    stream?.emit('run.snapshot',{
      run,
      toolCalls:[first,second],
      messages:[
        {id:'user',sessionId:'s1',runId:'run-active',role:'user',content:'inspect resources',createdAt:''},
        {id:'answer',sessionId:'s1',runId:'run-active',role:'assistant',content:'resources are healthy',modelName:'Mock',createdAt:''},
      ],
    })
    await flushPromises()
    const firstElement=wrapper.get('[data-tool-call-id="call-1"]').element
    const secondElement=wrapper.get('[data-tool-call-id="call-2"]').element
    const answerElement=wrapper.get('[data-message-id="answer"]').element
    expect(firstElement.compareDocumentPosition(secondElement)&Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(secondElement.compareDocumentPosition(answerElement)&Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    stream?.emit('tool.completed',{...first,resultPreview:'updated'})
    await flushPromises()
    const updatedFirst=wrapper.get('[data-tool-call-id="call-1"]').element
    const stableSecond=wrapper.get('[data-tool-call-id="call-2"]').element
    expect(updatedFirst.compareDocumentPosition(stableSecond)&Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    wrapper.unmount()
  })

  it('interleaves visible progress and tool execution by occurrence time', async()=>{
    mocks.messages.mockResolvedValue({
      items:[
        {id:'user',sessionId:'s1',runId:'run-active',role:'user',content:'检查并修复服务',createdAt:'2026-08-04T10:00:00.000Z'},
        {id:'plan',sessionId:'s1',runId:'run-active',role:'assistant',content:'我先读取当前配置。',modelName:'Mock',createdAt:'2026-08-04T10:00:01.000Z'},
        {id:'finding',sessionId:'s1',runId:'run-active',role:'assistant',content:'已定位到配置中的重复项，继续验证。',modelName:'Mock',createdAt:'2026-08-04T10:00:03.000Z'},
        {id:'answer',sessionId:'s1',runId:'run-active',role:'assistant',content:'修复完成，服务运行正常。',modelName:'Mock',createdAt:'2026-08-04T10:00:05.000Z'},
      ],
      toolCalls:[
        {id:'read',runId:'run-active',sessionId:'s1',name:'host_file_read',status:'completed',requiresApproval:false,createdAt:'2026-08-04T10:00:02.000Z',updatedAt:'2026-08-04T10:00:02.500Z'},
        {id:'test',runId:'run-active',sessionId:'s1',name:'host_nginx_test',status:'completed',requiresApproval:false,createdAt:'2026-08-04T10:00:04.000Z',updatedAt:'2026-08-04T10:00:04.500Z'},
      ],
      nextCursor:'',
    })
    const router=await makeRouter();const wrapper=mount(AiView,{global:{plugins:[router]}});await flushPromises()
    const elements=['[data-message-id="plan"]','[data-tool-call-id="read"]','[data-message-id="finding"]','[data-tool-call-id="test"]','[data-message-id="answer"]'].map(selector=>wrapper.get(selector).element)
    for(let index=0;index<elements.length-1;index++)expect(elements[index]!.compareDocumentPosition(elements[index+1]!)&Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(new Set(elements.map(element=>element.closest('.ai-agent-turn'))).size).toBe(1)
    expect(wrapper.findAll('.ai-message-meta')).toHaveLength(1)
    expect(wrapper.get('[data-tool-call-id="read"]').classes()).toContain('ai-tool-card--inline')
    wrapper.unmount()
  })

  it('restores the latest process cards with conversation history', async () => {
    mocks.messages.mockResolvedValue({
      items:[
        {id:'user',sessionId:'s1',runId:'run-active',role:'user',content:'inspect resources',createdAt:''},
        {id:'answer',sessionId:'s1',runId:'run-active',role:'assistant',content:'resources are healthy',modelName:'Mock',createdAt:''},
      ],
      toolCalls:[{id:'persisted-call',runId:'run-active',sessionId:'s1',name:'host_system_summary',status:'completed',requiresApproval:false,createdAt:'',updatedAt:''}],
      nextCursor:'',
    })
    const router = await makeRouter()
    const wrapper = mount(AiView, { global: { plugins: [router] } })
    await flushPromises()
    const process=wrapper.get('[data-tool-call-id="persisted-call"]').element
    const answer=wrapper.get('[data-message-id="answer"]').element
    expect(process.compareDocumentPosition(answer)&Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    wrapper.unmount()
  })

  it('keeps the completed answer visible while history reconciles', async () => {
    const router = await makeRouter()
    const wrapper = mount(AiView, { global: { plugins: [router] } })
    await flushPromises()
    const stream=MockEventSource.instances[0]
    stream?.emit('run.snapshot',{
      run:{id:'run-active',sessionId:'s1',providerId:'p1',providerName:'Primary',modelId:'m1',modelName:'Mock',status:'running',step:1,usage:{inputTokens:0,outputTokens:0,totalTokens:0},createdAt:'',updatedAt:''},
      toolCalls:[],messages:[{id:'user',sessionId:'s1',role:'user',content:'查询 CPU',createdAt:''}],
    })
    stream?.emit('message.completed',{id:'answer',sessionId:'s1',runId:'run-active',role:'assistant',content:'CPU 使用率正常',modelName:'Mock',createdAt:''})
    await flushPromises()
    expect(wrapper.text()).toContain('CPU 使用率正常')
    mocks.messages.mockReturnValueOnce(new Promise(()=>{}))
    stream?.emit('run.completed',{id:'run-active',sessionId:'s1',providerId:'p1',providerName:'Primary',modelId:'m1',modelName:'Mock',status:'completed',step:2,usage:{inputTokens:1,outputTokens:1,totalTokens:2},createdAt:'',updatedAt:''})
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('CPU 使用率正常')
    expect(wrapper.text()).not.toContain('今天想管理什么？')
    wrapper.unmount()
  })

  it('keeps process cards and answer inside one assistant turn', async()=>{
    mocks.messages.mockResolvedValue({items:[{id:'user',sessionId:'s1',runId:'run-active',role:'user',content:'检查',createdAt:''},{id:'answer',sessionId:'s1',runId:'run-active',role:'assistant',content:'正常',modelName:'Mock',createdAt:'2026-08-04T10:20:00Z'}],toolCalls:[{id:'call',runId:'run-active',sessionId:'s1',name:'host_system_summary',status:'completed',requiresApproval:false,createdAt:'',updatedAt:''}]})
    const router=await makeRouter();const wrapper=mount(AiView,{global:{plugins:[router]}});await flushPromises()
    const turn=wrapper.get('[data-tool-call-id="call"]').element.closest('.ai-agent-turn')
    expect(turn).toBe(wrapper.get('[data-message-id="answer"]').element.closest('.ai-agent-turn'))
    const meta=wrapper.get('.ai-message-meta')
    expect(meta.element.children.item(0)?.textContent).toContain('复制')
    expect(meta.element.children.item(1)?.textContent).toBe('Mock')
    expect(meta.element.children.item(2)?.tagName).toBe('TIME')
    wrapper.unmount()
  })

  it('does not force the reader back down after they scroll up',async()=>{
    const router=await makeRouter();const wrapper=mount(AiView,{global:{plugins:[router]}});await flushPromises()
    const pane=wrapper.get('.ai-messages').element
    Object.defineProperties(pane,{scrollHeight:{value:1000,configurable:true},scrollTop:{value:100,writable:true,configurable:true},clientHeight:{value:400,configurable:true}})
    await wrapper.get('.ai-messages').trigger('scroll')
    const scroll=vi.mocked(Element.prototype.scrollIntoView);scroll.mockClear()
    MockEventSource.instances[0]?.emit('message.delta',{delta:'继续输出'})
    await flushPromises()
    expect(scroll).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('回到最新')
    wrapper.unmount()
  })

  it('confines automatic follow scrolling to the AI message pane',async()=>{
    const router=await makeRouter();const wrapper=mount(AiView,{global:{plugins:[router]}});await flushPromises()
    const pane=wrapper.get('.ai-messages').element as HTMLElement
    Object.defineProperties(pane,{scrollHeight:{value:1000,configurable:true},scrollTop:{value:0,writable:true,configurable:true},clientHeight:{value:400,configurable:true}})
    const scrollIntoView=vi.mocked(Element.prototype.scrollIntoView);scrollIntoView.mockClear()
    MockEventSource.instances[0]?.emit('message.delta',{delta:'continue output'})
    await flushPromises()
    expect(pane.scrollTop).toBe(1000)
    expect(scrollIntoView).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('updates thinking level and uploads a supported image',async()=>{
    mocks.send.mockResolvedValue({runId:'run-next'})
    vi.stubGlobal('FileReader',class {result:string|ArrayBuffer|null=null;error:DOMException|null=null;onload:(()=>void)|null=null;onerror:(()=>void)|null=null;readAsDataURL(){this.result='data:image/png;base64,iVBORw0KGgo=';this.onload?.()}})
    const router=await makeRouter();const wrapper=mount(AiView,{global:{plugins:[router]}});await flushPromises()
    await wrapper.get('.ai-choice--thinking .ai-choice__trigger').trigger('click')
    await wrapper.get('.ai-choice--thinking [data-value="high"]').trigger('click');await flushPromises()
    expect(mocks.update).toHaveBeenCalledWith('s1',{thinkingLevel:'high'})
    const input=wrapper.get('input[type="file"]')
    const file=new File([new Uint8Array([137,80,78,71])],'screen.png',{type:'image/png'})
    Object.defineProperty(input.element,'files',{value:[file],configurable:true})
    await input.trigger('change');await flushPromises()
    expect(wrapper.text()).toContain('screen.png')
    await wrapper.get('.ai-composer-submit').trigger('click');await flushPromises()
    expect(mocks.send).toHaveBeenCalledWith('s1','',expect.arrayContaining([expect.objectContaining({name:'screen.png',kind:'image'})]))
    wrapper.unmount()
  })

  it('uses the active locale for native session dialogs', async () => {
    await setLocale('en-US', false)
    const promptMock = vi.fn().mockReturnValue(null)
    const confirmMock = vi.fn().mockReturnValue(false)
    vi.stubGlobal('prompt', promptMock)
    vi.stubGlobal('confirm', confirmMock)
    const router = await makeRouter()
    const wrapper = mount(AiView, { global: { plugins: [router] } })
    await flushPromises()

    await wrapper.get('.ai-chat-title').trigger('click')
    await wrapper.get('.ai-session-actions button:last-child').trigger('click')

    expect(promptMock).toHaveBeenCalledWith('Session name', 'Running')
    expect(confirmMock).toHaveBeenCalledWith('Delete session “Running”? This cannot be undone.')
    wrapper.unmount()
  })
})
