<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Boxes, Copy, Eye, EyeOff } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { copyText } from '@/lib/clipboard'
import { formatRunCommand, runCommandMask, type RunCommandSegment } from '@/lib/dockerRunCommand'
import { useToast } from '@/stores/toast'
import type { DockerContainerRunCommand } from '@/types/api'

const props = defineProps<{
  open: boolean
  containerId?: string
  containerName?: string
  /** Whether the Compose project behind this container can be opened here. */
  composeAvailable?: boolean
}>()

const emit = defineEmits<{
  close: []
  openCompose: [project: string]
}>()

interface DisplayPiece {
  text: string
  kind?: 'mask' | 'secret'
}

const toast = useToast()
const result = ref<DockerContainerRunCommand>()
const loading = ref(false)
const error = ref('')
const revealed = ref(false)
let controller: AbortController | undefined

const formatted = computed(() => result.value ? formatRunCommand(result.value) : undefined)
const title = computed(() => phrase(`${props.containerName || result.value?.name || ''} 创建命令`))

// Lines as shown: continuation lines are indented and end with a backslash;
// a hidden value keeps its KEY= prefix and shows a mask instead.
const displayLines = computed<DisplayPiece[][]>(() => {
  const value = formatted.value
  if (!value) return []
  const pieces = (line: RunCommandSegment[]): DisplayPiece[] => line.flatMap((segment): DisplayPiece[] => {
    if (!segment.concealed) return [{ text: segment.text }]
    if (revealed.value) return [{ text: segment.text, kind: 'secret' }]
    return [{ text: segment.concealed.prefix }, { text: runCommandMask, kind: 'mask' }]
  })
  const run = value.lines.map((line, index): DisplayPiece[] => [
    ...(index > 0 ? [{ text: '  ' }] : []),
    ...pieces(line),
    ...(index < value.lines.length - 1 ? [{ text: ' \\' }] : []),
  ])
  return value.followUps.length ? [...run, [], ...value.followUps.map(pieces)] : run
})

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

async function load(): Promise<void> {
  const id = props.containerId
  if (!id) return
  controller?.abort()
  const request = new AbortController()
  controller = request
  loading.value = true
  error.value = ''
  result.value = undefined
  try {
    const next = await api.docker.runCommand(id, request.signal)
    if (controller === request) result.value = next
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    if (controller !== request) return
    error.value = loadErrorMessage(reason)
  } finally {
    if (controller === request) {
      loading.value = false
      controller = undefined
    }
  }
}

function loadErrorMessage(reason: unknown): string {
  if (!(reason instanceof ApiError)) return '无法读取容器配置。'
  // An Agent older than this feature only accepts POST on unknown container sub-paths.
  if (reason.code === 'method_not_allowed' || reason.code === 'route_not_found') {
    return '当前 Agent 版本不支持查看创建命令，请先更新 KPanel。'
  }
  return reason.message
}

watch(() => [props.open, props.containerId] as const, ([open]) => {
  revealed.value = false
  if (open) void load()
  else {
    controller?.abort()
    controller = undefined
    loading.value = false
    result.value = undefined
    error.value = ''
  }
}, { immediate: true })

async function copyCommand(): Promise<void> {
  const value = formatted.value
  if (!value) return
  if (!await copyText(value.text)) {
    toast.danger(phrase('复制失败'), phrase('请手动选中命令复制。'))
    return
  }
  if (value.secrets && !revealed.value) {
    toast.success(phrase('完整命令已复制'), phrase(`包含 ${value.secrets} 个已隐藏的敏感值，请妥善保管。`))
  } else toast.success(phrase('创建命令已复制'))
}

function openCompose(): void {
  if (result.value?.composeProject) emit('openCompose', result.value.composeProject)
}
</script>

<template>
  <ModalDialog
    :open="open"
    :title="title"
    :description="phrase('根据容器当前配置还原的等价命令；Docker 不保存创建时输入的原始命令。')"
    size="large"
    @close="emit('close')"
  >
    <LoadingState v-if="loading" :rows="4" />
    <ErrorState v-else-if="error" :message="phrase(error)" :retry-label="phrase('重新读取')" @retry="load" />
    <div v-else-if="result && formatted" class="run-command">
      <div v-if="result.composeProject" class="inline-alert inline-alert--info run-command__compose">
        <Boxes :size="16" aria-hidden="true" />
        <span>{{ phrase(`该容器属于 Compose 项目 ${result.composeProject}，原始配置在项目的 Compose 文件中。`) }}</span>
        <button v-if="composeAvailable" type="button" @click="openCompose">{{ phrase('打开 Compose 配置') }}</button>
      </div>

      <div v-if="formatted.secrets" class="run-command__toolbar">
        <span>{{ phrase(revealed ? `正在显示 ${formatted.secrets} 个敏感值` : `已隐藏 ${formatted.secrets} 个疑似敏感值`) }}</span>
        <button
          class="button button--ghost button--small"
          type="button"
          :aria-pressed="revealed"
          @click="revealed = !revealed"
        >
          <EyeOff v-if="revealed" :size="14" /><Eye v-else :size="14" />
          {{ phrase(revealed ? '隐藏敏感值' : '显示敏感值') }}
        </button>
      </div>

      <pre class="run-command__code" data-i18n-ignore :aria-label="phrase('创建命令')" tabindex="0"><span
        v-for="(line, index) in displayLines" :key="index" class="run-command__line"
      ><span v-for="(piece, part) in line" :key="part" :class="piece.kind && `run-command__${piece.kind}`">{{ piece.text }}</span></span></pre>

      <ul class="run-command__notes">
        <li v-if="result.imageDefaults">{{ phrase('已省略镜像自带的默认值，例如 PATH 和默认启动命令。') }}</li>
        <li v-else class="is-warning">{{ phrase('无法读取镜像默认配置，命令中可能包含镜像自带的环境变量和启动命令。') }}</li>
        <li v-if="formatted.followUps.length">{{ phrase('容器加入了多个网络，其余网络通过 docker network connect 追加。') }}</li>
        <li v-if="result.unsupported.length" class="is-warning">
          {{ phrase('以下设置无法用 docker run 还原：') }}<code data-i18n-ignore>{{ result.unsupported.join('、') }}</code>
        </li>
        <li>{{ phrase('重新创建前，先删除原容器或改用新的名称和端口。') }}</li>
      </ul>
    </div>

    <template #footer>
      <button class="button button--secondary" type="button" @click="emit('close')">{{ phrase('关闭') }}</button>
      <button class="button button--primary" type="button" :disabled="!formatted" @click="copyCommand">
        <Copy :size="15" /> {{ phrase('复制命令') }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.run-command {
  display: grid;
  gap: 12px;
  min-width: 0;
}

.run-command__compose {
  flex-wrap: wrap;
  align-items: center;
}

.run-command__compose svg {
  flex: 0 0 auto;
}

/* On narrow dialogs the link moves under the sentence instead of breaking up. */
.run-command__compose span {
  flex: 1 1 16em;
}

.run-command__compose button {
  white-space: nowrap;
}

.run-command__toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 32px;
}

.run-command__toolbar span {
  color: var(--muted);
  font-size: 13px;
}

.run-command__code {
  --scrollbar-track: var(--terminal-shell-background);
  --scrollbar-thumb: var(--terminal-shell-scrollbar);
  --scrollbar-thumb-hover: var(--terminal-shell-scrollbar-hover);
  margin: 0;
  max-height: 52vh;
  overflow: auto;
  padding: 14px 16px;
  border: 1px solid var(--terminal-shell-border, var(--border));
  border-radius: var(--radius);
  background: var(--terminal-shell-background, var(--surface-subtle));
  color: var(--terminal-shell-text, var(--text));
  font: 400 13px/1.7 var(--font-mono);
  white-space: pre;
}

.run-command__code:focus-visible {
  outline: 2px solid var(--brand);
  outline-offset: 2px;
}

.run-command__line {
  display: block;
  min-height: 1.7em;
}

.run-command__mask,
.run-command__secret {
  color: var(--amber);
}

.run-command__mask {
  user-select: none;
}

.run-command__notes {
  display: grid;
  gap: 4px;
  margin: 0;
  padding: 0 0 0 18px;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.6;
}

.run-command__notes .is-warning {
  color: var(--amber);
}

.run-command__notes code {
  font: 400 12px/1.6 var(--font-mono);
  overflow-wrap: anywhere;
}
</style>
