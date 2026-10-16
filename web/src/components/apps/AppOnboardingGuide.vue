<script setup lang="ts">
import { ref } from 'vue'
import { ArrowUpRight, Check, Copy, FolderCode, Globe, ShieldAlert } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { copyText } from '@/lib/clipboard'

defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()

type GuideMode = 'official' | 'local'
const mode = ref<GuideMode>('official')
const copied = ref('')

const localDirectory = '/root/apps-local'
const template = `local app_name="My App"
local app_text="A brief description"
local app_url="https://github.com/..."
local docker_name="myapp"
local docker_port="8080"
local app_size="1"

docker_app_install() {
\tmkdir -p /home/docker/myapp && cd /home/docker/myapp
\tcurl -L -o docker-compose.yml "\${gh_proxy}https://raw.githubusercontent.com/..."
\tsed -i "s/8080:8080/\${docker_port}:8080/g" docker-compose.yml
\tdocker compose up -d
\tcheck_docker_app_ip
}

docker_app_update() {
\tcd /home/docker/myapp && docker compose pull && docker compose up -d
}

docker_app_uninstall() {
\tcd /home/docker/myapp && docker compose down --rmi all
\trm -rf /home/docker/myapp
}

docker_app_plus
`
const localCommand = `chown root:root ${localDirectory}/myapp.conf && chmod 644 ${localDirectory}/myapp.conf`

async function copy(key: string, value: string): Promise<void> {
  if (!await copyText(value)) return
  copied.value = key
  window.setTimeout(() => {
    if (copied.value === key) copied.value = ''
  }, 1600)
}
</script>

<template>
  <ModalDialog
    :open="open"
    title="应用入驻指南"
    description="把自己的 Docker 应用写成一个 .conf，即可出现在应用市场并使用安装、更新、卸载等管理操作。"
    size="medium"
    @close="emit('close')"
  >
    <div class="onboarding-modes" role="tablist" aria-label="入驻方式">
      <button
        type="button"
        role="tab"
        :aria-selected="mode === 'official'"
        :class="{ 'is-active': mode === 'official' }"
        @click="mode = 'official'"
      >
        <Globe :size="16" />
        <span><strong>提交到官方市场</strong><small>审核通过后所有用户可见</small></span>
      </button>
      <button
        type="button"
        role="tab"
        :aria-selected="mode === 'local'"
        :class="{ 'is-active': mode === 'local' }"
        @click="mode = 'local'"
      >
        <FolderCode :size="16" />
        <span><strong>仅本机使用</strong><small>无需审核，只在这台服务器出现</small></span>
      </button>
    </div>

    <ol v-if="mode === 'official'" class="onboarding-steps">
      <li><strong>按模板编写配置</strong><span>文件名即应用标识，例如 myapp.conf；数据统一放在 /home/docker/应用名 下。</span></li>
      <li><strong>先在本机自测</strong><span>放到 /root/apps-local 后在“本地应用”里完整走一遍安装、更新、卸载。</span></li>
      <li><strong>向 kejilion/apps 提交 Pull Request</strong><span>维护者审核脚本安全后合入。</span></li>
      <li><strong>自动上架</strong><span>合入后出现在“第三方”来源，所有 KPanel 与 kejilion.sh 用户都能看到。</span></li>
    </ol>
    <ol v-else class="onboarding-steps">
      <li><strong>按模板编写配置</strong><span>保存为 /root/apps-local/应用名.conf，可用文件管理直接新建。</span></li>
      <li><strong>收紧文件权限</strong><span>文件须属于 root，且不能被其他用户写入；软链接会被忽略。</span></li>
      <li><strong>刷新应用市场</strong><span>点击“刷新状态”，应用会出现在“本地应用”分类，kejilion.sh 菜单里也会同步出现。</span></li>
      <li><strong>与官方同名时以官方为准</strong><span>如需覆盖官方应用，请换一个文件名。</span></li>
    </ol>

    <div v-if="mode === 'local'" class="onboarding-command">
      <code>{{ localCommand }}</code>
      <button class="button button--ghost button--small" type="button" @click="copy('command', localCommand)">
        <Check v-if="copied === 'command'" :size="14" /><Copy v-else :size="14" />
        {{ copied === 'command' ? '已复制' : '复制命令' }}
      </button>
    </div>

    <section class="onboarding-template" aria-label="配置模板">
      <header>
        <strong>配置模板</strong>
        <button class="button button--ghost button--small" type="button" @click="copy('template', template)">
          <Check v-if="copied === 'template'" :size="14" /><Copy v-else :size="14" />
          {{ copied === 'template' ? '已复制' : '复制模板' }}
        </button>
      </header>
      <pre><code>{{ template }}</code></pre>
    </section>

    <div v-if="mode === 'local'" class="inline-alert inline-alert--warning">
      <ShieldAlert :size="17" />
      本地应用未经官方审核，会以 root 身份执行；请只使用自己编写或信任的配置。需要较新的 kejilion.sh。
    </div>

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
      <button v-else class="button button--primary" type="button" @click="emit('close')">我知道了</button>
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
  border-radius: 10px;
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

.onboarding-steps li {
  display: grid;
  gap: 2px;
}

.onboarding-command,
.onboarding-template header {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
}

.onboarding-command {
  margin-bottom: 12px;
  padding: 8px 10px;
  border-radius: 8px;
  background: var(--surface-muted);
}

.onboarding-command code {
  overflow-x: auto;
  white-space: nowrap;
}

.onboarding-template {
  margin-bottom: 12px;
}

.onboarding-template pre {
  max-height: 240px;
  margin: 8px 0 0;
  padding: 10px 12px;
  overflow: auto;
  border-radius: 8px;
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
