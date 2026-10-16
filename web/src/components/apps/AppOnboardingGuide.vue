<script setup lang="ts">
import { ref, watch } from 'vue'
import { ArrowUpRight, Check, Copy, FolderCode, Globe } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { copyText } from '@/lib/clipboard'
import type { AppMarketItem } from '@/types/api'
import LocalAppManager from './LocalAppManager.vue'
import { localAppTemplate } from './localAppTemplate'

type GuideMode = 'official' | 'local'
const props = defineProps<{ open: boolean; localApps: AppMarketItem[]; initialMode?: GuideMode }>()
const emit = defineEmits<{ close: []; changed: []; manage: [item: AppMarketItem] }>()

const mode = ref<GuideMode>(props.initialMode ?? 'official')
watch(() => props.open, (open) => {
  if (open) mode.value = props.initialMode ?? 'official'
})
const copied = ref('')

const template = localAppTemplate()

// Clipboard access is often denied on plain HTTP/IP panels; say so instead
// of silently doing nothing, the template stays selectable below.
async function copy(key: string, value: string): Promise<void> {
  const state = await copyText(value) ? key : `${key}:failed`
  copied.value = state
  window.setTimeout(() => {
    if (copied.value === state) copied.value = ''
  }, 2400)
}

function copyLabel(key: string, idle: string): string {
  if (copied.value === key) return '已复制'
  return copied.value === `${key}:failed` ? '复制失败，请手动选择' : idle
}
</script>

<template>
  <ModalDialog
    :open="open"
    title="应用入驻指南"
    description="把自己的 Docker 应用写成一个 .conf，即可出现在应用市场并使用安装、更新、卸载等管理操作。"
    size="large"
    @close="emit('close')"
  >
    <div class="onboarding-modes" role="group" aria-label="入驻方式">
      <button
        type="button"
        :aria-pressed="mode === 'official'"
        :class="{ 'is-active': mode === 'official' }"
        @click="mode = 'official'"
      >
        <Globe :size="16" />
        <span><strong>提交到官方市场</strong><small>审核通过后所有用户可见</small></span>
      </button>
      <button
        type="button"
        :aria-pressed="mode === 'local'"
        :class="{ 'is-active': mode === 'local' }"
        @click="mode = 'local'"
      >
        <FolderCode :size="16" />
        <span><strong>仅本机使用</strong><small>无需审核，只在这台服务器出现</small></span>
      </button>
    </div>

    <ol v-if="mode === 'official'" class="onboarding-steps">
      <li><strong>按模板编写配置</strong><span>文件名即应用标识，例如 myapp.conf；数据统一放在 /home/docker/应用名 下。</span></li>
      <li><strong>先在本机自测</strong><span>在“仅本机使用”里新建并安装，完整走一遍安装、更新、卸载；运行正常后可在列表里一键“提交官方”。</span></li>
      <li><strong>向 kejilion/apps 提交 Pull Request</strong><span>维护者审核脚本安全后合入。</span></li>
      <li><strong>自动上架</strong><span>合入后出现在“第三方”来源，所有 KPanel 与 kejilion.sh 用户都能看到。</span></li>
    </ol>
    <LocalAppManager v-else :apps="localApps" @changed="emit('changed')" @manage="emit('manage', $event)" />

    <section v-if="mode === 'official'" class="onboarding-template" aria-label="配置模板">
      <header>
        <strong>配置模板</strong>
        <button class="button button--ghost button--small" type="button" @click="copy('template', template)">
          <Check v-if="copied === 'template'" :size="14" /><Copy v-else :size="14" />
          {{ copyLabel('template', '复制模板') }}
        </button>
      </header>
      <pre><code>{{ template }}</code></pre>
    </section>

    <template #footer>
      <a class="button button--secondary" href="https://dev.kejilion.sh/" target="_blank" rel="noopener noreferrer">
        <ArrowUpRight :size="15" /> 开发者指南
      </a>
      <a
        v-if="mode === 'official'"
        class="button button--primary"
        href="https://github.com/kejilion/apps"
        target="_blank"
        rel="noopener noreferrer"
      >
        <ArrowUpRight :size="15" /> 前往 kejilion/apps
      </a>
      <button v-else class="button button--primary" type="button" @click="emit('close')">关闭</button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.onboarding-modes {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.onboarding-modes button {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--text-secondary);
  text-align: left;
  cursor: pointer;
}

.onboarding-modes button.is-active {
  border-color: var(--brand);
  color: var(--text-primary);
  background: var(--brand-soft);
}

.onboarding-modes span {
  display: grid;
  gap: 2px;
}

.onboarding-modes small,
.onboarding-steps span {
  color: var(--text-secondary);
}

.onboarding-steps {
  display: grid;
  gap: 10px;
  margin: 16px 0;
  padding-left: 20px;
}

.onboarding-steps li::marker {
  color: var(--text-secondary);
}

.onboarding-steps strong,
.onboarding-steps span {
  display: block;
}

.onboarding-steps span {
  margin-top: 2px;
}

.onboarding-template header {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
}

.onboarding-template {
  margin-bottom: 12px;
}

.onboarding-template pre {
  max-height: 240px;
  margin: 8px 0 0;
  padding: 10px 12px;
  overflow: auto;
  border-radius: var(--radius-sm);
  background: var(--surface-muted);
  font-size: 12px;
  line-height: 1.5;
  tab-size: 2;
}

@media (max-width: 560px) {
  .onboarding-modes {
    grid-template-columns: 1fr;
  }
}
</style>
