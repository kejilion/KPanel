<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute,useRouter } from 'vue-router'
import { usePhraseCatalog } from '@/i18n/phrase'
import { Archive, ArchiveRestore, ArrowDown, Bot, Brain, Check, CheckCircle2, ChevronLeft, CircleStop, Copy, FileText, Image, LoaderCircle, Menu, MessageSquarePlus, Paperclip, Pencil, Pin, Search, Send, Settings2, ShieldCheck, Sparkles, Trash2, Wifi, WifiOff, X } from '@lucide/vue'
import AiChoiceMenu from '@/components/ai/AiChoiceMenu.vue'
import type { AiChoiceOption } from '@/components/ai/AiChoiceMenu.vue'
import AiMarkdown from '@/components/ai/AiMarkdown.vue'
import AiSettings from '@/components/ai/AiSettings.vue'
import { aiApi,runEventURL } from '@/lib/aiApi'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import type { AIApprovalMode,AIMessage,AIModel,AIProvider,AIRun,AIRunSnapshot,AISession,AIToolCall,AIThinkingLevel,AIUploadAttachment } from '@/types/ai'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/AiView/en-US').then((module) => module.default)
  : import('@/i18n/pages/AiView/zh-TW').then((module) => module.default))

const route=useRoute();const router=useRouter()
const desktopWindowActive=inject(desktopWindowActiveKey,computed(()=>true))
const providers=ref<AIProvider[]>([]);const models=ref<AIModel[]>([]);const sessions=ref<AISession[]>([]);const messages=ref<AIMessage[]>([]);const toolCalls=ref<AIToolCall[]>([])
const currentRun=ref<AIRun>();const streamText=ref('');const search=ref('');const input=ref('');const error=ref('');const loading=ref(true);const sending=ref(false);const cancelling=ref(false);const connected=ref(false);const settingsOpen=ref(false);const sessionDrawer=ref(false)
const attachments=ref<AIUploadAttachment[]>([]);const followOutput=ref(true);const copiedMessage=ref('')
const creatingSession=ref(false);const showArchived=ref(false)
const messageCursor=ref('');const loadingOlder=ref(false)
const messagesPane=ref<HTMLElement>();const composer=ref<HTMLTextAreaElement>();const fileInput=ref<HTMLInputElement>();let source:EventSource|undefined;let searchTimer:number|undefined;let streamFrame=0;let streamQueue='';let completedStreamMessage:AIMessage|undefined
const activeId=computed(()=>String(route.params.sessionId||''));const active=computed(()=>sessions.value.find(item=>item.id===activeId.value));
const availableModels=computed(()=>models.value.filter(model=>model.enabled&&providers.value.some(provider=>provider.id===model.providerId&&provider.enabled)))
const modelGroups=computed(()=>providers.value.filter(provider=>provider.enabled).map(provider=>({provider,models:availableModels.value.filter(model=>model.providerId===provider.id)})).filter(group=>group.models.length))
const defaultModel=computed(()=>availableModels.value.find(item=>item.isDefault)||availableModels.value.find(item=>item.id===active.value?.modelId)||availableModels.value[0])
const activeModel=computed(()=>models.value.find(item=>item.id===active.value?.modelId))
const activeProvider=computed(()=>providers.value.find(item=>item.id===active.value?.providerId))
const runActive=computed(()=>!!currentRun.value&&!['completed','failed','cancelled','interrupted'].includes(currentRun.value.status))
const composerHasPayload=computed(()=>!!input.value.trim()||attachments.value.length>0)
const composerStopsRun=computed(()=>runActive.value&&!composerHasPayload.value)
const approvalChoices=computed<AiChoiceOption[]>(()=>[
  {value:'manual',label:'手动审批',shortLabel:'手动',description:'写操作逐次确认，只读操作自动执行'},
  {value:'auto',label:'安全自动审批',shortLabel:'自动',description:'常规结构化写操作自动执行，核心操作仍需确认'},
])
const thinkingChoices=computed<AiChoiceOption[]>(()=>[
  {value:'low',label:'低',description:'快速响应，适合简单查询'},
  {value:'medium',label:'中',description:'平衡速度、分析与结果核对'},
  {value:'high',label:'高',description:'深入检查复杂运维任务'},
])
const modelChoices=computed<AiChoiceOption[]>(()=>{
  const choices:AiChoiceOption[]=modelGroups.value.flatMap(group=>group.models.map(model=>({
    value:model.id,label:model.displayName,description:model.modelId,group:group.provider.name,
    badges:[model.toolCalling?'工具':'',model.vision?'图像':'',model.reasoning?'思考':''].filter(Boolean),
  })))
  if(active.value&&!active.value.modelAvailable&&!choices.some(item=>item.value===active.value?.modelId))choices.unshift({value:active.value.modelId,label:`${active.value.modelName}（不可用）`,disabled:true})
  return choices
})
const modelQueued=computed(()=>runActive.value&&!!active.value&&!!currentRun.value&&active.value.modelId!==currentRun.value.modelId)
const approvalModeQueued=computed(()=>runActive.value&&!!active.value&&!!currentRun.value&&active.value.approvalMode!==currentRun.value.approvalMode)
const thinkingQueued=computed(()=>runActive.value&&!!active.value&&!!currentRun.value&&(active.value.thinkingLevel||'medium')!==(currentRun.value.thinkingLevel||'medium'))
const runState=computed(()=>{if(!runActive.value)return{label:'就绪',tone:'idle'};if(currentRun.value?.status==='pending_approval')return{label:'等待确认',tone:'approval'};if(!connected.value)return{label:'正在重连',tone:'offline'};return{label:'正在生成',tone:'online'}})
const contextUsage=computed(()=>{const model=models.value.find(item=>item.id===active.value?.modelId);if(!model)return 0;const chars=messages.value.reduce((sum,item)=>sum+item.content.length,0)+streamText.value.length;return Math.min(100,Math.round(chars/4/model.contextWindow*100))})
const grouped=computed(()=>{const result:{label:string;items:AISession[]}[]=[];const now=new Date();for(const item of sessions.value.filter(item=>item.title.toLowerCase().includes(search.value.toLowerCase()))){const date=new Date(item.lastMessageAt);const days=Math.floor((new Date(now.getFullYear(),now.getMonth(),now.getDate()).getTime()-new Date(date.getFullYear(),date.getMonth(),date.getDate()).getTime())/86400000);const label=showArchived.value?'已归档':item.pinned?'置顶':days===0?'今天':days===1?'昨天':'更早';let group=result.find(group=>group.label===label);if(!group){group={label,items:[]};result.push(group)}group.items.push(item)}return result})
const pendingApproval=computed(()=>toolCalls.value.find(item=>item.status==='pending_approval'))
type TurnSegment={kind:'message';id:string;message:AIMessage;final:boolean}|{kind:'tool';id:string;call:AIToolCall}|{kind:'stream';id:string}
type TimelineItem={kind:'user';id:string;message:AIMessage}|{kind:'turn';id:string;runId:string;segments:TurnSegment[]}

function timelineTime(value:string){const parsed=Date.parse(value);return Number.isFinite(parsed)?parsed:undefined}
function turnSegments(turnMessages:AIMessage[],calls:AIToolCall[],streaming:boolean){
  const dated=[...turnMessages,...calls].every(item=>timelineTime(item.createdAt)!==undefined)
  let segments:TurnSegment[]
  if(dated){
    const ordered=[
      ...turnMessages.map((message,index)=>({kind:'message' as const,id:`message-${message.id}`,message,final:false,time:timelineTime(message.createdAt)!,priority:0,index})),
      ...calls.map((call,index)=>({kind:'tool' as const,id:`tool-${call.id}`,call,time:timelineTime(call.createdAt)!,priority:1,index})),
    ].sort((left,right)=>left.time-right.time||left.priority-right.priority||left.index-right.index)
    segments=ordered.map(({time:_,priority:__,index:___,...segment})=>segment)
  }else{
    // 旧版本记录可能没有时间戳，沿用“工具过程在最终回答前”的稳定顺序。
    segments=[...calls.map(call=>({kind:'tool' as const,id:`tool-${call.id}`,call})),...turnMessages.map(message=>({kind:'message' as const,id:`message-${message.id}`,message,final:false}))]
  }
  if(streaming)segments.push({kind:'stream',id:'stream'})
  if(!streaming){const last=[...segments].reverse().find(segment=>segment.kind==='message');if(last?.kind==='message')last.final=true}
  return segments
}
const timeline=computed<TimelineItem[]>(()=>{const result:TimelineItem[]=[];const inserted=new Set<string>();for(const message of messages.value){if(message.role==='user'){result.push({kind:'user',id:`message-${message.id}`,message});continue}const key=message.runId||message.id;if(inserted.has(key))continue;inserted.add(key);const turnMessages=messages.value.filter(item=>item.role==='assistant'&&(item.runId||item.id)===key);const calls=message.runId?toolCalls.value.filter(item=>item.runId===message.runId):[];const streaming=currentRun.value?.id===message.runId&&!!streamText.value;result.push({kind:'turn',id:`turn-${key}`,runId:message.runId||'',segments:turnSegments(turnMessages,calls,streaming)})}for(const call of toolCalls.value){if(inserted.has(call.runId))continue;inserted.add(call.runId);const calls=toolCalls.value.filter(item=>item.runId===call.runId);const streaming=currentRun.value?.id===call.runId&&!!streamText.value;result.push({kind:'turn',id:`turn-${call.runId}`,runId:call.runId,segments:turnSegments([],calls,streaming)})}if(streamText.value&&currentRun.value&&!inserted.has(currentRun.value.id))result.push({kind:'turn',id:`turn-${currentRun.value.id}`,runId:currentRun.value.id,segments:turnSegments([],[],true)});return result})

function friendlyTool(name:string){return name.replace(/^host_/,'').replaceAll('_',' · ')}
function hasActivityLink(name:string){return ['host_app_action','host_diagnostic_start','host_docker_task','host_site_change','host_job_input'].includes(name)}
function toolStatusLabel(call:AIToolCall){if(call.status==='pending_approval')return '等待批准';if(call.status==='running')return '正在执行';if(call.status==='completed')return '已完成';if(call.status==='failed')return '执行失败';if(call.status==='rejected')return '已拒绝';return call.status}
function scrollBottom(force=false,smooth=false){if(!force&&!followOutput.value)return;if(force)followOutput.value=true;void nextTick(()=>{const pane=messagesPane.value;if(!pane)return;if(smooth&&typeof pane.scrollTo==='function'){pane.scrollTo({top:pane.scrollHeight,behavior:'smooth'});return}pane.scrollTop=pane.scrollHeight})}
function onMessagesScroll(){const pane=messagesPane.value;if(!pane)return;followOutput.value=pane.scrollHeight-pane.scrollTop-pane.clientHeight<96}
function appendStream(delta:string){streamQueue+=delta;if(desktopWindowActive.value&&!streamFrame)streamFrame=requestAnimationFrame(flushStreamFrame)}
function flushStreamFrame(){streamFrame=0;if(streamQueue){const count=Math.max(2,Math.min(96,Math.ceil(streamQueue.length/10)));streamText.value+=streamQueue.slice(0,count);streamQueue=streamQueue.slice(count);scrollBottom();if(streamQueue)streamFrame=requestAnimationFrame(flushStreamFrame);else if(completedStreamMessage)finalizeStreamMessage()}else if(completedStreamMessage)finalizeStreamMessage()}
function finalizeStreamMessage(){const value=completedStreamMessage;if(!value)return;completedStreamMessage=undefined;streamText.value='';const index=messages.value.findIndex(item=>item.id===value.id);if(index>=0)messages.value.splice(index,1,value);else messages.value.push(value);scrollBottom()}
function resetStream(){if(streamFrame)cancelAnimationFrame(streamFrame);streamFrame=0;streamQueue='';completedStreamMessage=undefined;streamText.value=''}
function conversationMessages(items:AIMessage[]){return items.filter(item=>(item.role==='user'||item.role==='assistant')&&!item.toolCallId)}
async function loadAll(){error.value='';try{[providers.value,models.value,sessions.value]=await Promise.all([aiApi.providers.list(),aiApi.models(),aiApi.sessions.list(search.value,showArchived.value)]);if(activeId.value&&!sessions.value.some(item=>item.id===activeId.value)){await router.replace('/ai')}else if(activeId.value){await loadMessages()}else if(sessions.value[0])await router.replace(`/ai/s/${sessions.value[0].id}`)}catch(reason){error.value=reason instanceof Error?reason.message:'AI 工作台加载失败'}finally{loading.value=false}}
async function refreshConversationMessages(){const sessionId=activeId.value;if(!sessionId)return;const page=await aiApi.sessions.messages(sessionId);if(sessionId!==activeId.value)return;messages.value=conversationMessages(page.items);if(page.toolCalls)toolCalls.value=page.toolCalls;messageCursor.value=page.nextCursor||'';scrollBottom()}
async function loadMessages(){closeStream();messages.value=[];messageCursor.value='';toolCalls.value=[];currentRun.value=undefined;resetStream();attachments.value=[];followOutput.value=true;if(!activeId.value)return;try{await refreshConversationMessages();scrollBottom(true);const session=active.value;if(session?.running&&session.activeRunId)openStream(session.activeRunId);else if(session?.lastRunId&&['interrupted','failed'].includes(session.lastRunStatus||''))currentRun.value=await aiApi.runs.get(session.lastRunId)}catch(reason){error.value=reason instanceof Error?reason.message:'对话读取失败'}}
async function loadOlder(){if(!activeId.value||!messageCursor.value||loadingOlder.value)return;loadingOlder.value=true;try{const page=await aiApi.sessions.messages(activeId.value,messageCursor.value);messages.value=[...conversationMessages(page.items),...messages.value];messageCursor.value=page.nextCursor||''}catch(reason){error.value=reason instanceof Error?reason.message:'更早消息加载失败'}finally{loadingOlder.value=false}}
async function openNewSession(){const model=defaultModel.value;const provider=providers.value.find(item=>item.id===model?.providerId);if(!provider||!model){settingsOpen.value=true;return}if(creatingSession.value)return;creatingSession.value=true;error.value='';try{const item=await aiApi.sessions.create(provider.id,model.id);showArchived.value=false;sessions.value.unshift(item);await router.push(`/ai/s/${item.id}`);sessionDrawer.value=false;await nextTick();composer.value?.focus()}catch(reason){error.value=reason instanceof Error?reason.message:'新建会话失败'}finally{creatingSession.value=false}}
async function send(){const content=input.value.trim();const selected=attachments.value;if((!content&&!selected.length)||!active.value||sending.value)return;sending.value=true;error.value='';input.value='';attachments.value=[];void nextTick(()=>{if(composer.value)composer.value.style.height='auto'});messages.value.push({id:`local-${Date.now()}`,sessionId:active.value.id,role:'user',content,attachments:selected.map(({name,mimeType,size,kind,previewUrl})=>({name,mimeType,size,kind,previewUrl})),createdAt:new Date().toISOString()});scrollBottom(true);try{const result=await aiApi.sessions.send(active.value.id,content,selected);openStream(result.runId);void refreshSessions()}catch(reason){input.value=content;error.value=reason instanceof Error?reason.message:'发送失败';await loadMessages();attachments.value=selected}finally{sending.value=false}}
function fileData(file:File){return new Promise<string>((resolve,reject)=>{const reader=new FileReader();reader.onload=()=>resolve(String(reader.result||'').split(',',2)[1]||'');reader.onerror=()=>reject(reader.error||new Error('文件读取失败'));reader.readAsDataURL(file)})}
async function chooseAttachments(event:Event){const files=Array.from((event.target as HTMLInputElement).files||[]);(event.target as HTMLInputElement).value='';if(!files.length)return;error.value='';if(attachments.value.length+files.length>4){error.value='每条消息最多上传 4 个附件。';return}let total=attachments.value.reduce((sum,item)=>sum+item.size,0);for(const file of files){const kind=file.type.startsWith('image/')?'image':'text';if(kind==='image'&&!activeModel.value?.vision){error.value='当前模型未配置图像输入能力，请切换模型或在模型设置中启用。';return}if(kind==='image'&&file.size>4*1024*1024){error.value='单张图片不能超过 4 MiB。';return}if(kind==='text'&&file.size>512*1024){error.value='单个文本文件不能超过 512 KiB。';return}total+=file.size;if(total>8*1024*1024){error.value='附件总大小不能超过 8 MiB。';return}try{const data=await fileData(file);attachments.value.push({name:file.name,mimeType:file.type||'text/plain',data,file,size:file.size,kind,previewUrl:kind==='image'?`data:${file.type};base64,${data}`:undefined})}catch(reason){error.value=reason instanceof Error?reason.message:'文件读取失败';return}}}
function removeAttachment(index:number){attachments.value.splice(index,1)}
async function copyMessage(message:AIMessage){await navigator.clipboard.writeText(message.content);copiedMessage.value=message.id;window.setTimeout(()=>{if(copiedMessage.value===message.id)copiedMessage.value=''},1200)}
function outputTime(value:string){if(!value)return '';return new Intl.DateTimeFormat('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hour12:false}).format(new Date(value))}
function onKeydown(event:KeyboardEvent){if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();void send()}}
function resizeComposer(event:Event){const target=event.target as HTMLTextAreaElement;target.style.height='auto';target.style.height=`${Math.min(target.scrollHeight,160)}px`}
function openStream(runId:string){closeStream();connected.value=false;source=new EventSource(runEventURL(runId),{withCredentials:true});source.onopen=()=>connected.value=true;source.onerror=()=>{connected.value=false};const listen=(name:string,handler:(value:any)=>void)=>source?.addEventListener(name,event=>{try{handler(JSON.parse((event as MessageEvent).data))}catch{}});listen('run.snapshot',(value:AIRunSnapshot)=>{currentRun.value=value.run;toolCalls.value=value.toolCalls;messages.value=conversationMessages(value.messages)});listen('message.delta',(value:{delta:string})=>appendStream(value.delta));listen('message.completed',(value:AIMessage)=>{if(value.role!=='assistant'||value.toolCallId)return;completedStreamMessage=value;if(!streamQueue)finalizeStreamMessage()});listen('approval.required',(value:AIToolCall)=>{mergeTool(value);if(currentRun.value)currentRun.value.status='pending_approval'});listen('tool.started',mergeTool);listen('tool.completed',mergeTool);for(const terminal of ['run.completed','run.failed','run.cancelled'] as const)listen(terminal,(value:AIRun)=>{currentRun.value=value;closeStream();if(!streamQueue&&!completedStreamMessage)void refreshConversationMessages();void refreshSessions()});listen('auth.expired',()=>{closeStream();error.value='登录已过期，请重新登录。'})}
function mergeTool(value:AIToolCall){const index=toolCalls.value.findIndex(item=>item.id===value.id);if(index>=0)toolCalls.value.splice(index,1,value);else toolCalls.value.push(value);scrollBottom()}
function closeStream(){source?.close();source=undefined;connected.value=false}
async function decide(approve:boolean){const call=pendingApproval.value;if(!call||!currentRun.value)return;try{await aiApi.runs.decision(currentRun.value.id,call.id,approve);call.status=approve?'running':'rejected';openStream(currentRun.value.id)}catch(reason){error.value=reason instanceof Error?reason.message:'审批提交失败'}}
async function cancelRun(){if(!currentRun.value||cancelling.value)return;cancelling.value=true;error.value='';try{await aiApi.runs.cancel(currentRun.value.id);closeStream();await refreshSessions();await loadMessages()}catch(reason){error.value=reason instanceof Error?reason.message:'停止运行失败'}finally{cancelling.value=false}}
async function composerAction(){if(composerStopsRun.value){await cancelRun();return}await send()}
async function retryRun(){if(!currentRun.value)return;try{const result=await aiApi.runs.retry(currentRun.value.id);openStream(result.runId);await refreshSessions()}catch(reason){error.value=reason instanceof Error?reason.message:'重试失败'}}
async function refreshSessions(){sessions.value=await aiApi.sessions.list(search.value,showArchived.value)}
async function toggleArchiveView(){showArchived.value=!showArchived.value;search.value='';if(activeId.value)await router.push('/ai');await refreshSessions();if(sessions.value[0])await router.push(`/ai/s/${sessions.value[0].id}`)}
async function updateModel(modelId:string){const model=models.value.find(item=>item.id===modelId);const provider=providers.value.find(item=>item.id===model?.providerId);if(!active.value||!model||!provider)return;error.value='';try{const updated=await aiApi.sessions.update(active.value.id,{providerId:provider.id,modelId:model.id});sessions.value=sessions.value.map(item=>item.id===updated.id?updated:item)}catch(reason){error.value=reason instanceof Error?reason.message:'模型切换失败'}}
async function updateApprovalMode(value:string){const approvalMode=value as AIApprovalMode;if(!active.value)return;error.value='';try{const updated=await aiApi.sessions.update(active.value.id,{approvalMode});sessions.value=sessions.value.map(item=>item.id===updated.id?updated:item)}catch(reason){error.value=reason instanceof Error?reason.message:'权限模式切换失败'}}
async function updateThinkingLevel(value:string){const thinkingLevel=value as AIThinkingLevel;if(!active.value)return;error.value='';try{const updated=await aiApi.sessions.update(active.value.id,{thinkingLevel});sessions.value=sessions.value.map(item=>item.id===updated.id?updated:item)}catch(reason){error.value=reason instanceof Error?reason.message:'思考强度切换失败'}}
async function rename(item?:AISession){const target=item||active.value;if(!target)return;const title=prompt('会话名称',target.title)?.trim();if(!title)return;try{const updated=await aiApi.sessions.update(target.id,{title});sessions.value=sessions.value.map(value=>value.id===updated.id?updated:value)}catch(reason){error.value=reason instanceof Error?reason.message:'会话重命名失败'}}
async function togglePin(item:AISession){try{const updated=await aiApi.sessions.update(item.id,{pinned:!item.pinned});sessions.value=sessions.value.map(value=>value.id===updated.id?updated:value)}catch(reason){error.value=reason instanceof Error?reason.message:'会话置顶失败'}}
async function refreshAfterSessionLeaves(item:AISession){const wasActive=item.id===activeId.value;if(wasActive)await router.push('/ai');await refreshSessions();if(wasActive&&sessions.value[0])await router.push(`/ai/s/${sessions.value[0].id}`)}
async function archive(item:AISession){try{await aiApi.sessions.update(item.id,{archived:true});await refreshAfterSessionLeaves(item)}catch(reason){error.value=reason instanceof Error?reason.message:'会话归档失败'}}
async function restore(item:AISession){try{await aiApi.sessions.update(item.id,{archived:false});await refreshAfterSessionLeaves(item)}catch(reason){error.value=reason instanceof Error?reason.message:'会话恢复失败'}}
async function remove(item:AISession){if(!confirm(`删除会话“${item.title}”？此操作不可恢复。`))return;try{await aiApi.sessions.remove(item.id);await refreshAfterSessionLeaves(item)}catch(reason){error.value=reason instanceof Error?reason.message:'会话删除失败'}}
watch(activeId,loadMessages);watch(search,()=>{if(searchTimer)window.clearTimeout(searchTimer);searchTimer=window.setTimeout(refreshSessions,250)});watch(desktopWindowActive,active=>{if(!active){if(streamFrame)cancelAnimationFrame(streamFrame);streamFrame=0;return}if(streamQueue){streamText.value+=streamQueue;streamQueue='';if(completedStreamMessage)finalizeStreamMessage();else scrollBottom()}})
onMounted(loadAll);onBeforeUnmount(()=>{closeStream();resetStream();if(searchTimer)window.clearTimeout(searchTimer)})
</script>

<template>
  <div class="ai-workspace" :class="{'ai-workspace--drawer':sessionDrawer}">
    <button v-if="sessionDrawer" class="ai-session-overlay" aria-label="关闭会话列表" @click="sessionDrawer=false" />
    <aside class="ai-session-panel">
      <header><div><span class="ai-brand-mark"><Sparkles :size="16"/></span><span><strong>AI 助手</strong><small>{{showArchived?'已归档会话':`${sessions.length} 个会话`}}</small></span></div><button class="icon-button ai-session-close" aria-label="关闭会话列表" @click="sessionDrawer=false"><X :size="18"/></button></header>
      <button class="ai-new-chat" @click="openNewSession"><MessageSquarePlus :size="17"/><span>新建会话</span></button>
      <div class="ai-session-filter"><label class="ai-search"><Search :size="15"/><input v-model="search" placeholder="搜索会话"/></label><button class="ai-archive-filter" :class="{active:showArchived}" :title="showArchived?'返回当前会话':'查看已归档会话'" :aria-label="showArchived?'返回当前会话':'查看已归档会话'" @click="toggleArchiveView"><ArchiveRestore v-if="showArchived" :size="16"/><Archive v-else :size="16"/></button></div>
      <div class="ai-session-groups">
        <section v-for="group in grouped" :key="group.label"><h3>{{group.label}}</h3>
          <div v-for="item in group.items" :key="item.id" class="ai-session-item" :class="{active:item.id===activeId}">
            <RouterLink :to="`/ai/s/${item.id}`" @click="sessionDrawer=false"><strong>{{item.title}}</strong><small>{{item.modelName}} · {{new Date(item.lastMessageAt).toLocaleTimeString('zh-CN',{hour:'2-digit',minute:'2-digit'})}}</small></RouterLink>
            <span v-if="item.running" class="ai-running-dot" title="运行中"/>
            <div class="ai-session-actions"><button title="重命名" aria-label="重命名会话" @click="rename(item)"><Pencil :size="13"/></button><button v-if="!showArchived" :title="item.pinned?'取消置顶':'置顶'" :aria-label="item.pinned?'取消置顶':'置顶会话'" @click="togglePin(item)"><Pin :size="13"/></button><button v-if="showArchived" title="恢复会话" aria-label="恢复会话" @click="restore(item)"><ArchiveRestore :size="13"/></button><button v-else title="归档" aria-label="归档会话" @click="archive(item)"><Archive :size="13"/></button><button title="删除" aria-label="删除会话" @click="remove(item)"><Trash2 :size="13"/></button></div>
          </div>
        </section>
        <div v-if="!grouped.length" class="ai-session-empty"><Search v-if="search" :size="20"/><Archive v-else-if="showArchived" :size="20"/><MessageSquarePlus v-else :size="20"/><strong>{{search?'没有匹配会话':showArchived?'暂无归档会话':'还没有会话'}}</strong><small>{{search?'换个关键词试试':showArchived?'归档的会话会保留在这里':'新建会话后可独立选择模型'}}</small></div>
      </div>
    </aside>

    <section class="ai-chat-panel">
      <div v-if="!availableModels.length&&!loading" class="ai-onboarding"><div class="ai-orb"><Bot :size="32"/></div><span class="eyebrow">KPanel AI 工作区</span><h1>{{providers.length?'完成模型配置':'连接第一个 AI 服务'}}</h1><p>{{providers.length?'测试 API 连接并同步模型，或手动添加兼容模型。':'选择 AI 服务并填写密钥，KPanel 会自动测试连接并同步模型。'}} 密钥加密保存在本机，模型只能调用 KPanel 注册工具。</p><div class="ai-onboarding-steps"><span class="done"><CheckCircle2 :size="15"/>选择 API</span><i/><span>验证连接</span><i/><span>选择模型</span></div><button class="button button--primary" @click="settingsOpen=true"><Settings2 :size="17"/>{{providers.length?'继续配置模型':'添加 API'}}</button></div>
      <template v-else-if="active">
        <header class="ai-chat-header">
          <button class="icon-button ai-session-toggle" aria-label="打开会话列表" @click="sessionDrawer=true"><Menu :size="19"/></button>
          <button class="ai-chat-title" title="重命名会话" @click="rename()"><strong>{{active.title}}</strong><small>{{activeProvider?.name||active.providerName}} · {{activeModel?.displayName||active.modelName}}</small></button>
          <div class="ai-chat-controls"><span class="ai-context" :title="`上下文使用约 ${contextUsage}%`"><i :style="{width:`${contextUsage}%`}"/>{{contextUsage}}%</span><span class="ai-connection" :class="runState.tone"><Wifi v-if="runState.tone==='online'" :size="14"/><WifiOff v-else-if="runState.tone==='offline'" :size="14"/><CheckCircle2 v-else :size="14"/>{{runState.label}}</span><button class="icon-button" title="添加 API 或管理模型" aria-label="AI 设置" @click="settingsOpen=true"><Settings2 :size="18"/></button></div>
        </header>
        <div v-if="!active.modelAvailable" class="ai-model-warning"><span>当前模型已停用或 Provider 不可用，请切换模型后继续。</span><button class="button button--small" @click="settingsOpen=true">管理模型</button></div>
        <div ref="messagesPane" class="ai-messages" @scroll.passive="onMessagesScroll">
          <button v-if="messageCursor" class="button button--small ai-load-older" :disabled="loadingOlder" @click="loadOlder">{{loadingOlder?'加载中…':'加载更早消息'}}</button>
          <div v-if="!messages.length&&!streamText" class="ai-welcome"><div class="ai-orb ai-orb--small"><Sparkles :size="23"/></div><h2>今天想管理什么？</h2><p>我会先读取服务器实际状态；常规操作按会话设置执行，删除、系统核心设置和交互式命令始终需要确认。</p><div><button @click="input='检查服务器资源与异常状态'">检查服务器状态</button><button @click="input='列出当前运行中的容器并总结健康情况'">查看容器健康</button><button @click="input='检查网站与应用状态，不要执行修改'">巡检网站和应用</button></div></div>
          <template v-for="item in timeline" :key="item.id">
            <article v-if="item.kind==='user'" :data-message-id="item.message.id" class="ai-message ai-message--user"><div class="ai-message__avatar"><span>你</span></div><div class="ai-message__body"><div v-if="item.message.attachments?.length" class="ai-message-attachments"><span v-for="file in item.message.attachments" :key="file.name"><Image v-if="file.kind==='image'" :size="14"/><FileText v-else :size="14"/><img v-if="file.previewUrl" :src="file.previewUrl" :alt="file.name"/><b>{{file.name}}</b></span></div><AiMarkdown v-if="item.message.content" :content="item.message.content"/></div></article>
            <article v-else class="ai-agent-turn"><div class="ai-message__avatar"><Bot :size="17"/></div><div class="ai-agent-turn__content ai-turn-flow">
              <template v-for="segment in item.segments" :key="segment.id">
                <div v-if="segment.kind==='message'" :data-message-id="segment.message.id" class="ai-assistant-output"><AiMarkdown :content="segment.message.content"/><div v-if="segment.final" class="ai-message-meta"><button type="button" :aria-label="copiedMessage===segment.message.id?'已复制回答':'复制回答'" @click="copyMessage(segment.message)"><Check v-if="copiedMessage===segment.message.id" :size="13"/><Copy v-else :size="13"/><span>{{copiedMessage===segment.message.id?'已复制':'复制'}}</span></button><span>{{segment.message.modelName||active.modelName}}</span><time :datetime="segment.message.createdAt">{{outputTime(segment.message.createdAt)}}</time></div></div>
                <div v-else-if="segment.kind==='tool'" :data-tool-call-id="segment.call.id" class="ai-tool-card ai-tool-card--inline" :class="`ai-tool-card--${segment.call.status}`"><details><summary><span class="ai-tool-icon"><Settings2 :size="14"/></span><strong>{{toolStatusLabel(segment.call)}} · {{friendlyTool(segment.call.name)}}</strong></summary><div class="ai-tool-detail"><pre>{{segment.call.argumentsPreview||segment.call.arguments}}</pre><pre v-if="segment.call.resultPreview">{{segment.call.resultPreview}}</pre></div></details><footer v-if="segment.call.status==='pending_approval'"><p>删除、系统核心、容器 exec 或交互输入必须由你确认。批准前请核对目标与参数。</p><div><button class="button" @click="decide(false)">拒绝</button><button class="button button--primary" @click="decide(true)">批准本次操作</button></div></footer><footer v-else-if="segment.call.status==='completed'&&hasActivityLink(segment.call.name)"><p>后台任务已提交，可在活动记录中继续查看进度。</p><RouterLink class="button" to="/activity">打开活动记录</RouterLink></footer></div>
                <div v-else class="ai-assistant-output ai-assistant-output--streaming"><AiMarkdown :content="streamText"/><span class="ai-cursor"/></div>
              </template>
            </div></article>
          </template>
          <article v-if="runActive&&!pendingApproval&&!timeline.some(item=>item.kind==='turn'&&item.runId===currentRun?.id)" class="ai-agent-turn ai-message--thinking"><div class="ai-message__avatar"><Bot :size="17"/></div><div class="ai-agent-turn__content"><div class="ai-thinking-dots"><i/><i/><i/></div><small>{{connected?'正在理解并规划任务…':'连接中，正在恢复本轮输出…'}}</small></div></article>
          <div v-if="currentRun&&['failed','interrupted'].includes(currentRun.status)" class="ai-run-error"><strong>{{currentRun.status==='interrupted'?'运行因 Panel 重启中断':'运行失败'}}</strong><span>{{currentRun.errorMessage||currentRun.errorCode}}</span><button class="button button--small" @click="retryRun">重试本轮</button></div>
          <button v-if="!followOutput" class="ai-follow-output" type="button" @click="scrollBottom(true,true)"><ArrowDown :size="14"/>回到最新</button>
        </div>
        <footer class="ai-composer-wrap">
          <p v-if="error" class="ai-inline-error">{{error}}</p>
          <div class="ai-composer">
            <input ref="fileInput" hidden type="file" multiple accept="image/png,image/jpeg,image/webp,image/gif,.txt,.log,.md,.json,.yaml,.yml,.toml,.ini,.conf,.csv,.xml,.html,.css,.js,.ts,.vue,.go,.py,.sh" @change="chooseAttachments"/>
            <div v-if="attachments.length" class="ai-attachment-tray"><span v-for="(file,index) in attachments" :key="`${file.name}-${index}`"><img v-if="file.previewUrl" :src="file.previewUrl" :alt="file.name"/><FileText v-else :size="15"/><b>{{file.name}}</b><button type="button" :aria-label="`移除 ${file.name}`" @click="removeAttachment(index)"><X :size="13"/></button></span></div>
            <textarea ref="composer" v-model="input" rows="1" maxlength="16384" :placeholder="runActive?'可继续补充要求，将加入当前任务…':'告诉 AI 你想完成什么…'" @input="resizeComposer" @keydown="onKeydown"/>
            <div class="ai-composer-toolbar">
              <div class="ai-composer-left">
                <button type="button" class="ai-attach-button" title="上传图片或文本文件" aria-label="上传附件" @click="fileInput?.click()"><Paperclip :size="15"/></button>
                <AiChoiceMenu :model-value="active.approvalMode" :options="approvalChoices" label="权限模式" variant="access" :title="active.approvalMode==='manual'?'所有写操作逐次确认，只读自动执行':'常规结构化写操作自动执行；核心、删除、exec 和受保护路径仍需确认'" @change="updateApprovalMode"><template #icon><ShieldCheck :size="14"/></template></AiChoiceMenu>
              </div>
              <div class="ai-composer-actions">
                <span v-if="modelQueued||approvalModeQueued||thinkingQueued" class="ai-next-model">下一轮</span>
                <AiChoiceMenu :model-value="active.thinkingLevel||'medium'" :options="thinkingChoices" label="思考强度" variant="thinking" :title="activeModel?.reasoning?'使用模型原生思考强度':'使用通用规划强度提示'" @change="updateThinkingLevel"><template #icon><Brain :size="14"/></template></AiChoiceMenu>
                <AiChoiceMenu :model-value="active.modelId" :options="modelChoices" label="选择模型" variant="model" searchable :title="runActive?'切换后从下一轮开始使用':'切换当前会话模型'" @change="updateModel"/>
                <button class="ai-composer-submit" :class="{'ai-composer-submit--stop':composerStopsRun}" :disabled="sending||cancelling||(!composerHasPayload&&!runActive)" :title="cancelling?'正在停止':composerStopsRun?'停止运行':runActive?'追加到当前任务':'发送消息'" :aria-label="composerStopsRun?'停止运行':'发送'" @click="composerAction"><LoaderCircle v-if="sending||cancelling" class="spin" :size="18"/><CircleStop v-else-if="composerStopsRun" :size="18"/><Send v-else :size="18"/></button>
              </div>
            </div>
          </div>
        </footer>
      </template>
      <div v-else-if="!loading" class="ai-empty-chat"><button class="ai-session-toggle button" @click="sessionDrawer=true"><ChevronLeft :size="16"/>打开会话列表</button><Sparkles :size="30"/><h2>{{showArchived?'选择一个归档会话':'开始一次新的对话'}}</h2><p v-if="showArchived">可以恢复、查看或删除归档会话。</p><p v-else>自动使用默认模型创建会话，随后可在输入框中随时切换。</p><button v-if="!showArchived" class="button button--primary" :disabled="creatingSession" @click="openNewSession"><LoaderCircle v-if="creatingSession" class="spin" :size="16"/><span>新建会话</span></button></div>
    </section>
    <AiSettings v-if="settingsOpen" :providers="providers" :models="models" @close="settingsOpen=false" @refresh="loadAll"/>
  </div>
</template>
