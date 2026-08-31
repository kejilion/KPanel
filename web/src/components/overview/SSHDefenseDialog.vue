<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RefreshCw, ShieldCheck, ShieldOff, Trash2, Unlock } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { useToast } from '@/stores/toast'
import type { SSHDefenseActionInput, SSHDefenseSnapshot } from '@/types/api'

type SSHDefenseActionPayload =
  | { action: 'enable' | 'disable' | 'uninstall' | 'unban-all' }
  | { action: 'set-profile'; profile: 'mild' | 'standard' | 'strict' }
  | { action: 'add-trusted' | 'remove-trusted' | 'unban'; address: string }

const props = withDefaults(defineProps<{ open: boolean; readable: boolean; writable: boolean; unavailableReason?: string }>(), {
  unavailableReason: '',
})
const emit = defineEmits<{ close: [] }>()
const toast = useToast()
const snapshot = ref<SSHDefenseSnapshot>()
const loading = ref(false)
const refreshing = ref(false)
const running = ref(false)
const error = ref('')
const banSearch = ref('')
const trustedAddress = ref('')
let controller: AbortController | undefined
let pollTimer: number | undefined

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const profiles = [
  { id: 'mild', title: '温和', detail: '10 分钟内失败 8 次，封禁 10 分钟' },
  { id: 'standard', title: '标准', detail: '10 分钟内失败 5 次，封禁 1 小时' },
  { id: 'strict', title: '严格', detail: '10 分钟内失败 3 次，封禁 12 小时' },
] as const

const maintenanceRunning = computed(() => snapshot.value?.maintenance.state === 'running' && snapshot.value.maintenance.action === 'ssh-defense')
const filteredBans = computed(() => {
  const query = banSearch.value.trim().toLowerCase()
  return (snapshot.value?.bannedIps || []).filter((address) => !query || address.toLowerCase().includes(query))
})
const recentEvents = computed(() => [...(snapshot.value?.recentEvents || [])].reverse())
const trustedAddressValid = computed(() => {
  const value = trustedAddress.value.trim()
  return value.length > 0 && value.length <= 80 && /^[0-9a-fA-F:.]+(?:\/\d{1,3})?$/.test(value)
})

function clearPoll(): void {
  if (pollTimer !== undefined) window.clearTimeout(pollTimer)
  pollTimer = undefined
}

function schedulePoll(): void {
  clearPoll()
  if (!props.open || !maintenanceRunning.value) return
  pollTimer = window.setTimeout(async () => {
    await load(true)
    schedulePoll()
  }, 2500)
}

async function load(silent = false): Promise<void> {
  if (!props.open || !props.readable) return
  controller?.abort()
  controller = new AbortController()
  if (silent) refreshing.value = true
  else loading.value = true
  error.value = ''
  try {
    snapshot.value = await api.system.sshDefense(controller.signal)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reason instanceof ApiError ? reason.message : '无法读取 SSH 防御状态。'
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

async function execute(input: SSHDefenseActionPayload, confirmation = ''): Promise<boolean> {
  if (!props.writable || running.value || !snapshot.value?.resourceVersion || maintenanceRunning.value) return false
  if (confirmation && typeof window !== 'undefined' && !window.confirm(translatePhrase(confirmation))) return false
  running.value = true
  try {
    const result = await api.system.sshDefenseAction({ ...input, expectedResourceVersion: snapshot.value.resourceVersion } as SSHDefenseActionInput)
    toast.success(result.status === 'accepted' ? 'SSH 防御任务已提交' : result.changed ? 'SSH 防御已更新' : 'SSH 防御无需变更', result.message)
    await load(true)
    schedulePoll()
    return true
  } catch (reason) {
    toast.danger('SSH 防御操作失败', reason instanceof ApiError ? reason.message : 'Agent 未能完成 SSH 防御操作。')
    await load(true)
    return false
  } finally {
    running.value = false
  }
}

async function toggleDefense(): Promise<void> {
  if (!snapshot.value) return
  if (snapshot.value.enabled) {
    await execute({ action: 'disable' }, '确认停用 SSH 防御吗？现有配置和封禁记录会保留。')
  } else {
    await execute({ action: 'enable' })
  }
}

async function setProfile(profile: 'mild' | 'standard' | 'strict'): Promise<void> {
  await execute({ action: 'set-profile', profile })
}

async function unban(address: string): Promise<void> {
  await execute({ action: 'unban', address })
}

async function unbanAll(): Promise<void> {
  await execute({ action: 'unban-all' }, '确认解除当前全部 SSH 封禁吗？攻击来源可能会立即重新尝试登录。')
}

async function addTrusted(): Promise<void> {
  const address = trustedAddress.value.trim()
  if (!trustedAddressValid.value) return
  if (await execute({ action: 'add-trusted', address })) trustedAddress.value = ''
}

async function removeTrusted(address: string): Promise<void> {
  await execute({ action: 'remove-trusted', address }, `确认从信任列表移除 ${address} 吗？`)
}

async function uninstall(): Promise<void> {
  await execute({ action: 'uninstall' }, '确认卸载 Fail2Ban 吗？SSH 防御配置和当前封禁将被移除；之后可重新安装。')
}

function eventActionLabel(action: 'found' | 'ban' | 'unban'): string {
  return phrase({ found: '登录失败', ban: '已封禁', unban: '已解封' }[action])
}

watch(() => [props.open, props.readable] as const, ([open, readable]) => {
  if (open && readable) {
    void load().then(schedulePoll)
  } else {
    controller?.abort()
    clearPoll()
  }
}, { immediate: true })

onBeforeUnmount(() => {
  controller?.abort()
  clearPoll()
})
</script>

<template>
  <ModalDialog
    :open="open"
    :title="phrase('SSH 防御')"
    :description="phrase('自动封禁反复尝试 SSH 登录的来源；由 kejilion.sh 管理 Fail2Ban。')"
    size="wide"
    @close="emit('close')"
  >
    <div class="ssh-defense-manager">
      <div v-if="!readable" class="inline-alert inline-alert--warning">
        {{ phrase(unavailableReason || '当前 Agent 的 SSH 防御适配器未就绪。') }}
      </div>
      <LoadingState v-else-if="loading && !snapshot" :rows="5" />
      <ErrorState v-else-if="error && !snapshot" :message="phrase(error)" @retry="load()" />
      <template v-else-if="snapshot">
        <header class="ssh-defense-manager__summary">
          <span class="ssh-defense-manager__state" :class="{ 'is-enabled': snapshot.enabled }">
            <ShieldCheck v-if="snapshot.enabled" :size="18" />
            <ShieldOff v-else :size="18" />
            <strong>{{ phrase(snapshot.enabled ? '防御已开启' : snapshot.installed ? '防御已停用' : '尚未安装') }}</strong>
          </span>
          <span>{{ phrase('当前封禁') }} <strong>{{ snapshot.currentBanned }}</strong></span>
          <span>{{ phrase('累计拦截') }} <strong>{{ snapshot.totalBanned }}</strong></span>
          <button class="icon-button" type="button" :disabled="refreshing || running" :title="phrase('刷新 SSH 防御')" :aria-label="phrase('刷新 SSH 防御')" @click="load(true)">
            <RefreshCw :size="16" :class="{ spin: refreshing }" />
          </button>
        </header>

        <div v-if="maintenanceRunning" class="inline-alert inline-alert--info">
          {{ phrase(snapshot.maintenance.message || 'SSH 防御任务正在后台执行…') }}（{{ snapshot.maintenance.progress }}%）
        </div>
        <div v-if="!writable" class="inline-alert inline-alert--warning">{{ phrase('当前 Agent 仅支持查看，写入适配器未就绪。') }}</div>

        <section class="ssh-defense-manager__section ssh-defense-manager__primary">
          <div>
            <h3>{{ phrase('防御开关') }}</h3>
            <p>{{ phrase(snapshot.enabled ? 'Fail2Ban 正在监控 SSH 登录失败。停用后保留配置，随时可以重新开启。' : '开启后自动安装并启用 Fail2Ban，不需要手动配置规则。') }}</p>
          </div>
          <button class="button" :class="snapshot.enabled ? 'button--secondary' : 'button--primary'" type="button" :disabled="!writable || running || maintenanceRunning" @click="toggleDefense">
            {{ phrase(running || maintenanceRunning ? '正在处理…' : snapshot.enabled ? '停用防御' : '开启防御') }}
          </button>
        </section>

        <section v-if="snapshot.enabled" class="ssh-defense-manager__section">
          <div class="ssh-defense-manager__heading">
            <div><h3>{{ phrase('防御强度') }}</h3><p>{{ phrase('推荐“标准”，调整后立即验证并重载配置。') }}</p></div>
            <span v-if="snapshot.profile === 'custom'" class="ssh-defense-manager__custom">{{ phrase('当前为自定义规则') }}</span>
          </div>
          <div class="ssh-defense-manager__profiles">
            <button v-for="profile in profiles" :key="profile.id" type="button" :class="{ 'is-active': snapshot.profile === profile.id }" :disabled="!writable || running" @click="setProfile(profile.id)">
              <strong>{{ phrase(profile.title) }}</strong><small>{{ phrase(profile.detail) }}</small>
            </button>
          </div>
        </section>

        <section v-if="snapshot.installed" class="ssh-defense-manager__section">
          <div class="ssh-defense-manager__heading">
            <div><h3>{{ phrase('已封禁 IP') }}</h3><p>{{ phrase('按 IP 搜索并单独解封。') }}</p></div>
            <button class="button button--secondary button--small" type="button" :disabled="!writable || running || snapshot.currentBanned === 0" @click="unbanAll">{{ phrase('全部解封') }}</button>
          </div>
          <input v-model.trim="banSearch" class="ssh-defense-manager__search" type="search" :placeholder="phrase('搜索 IP 地址')" />
          <div v-if="filteredBans.length" class="ssh-defense-manager__list">
            <div v-for="address in filteredBans" :key="address"><code>{{ address }}</code><button class="button button--secondary button--small" type="button" :disabled="!writable || running" @click="unban(address)"><Unlock :size="14" /> {{ phrase('解封') }}</button></div>
          </div>
          <p v-else class="ssh-defense-manager__empty">{{ phrase(snapshot.currentBanned ? '没有匹配的 IP。' : '当前没有被封禁的 IP。') }}</p>
          <small v-if="snapshot.bansTruncated">{{ phrase('封禁数量较多，仅显示前 256 个 IP。') }}</small>
        </section>

        <section v-if="snapshot.enabled" class="ssh-defense-manager__section">
          <div class="ssh-defense-manager__heading"><div><h3>{{ phrase('信任地址') }}</h3><p>{{ phrase('可信办公网或固定出口不会被自动封禁。') }}</p></div></div>
          <form class="ssh-defense-manager__trusted-form" @submit.prevent="addTrusted">
            <input v-model.trim="trustedAddress" maxlength="80" :placeholder="phrase('例如 203.0.113.10 或 203.0.113.0/24')" />
            <button class="button button--secondary" type="submit" :disabled="!writable || running || !trustedAddressValid">{{ phrase('添加') }}</button>
          </form>
          <div class="ssh-defense-manager__chips">
            <span v-for="address in snapshot.trustedAddresses" :key="address"><code>{{ address }}</code><button type="button" :disabled="!writable || running" :aria-label="phrase(`移除信任地址 ${address}`)" @click="removeTrusted(address)">×</button></span>
          </div>
        </section>

        <section v-if="snapshot.installed" class="ssh-defense-manager__section">
          <h3>{{ phrase('最近事件') }}</h3>
          <div v-if="recentEvents.length" class="ssh-defense-manager__events">
            <div v-for="(event, index) in recentEvents" :key="`${event.occurredAt}-${event.address}-${index}`">
              <time>{{ event.occurredAt }}</time><span :class="`is-${event.action}`">{{ eventActionLabel(event.action) }}</span><code>{{ event.address }}</code>
            </div>
          </div>
          <p v-else class="ssh-defense-manager__empty">{{ phrase('最近没有 SSH 防御事件。') }}</p>
        </section>

        <section v-if="snapshot.installed" class="ssh-defense-manager__danger">
          <div><strong>{{ phrase('卸载 Fail2Ban') }}</strong><small>{{ phrase('仅在不再需要 SSH 防御时使用。') }}</small></div>
          <button class="button button--secondary button--small" type="button" :disabled="!writable || running || maintenanceRunning" @click="uninstall"><Trash2 :size="14" /> {{ phrase('卸载') }}</button>
        </section>
      </template>
    </div>
  </ModalDialog>
</template>

<style scoped>
.ssh-defense-manager { display: grid; gap: 14px; }
.ssh-defense-manager__summary, .ssh-defense-manager__heading, .ssh-defense-manager__primary, .ssh-defense-manager__danger { display: flex; align-items: center; justify-content: space-between; gap: 14px; }
.ssh-defense-manager__summary { min-height: 58px; padding: 12px 14px; border: 1px solid var(--border); border-radius: 12px; background: var(--surface-subtle); flex-wrap: wrap; }
.ssh-defense-manager__state { display: inline-flex; align-items: center; gap: 8px; color: var(--muted); }
.ssh-defense-manager__state.is-enabled { color: var(--brand); }
.ssh-defense-manager__section { display: grid; gap: 12px; padding: 16px; border: 1px solid var(--border); border-radius: 12px; background: var(--surface-subtle); }
.ssh-defense-manager__section h3, .ssh-defense-manager__section p { margin: 0; }
.ssh-defense-manager__section p, .ssh-defense-manager__danger small { color: var(--muted); }
.ssh-defense-manager__profiles { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }
.ssh-defense-manager__profiles button { display: grid; gap: 5px; padding: 12px; text-align: left; color: inherit; border: 1px solid var(--border); border-radius: 10px; background: var(--surface); cursor: pointer; }
.ssh-defense-manager__profiles button.is-active { border-color: var(--brand); box-shadow: 0 0 0 1px var(--brand); }
.ssh-defense-manager__profiles small, .ssh-defense-manager__empty { color: var(--muted); }
.ssh-defense-manager__custom { color: var(--amber); font-size: 12px; }
.ssh-defense-manager__search, .ssh-defense-manager__trusted-form input { width: 100%; min-height: 40px; padding: 0 12px; color: inherit; border: 1px solid var(--border); border-radius: 9px; background: var(--surface); }
.ssh-defense-manager__list, .ssh-defense-manager__events { display: grid; max-height: 230px; overflow: auto; }
.ssh-defense-manager__list > div, .ssh-defense-manager__events > div { display: grid; align-items: center; gap: 10px; min-height: 42px; border-bottom: 1px solid var(--border); }
.ssh-defense-manager__list > div { grid-template-columns: 1fr auto; }
.ssh-defense-manager__events > div { grid-template-columns: minmax(160px, auto) 70px 1fr; }
.ssh-defense-manager__events time { color: var(--muted); font-size: 12px; }
.ssh-defense-manager__events span.is-ban { color: var(--danger); }
.ssh-defense-manager__events span.is-unban { color: var(--brand); }
.ssh-defense-manager__events span.is-found { color: var(--amber); }
.ssh-defense-manager__trusted-form { display: grid; grid-template-columns: 1fr auto; gap: 10px; }
.ssh-defense-manager__chips { display: flex; flex-wrap: wrap; gap: 8px; }
.ssh-defense-manager__chips span { display: inline-flex; align-items: center; gap: 6px; padding: 6px 8px; border: 1px solid var(--border); border-radius: 999px; }
.ssh-defense-manager__chips button { border: 0; color: var(--muted); background: transparent; cursor: pointer; }
.ssh-defense-manager__danger { padding: 4px 2px; }
.ssh-defense-manager__danger div { display: grid; gap: 3px; }
@media (max-width: 720px) {
  .ssh-defense-manager__profiles { grid-template-columns: 1fr; }
  .ssh-defense-manager__primary, .ssh-defense-manager__danger { align-items: stretch; flex-direction: column; }
  .ssh-defense-manager__events > div { grid-template-columns: 1fr auto; padding: 8px 0; }
  .ssh-defense-manager__events code { grid-column: 1 / -1; }
}
</style>
