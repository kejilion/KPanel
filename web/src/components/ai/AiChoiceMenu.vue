<script lang="ts">
export interface AiChoiceOption {
  value: string
  label: string
  shortLabel?: string
  description?: string
  group?: string
  badges?: string[]
  disabled?: boolean
}
</script>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { Check, ChevronDown, Search } from '@lucide/vue'

const props=withDefaults(defineProps<{
  modelValue:string
  options:AiChoiceOption[]
  label:string
  title?:string
  placeholder?:string
  searchable?:boolean
  variant?:'access'|'thinking'|'model'
}>(),{title:'',placeholder:'选择',searchable:false,variant:'model'})
const emit=defineEmits<{change:[value:string]}>()

const root=ref<HTMLElement>();const trigger=ref<HTMLButtonElement>();const searchInput=ref<HTMLInputElement>();const open=ref(false);const query=ref('');const menuStyle=ref<Record<string,string>>({});const mobileViewport=ref(false);const menuId=`ai-choice-${useId()}`
const selected=computed(()=>props.options.find(item=>item.value===props.modelValue))
const filtered=computed(()=>{const keyword=query.value.trim().toLowerCase();if(!keyword)return props.options;return props.options.filter(item=>[item.label,item.description,item.group,item.value].some(value=>value?.toLowerCase().includes(keyword)))})
const groups=computed(()=>{const result:{label:string;options:AiChoiceOption[]}[]=[];for(const option of filtered.value){const label=option.group||'';let group=result.find(item=>item.label===label);if(!group){group={label,options:[]};result.push(group)}group.options.push(option)}return result})
const showSearch=computed(()=>props.searchable&&props.options.length>8)

function availableOptions(){return Array.from(root.value?.querySelectorAll<HTMLButtonElement>('.ai-choice__option:not(:disabled)')||[])}
function focusOption(preferSelected=true){const options=availableOptions();if(!options.length)return;const target=preferSelected?options.find(item=>item.dataset.value===props.modelValue):options[0];(target||options[0])?.focus()}
function updateMenuPosition(){if(!open.value||!mobileViewport.value||!trigger.value)return;const rect=trigger.value.getBoundingClientRect();menuStyle.value={'--ai-choice-menu-bottom':`${Math.max(12,window.innerHeight-rect.top+8)}px`}}
function updateViewportMode(){mobileViewport.value=window.innerWidth<=680;if(!mobileViewport.value)menuStyle.value={};else updateMenuPosition()}
function show(){if(open.value)return;open.value=true;query.value='';void nextTick(()=>{updateViewportMode();showSearch.value?searchInput.value?.focus():focusOption()})}
function close(restoreFocus=false){if(!open.value)return;open.value=false;query.value='';if(restoreFocus)void nextTick(()=>trigger.value?.focus())}
function toggle(){open.value?close():show()}
function choose(option:AiChoiceOption){if(option.disabled)return;emit('change',option.value);close(true)}
function moveFocus(direction:number){const options=availableOptions();if(!options.length)return;const current=options.indexOf(document.activeElement as HTMLButtonElement);options[(current+direction+options.length)%options.length]?.focus()}
function onTriggerKeydown(event:KeyboardEvent){if(event.key==='ArrowDown'||event.key==='ArrowUp'){event.preventDefault();show();void nextTick(()=>{focusOption();if(event.key==='ArrowUp')moveFocus(-1)});return}if(event.key==='Enter'||event.key===' '){event.preventDefault();toggle()}else if(event.key==='Escape')close()}
function onMenuKeydown(event:KeyboardEvent){if(event.key==='ArrowDown'){event.preventDefault();moveFocus(1)}else if(event.key==='ArrowUp'){event.preventDefault();moveFocus(-1)}else if(event.key==='Home'){event.preventDefault();availableOptions()[0]?.focus()}else if(event.key==='End'){event.preventDefault();availableOptions().at(-1)?.focus()}else if(event.key==='Escape'){event.preventDefault();close(true)}else if(event.key==='Tab')close()}
function onDocumentPointer(event:PointerEvent){if(open.value&&!root.value?.contains(event.target as Node))close()}

watch(filtered,()=>{if(open.value&&!showSearch.value)void nextTick(()=>focusOption(false))})
onMounted(()=>{updateViewportMode();document.addEventListener('pointerdown',onDocumentPointer);window.addEventListener('resize',updateViewportMode);window.addEventListener('scroll',updateMenuPosition,true)})
onBeforeUnmount(()=>{document.removeEventListener('pointerdown',onDocumentPointer);window.removeEventListener('resize',updateViewportMode);window.removeEventListener('scroll',updateMenuPosition,true)})
</script>

<template>
  <div ref="root" class="ai-choice" :class="[`ai-choice--${variant}`,{'is-open':open}]">
    <button
      ref="trigger"
      class="ai-choice__trigger"
      type="button"
      :aria-label="label"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :aria-controls="menuId"
      :title="title"
      @click="toggle"
      @keydown="onTriggerKeydown"
    >
      <span class="ai-choice__icon"><slot name="icon"/></span>
      <span class="ai-choice__label">{{selected?.label||placeholder}}</span>
      <span class="ai-choice__short-label">{{selected?.shortLabel||selected?.label||placeholder}}</span>
      <ChevronDown :size="14"/>
    </button>
    <div v-if="open" class="ai-choice__menu" :style="menuStyle" @keydown="onMenuKeydown">
      <label v-if="showSearch" class="ai-choice__search"><Search :size="14"/><input ref="searchInput" v-model="query" type="search" placeholder="搜索模型" aria-label="搜索模型"/></label>
      <div :id="menuId" class="ai-choice__options" role="listbox" :aria-label="label">
        <template v-for="group in groups" :key="group.label||'default'">
          <span v-if="group.label" class="ai-choice__group">{{group.label}}</span>
          <button
            v-for="option in group.options"
            :key="option.value"
            class="ai-choice__option"
            type="button"
            role="option"
            :data-value="option.value"
            :aria-selected="option.value===modelValue"
            :disabled="option.disabled"
            @click="choose(option)"
          >
            <span class="ai-choice__option-copy"><strong>{{option.label}}</strong><small v-if="option.description">{{option.description}}</small></span>
            <span v-if="option.badges?.length" class="ai-choice__badges"><i v-for="badge in option.badges" :key="badge">{{badge}}</i></span>
            <Check v-if="option.value===modelValue" class="ai-choice__check" :size="15"/>
          </button>
        </template>
        <p v-if="!filtered.length" class="ai-choice__empty">没有匹配的模型</p>
      </div>
    </div>
  </div>
</template>
