<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Plug, RefreshCw } from '@lucide/vue'
import { api } from '@/lib/api'
import { mcpAccess, mcpDomains as domainOptions, type MCPSettings } from '@/lib/mcp'
import { mcpClientConfig, mcpClients, type MCPClientKind } from '@/lib/mcp-clients'
import MCPOperations from './MCPOperations.vue'
import MCPClusterGrants from './MCPClusterGrants.vue'
import MCPOAuthConsent from './MCPOAuthConsent.vue'
import { formatDateTime } from '@/lib/format'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'

function phrase(value: string) { phraseCatalogVersion.value; return translatePhrase(value) }
const settings = ref<MCPSettings>()
const hosts = ref<Array<{ id: string; name: string; isLocal: boolean }>>([])
const selected = ref<string[]>(['local'])
const name = ref('')
const days = ref(30)
const accessMode = ref<'inspect' | 'read' | 'manage'>('inspect')
const domains = ref(['system', 'docker', 'sites', 'apps', 'diagnostics', 'backups'])
const fileRoots = ref('')
const autoApprove = ref(false)
const oauthRequestId = new URLSearchParams(window.location.search).get('mcpAuthorize') || ''
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const token = ref('')
const credentialName = ref('')
const credentialID = ref('')
const clientKind = ref<MCPClientKind>('generic')
const clientLocation = computed(() => mcpClients.find(client => client.id === clientKind.value)?.location)
let disposed = false
const config = computed(() => settings.value?.endpoint && token.value ? mcpClientConfig(clientKind.value, settings.value.endpoint, token.value) : '')
const canCreate = computed(() => settings.value?.access.enabled && settings.value.access.available && settings.value.transportReady && !busy.value && !token.value && name.value.trim().length > 0 && selected.value.length > 0 && settings.value.access.clients.length < settings.value.maxClients && (accessMode.value === 'inspect' || (domains.value.length > 0 && (!domains.value.includes('files') || !!fileRoots.value.trim()))))

function clearCredential() { token.value = ''; credentialName.value = ''; credentialID.value = '' }
async function refresh() {
  loading.value = true; error.value = ''
  try {
    const [value, list] = await Promise.all([mcpAccess.get(), api.cluster.hosts()])
    if (disposed) return
    settings.value = value; hosts.value = list.items
    selected.value = selected.value.filter(id => list.items.some(host => host.id === id))
    if (!value.access.enabled || !value.access.clients.some(c => c.id === credentialID.value)) clearCredential()
  } catch { if (!disposed) error.value = '无法加载 MCP 接入，请重试。' }
  finally { if (!disposed) loading.value = false }
}
async function toggle() {
  if (!settings.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const value = await mcpAccess.enable(!settings.value.access.enabled, settings.value.access.resourceVersion)
    if (disposed) return
    settings.value = value
    if (!value.access.enabled) clearCredential()
    notice.value = value.access.enabled ? 'MCP 已启用，请创建客户端。' : 'MCP 已关闭，将拒绝新的客户端和集群管理请求；已受理任务请在任务页面核对结果。'
  } catch { if (!disposed) error.value = '操作未完成，请刷新状态后重试。' }
  finally { if (!disposed) busy.value = false }
}
async function create() {
  if (!settings.value || !canCreate.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const policy = accessMode.value === 'inspect' ? {} : { domains: [...domains.value], write: accessMode.value === 'manage', autoApprove: accessMode.value === 'manage' && autoApprove.value, fileRoots: domains.value.includes('files') ? fileRoots.value.split('\n').map(root => root.trim()).filter(Boolean) : [] }
    const result = await mcpAccess.create({ name: name.value.trim(), hostIds: [...selected.value], expiresInDays: days.value, expectedResourceVersion: settings.value.access.resourceVersion, ...policy })
    if (disposed) return
    settings.value = result.settings; token.value = result.token; credentialName.value = result.client.name; credentialID.value = result.client.id; name.value = ''
  } catch { if (!disposed) error.value = '创建未完成，请刷新列表核对；凭据不会重复显示，需要时撤销后重新创建。' }
  finally { if (!disposed) busy.value = false }
}
async function revoke(id: string) {
  if (!settings.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const value = await mcpAccess.revoke(id, settings.value.access.resourceVersion)
    if (disposed) return
    settings.value = value
    if (id === credentialID.value) clearCredential()
    notice.value = '客户端已撤销，旧凭据立即失效。'
  } catch { if (!disposed) error.value = '操作未完成，请刷新状态后重试。' }
  finally { if (!disposed) busy.value = false }
}
async function copy() {
  try { await navigator.clipboard.writeText(config.value); if (!disposed) notice.value = '连接配置已复制。' }
  catch { if (!disposed) error.value = '无法访问剪贴板，请手动复制连接配置。' }
}
async function test() {
  busy.value = true; error.value = ''; notice.value = ''
  try { await mcpAccess.test(token.value); if (!disposed) notice.value = '连接成功，客户端授权已验证。' }
  catch { if (!disposed) error.value = '连接失败，请核对 HTTPS、反向代理路由和凭据有效期。' }
  finally { if (!disposed) busy.value = false }
}
function hostLabel(id: string) { return hosts.value.find(h => h.id === id)?.name || phrase('已移除的主机') }
onMounted(refresh)
onBeforeUnmount(() => { disposed = true; clearCredential() })
</script>

<template>
  <section id="mcp-access" class="settings-section panel-card mcp-access" aria-labelledby="mcp-title">
    <header class="settings-section__header">
      <span class="section-icon"><Plug :size="20" aria-hidden="true" /></span>
      <div><h2 id="mcp-title">MCP 接入</h2><p>让外部 AI 客户端按授权查询和管理主机，无需配置面板内置 AI。</p></div>
    </header>
    <div v-if="loading" role="status">正在加载 MCP 接入…</div>
    <div v-else-if="settings" class="mcp-content">
      <MCPOAuthConsent v-if="oauthRequestId" :request-id="oauthRequestId" :hosts="hosts" :version="settings.access.resourceVersion" />
      <div class="mcp-toolbar">
        <div><strong>{{ settings.access.enabled ? '已启用' : '未启用' }}</strong><p class="mcp-note">默认仅提供巡检权限。管理权限按主机和业务范围单独授予，写操作默认需要审批。</p></div>
        <button class="button button--secondary" type="button" :disabled="busy || !settings.access.available || (!settings.access.enabled && !settings.transportReady)" @click="toggle">{{ settings.access.enabled ? '关闭 MCP' : '启用 MCP' }}</button>
      </div>
      <p v-if="!settings.access.available" role="alert">MCP 授权存储不可用，请检查数据目录后重启面板。</p>
      <p v-if="!settings.transportReady" role="status">请通过 HTTPS 或本机回环地址访问面板，再创建 MCP 客户端。</p>
      <p v-if="settings.access.enabled && settings.endpoint" class="mcp-endpoint"><span>服务地址</span><code>{{ settings.endpoint }}</code></p>
      <p v-if="settings.access.enabled" class="mcp-note">支持 OAuth 的客户端可直接填写服务地址，在浏览器中选择主机和权限完成授权；无需复制访问凭据。</p>
      <form v-if="settings.access.enabled" class="mcp-form" @submit.prevent="create">
        <label><span>客户端名称</span><input v-model="name" maxlength="48" required autocomplete="off" :placeholder="phrase('例如：我的桌面助手')" :disabled="busy || !!token" /></label>
        <label><span>有效期</span><select v-model="days" :disabled="busy || !!token"><option :value="7">7 天</option><option :value="30">30 天</option><option :value="90">90 天</option></select></label>
        <label><span>访问权限</span><select v-model="accessMode" :disabled="busy || !!token"><option value="inspect">基础巡检</option><option value="read">资源查询</option><option value="manage">资源管理</option></select></label>
        <fieldset v-if="accessMode !== 'inspect'" :disabled="busy || !!token"><legend>授权范围</legend><div class="mcp-hosts"><label v-for="domain in domainOptions" :key="domain.id"><input v-model="domains" type="checkbox" :value="domain.id" /><span>{{ phrase(domain.name) }}</span></label></div>
          <label v-if="domains.includes('files')" class="mcp-roots"><span>允许访问的文件目录（每行一个绝对路径）</span><textarea v-model="fileRoots" rows="3" placeholder="/home/web" /></label>
          <label v-if="accessMode === 'manage'" class="mcp-auto"><input v-model="autoApprove" type="checkbox" /><span>允许自动执行日常启停和固定诊断；其他修改仍需逐项审批。</span></label>
        </fieldset>
        <fieldset :disabled="busy || !!token"><legend>授权主机</legend><div class="mcp-hosts"><label v-for="host in hosts" :key="host.id"><input v-model="selected" type="checkbox" :value="host.id" /><span>{{ host.name }} <small>{{ host.isLocal ? '本机' : '远程管理还需被控节点授权；轻节点仅支持摘要。' }}</small></span></label></div></fieldset>
        <p class="mcp-note">新加入的主机不会自动授权。需要更换权限或凭据时，撤销该客户端并重新创建。</p>
        <p v-if="settings.access.clients.length >= settings.maxClients" role="status">客户端数量已达上限，请撤销不再使用的客户端。</p>
        <button class="button button--primary" type="submit" :disabled="!canCreate">{{ accessMode === 'inspect' ? '创建巡检客户端' : '创建授权客户端' }}</button>
      </form>
      <div v-if="token" class="mcp-credential" role="region" :aria-label="phrase('一次性连接配置')">
        <label><span>AI 客户端</span><select v-model="clientKind"><option v-for="client in mcpClients" :key="client.id" :value="client.id">{{ client.name }}</option></select></label>
        <p v-if="clientKind === 'stdio'" class="mcp-note">先在 AI 客户端所在电脑安装对应系统的 kpanel-mcp，并确保客户端能找到该命令；也可将 command 改为程序的绝对路径。</p>
        <p class="mcp-note">{{ phrase('将配置合并到客户端的个人设置，保留已有服务器。') }} <code>{{ clientLocation }}</code></p>
        <h3>{{ credentialName }} · {{ phrase('连接配置') }}</h3>
        <p>凭据只显示这一次。将配置加入支持 HTTP 请求头的 MCP 客户端，并妥善保存；面板备份不会包含这些凭据。</p>
        <label><span>通用连接配置</span><textarea :value="config" readonly rows="10" spellcheck="false" :aria-label="phrase('通用连接配置')" /></label>
        <div class="mcp-actions"><button type="button" class="button button--secondary" :disabled="busy" @click="copy">复制配置</button><button type="button" class="button button--secondary" :disabled="busy" @click="test">测试连接</button><button type="button" class="button button--secondary" :disabled="busy" @click="clearCredential">已保存，关闭凭据</button></div>
      </div>
      <div class="mcp-history">
        <h3>已授权客户端</h3>
        <p v-if="!settings.access.clients.length" class="mcp-note" role="status">尚未创建客户端。</p>
        <ul v-else>
          <li v-for="client in settings.access.clients" :key="client.id">
            <div><strong>{{ client.name }}</strong><p>{{ client.hosts.map(h => hostLabel(h.id)).join('、') }}</p><p>{{ client.policy ? (client.policy.write ? '资源管理' : '资源查询') : '基础巡检' }} <span v-if="client.policy">· {{ client.policy.domains.map(id => phrase(domainOptions.find(domain => domain.id === id)?.name || id)).join('、') }}</span></p><small>{{ Date.parse(client.expiresAt) <= Date.now() ? '已过期' : '到期时间' }} · {{ formatDateTime(client.expiresAt) }}</small></div>
            <button type="button" class="button button--secondary" :disabled="busy" :aria-label="`${phrase('撤销客户端')} ${client.name}`" @click="revoke(client.id)">撤销</button>
          </li>
        </ul>
      </div>
      <MCPOperations :clients="settings.access.clients" :hosts="hosts" />
      <MCPClusterGrants :enabled="settings.access.enabled" :transport-ready="settings.transportReady" />
    </div>
    <p v-if="error" class="mcp-error" role="alert">{{ phrase(error) }}</p>
    <p v-if="notice" class="mcp-notice" role="status">{{ phrase(notice) }}</p>
    <button class="button button--secondary mcp-refresh" type="button" :disabled="loading || busy" @click="refresh"><RefreshCw :size="15" aria-hidden="true" />刷新接入状态</button>
  </section>
</template>

<style scoped>
.mcp-access { min-width: 0; font-size: 14px; line-height: 1.6; }
.mcp-access > :not(header) { margin-left: 24px; margin-right: 24px; }
.mcp-access { padding-bottom: 24px; }
.mcp-form input:not([type=checkbox]), .mcp-form select, .mcp-form textarea, .mcp-credential select, .mcp-credential textarea { min-height: 40px; padding: 8px 12px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface); color: var(--text); font-size: 14px; width: 100%; min-width: 0; box-sizing: border-box; }
.mcp-roots { display: grid; gap: 6px; margin-top: 16px; }
.mcp-auto { display: flex; gap: 8px; margin-top: 16px; align-items: flex-start; }
.mcp-access :focus-visible { outline: 2px solid var(--brand); outline-offset: 3px; }
.mcp-content { display: grid; gap: 20px; }
.mcp-toolbar, .mcp-history li { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.mcp-toolbar > div, .mcp-history li > div { min-width: 0; }
.mcp-toolbar button, .mcp-history li button { flex: none; }
.mcp-access p { margin: 6px 0; overflow-wrap: anywhere; }
.mcp-note, .mcp-access small { color: var(--text-secondary); font-size: 13px; }
.mcp-endpoint { display: flex; flex-wrap: wrap; gap: 12px; }
.mcp-endpoint code { font-size: 13px; overflow-wrap: anywhere; }
.mcp-form { display: grid; grid-template-columns: minmax(0, 2fr) minmax(0, 1fr); gap: 16px; padding-top: 16px; border-top: 1px solid var(--border); }
.mcp-form > label, .mcp-credential label { display: grid; gap: 6px; }
.mcp-form fieldset, .mcp-form > p { grid-column: 1 / -1; }
.mcp-form fieldset { border: 1px solid var(--border); border-radius: var(--radius); padding: 12px; min-width: 0; }
.mcp-form legend { padding: 0 6px; }
.mcp-hosts { display: grid; gap: 12px; max-height: 240px; overflow: auto; }
.mcp-hosts label { display: flex; gap: 10px; align-items: flex-start; overflow-wrap: anywhere; }
.mcp-hosts input { width: 18px; height: 18px; flex: none; margin-top: 3px; }
.mcp-hosts small { display: block; }
.mcp-form > button { justify-self: start; }
.mcp-credential { padding: 18px; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface-subtle); }
.mcp-access h3 { font-size: 16px; margin: 0 0 8px; }
.mcp-credential textarea { width: 100%; min-width: 0; box-sizing: border-box; resize: vertical; font-family: var(--font-mono, monospace); font-size: 13px; overflow-wrap: anywhere; }
.mcp-actions { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 12px; }
.mcp-history ul { list-style: none; padding: 0; margin: 0; }
.mcp-history li { padding: 12px 0; border-bottom: 1px solid var(--border); overflow-wrap: anywhere; }
.mcp-error { color: var(--danger); }
.mcp-notice { color: var(--text-primary); }
.mcp-refresh { margin-top: 18px; }
@media (max-width: 600px) { .mcp-form { grid-template-columns: minmax(0, 1fr); } .mcp-toolbar { flex-direction: column; } }
</style>
