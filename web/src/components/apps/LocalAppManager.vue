<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import {
  ArrowLeft, ArrowUpRight, Check, CheckCircle2, Copy, ImagePlus, LoaderCircle, Pencil, Plus, Send, ShieldAlert, Trash2, X,
} from '@lucide/vue'
import { ApiError, api } from '@/lib/api'
import { copyText } from '@/lib/clipboard'
import type { AppMarketItem, LocalAppDocument, LocalAppWriteInput } from '@/types/api'
import { localAppNamePattern, localAppTemplate, officialSubmissionUrl } from './localAppTemplate'

const props = defineProps<{ apps: AppMarketItem[] }>()
const emit = defineEmits<{ changed: []; manage: [item: AppMarketItem] }>()

const defaultIcon = '/app-icons/thirdparty-default.svg'
const maxIconBytes = 64 << 10

type View = 'list' | 'edit' | 'saved' | 'submit'
const view = ref<View>('list')
const isNew = ref(false)
const name = ref('')
const content = ref('')
const loaded = ref<LocalAppDocument>()
const iconUpload = ref<LocalAppWriteInput['icon']>()
const iconPreview = ref('')
const removeIcon = ref(false)
const busy = ref(false)
const error = ref('')
const conflict = ref(false)
const pendingDelete = ref('')
const copied = ref('')
const iconInput = ref<HTMLInputElement>()

const sortedApps = computed(() => [...props.apps].sort((a, b) => a.token.localeCompare(b.token)))
const nameError = computed(() => {
  if (!isNew.value || !name.value) return ''
  return localAppNamePattern.test(name.value) ? '' : '名称只能使用字母、数字、下划线和短横线，且以字母或数字开头'
})
const currentIcon = computed(() => {
  if (iconPreview.value) return iconPreview.value
  if (removeIcon.value) return defaultIcon
  return loaded.value?.iconUrl || defaultIcon
})
const submission = computed(() => officialSubmissionUrl(name.value, content.value))

function appStatus(item: AppMarketItem): string {
  if (!item.runtime.installed) return '未安装'
  return item.runtime.state === 'running' ? '运行中' : '已安装'
}

function resetEditor(): void {
  if (iconPreview.value) URL.revokeObjectURL(iconPreview.value)
  iconPreview.value = ''
  iconUpload.value = undefined
  removeIcon.value = false
  error.value = ''
  conflict.value = false
  loaded.value = undefined
}

function startCreate(): void {
  resetEditor()
  isNew.value = true
  name.value = ''
  content.value = localAppTemplate()
  view.value = 'edit'
}

async function loadDocument(token: string): Promise<boolean> {
  busy.value = true
  error.value = ''
  try {
    loaded.value = await api.apps.localApp(token)
    content.value = loaded.value.content
    name.value = token
    return true
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '读取配置失败'
    return false
  } finally {
    busy.value = false
  }
}

async function startEdit(item: AppMarketItem): Promise<void> {
  resetEditor()
  isNew.value = false
  name.value = item.token
  view.value = 'edit'
  await loadDocument(item.token)
}

async function startSubmit(token: string): Promise<void> {
  resetEditor()
  isNew.value = false
  view.value = 'submit'
  await loadDocument(token)
}

// The saved app appears in props.apps once the market reloads its inventory.
const savedApp = computed(() => props.apps.find((item) => item.token === name.value))

// Rename the template's placeholder while the user is still naming a new app.
function onNameInput(): void {
  if (!isNew.value || !localAppNamePattern.test(name.value)) return
  if (content.value.includes('local app_id="myapp"')) content.value = localAppTemplate(name.value)
}

function backToList(): void {
  resetEditor()
  view.value = 'list'
}

async function pickIcon(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  error.value = ''
  if (file.type !== 'image/png' && file.type !== 'image/webp') {
    error.value = '图标只支持 PNG 或 WebP'
    return
  }
  if (file.size > maxIconBytes) {
    error.value = '图标不能超过 64 KB'
    return
  }
  const url = URL.createObjectURL(file)
  const size = await new Promise<{ width: number; height: number } | undefined>((resolve) => {
    const image = new Image()
    image.onload = () => resolve({ width: image.naturalWidth, height: image.naturalHeight })
    image.onerror = () => resolve(undefined)
    image.src = url
  })
  if (!size || size.width !== size.height || size.width < 64 || size.width > 512) {
    URL.revokeObjectURL(url)
    error.value = '图标需为正方形，边长 64–512 px'
    return
  }
  let bytes: Uint8Array
  try {
    bytes = new Uint8Array(await file.arrayBuffer())
  } catch {
    URL.revokeObjectURL(url)
    error.value = '图标读取失败，请重新选择'
    return
  }
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  if (iconPreview.value) URL.revokeObjectURL(iconPreview.value)
  iconPreview.value = url
  iconUpload.value = { contentType: file.type, data: btoa(binary) }
  removeIcon.value = false
}

function clearIcon(): void {
  if (iconPreview.value) URL.revokeObjectURL(iconPreview.value)
  iconPreview.value = ''
  iconUpload.value = undefined
  removeIcon.value = Boolean(loaded.value?.iconUrl)
}

async function save(): Promise<void> {
  if (busy.value || nameError.value || !name.value) return
  busy.value = true
  error.value = ''
  conflict.value = false
  const body: LocalAppWriteInput = {
    token: name.value,
    content: content.value,
    resourceVersion: loaded.value?.resourceVersion || '',
    ...(iconUpload.value ? { icon: iconUpload.value } : {}),
    ...(removeIcon.value ? { removeIcon: true } : {}),
  }
  try {
    const created = isNew.value
    if (created) await api.apps.createLocalApp(body)
    else await api.apps.updateLocalApp(name.value, body)
    emit('changed')
    if (created) {
      resetEditor()
      view.value = 'saved'
    } else {
      backToList()
    }
  } catch (reason) {
    conflict.value = reason instanceof ApiError && reason.code === 'resource_conflict'
    error.value = reason instanceof Error ? reason.message : '保存失败'
  } finally {
    busy.value = false
  }
}

async function reloadAfterConflict(): Promise<void> {
  const token = name.value
  resetEditor()
  await loadDocument(token)
}

async function confirmDelete(item: AppMarketItem): Promise<void> {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    const current = await api.apps.localApp(item.token)
    await api.apps.deleteLocalApp(item.token, current.resourceVersion)
    pendingDelete.value = ''
    emit('changed')
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '删除失败'
  } finally {
    busy.value = false
  }
}

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

onBeforeUnmount(() => {
  if (iconPreview.value) URL.revokeObjectURL(iconPreview.value)
})
</script>

<template>
  <section v-if="view === 'list'" class="local-apps" aria-label="本机的本地应用">
    <header class="local-apps__header">
      <div>
        <strong>本机的本地应用</strong>
        <small>保存在 /root/apps-local，kejilion.sh 菜单同步可见</small>
      </div>
      <button class="button button--primary button--small" type="button" @click="startCreate">
        <Plus :size="14" /> 新建本地应用
      </button>
    </header>

    <p v-if="!sortedApps.length" class="local-apps__empty">
      还没有本地应用。点击“新建本地应用”，粘贴配置并保存，即可在应用市场安装和管理。
    </p>
    <ul v-else class="local-apps__list">
      <li v-for="item in sortedApps" :key="item.id">
        <img :src="item.icon" alt="" />
        <div class="local-apps__meta">
          <strong>{{ item.name_zh }}</strong>
          <small>{{ item.token }}.conf · {{ appStatus(item) }}</small>
        </div>
        <template v-if="pendingDelete === item.token">
          <span class="local-apps__confirm">确认删除？</span>
          <button class="button button--danger button--small" type="button" :disabled="busy" @click="confirmDelete(item)">
            <LoaderCircle v-if="busy" class="spin" :size="14" /> 删除
          </button>
          <button class="button button--ghost button--small" type="button" @click="pendingDelete = ''">取消</button>
        </template>
        <template v-else>
          <button
            class="button button--ghost button--small"
            type="button"
            title="本机验证通过后提交到官方市场"
            @click="startSubmit(item.token)"
          >
            <Send :size="14" /> 提交官方
          </button>
          <button class="button button--ghost button--small" type="button" @click="startEdit(item)">
            <Pencil :size="14" /> 编辑
          </button>
          <button
            v-if="item.runtime.installed"
            class="button button--ghost button--small"
            type="button"
            title="已安装的应用需先卸载再删除配置"
            @click="emit('manage', item)"
          >
            先卸载
          </button>
          <button v-else class="button button--ghost button--small" type="button" @click="pendingDelete = item.token">
            <Trash2 :size="14" /> 删除
          </button>
        </template>
      </li>
    </ul>
    <p v-if="error" class="local-apps__error" role="alert">{{ error }}</p>

    <div class="inline-alert inline-alert--warning">
      <ShieldAlert :size="17" />
      本地应用未经官方审核，会以 root 身份执行；请只使用自己编写或信任的配置。需要较新的 kejilion.sh。
    </div>
  </section>

  <section v-else-if="view === 'edit'" class="local-editor" :aria-label="isNew ? '新建本地应用' : '编辑本地应用'">
    <header class="local-editor__header">
      <button class="button button--ghost button--small" type="button" @click="backToList">
        <ArrowLeft :size="14" /> 返回列表
      </button>
      <strong v-if="isNew">新建本地应用</strong>
      <strong v-else>编辑配置</strong>
      <code v-if="!isNew">{{ name }}.conf</code>
    </header>

    <div class="local-editor__identity">
      <div class="local-editor__icon">
        <img :src="currentIcon" alt="应用图标预览" />
        <div>
          <button class="button button--secondary button--small" type="button" @click="iconInput?.click()">
            <ImagePlus :size="14" /> 上传图标
          </button>
          <button
            v-if="iconPreview || (loaded?.iconUrl && !removeIcon)"
            class="button button--ghost button--small"
            type="button"
            @click="clearIcon"
          >
            <X :size="14" /> 移除
          </button>
          <small>PNG / WebP，正方形 64–512 px，≤64 KB</small>
        </div>
        <input ref="iconInput" class="local-editor__file" type="file" accept="image/png,image/webp" @change="pickIcon" />
      </div>
      <label class="local-editor__name">
        <span>名称（即文件名与 app_id）</span>
        <input
          v-model.trim="name"
          type="text"
          :readonly="!isNew"
          maxlength="64"
          placeholder="myapp"
          spellcheck="false"
          @input="onNameInput"
        />
        <small v-if="nameError" class="local-apps__error">{{ nameError }}</small>
      </label>
    </div>

    <div v-if="loaded?.installed" class="inline-alert inline-alert--info">
      应用已安装：docker_name 不能修改；端口等改动在下次更新或重新安装时生效。
    </div>

    <details class="local-editor__tips" :open="isNew">
      <summary>编写要点</summary>
      <ul>
        <li>app_id 与名称保持一致，docker_name 写成固定的容器名</li>
        <li>数据统一放在 /home/docker/应用名 下，不要写到 /root、/etc 等目录</li>
        <li>docker-compose 设置 restart: always 或 unless-stopped</li>
        <li>卸载要删除容器、镜像和 /home/docker/应用名 目录</li>
        <li>下载 GitHub 资源时加上 ${gh_proxy} 前缀，结尾保留 docker_app_plus</li>
      </ul>
    </details>

    <label class="local-editor__content">
      <span>配置内容（可直接粘贴）</span>
      <textarea v-model="content" spellcheck="false" rows="16" :disabled="busy && !loaded && !isNew" />
    </label>

    <p v-if="error" class="local-apps__error" role="alert">
      {{ error }}
      <button v-if="conflict" class="button button--ghost button--small" type="button" @click="reloadAfterConflict">
        重新加载
      </button>
    </p>

    <footer class="local-editor__footer">
      <button class="button button--secondary" type="button" @click="backToList">取消</button>
      <button
        class="button button--primary"
        type="button"
        :disabled="busy || !name || Boolean(nameError)"
        @click="save"
      >
        <LoaderCircle v-if="busy" class="spin" :size="16" /> 保存
      </button>
    </footer>
  </section>

  <section v-else-if="view === 'saved'" class="local-saved" aria-label="已添加本地应用">
    <CheckCircle2 :size="28" />
    <strong>已添加</strong>
    <code>{{ name }}.conf</code>
    <p>接下来在应用市场安装并验证，运行正常后即可提交到官方市场。</p>
    <ol class="local-saved__steps">
      <li>
        <span>安装验证：完整走一遍安装、更新、卸载</span>
        <button class="button button--primary button--small" type="button" :disabled="!savedApp" @click="savedApp && emit('manage', savedApp)">
          去安装
        </button>
      </li>
      <li>
        <span>提交官方：在 GitHub 发起 PR，审核后所有用户可见</span>
        <button class="button button--secondary button--small" type="button" @click="startSubmit(name)">
          <Send :size="14" /> 提交官方
        </button>
      </li>
    </ol>
    <button class="button button--ghost button--small" type="button" @click="backToList">返回列表</button>
  </section>

  <section v-else class="local-editor" aria-label="提交到官方市场">
    <header class="local-editor__header">
      <button class="button button--ghost button--small" type="button" @click="backToList">
        <ArrowLeft :size="14" /> 返回列表
      </button>
      <strong>提交到官方市场</strong>
      <code>{{ name }}.conf</code>
    </header>
    <ol class="local-submit__checks">
      <li>已在本机完成安装、更新、卸载验证</li>
      <li>数据与配置都放在 /home/docker/应用名 下，没有写到 /root、/etc 等目录</li>
      <li>docker-compose 设置了 restart: always 或 unless-stopped</li>
      <li>卸载会删除容器、镜像和 /home/docker/应用名 目录</li>
      <li>下载 GitHub 资源时使用了 ${gh_proxy} 前缀</li>
    </ol>
    <p v-if="submission.prefilled" class="local-submit__hint">打开后 GitHub 会自动 fork 并填好配置，确认无误后点击“Propose new file”发起 PR。</p>
    <p v-else class="local-submit__hint">配置较长，GitHub 页面不会自动填入：请先复制配置，再粘贴到新建文件页。</p>
    <p class="local-submit__hint">如有图标，可在 PR 描述里附上图片。</p>
    <p v-if="error" class="local-apps__error" role="alert">{{ error }}</p>
    <footer class="local-editor__footer">
      <button class="button button--secondary" type="button" :disabled="!content" @click="copy('config', content)">
        <Check v-if="copied === 'config'" :size="14" /><Copy v-else :size="14" />
        {{ copyLabel('config', '复制配置') }}
      </button>
      <a
        class="button button--primary"
        :class="{ 'is-disabled': !content }"
        :href="content ? submission.url : undefined"
        target="_blank"
        rel="noopener noreferrer"
      >
        <ArrowUpRight :size="15" /> 在 GitHub 发起 PR
      </a>
    </footer>
  </section>
</template>

<style scoped>
.local-apps,
.local-editor {
  display: grid;
  gap: 12px;
}

.local-apps__header,
.local-editor__header,
.local-editor__footer {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
}

.local-editor__header {
  justify-content: flex-start;
}

.local-editor__footer {
  justify-content: flex-end;
}

.local-apps__header > div {
  display: grid;
  gap: 2px;
}

.local-apps__header small,
.local-apps__meta small,
.local-editor__icon small,
.local-editor__name span,
.local-editor__content span,
.local-apps__empty,
.local-submit__hint,
.local-editor__tips {
  padding: 8px 12px;
  border-radius: var(--radius-sm);
  background: var(--surface-muted);
}

.local-editor__tips summary {
  cursor: pointer;
  font-weight: 600;
}

.local-editor__tips ul {
  display: grid;
  gap: 4px;
  margin: 8px 0 2px;
  padding-left: 18px;
  color: var(--text-secondary);
}

.local-saved {
  display: grid;
  gap: 10px;
  justify-items: center;
  padding: 8px 0;
  text-align: center;
}

.local-saved > svg {
  color: var(--success);
}

.local-saved p {
  margin: 0;
  color: var(--text-secondary);
}

.local-saved__steps {
  display: grid;
  gap: 8px;
  width: 100%;
  margin: 4px 0;
  padding: 0;
  list-style: none;
  text-align: left;
}

.local-saved__steps li {
  display: flex;
  gap: 10px;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}

.local-submit__checks {
  color: var(--text-secondary);
}

.local-apps__empty {
  margin: 0;
  padding: 16px;
  border: 1px dashed var(--border);
  border-radius: var(--radius-sm);
  text-align: center;
}

.local-apps__list {
  display: grid;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.local-apps__list li {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 8px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}

.local-apps__list img,
.local-editor__icon img {
  flex: none;
  width: 36px;
  height: 36px;
  border-radius: var(--radius-sm);
  object-fit: cover;
}

.local-editor__icon img {
  width: 56px;
  height: 56px;
}

.local-apps__meta {
  display: grid;
  flex: 1;
  min-width: 0;
  gap: 2px;
}

.local-apps__meta strong,
.local-apps__meta small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.local-apps__confirm {
  color: var(--danger);
}

.local-apps__error {
  margin: 0;
  color: var(--danger);
}

.local-editor__identity {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}

.local-editor__icon {
  display: flex;
  gap: 10px;
  align-items: center;
}

.local-editor__icon > div {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  max-width: 220px;
}

.local-editor__icon small {
  flex-basis: 100%;
}

.local-editor__file {
  display: none;
}

.local-editor__name,
.local-editor__content {
  display: grid;
  gap: 6px;
}

.local-editor__name input,
.local-editor__content textarea {
  width: 100%;
  padding: 8px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--text-primary);
  font-family: var(--font-mono);
}

.local-editor__name input[readonly] {
  background: var(--surface-muted);
}

.local-editor__content textarea {
  min-height: 280px;
  resize: vertical;
  font-size: 12px;
  line-height: 1.5;
  tab-size: 2;
}

.local-submit__checks {
  display: grid;
  gap: 6px;
  margin: 0;
  padding-left: 20px;
}

.local-submit__hint {
  margin: 0;
}

.button.is-disabled {
  pointer-events: none;
  opacity: 0.5;
}

@media (max-width: 560px) {
  .local-editor__identity {
    grid-template-columns: 1fr;
  }

  .local-apps__list li {
    flex-wrap: wrap;
  }
}
</style>
