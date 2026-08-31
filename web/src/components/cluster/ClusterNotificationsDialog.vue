<script setup lang="ts">
import { nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { CheckCircle2, LoaderCircle, RefreshCw, Send, ShieldCheck } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import type { ClusterNotificationRules, ClusterNotificationSnapshot } from '@/types/api'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  close: []
}>()

const loading = ref(false)
const saving = ref(false)
const discovering = ref(false)
const testing = ref(false)
const snapshot = ref<ClusterNotificationSnapshot>()
const errorMessage = ref('')
const statusMessage = ref('')
let loadController: AbortController | undefined
let loadSequence = 0

const form = reactive({
  enabled: false,
  cpuEnabled: true,
  cpuThresholdPercent: 90,
  memoryEnabled: true,
  memoryThresholdPercent: 90,
  diskEnabled: true,
  diskThresholdPercent: 90,
  trafficEnabled: false,
  trafficThresholdMiBPerSecond: 100,
  sshLoginEnabled: true,
  hostOfflineEnabled: true,
  telegramBotToken: '',
})

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

function applySnapshot(value: ClusterNotificationSnapshot): void {
  const activeElement = typeof document !== 'undefined' && document.activeElement instanceof HTMLElement
    ? document.activeElement
    : undefined
  const modalControl = activeElement?.closest('.modal-panel') ? activeElement : undefined
  const activeLabel = modalControl?.getAttribute('aria-label') || ''
  const activeText = modalControl?.textContent?.trim() || ''
  snapshot.value = value
  form.enabled = value.enabled
  form.cpuEnabled = value.rules.cpuEnabled
  form.cpuThresholdPercent = value.rules.cpuThresholdPercent
  form.memoryEnabled = value.rules.memoryEnabled
  form.memoryThresholdPercent = value.rules.memoryThresholdPercent
  form.diskEnabled = value.rules.diskEnabled
  form.diskThresholdPercent = value.rules.diskThresholdPercent
  form.trafficEnabled = value.rules.trafficEnabled
  form.trafficThresholdMiBPerSecond = value.rules.trafficThresholdMiBPerSecond
  form.sshLoginEnabled = value.rules.sshLoginEnabled
  form.hostOfflineEnabled = value.rules.hostOfflineEnabled
  form.telegramBotToken = ''
  if (modalControl) {
    void nextTick(() => {
      if (modalControl.isConnected) {
        modalControl.focus({ preventScroll: true })
        return
      }
      const panel = document.querySelector('.modal-panel')
      if (!panel) return
      const replacement = Array.from(panel.querySelectorAll<HTMLElement>('button, input, select, textarea'))
        .find((element) =>
          (activeLabel && element.getAttribute('aria-label') === activeLabel) ||
          (!activeLabel && activeText && element.textContent?.trim() === activeText),
        )
      replacement?.focus({ preventScroll: true })
    })
  }
}

function rulesFromForm(): ClusterNotificationRules {
  return {
    cpuEnabled: form.cpuEnabled,
    cpuThresholdPercent: form.cpuThresholdPercent,
    memoryEnabled: form.memoryEnabled,
    memoryThresholdPercent: form.memoryThresholdPercent,
    diskEnabled: form.diskEnabled,
    diskThresholdPercent: form.diskThresholdPercent,
    trafficEnabled: form.trafficEnabled,
    trafficThresholdMiBPerSecond: form.trafficThresholdMiBPerSecond,
    sshLoginEnabled: form.sshLoginEnabled,
    hostOfflineEnabled: form.hostOfflineEnabled,
  }
}

function friendlyError(reason: unknown): string {
  if (reason instanceof ApiError) {
    const messages: Record<string, string> = {
      cluster_notifications_changed: '配置已被其他页面更新，请重新读取后再保存。',
      cluster_notifications_token_required: '请先输入 Telegram Bot API key。',
      cluster_notifications_not_configured: '还没有配置 Telegram Bot API key。',
      cluster_notifications_not_ready: '请先私聊机器人发送 /start，再重新发现聊天。',
      cluster_notifications_chat_not_found: '没有找到私聊会话，请先私聊机器人发送 /start。',
      cluster_notifications_invalid_token: 'Bot API key 无效，请检查后重试。',
      cluster_notifications_webhook_active: '这个机器人启用了 webhook，暂时无法自动发现私聊；请先关闭 webhook。',
      cluster_notifications_telegram_unavailable: 'Telegram 暂时不可用，请稍后重试。',
      cluster_notifications_unavailable: '通知配置暂时不可用，请检查 KPanel 数据目录权限。',
    }
    return messages[reason.code] || reason.message || '通知操作失败，请稍后重试。'
  }
  return '通知操作失败，请稍后重试。'
}

async function load(): Promise<void> {
  loadController?.abort()
  const controller = new AbortController()
  loadController = controller
  const sequence = ++loadSequence
  loading.value = true
  errorMessage.value = ''
  statusMessage.value = ''
  try {
    const value = await api.cluster.notifications(controller.signal)
    if (sequence !== loadSequence) return
    applySnapshot(value)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    if (sequence !== loadSequence) return
    errorMessage.value = friendlyError(reason)
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

async function save(): Promise<void> {
  if (!snapshot.value || saving.value) return
  const tokenProvided = Boolean(form.telegramBotToken.trim())
  saving.value = true
  errorMessage.value = ''
  statusMessage.value = ''
  try {
    const value = await api.cluster.updateNotifications({
      enabled: form.enabled,
      rules: rulesFromForm(),
      telegramBotToken: form.telegramBotToken.trim() || undefined,
      expectedResourceVersion: snapshot.value.resourceVersion,
    })
    applySnapshot(value)
    statusMessage.value = tokenProvided
      ? 'Bot API key 已保存并完成连接。'
      : '通知设置已保存。'
  } catch (reason) {
    errorMessage.value = friendlyError(reason)
  } finally {
    saving.value = false
  }
}

async function discover(): Promise<void> {
  if (!snapshot.value || discovering.value) return
  discovering.value = true
  errorMessage.value = ''
  statusMessage.value = ''
  try {
    applySnapshot(await api.cluster.discoverNotifications(snapshot.value.resourceVersion))
    statusMessage.value = '已找到私聊会话，通知通道已就绪。'
  } catch (reason) {
    errorMessage.value = friendlyError(reason)
    await load()
  } finally {
    discovering.value = false
  }
}

async function testChannel(): Promise<void> {
  if (testing.value) return
  testing.value = true
  errorMessage.value = ''
  statusMessage.value = ''
  try {
    applySnapshot(await api.cluster.testNotifications())
    statusMessage.value = '测试消息已发送。'
  } catch (reason) {
    errorMessage.value = friendlyError(reason)
    await load()
  } finally {
    testing.value = false
  }
}

function statusLabel(value?: ClusterNotificationSnapshot['telegram']['status']): string {
  switch (value) {
    case 'ready': return '已连接'
    case 'waiting_for_chat': return '等待私聊'
    case 'error': return '需要检查'
    default: return '未配置'
  }
}

function statusClass(value?: ClusterNotificationSnapshot['telegram']['status']): string {
  return value === 'ready' ? 'is-ready' : value === 'error' ? 'is-error' : 'is-pending'
}

watch(
  () => props.open,
  (open) => {
    if (open) void load()
    else {
      loadSequence += 1
      loadController?.abort()
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  loadSequence += 1
  loadController?.abort()
})
</script>

<template>
  <ModalDialog
    :open="open"
    :title="phrase('集群通知')"
    :description="phrase('只需一个 Telegram Bot API key，即可接收所有集群主机的关键变化。')"
    size="medium"
    @close="emit('close')"
  >
    <div class="cluster-notifications">
      <div v-if="loading" class="cluster-notifications__state" role="status" aria-live="polite">
        <LoaderCircle class="spin" :size="20" />
        <span>{{ phrase('正在读取通知设置…') }}</span>
      </div>

      <div v-else-if="errorMessage && !snapshot" class="cluster-notifications__state cluster-notifications__state--error" role="alert">
        <strong>{{ phrase('暂时无法读取通知设置') }}</strong>
        <p>{{ phrase(errorMessage) }}</p>
        <button class="button button--secondary" type="button" @click="load">
          <RefreshCw :size="15" /> {{ phrase('重试') }}
        </button>
      </div>

      <template v-else-if="snapshot">
        <section class="cluster-notifications__channel" :class="statusClass(snapshot.telegram.status)">
          <div class="cluster-notifications__channel-icon"><Send :size="18" /></div>
          <div class="cluster-notifications__channel-main">
            <div class="cluster-notifications__channel-title">
              <strong>Telegram</strong>
              <span class="cluster-notifications__status">
                <span class="cluster-notifications__status-dot" />{{ phrase(statusLabel(snapshot.telegram.status)) }}
              </span>
            </div>
            <p v-if="snapshot.telegram.botUsername">
              @{{ snapshot.telegram.botUsername }}<span v-if="snapshot.telegram.lastCheckedAt"> · {{ phrase('最近检查') }} {{ formatDateTime(snapshot.telegram.lastCheckedAt) }}</span>
            </p>
            <p v-else>{{ phrase('私聊机器人发送 /start，KPanel 会自动发现接收会话。') }}</p>
          </div>
          <ShieldCheck v-if="snapshot.telegram.ready" class="cluster-notifications__ready-icon" :size="18" />
        </section>

        <label class="field cluster-notifications__token">
          <span>{{ phrase('Bot API key') }}</span>
          <input
            v-model="form.telegramBotToken"
            type="password"
            autocomplete="new-password"
            :placeholder="snapshot.telegram.configured ? phrase('已保存，留空则保持不变') : phrase('粘贴 BotFather 提供的 key')"
          />
          <small>{{ phrase('凭据仅保存在当前 KPanel 的受保护数据目录，不会显示在页面或通知内容中。') }}</small>
        </label>

        <div class="cluster-notifications__channel-actions">
          <button class="button button--secondary" type="button" :disabled="saving || discovering || testing || !form.telegramBotToken.trim()" @click="save">
            <LoaderCircle v-if="saving" class="spin" :size="15" />
            <Send v-else :size="15" />
            {{ phrase(saving ? '正在连接…' : '保存并连接') }}
          </button>
          <button class="button button--secondary" type="button" :disabled="saving || discovering || testing || !snapshot.telegram.configured" @click="discover">
            <LoaderCircle v-if="discovering" class="spin" :size="15" />
            <RefreshCw v-else :size="15" />
            {{ phrase(discovering ? '正在发现…' : '重新发现私聊') }}
          </button>
          <button class="button button--secondary" type="button" :disabled="saving || discovering || testing || !snapshot.telegram.ready" @click="testChannel">
            <LoaderCircle v-if="testing" class="spin" :size="15" />
            <CheckCircle2 v-else :size="15" />
            {{ phrase(testing ? '正在发送…' : '发送测试消息') }}
          </button>
        </div>

        <section class="cluster-notifications__section">
          <div class="cluster-notifications__section-heading">
            <div>
              <h3>{{ phrase('通知开关') }}</h3>
              <p>{{ phrase('关闭后保留设置，不再主动发送告警。') }}</p>
            </div>
            <label class="cluster-notifications__switch">
              <input v-model="form.enabled" type="checkbox" :aria-label="phrase('启用集群通知')" />
              <span aria-hidden="true" />
            </label>
          </div>
        </section>

        <section class="cluster-notifications__section">
          <div class="cluster-notifications__section-heading">
            <div>
              <h3>{{ phrase('资源阈值') }}</h3>
              <p>{{ phrase('应用于本机和所有已接入主机；连续 3 次采样达到阈值才通知。') }}</p>
            </div>
          </div>
          <div class="cluster-notifications__rules">
            <label class="cluster-notifications__rule">
              <input v-model="form.cpuEnabled" type="checkbox" :aria-label="phrase('启用 CPU 通知')" />
              <span><strong>{{ phrase('CPU 使用率') }}</strong><small>{{ phrase('超过阈值提醒，恢复后发送一条恢复消息。') }}</small></span>
              <span class="cluster-notifications__threshold"><input v-model.number="form.cpuThresholdPercent" type="number" min="1" max="100" :aria-label="phrase('CPU 阈值百分比')" /><em>%</em></span>
            </label>
            <label class="cluster-notifications__rule">
              <input v-model="form.memoryEnabled" type="checkbox" :aria-label="phrase('启用内存通知')" />
              <span><strong>{{ phrase('内存使用率') }}</strong><small>{{ phrase('超过阈值提醒，恢复后发送一条恢复消息。') }}</small></span>
              <span class="cluster-notifications__threshold"><input v-model.number="form.memoryThresholdPercent" type="number" min="1" max="100" :aria-label="phrase('内存阈值百分比')" /><em>%</em></span>
            </label>
            <label class="cluster-notifications__rule">
              <input v-model="form.diskEnabled" type="checkbox" :aria-label="phrase('启用磁盘通知')" />
              <span><strong>{{ phrase('磁盘使用率') }}</strong><small>{{ phrase('超过阈值提醒；无效或未知数据不会误报。') }}</small></span>
              <span class="cluster-notifications__threshold"><input v-model.number="form.diskThresholdPercent" type="number" min="1" max="100" :aria-label="phrase('磁盘阈值百分比')" /><em>%</em></span>
            </label>
            <label class="cluster-notifications__rule">
              <input v-model="form.trafficEnabled" type="checkbox" :aria-label="phrase('启用流量通知')" />
              <span><strong>{{ phrase('网络吞吐') }}</strong><small>{{ phrase('按收发总速率计算，首次采样使用已有速率。') }}</small></span>
              <span class="cluster-notifications__threshold"><input v-model.number="form.trafficThresholdMiBPerSecond" type="number" min="1" max="1048576" :aria-label="phrase('流量阈值')" /><em>MiB/s</em></span>
            </label>
          </div>
        </section>

        <section class="cluster-notifications__section">
          <div class="cluster-notifications__section-heading">
            <div>
              <h3>{{ phrase('事件通知') }}</h3>
              <p>{{ phrase('失联沿用集群现有状态；SSH 登录只发送新的登录事件。') }}</p>
            </div>
          </div>
          <div class="cluster-notifications__event-rules">
            <label class="cluster-notifications__event-rule">
              <span><strong>{{ phrase('主机掉线 / 失联') }}</strong><small>{{ phrase('进入过期、离线、授权失败或协议异常状态时提醒。') }}</small></span>
              <input v-model="form.hostOfflineEnabled" type="checkbox" :aria-label="phrase('启用主机掉线通知')" />
            </label>
            <label class="cluster-notifications__event-rule">
              <span><strong>{{ phrase('SSH 登录') }}</strong><small>{{ phrase('仅传递用户、来源、方式和时间，不传递原始日志。') }}</small></span>
              <input v-model="form.sshLoginEnabled" type="checkbox" :aria-label="phrase('启用 SSH 登录通知')" />
            </label>
          </div>
        </section>

        <p v-if="statusMessage" class="cluster-notifications__message" role="status" aria-live="polite">
          <CheckCircle2 :size="16" />{{ phrase(statusMessage) }}
        </p>
        <p v-if="errorMessage" class="cluster-notifications__error" role="alert">{{ phrase(errorMessage) }}</p>
      </template>
    </div>

    <template #footer>
      <button class="button button--secondary" type="button" :disabled="saving" @click="emit('close')">{{ phrase('关闭') }}</button>
      <button class="button button--primary" type="button" :disabled="saving || loading || !snapshot" @click="save">
        <LoaderCircle v-if="saving" class="spin" :size="15" />
        {{ phrase(saving ? '正在保存…' : '保存设置') }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.cluster-notifications {
  display: grid;
  gap: 14px;
  color: var(--text-soft);
  font-size: 14px;
}

.cluster-notifications__state {
  display: grid;
  min-height: 180px;
  place-items: center;
  align-content: center;
  gap: 9px;
  color: var(--muted);
  text-align: center;
}

.cluster-notifications__state--error {
  justify-items: center;
  color: var(--danger);
}

.cluster-notifications__state--error strong {
  color: var(--text);
}

.cluster-notifications__state--error p,
.cluster-notifications__error,
.cluster-notifications__message {
  margin: 0;
  line-height: 1.5;
}

.cluster-notifications__state--error p {
  max-width: 38em;
  color: var(--text-soft);
}

.cluster-notifications__channel {
  display: flex;
  align-items: center;
  gap: 11px;
  padding: 12px;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
}

.cluster-notifications__channel-icon {
  display: grid;
  width: 34px;
  height: 34px;
  flex: 0 0 auto;
  place-items: center;
  color: var(--brand);
  background: var(--brand-soft);
  border-radius: 10px;
}

.cluster-notifications__channel-main {
  min-width: 0;
  flex: 1;
}

.cluster-notifications__channel-title,
.cluster-notifications__status {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cluster-notifications__channel-title {
  justify-content: space-between;
}

.cluster-notifications__status {
  gap: 5px;
  color: var(--muted);
  font-size: 12px;
  font-weight: 700;
}

.cluster-notifications__status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--muted);
}

.is-ready .cluster-notifications__status {
  color: var(--success);
}

.is-ready .cluster-notifications__status-dot {
  background: var(--success);
}

.is-error .cluster-notifications__status {
  color: var(--danger);
}

.is-error .cluster-notifications__status-dot {
  background: var(--danger);
}

.cluster-notifications__channel-main p {
  margin: 3px 0 0;
  color: var(--muted);
  font-size: 12px;
  overflow-wrap: anywhere;
}

.cluster-notifications__ready-icon {
  color: var(--success);
}

.field.cluster-notifications__token {
  gap: 6px;
}

.cluster-notifications__token small {
  color: var(--muted);
  font-size: 12px;
  font-weight: 450;
  line-height: 1.45;
}

.cluster-notifications__channel-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.cluster-notifications__section {
  display: grid;
  gap: 11px;
  padding-top: 13px;
  border-top: 1px solid var(--border);
}

.cluster-notifications__section-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.cluster-notifications__section-heading h3 {
  margin: 0;
  color: var(--text);
  font-size: 14px;
}

.cluster-notifications__section-heading p {
  margin: 3px 0 0;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.45;
}

.cluster-notifications__switch {
  position: relative;
  display: inline-flex;
  width: 38px;
  height: 22px;
  flex: 0 0 auto;
  align-items: center;
}

.cluster-notifications__switch input {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
}

.cluster-notifications__switch span {
  width: 38px;
  height: 22px;
  cursor: pointer;
  background: var(--border-strong);
  border-radius: 999px;
  transition: background 160ms ease;
}

.cluster-notifications__switch span::after {
  display: block;
  width: 16px;
  height: 16px;
  margin: 3px;
  content: '';
  background: var(--surface-raised);
  border-radius: 50%;
  box-shadow: 0 1px 3px rgba(0, 0, 0, .18);
  transition: transform 160ms ease;
}

.cluster-notifications__switch input:checked + span {
  background: var(--brand);
}

.cluster-notifications__switch input:checked + span::after {
  transform: translateX(16px);
}

.cluster-notifications__switch input:focus-visible + span {
  outline: 2px solid var(--brand);
  outline-offset: 2px;
}

.cluster-notifications__rules,
.cluster-notifications__event-rules {
  display: grid;
  gap: 8px;
}

.cluster-notifications__rule,
.cluster-notifications__event-rule {
  display: grid;
  align-items: center;
  gap: 10px;
  padding: 9px 10px;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}

.cluster-notifications__rule {
  grid-template-columns: auto minmax(0, 1fr) auto;
}

.cluster-notifications__event-rule {
  grid-template-columns: minmax(0, 1fr) auto;
}

.cluster-notifications__rule > input,
.cluster-notifications__event-rule > input {
  width: 16px;
  height: 16px;
  accent-color: var(--brand);
}

.cluster-notifications__rule strong,
.cluster-notifications__event-rule strong {
  display: block;
  color: var(--text);
  font-size: 13px;
}

.cluster-notifications__rule small,
.cluster-notifications__event-rule small {
  display: block;
  margin-top: 2px;
  color: var(--muted);
  font-size: 12px;
  font-weight: 450;
  line-height: 1.4;
}

.cluster-notifications__threshold {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--muted);
  font-size: 12px;
  white-space: nowrap;
}

.cluster-notifications__threshold input {
  width: 72px;
  height: 34px;
  padding: 0 8px;
  color: var(--text);
  text-align: right;
  background: var(--surface);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
}

.cluster-notifications__threshold input:focus {
  border-color: var(--brand);
  outline: none;
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--brand) 16%, transparent);
}

.cluster-notifications__threshold em {
  min-width: 34px;
  font-style: normal;
}

.cluster-notifications__message {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--success);
}

.cluster-notifications__error {
  color: var(--danger);
}

@media (max-width: 620px) {
  .cluster-notifications__channel-actions .button {
    flex: 1 1 145px;
  }

  .cluster-notifications__rule {
    grid-template-columns: auto minmax(0, 1fr);
  }

  .cluster-notifications__threshold {
    grid-column: 2;
  }
}
</style>
