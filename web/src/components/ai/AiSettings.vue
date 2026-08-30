<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Check, CheckCircle2, ChevronRight, KeyRound, LoaderCircle, Plus, RefreshCw, ShieldCheck, SlidersHorizontal, Trash2, X } from '@lucide/vue'
import { useI18n } from '@/i18n'
import { localizeError } from '@/i18n/errors'
import { aiApi } from '@/lib/aiApi'
import type { AIEvolutionProposal, AIMemory, AIModel, AIOpenAIAPIMode, AIProcedure, AIProvider, AIProtocol } from '@/types/ai'

const props=defineProps<{providers:AIProvider[];models:AIModel[]}>()
const emit=defineEmits<{close:[];refresh:[]}>()
const i18n=useI18n()
const tab=ref<'providers'|'memories'|'procedures'|'proposals'>('providers')
const busy=ref('');const error=ref('');const notice=ref('');const editing=ref<string>();const selectedPreset=ref('openai')
const memories=ref<AIMemory[]>([]);const procedures=ref<AIProcedure[]>([]);const proposals=ref<AIEvolutionProposal[]>([])
const form=reactive({name:'OpenAI',protocol:'openai_compatible' as AIProtocol,apiMode:'responses' as AIOpenAIAPIMode,baseUrl:'https://api.openai.com/v1',endpointScope:'public' as 'public'|'private',apiKey:'',enabled:true,privateConfirmed:false})
const modelForm=reactive({providerId:'',modelId:'',displayName:'',contextWindow:32000,toolCalling:true,vision:true,reasoning:true})
const selectedProvider=computed(()=>props.providers.find(item=>item.id===modelForm.providerId))
const selectedModels=computed(()=>props.models.filter(item=>item.providerId===modelForm.providerId))
const settingsTabs=computed(()=>[
  {id:'providers' as const,label:'API 与模型',count:0},
  {id:'memories' as const,label:'后台记忆',count:0},
  {id:'procedures' as const,label:'后台流程',count:0},
  {id:'proposals' as const,label:'待处理',count:proposals.value.length},
])
const providerPresets:Array<{id:string;name:string;protocol:AIProtocol;apiMode?:AIOpenAIAPIMode;baseUrl:string;endpointScope:'public'|'private'}>=[
  {id:'openai',name:'OpenAI',protocol:'openai_compatible',apiMode:'responses',baseUrl:'https://api.openai.com/v1',endpointScope:'public'},
  {id:'anthropic',name:'Anthropic',protocol:'anthropic',baseUrl:'https://api.anthropic.com/v1',endpointScope:'public'},
  {id:'gemini',name:'Google Gemini',protocol:'gemini',baseUrl:'https://generativelanguage.googleapis.com/v1beta',endpointScope:'public'},
  {id:'openrouter',name:'OpenRouter',protocol:'openai_compatible',baseUrl:'https://openrouter.ai/api/v1',endpointScope:'public'},
  {id:'deepseek',name:'DeepSeek',protocol:'openai_compatible',baseUrl:'https://api.deepseek.com',endpointScope:'public'},
  {id:'qwen',name:'Qwen / 百炼',protocol:'openai_compatible',baseUrl:'https://dashscope.aliyuncs.com/compatible-mode/v1',endpointScope:'public'},
  {id:'moonshot',name:'Moonshot',protocol:'openai_compatible',baseUrl:'https://api.moonshot.cn/v1',endpointScope:'public'},
  {id:'zhipu',name:'智谱 GLM',protocol:'openai_compatible',baseUrl:'https://open.bigmodel.cn/api/paas/v4',endpointScope:'public'},
  {id:'siliconflow',name:'硅基流动',protocol:'openai_compatible',baseUrl:'https://api.siliconflow.cn/v1',endpointScope:'public'},
  {id:'ollama',name:'Ollama',protocol:'openai_compatible',baseUrl:'http://host.docker.internal:11434/v1',endpointScope:'private'},
  {id:'lmstudio',name:'LM Studio',protocol:'openai_compatible',baseUrl:'http://host.docker.internal:1234/v1',endpointScope:'private'},
]
const quickPresets=computed(()=>providerPresets.filter(item=>['openai','openrouter','anthropic','gemini'].includes(item.id)))
function providerModels(id:string){return props.models.filter(item=>item.providerId===id)}
function providerModelCount(id:string){const items=providerModels(id);return `${items.filter(item=>item.enabled).length}/${items.length}`}

function resetProvider(){editing.value=undefined;selectedPreset.value='openai';notice.value='';error.value='';applyPreset('openai')}
function applyPreset(id:string){const preset=providerPresets.find(item=>item.id===id);if(!preset)return;selectedPreset.value=id;editing.value=undefined;Object.assign(form,{name:preset.name,protocol:preset.protocol,apiMode:preset.apiMode||'chat_completions',baseUrl:preset.baseUrl,endpointScope:preset.endpointScope,apiKey:'',enabled:true,privateConfirmed:false})}
function applyCustom(){selectedPreset.value='custom';editing.value=undefined;Object.assign(form,{name:'',protocol:'openai_compatible',apiMode:'chat_completions',baseUrl:'',endpointScope:'public',apiKey:'',enabled:true,privateConfirmed:false})}
function selectPreset(event:Event){const value=(event.target as HTMLSelectElement).value;value==='custom'?applyCustom():applyPreset(value)}
function edit(provider:AIProvider){editing.value=provider.id;selectedPreset.value='';notice.value='';error.value='';Object.assign(form,{name:provider.name,protocol:provider.protocol,apiMode:provider.apiMode||'chat_completions',baseUrl:provider.baseUrl,endpointScope:provider.endpointScope,apiKey:'',enabled:provider.enabled,privateConfirmed:provider.endpointScope==='private'});modelForm.providerId=provider.id}
async function saveProvider(){if(form.endpointScope==='private'&&!form.privateConfirmed){error.value=i18n.t('ai.settings.privateAddressConfirmation');return};busy.value='save';error.value='';notice.value='';let created:AIProvider|undefined;try{const body={name:form.name,protocol:form.protocol,...(form.protocol==='openai_compatible'?{apiMode:form.apiMode}:{}),baseUrl:form.baseUrl,endpointScope:form.endpointScope,privateConfirmed:form.privateConfirmed,enabled:form.enabled,...(form.apiKey?{apiKey:form.apiKey}:{})};if(editing.value){const current=props.providers.find(item=>item.id===editing.value);if(!current)throw new Error(i18n.t('ai.settings.apiInfoRefreshing'));await aiApi.providers.update(editing.value,{...body,expectedVersion:current.version});form.apiKey='';notice.value='连接信息已保存。';emit('refresh');return}created=await aiApi.providers.create(body);editing.value=created.id;modelForm.providerId=created.id;form.apiKey='';emit('refresh');busy.value='verify';await aiApi.providers.test(created.id);const synced=await aiApi.providers.sync(created.id);notice.value=`连接成功，已同步 ${synced.length} 个模型。`;emit('refresh')}catch(reason){const message=localizeError(reason,'ai.settings.saveFailed');if(created){error.value=`API 已安全保存，但自动验证未完成：${message}`;notice.value='可以检查地址或密钥后重新测试。';emit('refresh')}else error.value=message}finally{busy.value=''}}
async function action(id:string,type:'test'|'sync'|'delete'){busy.value=type+id;error.value='';notice.value='';try{if(type==='test'){await aiApi.providers.test(id);notice.value='连接测试成功。'}else if(type==='sync'){const synced=await aiApi.providers.sync(id);notice.value=`模型同步完成，共 ${synced.length} 个。`}else if(confirm(i18n.t('ai.settings.deleteProviderConfirm'))){await aiApi.providers.remove(id);if(editing.value===id)resetProvider();notice.value='API 已删除。'}emit('refresh')}catch(reason){error.value=localizeError(reason,'ai.settings.actionFailed')}finally{busy.value=''}}
async function addModel(){if(!modelForm.providerId||!modelForm.modelId)return;busy.value='model';try{await aiApi.providers.addModel(modelForm.providerId,{modelId:modelForm.modelId,displayName:modelForm.displayName||modelForm.modelId,contextWindow:modelForm.contextWindow,toolCalling:modelForm.toolCalling,vision:modelForm.vision,reasoning:modelForm.reasoning,enabled:true});modelForm.modelId='';modelForm.displayName='';emit('refresh')}catch(reason){error.value=localizeError(reason,'ai.settings.modelSaveFailed')}finally{busy.value=''}}
async function toggleModel(item:AIModel){busy.value='model-'+item.id;try{await aiApi.providers.addModel(item.providerId,{modelId:item.modelId,displayName:item.displayName,contextWindow:item.contextWindow,toolCalling:item.toolCalling,vision:item.vision,reasoning:item.reasoning,enabled:!item.enabled,isDefault:item.isDefault});emit('refresh')}catch(reason){error.value=localizeError(reason,'ai.settings.modelStateSaveFailed')}finally{busy.value=''}}
async function loadEvolution(){try{[memories.value,procedures.value,proposals.value]=await Promise.all([aiApi.evolution.memories(),aiApi.evolution.procedures(),aiApi.evolution.proposals()])}catch{ /* optional until configured */ }}
async function proposal(id:string,approve:boolean){busy.value=id;try{approve?await aiApi.evolution.approve(id):await aiApi.evolution.reject(id);await loadEvolution()}finally{busy.value=''}}
async function toggleMemory(item:AIMemory){await aiApi.evolution.updateMemory(item.id,{enabled:!item.enabled});await loadEvolution()}
async function removeMemory(item:AIMemory){if(confirm(i18n.t('ai.settings.retireMemoryConfirm',{title:item.title}))){await aiApi.evolution.removeMemory(item.id);await loadEvolution()}}
async function toggleProcedure(item:AIProcedure){await aiApi.evolution.updateProcedure(item.id,{enabled:!item.enabled});await loadEvolution()}
async function rollbackProcedure(item:AIProcedure){const value=Number(prompt(i18n.t('ai.settings.rollbackProcedurePrompt',{title:item.title}),'1'));if(value>0){await aiApi.evolution.updateProcedure(item.id,{rollbackToVersion:value});await loadEvolution()}}
async function removeProcedure(item:AIProcedure){if(confirm(i18n.t('ai.settings.retireProcedureConfirm',{title:item.title}))){await aiApi.evolution.removeProcedure(item.id);await loadEvolution()}}
onMounted(loadEvolution)
</script>

<template>
  <div class="ai-settings-backdrop" @click.self="emit('close')">
    <section class="ai-settings" role="dialog" aria-modal="true" aria-label="AI 设置">
      <header><div><span class="eyebrow">AI workspace</span><h2>AI 设置</h2><p>连接模型服务；系统会在后台学习稳定偏好和成功流程，可随时停用或回滚。</p></div><button class="icon-button" aria-label="关闭" @click="emit('close')"><X :size="19"/></button></header>
      <nav class="ai-settings__tabs">
        <button v-for="item in settingsTabs" :key="item.id" :class="{active:tab===item.id}" @click="tab=item.id"><span>{{ item.label }}</span><i v-if="item.count">{{item.count}}</i></button>
      </nav>
      <p v-if="error" class="ai-inline-error">{{ error }}</p>
      <p v-if="notice" class="ai-inline-success"><CheckCircle2 :size="15"/>{{notice}}</p>
      <div v-if="tab==='providers'" class="ai-settings__body ai-provider-layout">
        <div class="ai-provider-list">
          <div class="ai-provider-list__header"><div><strong>模型连接</strong><small>{{providers.length}} 个 API</small></div><button class="icon-button" aria-label="添加 API" title="添加 API" @click="resetProvider"><Plus :size="17"/></button></div>
          <div v-if="!providers.length" class="ai-provider-empty"><KeyRound :size="22"/><strong>还没有 API</strong><small>选择右侧预设开始配置</small></div>
          <div v-for="provider in providers" :key="provider.id" class="ai-provider-card" :class="{selected:editing===provider.id}">
            <button class="ai-provider-card__main" @click="edit(provider)"><span :class="['ai-dot',provider.enabled?'online':'']"/><span><strong>{{ provider.name }}</strong><small>{{ provider.apiMode==='responses'?'Responses':provider.protocol==='openai_compatible'?'Chat Completions':provider.protocol }} · 模型 {{providerModelCount(provider.id)}}</small></span><ChevronRight :size="15"/></button>
            <div class="ai-provider-card__meta"><span>{{ provider.apiKeySet ? `密钥 •••• ${provider.apiKeyHint}` : '未设置密钥' }}</span><span>{{provider.enabled?'已启用':'已停用'}}</span></div>
            <div class="ai-provider-card__actions">
              <button :disabled="!!busy" aria-label="测试连接" title="测试连接" @click="action(provider.id,'test')"><LoaderCircle v-if="busy==='test'+provider.id" class="spin" :size="14"/><Check v-else :size="14"/><span>测试</span></button>
              <button :disabled="!!busy" aria-label="同步模型" title="同步模型" @click="action(provider.id,'sync')"><LoaderCircle v-if="busy==='sync'+provider.id" class="spin" :size="14"/><RefreshCw v-else :size="14"/><span>同步</span></button>
              <button class="danger" :disabled="!!busy" aria-label="删除 API" title="删除 API" @click="action(provider.id,'delete')"><Trash2 :size="14"/></button>
            </div>
          </div>
          <button class="ai-add-card" @click="resetProvider"><Plus :size="17"/> 添加 API</button>
        </div>
        <div class="ai-provider-form">
          <div class="ai-provider-form__title"><div><span class="eyebrow">{{editing?'Connection details':'Quick setup'}}</span><h3>{{ editing ? `编辑 ${selectedProvider?.name||'API'}` : '添加模型 API' }}</h3><p>{{editing?'修改只影响后续请求；已有会话历史保持不变。':'选择服务、填写密钥，KPanel 会自动测试连接并同步模型。'}}</p></div><button v-if="editing" class="button button--small" @click="resetProvider"><Plus :size="14"/>添加新 API</button></div>
          <div class="ai-setup-progress"><span class="active"><i>1</i>选择服务</span><b/><span :class="{active:!!editing}"><i>2</i>连接验证</span><b/><span :class="{active:!!selectedModels.length}"><i>3</i>启用模型</span></div>
          <section v-if="!editing" class="ai-form-section"><header><div><strong>选择服务</strong><small>常用服务已预设协议和地址</small></div></header><div class="ai-preset-grid"><button v-for="preset in quickPresets" :key="preset.id" :class="{active:selectedPreset===preset.id}" @click="applyPreset(preset.id)"><span>{{preset.name.slice(0,1)}}</span><strong>{{preset.name}}</strong><Check v-if="selectedPreset===preset.id" :size="14"/></button><button :class="{active:selectedPreset==='custom'}" @click="applyCustom"><SlidersHorizontal :size="17"/><strong>自定义</strong><Check v-if="selectedPreset==='custom'" :size="14"/></button></div><label class="ai-all-presets">更多预设<select aria-label="Provider 快速预设" :value="selectedPreset" @change="selectPreset"><option value="custom">自定义兼容 API</option><option v-for="preset in providerPresets" :key="preset.id" :value="preset.id">{{preset.name}}</option></select></label></section>
          <section class="ai-form-section"><header><div><strong>连接信息</strong><small>密钥只加密保存在这台 KPanel 主机</small></div><ShieldCheck :size="18"/></header><div class="ai-form-grid"><label>显示名称<input v-model="form.name" placeholder="例如 OpenRouter" maxlength="80"/></label><label>协议<select v-model="form.protocol"><option value="openai_compatible">OpenAI-compatible</option><option value="anthropic">Anthropic Messages</option><option value="gemini">Google Gemini</option></select></label></div><label v-if="form.protocol==='openai_compatible'">API 模式<select v-model="form.apiMode" aria-label="OpenAI API 模式"><option value="responses">Responses API（OpenAI 推荐）</option><option value="chat_completions">Chat Completions（兼容模式）</option></select><small class="ai-field-help">第三方兼容服务不支持 Responses 时，请选择 Chat Completions。</small></label><label>Base URL<input v-model="form.baseUrl" placeholder="https://api.example.com/v1" spellcheck="false"/></label><label>API Key<div class="ai-secret-input"><KeyRound :size="16"/><input v-model="form.apiKey" type="password" :placeholder="editing?'留空表示保留现有密钥':'输入 API Key'" autocomplete="new-password"/></div></label><details class="ai-provider-advanced"><summary><SlidersHorizontal :size="15"/>高级连接选项</summary><div><label>网络范围<select v-model="form.endpointScope"><option value="public">公网（强制 HTTPS）</option><option value="private">内网/本地 API</option></select></label><label v-if="form.endpointScope==='private'" class="ai-check"><input v-model="form.privateConfirmed" type="checkbox"/>我确认信任该内网地址；HTTP 请求可能为明文。</label><label class="ai-check"><input v-model="form.enabled" type="checkbox"/>启用此 API</label></div></details><div class="ai-provider-submit"><button class="button button--primary" :disabled="!!busy||!form.name||!form.baseUrl" @click="saveProvider"><LoaderCircle v-if="busy==='save'||busy==='verify'" class="spin" :size="16"/>{{editing?(busy?'保存中…':'保存更改'):(busy==='verify'?'正在验证并同步…':busy?'保存中…':'保存、测试并同步')}}</button><span v-if="!editing">保存后无需退出此页面即可选择新模型</span></div></section>
          <section v-if="selectedProvider" class="ai-form-section ai-model-section"><header><div><strong>可用模型</strong><small>{{selectedModels.filter(item=>item.enabled).length}} 个已启用 · {{selectedModels.length}} 个已同步</small></div><button class="button button--small" :disabled="!!busy" @click="action(selectedProvider.id,'sync')"><RefreshCw :class="{spin:busy==='sync'+selectedProvider.id}" :size="14"/>同步模型</button></header><div v-if="selectedModels.length" class="ai-model-list"><button v-for="item in selectedModels" :key="item.id" type="button" :disabled="!!busy" :class="{enabled:item.enabled}" @click="toggleModel(item)"><span><strong>{{item.displayName}}</strong><small>{{item.modelId}} · {{item.contextWindow.toLocaleString()}} tokens · {{item.vision?'图像 ':''}}{{item.reasoning?'思考':''}}</small></span><i>{{item.enabled?'已启用':'已停用'}}</i></button></div><p v-else class="ai-model-empty">还没有模型。先测试连接并同步，或在下方手工添加。</p><details class="ai-manual-model"><summary><Plus :size="14"/>手工添加模型</summary><div class="ai-model-form"><input v-model="modelForm.modelId" placeholder="模型 ID"/><input v-model="modelForm.displayName" placeholder="显示名称（可选）"/><input v-model.number="modelForm.contextWindow" type="number" min="1024" step="1024" aria-label="上下文窗口"/><label class="ai-check"><input v-model="modelForm.toolCalling" type="checkbox"/>支持工具调用</label><label class="ai-check"><input v-model="modelForm.vision" type="checkbox"/>支持图像</label><label class="ai-check"><input v-model="modelForm.reasoning" type="checkbox"/>支持原生思考强度</label><button class="button" :disabled="!modelForm.modelId||!!busy" @click="addModel">添加模型</button></div></details></section>
        </div>
      </div>
      <div v-else class="ai-settings__body ai-evolution-list">
        <template v-if="tab==='memories'"><article v-for="item in memories" :key="item.id"><div><strong>{{item.title}}</strong><small>v{{item.version}} · {{item.retired?'已退休':item.enabled?'生效中':'已停用'}}</small></div><p>{{item.content}}</p><footer v-if="!item.retired"><button class="button" @click="toggleMemory(item)">{{item.enabled?'停用':'启用'}}</button><button class="button" @click="removeMemory(item)">退休</button></footer></article><p v-if="!memories.length" class="ai-empty-mini">尚未学习到可复用记忆</p></template>
        <template v-if="tab==='procedures'"><article v-for="item in procedures" :key="item.id"><div><strong>{{item.title}}</strong><small>v{{item.version}} · {{item.retired?'已退休':item.enabled?'生效中':'已停用'}}</small></div><p>{{item.condition}}</p><footer v-if="!item.retired"><button class="button" @click="toggleProcedure(item)">{{item.enabled?'停用':'启用'}}</button><button class="button" @click="rollbackProcedure(item)">回滚版本</button><button class="button" @click="removeProcedure(item)">退休</button></footer></article><p v-if="!procedures.length" class="ai-empty-mini">尚未学习到可复用流程</p></template>
        <template v-if="tab==='proposals'"><article v-for="item in proposals" :key="item.id"><div><strong>{{item.title}}</strong><small>{{item.type}} · 待审核</small></div><p>{{item.content}}</p><footer><button class="button" @click="proposal(item.id,false)">拒绝</button><button class="button button--primary" @click="proposal(item.id,true)">批准生效</button></footer></article><p v-if="!proposals.length" class="ai-empty-mini">暂无待审核提案</p></template>
      </div>
    </section>
  </div>
</template>
