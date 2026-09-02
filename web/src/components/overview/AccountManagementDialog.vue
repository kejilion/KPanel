<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { KeyRound, RefreshCw, ShieldCheck, UserPlus, Users } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { useToast } from '@/stores/toast'
import type { AccountManagementActionInput, AccountManagementSnapshot, SystemAccount } from '@/types/api'

const props = withDefaults(defineProps<{ open: boolean; readable: boolean; writable: boolean; unavailableReason?: string }>(), {
  unavailableReason: '',
})
const emit = defineEmits<{ close: [] }>()
const toast = useToast()
const snapshot = ref<AccountManagementSnapshot>()
const loading = ref(false)
const refreshing = ref(false)
const running = ref(false)
const error = ref('')
const activeTab = ref<'accounts' | 'ssh' | 'migration'>('accounts')
const search = ref('')
const showSystem = ref(false)
const showCreate = ref(false)
const createUsername = ref('')
const createRole = ref<'standard' | 'administrator' | 'passwordless-admin'>('standard')
const createCredential = ref<'password' | 'key'>('password')
const createSecret = ref('')
const editingUsername = ref('')
const editingAction = ref<'password' | 'key' | 'role' | ''>('')
const editingSecret = ref('')
const editingRole = ref<'standard' | 'administrator' | 'passwordless-admin'>('standard')
const passwordAuthentication = ref(false)
const rootLogin = ref<'enabled' | 'key-only' | 'disabled'>('key-only')
const migrationUsername = ref('operator')
const migrationCredential = ref<'password' | 'key'>('key')
const migrationSecret = ref('')
let controller: AbortController | undefined

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

function validSecretFrame(value: string, minimum: number, maximum: number): boolean {
  if (value.includes('\n') || value.includes('\r') || value.includes('\0')) return false
  const bytes = new TextEncoder().encode(value).length
  return bytes >= minimum && bytes <= maximum
}

const visibleAccounts = computed(() => {
  const query = search.value.trim().toLowerCase()
  return (snapshot.value?.accounts || []).filter((account) => {
    if (!showSystem.value && account.kind === 'system') return false
    if (!query) return true
    return [account.username, account.home, account.shell, account.groups.join(' ')].some((value) => value.toLowerCase().includes(query))
  })
})

const createValid = computed(() => {
  if (!/^[a-z_][a-z0-9_-]{0,31}$/.test(createUsername.value) || createUsername.value === 'root') return false
  if (createCredential.value === 'password') return validSecretFrame(createSecret.value, 8, 256)
  return validSecretFrame(createSecret.value, 1, 4096) && createRole.value !== 'administrator'
})

const migrationValid = computed(() => {
  if (!/^[a-z_][a-z0-9_-]{0,31}$/.test(migrationUsername.value) || migrationUsername.value === 'root') return false
  return migrationCredential.value === 'password'
    ? validSecretFrame(migrationSecret.value, 8, 256)
    : validSecretFrame(migrationSecret.value, 1, 4096)
})

function roleLabel(role: SystemAccount['role']): string {
  return phrase(({ root: 'Root', standard: '普通账户', administrator: '管理员', 'passwordless-admin': '免密管理员' } as const)[role])
}

function passwordLabel(status: SystemAccount['passwordStatus']): string {
  return phrase(({ enabled: '密码可用', locked: '密码已锁定', unset: '未设置密码', unknown: '状态未知' } as const)[status])
}

function rootLoginLabel(value: AccountManagementSnapshot['sshPolicy']['rootLogin']): string {
  return phrase(({ enabled: '密码或密钥', 'key-only': '仅密钥', disabled: '禁止登录', custom: '自定义配置' } as const)[value])
}

async function load(silent = false): Promise<void> {
  if (!props.open || !props.readable) return
  controller?.abort()
  controller = new AbortController()
  if (silent) refreshing.value = true
  else loading.value = true
  error.value = ''
  try {
    snapshot.value = await api.system.accounts(controller.signal)
    passwordAuthentication.value = snapshot.value.sshPolicy.passwordAuthentication
    if (snapshot.value.sshPolicy.rootLogin !== 'custom') rootLogin.value = snapshot.value.sshPolicy.rootLogin
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reason instanceof ApiError ? reason.message : '无法读取系统账户。'
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

async function execute(input: AccountManagementActionInput, confirmation = ''): Promise<boolean> {
  if (!props.writable || running.value || !snapshot.value?.resourceVersion) return false
  if (confirmation && typeof window !== 'undefined' && !window.confirm(translatePhrase(confirmation))) return false
  running.value = true
  try {
    const result = await api.system.accountAction(input)
    toast.success(result.changed ? '账户设置已更新' : '账户设置无需变更', result.message)
    await load(true)
    return true
  } catch (reason) {
    toast.danger('账户操作失败', reason instanceof ApiError ? reason.message : 'Agent 未能完成账户操作。')
    await load(true)
    return false
  } finally {
    running.value = false
  }
}

async function createAccount(): Promise<void> {
  if (!createValid.value || !snapshot.value) return
  const input: AccountManagementActionInput = {
    action: 'create', expectedResourceVersion: snapshot.value.resourceVersion,
    username: createUsername.value, role: createRole.value, credential: createCredential.value, secret: createSecret.value,
  }
  const success = await execute(input)
  createSecret.value = ''
  if (success) {
    createUsername.value = ''
    showCreate.value = false
  }
}

function startAccountAction(account: SystemAccount, action: 'password' | 'key' | 'role'): void {
  editingUsername.value = account.username
  editingAction.value = action
  editingSecret.value = ''
  if (account.role !== 'root') editingRole.value = account.role
}

async function applyAccountAction(): Promise<void> {
  if (!snapshot.value || !editingUsername.value) return
  let input: AccountManagementActionInput
  if (editingAction.value === 'password') {
    if (!validSecretFrame(editingSecret.value, 8, 256)) return
    input = { action: 'set-password', expectedResourceVersion: snapshot.value.resourceVersion, username: editingUsername.value, secret: editingSecret.value }
  } else if (editingAction.value === 'key') {
    if (!validSecretFrame(editingSecret.value, 1, 4096)) return
    input = { action: 'add-key', expectedResourceVersion: snapshot.value.resourceVersion, username: editingUsername.value, secret: editingSecret.value }
  } else if (editingAction.value === 'role') {
    input = { action: 'set-role', expectedResourceVersion: snapshot.value.resourceVersion, username: editingUsername.value, role: editingRole.value }
  } else return
  const success = await execute(input)
  editingSecret.value = ''
  if (success) editingAction.value = ''
}

async function deleteKey(account: SystemAccount, keyId: string): Promise<void> {
  if (!snapshot.value) return
  await execute(
    { action: 'delete-key', expectedResourceVersion: snapshot.value.resourceVersion, username: account.username, keyId },
    `确认删除 ${account.username} 的这把 SSH 公钥吗？使用对应私钥的连接将不再可用。`,
  )
}

async function deleteAccount(account: SystemAccount): Promise<void> {
  if (!snapshot.value || account.username === 'root') return
  await execute(
    { action: 'delete', expectedResourceVersion: snapshot.value.resourceVersion, username: account.username, removeHome: true },
    `确认删除账户 ${account.username}，并删除主目录 ${account.home} 吗？该账户的文件和登录能力将被移除。`,
  )
}

function choosePolicy(mode: 'key' | 'password'): void {
  if (mode === 'key') {
    passwordAuthentication.value = false
    rootLogin.value = 'key-only'
  } else {
    passwordAuthentication.value = true
    rootLogin.value = 'enabled'
  }
}

async function applyPolicy(): Promise<void> {
  if (!snapshot.value) return
  const warning = passwordAuthentication.value
    ? '密码登录将对所有 SSH 账户开放。确认应用该登录策略吗？'
    : '关闭密码登录前，请确认至少有一把可用私钥。现有 SSH 会话通常不会立即断开，确认继续吗？'
  await execute({
    action: 'set-ssh-policy', expectedResourceVersion: snapshot.value.resourceVersion,
    passwordAuthentication: passwordAuthentication.value, rootLogin: rootLogin.value,
  }, warning)
}

async function disableRoot(): Promise<void> {
  if (!snapshot.value) return
  await execute(
    { action: 'disable-root', expectedResourceVersion: snapshot.value.resourceVersion },
    '这会锁定 Root 密码并禁止 Root 通过 SSH 登录。请先确认已有其他管理员账户可以登录，是否继续？',
  )
}

async function migrateRoot(): Promise<void> {
  if (!snapshot.value || !migrationValid.value) return
  const input: AccountManagementActionInput = {
    action: 'create-admin-disable-root', expectedResourceVersion: snapshot.value.resourceVersion,
    username: migrationUsername.value, credential: migrationCredential.value, secret: migrationSecret.value,
  }
  const success = await execute(input, `将创建管理员 ${migrationUsername.value}，随后锁定 Root 密码并禁止 Root SSH 登录。请确认凭据已经妥善保存，是否继续？`)
  migrationSecret.value = ''
  if (success) activeTab.value = 'accounts'
}

watch(createCredential, (credential) => {
  if (credential === 'key' && createRole.value === 'administrator') createRole.value = 'passwordless-admin'
})

watch(() => [props.open, props.readable] as const, ([open, readable]) => {
  if (open && readable) void load()
  else controller?.abort()
}, { immediate: true })

onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <ModalDialog
    :open="open"
    :title="phrase('账户管理')"
    :description="phrase('管理 Linux 账户、登录凭据和 SSH 登录策略，所有写入由 kejilion.sh 完成。')"
    size="wide"
    @close="emit('close')"
  >
    <div class="account-manager">
      <div v-if="!readable" class="inline-alert inline-alert--warning">
        {{ phrase(unavailableReason || '当前 Agent 的账户管理适配器未就绪。') }}
      </div>
      <LoadingState v-else-if="loading && !snapshot" :rows="6" />
      <ErrorState v-else-if="error && !snapshot" :message="phrase(error)" @retry="load()" />
      <template v-else-if="snapshot">
        <header class="account-manager__summary">
          <span><Users :size="16" /> {{ phrase(`${snapshot.total} 个系统账户`) }}</span>
          <span>{{ phrase(`密码登录：${phrase(snapshot.sshPolicy.passwordAuthentication ? '允许' : '关闭')}`) }}</span>
          <span>{{ phrase(`Root：${rootLoginLabel(snapshot.sshPolicy.rootLogin)}`) }}</span>
          <button class="icon-button" type="button" :disabled="refreshing || running" :title="phrase('刷新账户')" :aria-label="phrase('刷新账户')" @click="load(true)">
            <RefreshCw :size="16" :class="{ spin: refreshing }" />
          </button>
        </header>

        <nav class="account-manager__tabs" :aria-label="phrase('账户管理区域')">
          <button type="button" :class="{ 'is-active': activeTab === 'accounts' }" @click="activeTab = 'accounts'">{{ phrase('账户') }}</button>
          <button type="button" :class="{ 'is-active': activeTab === 'ssh' }" @click="activeTab = 'ssh'">{{ phrase('SSH 登录策略') }}</button>
          <button type="button" :class="{ 'is-active': activeTab === 'migration' }" @click="activeTab = 'migration'">{{ phrase('禁用 Root 向导') }}</button>
        </nav>

        <div v-if="!writable" class="inline-alert inline-alert--warning">{{ phrase('当前 Agent 仅支持查看，账户写入适配器未就绪。') }}</div>

        <section v-if="activeTab === 'accounts'" class="account-manager__section">
          <div class="account-manager__toolbar">
            <input v-model="search" type="search" :placeholder="phrase('搜索用户名、主目录或用户组')" />
            <label class="account-manager__check"><input v-model="showSystem" type="checkbox" /> {{ phrase('显示系统账户') }}</label>
            <button class="button button--primary button--small" type="button" :disabled="!writable || running" @click="showCreate = !showCreate">
              <UserPlus :size="16" /> {{ phrase('创建账户') }}
            </button>
          </div>

          <form v-if="showCreate" class="account-manager__form" @submit.prevent="createAccount">
            <h3>{{ phrase('创建新账户') }}</h3>
            <label class="field"><span>{{ phrase('用户名') }}</span><input v-model.trim="createUsername" maxlength="32" autocomplete="off" placeholder="operator" /></label>
            <label class="field"><span>{{ phrase('账户角色') }}</span><select v-model="createRole"><option value="standard">{{ phrase('普通账户') }}</option><option value="administrator">{{ phrase('管理员（sudo 需要密码）') }}</option><option value="passwordless-admin">{{ phrase('免密管理员（NOPASSWD sudo）') }}</option></select></label>
            <label class="field"><span>{{ phrase('初始凭据') }}</span><select v-model="createCredential"><option value="password">{{ phrase('账户密码') }}</option><option value="key">{{ phrase('SSH 公钥') }}</option></select></label>
            <label class="field account-manager__secret"><span>{{ phrase(createCredential === 'password' ? '初始密码' : 'SSH 公钥') }}</span><textarea v-if="createCredential === 'key'" v-model="createSecret" rows="3" maxlength="4096" placeholder="ssh-ed25519 AAAA... laptop"></textarea><input v-else v-model="createSecret" type="password" maxlength="256" autocomplete="new-password" :placeholder="phrase('至少 8 个字符')" /></label>
            <small v-if="createCredential === 'key'">{{ phrase('密钥账户的系统密码会保持锁定；如需 sudo，请选择免密管理员。') }}</small>
            <div class="account-manager__actions"><button class="button button--secondary" type="button" @click="showCreate = false; createSecret = ''">{{ phrase('取消') }}</button><button class="button button--primary" type="submit" :disabled="!createValid || running">{{ phrase('创建并验证') }}</button></div>
          </form>

          <div v-if="snapshot.truncated" class="inline-alert inline-alert--warning">{{ phrase('账户数量超过显示上限，仅展示前 256 个账户。') }}</div>
          <div class="account-list">
            <article v-for="account in visibleAccounts" :key="account.username" class="account-card">
              <header>
                <span class="account-card__identity"><strong>{{ account.username }}</strong><small>UID {{ account.uid }} · {{ account.home }}</small></span>
                <span class="account-card__badges"><span>{{ roleLabel(account.role) }}</span><span>{{ passwordLabel(account.passwordStatus) }}</span><span>{{ phrase(`${account.sshKeys.length} 把密钥`) }}</span></span>
              </header>
              <p>{{ account.shell }}<template v-if="account.groups.length"> · {{ account.groups.join(', ') }}</template></p>
              <div class="account-card__buttons">
                <button type="button" :disabled="!writable || running" @click="startAccountAction(account, 'password')">{{ phrase('修改密码') }}</button>
                <button type="button" :disabled="!writable || running" @click="startAccountAction(account, 'key')">{{ phrase('添加公钥') }}</button>
                <button v-if="account.username !== 'root'" type="button" :disabled="!writable || running" @click="startAccountAction(account, 'role')">{{ phrase('调整角色') }}</button>
                <button v-if="account.username !== 'root'" class="is-danger" type="button" :disabled="!writable || running" @click="deleteAccount(account)">{{ phrase('删除账户') }}</button>
              </div>
              <ul v-if="account.sshKeys.length" class="account-card__keys">
                <li v-for="key in account.sshKeys" :key="key.id"><span><KeyRound :size="14" /> {{ key.type }} · {{ key.fingerprint }}<small v-if="key.comment">{{ key.comment }}</small></span><button type="button" :disabled="!writable || running" @click="deleteKey(account, key.id)">{{ phrase('删除') }}</button></li>
              </ul>
              <form v-if="editingUsername === account.username && editingAction" class="account-card__editor" @submit.prevent="applyAccountAction">
                <label v-if="editingAction === 'password'" class="field"><span>{{ phrase('新密码') }}</span><input v-model="editingSecret" type="password" maxlength="256" autocomplete="new-password" :placeholder="phrase('至少 8 个字符')" /></label>
                <label v-else-if="editingAction === 'key'" class="field"><span>{{ phrase('SSH 公钥') }}</span><textarea v-model="editingSecret" rows="3" maxlength="4096" placeholder="ssh-ed25519 AAAA... laptop"></textarea></label>
                <label v-else class="field"><span>{{ phrase('账户角色') }}</span><select v-model="editingRole"><option value="standard">{{ phrase('普通账户') }}</option><option value="administrator">{{ phrase('管理员（sudo 需要密码）') }}</option><option value="passwordless-admin">{{ phrase('免密管理员') }}</option></select></label>
                <div class="account-manager__actions"><button class="button button--secondary button--small" type="button" @click="editingAction = ''; editingSecret = ''">{{ phrase('取消') }}</button><button class="button button--primary button--small" type="submit" :disabled="running">{{ phrase('保存') }}</button></div>
              </form>
            </article>
          </div>
        </section>

        <section v-else-if="activeTab === 'ssh'" class="account-manager__section">
          <div class="account-policy-grid">
            <button type="button" :class="{ 'is-selected': !passwordAuthentication && rootLogin === 'key-only' }" @click="choosePolicy('key')"><KeyRound :size="20" /><strong>{{ phrase('密钥登录模式') }}</strong><small>{{ phrase('关闭全局密码认证；Root 仅允许密钥。推荐用于公网服务器。') }}</small></button>
            <button type="button" :class="{ 'is-selected': passwordAuthentication && rootLogin === 'enabled' }" @click="choosePolicy('password')"><Users :size="20" /><strong>{{ phrase('密码兼容模式') }}</strong><small>{{ phrase('密码和密钥都可用；Root 也允许密码登录。') }}</small></button>
          </div>
          <div class="account-manager__form">
            <label class="field"><span>{{ phrase('普通账户密码登录') }}</span><select v-model="passwordAuthentication"><option :value="false">{{ phrase('关闭，只允许 SSH 公钥') }}</option><option :value="true">{{ phrase('允许密码和 SSH 公钥') }}</option></select></label>
            <label class="field"><span>{{ phrase('Root SSH 登录') }}</span><select v-model="rootLogin"><option value="disabled">{{ phrase('禁止 Root 登录') }}</option><option value="key-only">{{ phrase('Root 仅密钥登录') }}</option><option value="enabled">{{ phrase('Root 密码或密钥登录') }}</option></select></label>
            <div class="inline-alert inline-alert--info">{{ phrase('策略通过独立 sshd 配置片段应用，先执行') }} <code>sshd -t</code>{{ phrase('，验证失败会恢复原配置。') }}</div>
            <div class="account-manager__actions"><button class="button button--primary" type="button" :disabled="!writable || running" @click="applyPolicy"><ShieldCheck :size="16" /> {{ phrase('应用登录策略') }}</button><button class="button button--danger" type="button" :disabled="!writable || running" @click="disableRoot">{{ phrase('锁定并禁用 Root') }}</button></div>
          </div>
        </section>

        <section v-else class="account-manager__section">
          <div class="inline-alert inline-alert--warning">{{ phrase('该向导会先创建可登录管理员，再锁定 Root 密码并禁止 Root SSH 登录；任何一步失败都会尝试恢复账户数据库和 SSH 配置。') }}</div>
          <form class="account-manager__form" @submit.prevent="migrateRoot">
            <h3>{{ phrase('创建替代管理员并禁用 Root') }}</h3>
            <label class="field"><span>{{ phrase('新管理员用户名') }}</span><input v-model.trim="migrationUsername" maxlength="32" autocomplete="off" placeholder="operator" /></label>
            <label class="field"><span>{{ phrase('登录凭据') }}</span><select v-model="migrationCredential"><option value="key">{{ phrase('SSH 公钥（推荐，自动使用免密 sudo）') }}</option><option value="password">{{ phrase('账户密码（sudo 需要密码）') }}</option></select></label>
            <label class="field account-manager__secret"><span>{{ phrase(migrationCredential === 'key' ? 'SSH 公钥' : '账户密码') }}</span><textarea v-if="migrationCredential === 'key'" v-model="migrationSecret" rows="4" maxlength="4096" placeholder="ssh-ed25519 AAAA... laptop"></textarea><input v-else v-model="migrationSecret" type="password" maxlength="256" autocomplete="new-password" :placeholder="phrase('至少 8 个字符')" /></label>
            <small>{{ phrase('密钥模式会同时关闭全局密码认证；密码模式会保留普通账户密码认证，但 Root 仍被禁用。') }}</small>
            <div class="account-manager__actions"><button class="button button--danger" type="submit" :disabled="!writable || running || !migrationValid">{{ phrase('创建管理员并禁用 Root') }}</button></div>
          </form>
        </section>
      </template>
    </div>
  </ModalDialog>
</template>

<style scoped>
.account-manager { display: grid; gap: 16px; }
.account-manager__summary { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; padding: 12px 14px; border: 1px solid var(--border-subtle); border-radius: 12px; background: var(--surface-muted); }
.account-manager__summary > span { display: inline-flex; align-items: center; gap: 6px; font-size: 13px; }
.account-manager__summary .icon-button { margin-left: auto; }
.account-manager__tabs { display: flex; gap: 6px; border-bottom: 1px solid var(--border-subtle); }
.account-manager__tabs button { border: 0; border-bottom: 2px solid transparent; padding: 10px 12px; background: transparent; color: var(--text-secondary); cursor: pointer; }
.account-manager__tabs button.is-active { border-color: var(--accent); color: var(--text-primary); font-weight: 600; }
.account-manager__section { display: grid; gap: 14px; }
.account-manager__toolbar { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.account-manager__toolbar > input { flex: 1 1 220px; }
.account-manager__check { display: inline-flex; align-items: center; gap: 6px; font-size: 13px; }
.account-manager__form { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; padding: 16px; border: 1px solid var(--border-subtle); border-radius: 12px; background: var(--surface-muted); }
.account-manager__form h3, .account-manager__form > small, .account-manager__form .inline-alert, .account-manager__secret, .account-manager__actions { grid-column: 1 / -1; }
.account-manager__actions { display: flex; justify-content: flex-end; gap: 8px; }
.account-list { display: grid; gap: 10px; }
.account-card { padding: 14px; border: 1px solid var(--border-subtle); border-radius: 12px; }
.account-card > header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.account-card__identity { display: grid; gap: 3px; }
.account-card__badges { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 6px; }
.account-card__badges > span { padding: 3px 7px; border-radius: 999px; background: var(--surface-muted); font-size: 11px; }
.account-card > p { margin: 8px 0; color: var(--text-secondary); font-size: 12px; }
.account-card__buttons { display: flex; flex-wrap: wrap; gap: 6px; }
.account-card__buttons button, .account-card__keys button { border: 1px solid var(--border-subtle); border-radius: 7px; padding: 5px 8px; background: transparent; color: var(--text-secondary); cursor: pointer; }
.account-card__buttons button.is-danger { color: var(--danger); }
.account-card__keys { display: grid; gap: 6px; margin: 12px 0 0; padding: 0; list-style: none; }
.account-card__keys li { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 8px 10px; border-radius: 8px; background: var(--surface-muted); font-size: 12px; }
.account-card__keys span { display: flex; align-items: center; gap: 5px; min-width: 0; overflow-wrap: anywhere; }
.account-card__keys small { color: var(--text-secondary); }
.account-card__editor { display: grid; gap: 10px; margin-top: 12px; padding-top: 12px; border-top: 1px solid var(--border-subtle); }
.account-policy-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
.account-policy-grid button { display: grid; gap: 7px; text-align: left; padding: 16px; border: 1px solid var(--border-subtle); border-radius: 12px; background: transparent; color: inherit; cursor: pointer; }
.account-policy-grid button.is-selected { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 8%, transparent); }
.account-policy-grid small { color: var(--text-secondary); }
@media (max-width: 720px) { .account-manager__form, .account-policy-grid { grid-template-columns: 1fr; } .account-manager__form > * { grid-column: 1; } .account-card > header { display: grid; } .account-card__badges { justify-content: flex-start; } }
</style>
