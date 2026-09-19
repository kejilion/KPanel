<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { mcpAccess, mcpDomains, type MCPOAuthRequest } from '@/lib/mcp'
import { formatDateTime } from '@/lib/format'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'

const props = defineProps<{ requestId: string; hosts: Array<{ id: string; name: string }>; version: string }>()
const request = ref<MCPOAuthRequest>()
const selected = ref(['local'])
const mode = ref<'inspect' | 'read' | 'manage'>('inspect')
const domains = ref(['system', 'docker', 'sites', 'apps', 'diagnostics', 'backups'])
const roots = ref('')
const days = ref(30)
const acknowledged = ref(false)
const busy = ref(false)
const error = ref('')
let disposed = false
function phrase(value: string) { phraseCatalogVersion.value; return translatePhrase(value) }
const canApprove = computed(() => !!request.value && selected.value.length > 0 && acknowledged.value && !busy.value && (mode.value === 'inspect' || domains.value.length > 0 && (!domains.value.includes('files') || !!roots.value.trim())))
watch([selected, mode, domains, roots, days], () => { acknowledged.value = false }, { deep: true })
async function load() {
  busy.value = true; error.value = ''
  try { const value = await mcpAccess.oauthRequest(props.requestId); if (!disposed) request.value = value }
  catch { if (!disposed) error.value = '授权请求不可用或已过期，请从 AI 客户端重新连接。' }
  finally { if (!disposed) busy.value = false }
}
async function decide(approve: boolean) {
  if (!request.value || busy.value || approve && !canApprove.value) return
  busy.value = true; error.value = ''
  try {
    const result = await mcpAccess.oauthConsent(props.requestId, { approve, hostIds: [...selected.value], domains: mode.value === 'inspect' ? [] : [...domains.value], write: mode.value === 'manage', fileRoots: mode.value !== 'inspect' && domains.value.includes('files') ? roots.value.split('\n').map(path => path.trim()).filter(Boolean) : [], expiresInDays: days.value, expectedResourceVersion: props.version })
    if (!disposed) window.location.assign(result.redirectUrl)
  } catch { if (!disposed) error.value = '授权结果尚未确认，请检查已授权客户端列表，再从 AI 客户端重新连接。' }
  finally { if (!disposed) busy.value = false }
}
onMounted(load)
onBeforeUnmount(() => { disposed = true })
</script>

<template>
  <section class="oauth-consent" aria-labelledby="mcp-oauth-title">
    <h3 id="mcp-oauth-title">授权 AI 客户端连接</h3>
    <p v-if="error" role="alert">{{ phrase(error) }}</p>
    <p v-if="busy && !request" role="status">正在加载授权请求…</p>
    <template v-if="request">
      <p><strong>{{ request.clientName }}</strong></p>
      <p>名称由客户端提供。请核对回调地址，确认这是您正在连接的应用。</p>
      <code>{{ request.redirectUri }}</code>
      <p>{{ phrase('到期时间') }} · {{ formatDateTime(request.expiresAt) }}</p>
      <form @submit.prevent="decide(true)">
        <label><span>访问权限</span><select v-model="mode" :disabled="busy"><option value="inspect">基础巡检</option><option value="read">资源查询</option><option value="manage">资源管理</option></select></label>
        <label><span>有效期</span><select v-model="days" :disabled="busy"><option :value="7">7 天</option><option :value="30">30 天</option><option :value="90">90 天</option></select></label>
        <fieldset :disabled="busy"><legend>授权主机</legend><label v-for="host in hosts" :key="host.id" class="check"><input v-model="selected" type="checkbox" :value="host.id" />{{ host.name }}</label></fieldset>
        <fieldset v-if="mode !== 'inspect'" :disabled="busy"><legend>授权范围</legend><label v-for="domain in mcpDomains" :key="domain.id" class="check"><input v-model="domains" type="checkbox" :value="domain.id" />{{ phrase(domain.name) }}</label></fieldset>
        <label v-if="mode !== 'inspect' && domains.includes('files')"><span>允许访问的文件目录（每行一个绝对路径）</span><textarea v-model="roots" rows="3" placeholder="/home/web" :disabled="busy" /></label>
        <p>写操作默认需要在面板审批。远程管理还需要被控节点授权；您可以随时撤销此客户端。</p>
        <label class="check"><input v-model="acknowledged" type="checkbox" :disabled="busy" /><span>我已核对客户端回调地址、主机和权限范围。</span></label>
        <div class="actions"><button type="submit" class="button button--primary" :disabled="!canApprove">授权并返回客户端</button><button type="button" class="button button--secondary" :disabled="busy" @click="decide(false)">拒绝授权</button></div>
      </form>
    </template>
  </section>
</template>

<style scoped>
.oauth-consent { padding: 20px; border: 2px solid var(--brand); border-radius: var(--radius); background: var(--surface-subtle); font-size: 14px; line-height: 1.6; min-width: 0; }
h3 { font-size: 18px; margin-top: 0; }
form { display: grid; gap: 16px; }
label { display: grid; gap: 6px; }
fieldset { border: 1px solid var(--border); border-radius: var(--radius); display: grid; gap: 12px; padding: 12px; min-width: 0; }
.check, .actions { display: flex; gap: 10px; align-items: flex-start; }
.actions { flex-wrap: wrap; }
input { width: 18px; height: 18px; flex: none; margin-top: 3px; }
select, textarea { min-height: 40px; padding: 8px 12px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface); color: var(--text); font-size: 14px; width: 100%; min-width: 0; box-sizing: border-box; }
code, p { overflow-wrap: anywhere; }
code { font-size: 14px; }
:focus-visible { outline: 2px solid var(--brand); outline-offset: 3px; }
[role=alert] { color: var(--danger); }
</style>
