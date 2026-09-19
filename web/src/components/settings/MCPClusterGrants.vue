<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { mcpAccess, mcpDomains, type MCPClusterGrants } from '@/lib/mcp'
import { formatDateTime } from '@/lib/format'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'

const props = defineProps<{ enabled: boolean; transportReady: boolean }>()
const state = ref<MCPClusterGrants>()
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const controller = ref('')
const domains = ref(['system', 'docker', 'sites', 'apps', 'diagnostics', 'backups'])
const write = ref(false)
const roots = ref('')
const days = ref(30)
const acknowledged = ref(false)
let disposed = false
function phrase(value: string) { phraseCatalogVersion.value; return translatePhrase(value) }
const canGrant = computed(() => props.enabled && props.transportReady && state.value?.grants.available && !!controller.value && domains.value.length > 0 && (!domains.value.includes('files') || !!roots.value.trim()) && acknowledged.value && !busy.value && !loading.value)
watch([controller, domains, write, roots, days], () => { acknowledged.value = false }, { deep: true })
async function refresh() {
  loading.value = true; error.value = ''
  try { const value = await mcpAccess.clusterGrants(); if (!disposed) { state.value = value; if (!value.controllers.some(item => item.id === controller.value)) controller.value = '' } }
  catch { if (!disposed) error.value = '无法加载集群管理授权，请刷新重试。' }
  finally { if (!disposed) loading.value = false }
}
async function grant() {
  if (!canGrant.value || !state.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const value = await mcpAccess.grantCluster({ controllerId: controller.value, domains: [...domains.value], write: write.value, fileRoots: domains.value.includes('files') ? roots.value.split('\n').map(root => root.trim()).filter(Boolean) : [], expiresInDays: days.value, expectedResourceVersion: state.value.grants.resourceVersion })
    if (!disposed) { state.value = value; acknowledged.value = false; notice.value = '集群管理授权已保存。' }
  } catch { if (!disposed) error.value = '授权结果尚未确认，请刷新核对后再操作。' }
  finally { if (!disposed) busy.value = false }
}
async function revoke(id: string) {
  if (!state.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try { const value = await mcpAccess.revokeCluster(id, state.value.grants.resourceVersion); if (!disposed) { state.value = value; notice.value = '集群管理授权已撤销。' } }
  catch { if (!disposed) error.value = '授权结果尚未确认，请刷新核对后再操作。' }
  finally { if (!disposed) busy.value = false }
}
function controllerName(id: string) { return state.value?.controllers.find(item => item.id === id)?.name || phrase('已解除配对的控制端') }
onMounted(refresh)
onBeforeUnmount(() => { disposed = true })
</script>

<template>
  <details class="cluster-grants">
    <summary>允许其他 KPanel 管理本机</summary>
    <p>仅配对不会授予远程操作权限。在被控节点明确授权后，控制端才能通过 MCP 管理本机。</p>
    <p>写入授权表示信任该控制端在所选范围内发起操作；客户端权限和人工审批由控制端负责。关闭本机 MCP 会停止接收新的远程操作。</p>
    <p v-if="loading" role="status">正在加载集群管理授权…</p>
    <template v-if="state">
      <p v-if="!state.grants.available" role="alert">集群管理授权存储不可用。</p>
      <p v-if="!enabled" role="status">启用本机 MCP 后，集群管理授权才会生效。</p>
      <p v-if="!state.controllers.length">尚无已配对控制端。请先完成集群配对，再在这里授权。</p>
      <form v-else @submit.prevent="grant">
        <label><span>控制端</span><select v-model="controller" :disabled="busy"><option value="">选择已配对控制端</option><option v-for="item in state.controllers" :key="item.id" :value="item.id">{{ item.name }} · {{ item.fingerprint }}</option></select></label>
        <label><span>访问权限</span><select v-model="write" :disabled="busy"><option :value="false">资源查询</option><option :value="true">资源管理</option></select></label>
        <label><span>有效期</span><select v-model="days" :disabled="busy"><option :value="7">7 天</option><option :value="30">30 天</option><option :value="90">90 天</option></select></label>
        <fieldset :disabled="busy"><legend>授权范围</legend><label v-for="domain in mcpDomains" :key="domain.id" class="check"><input v-model="domains" type="checkbox" :value="domain.id" />{{ phrase(domain.name) }}</label></fieldset>
        <label v-if="domains.includes('files')"><span>允许访问的文件目录（每行一个绝对路径）</span><textarea v-model="roots" rows="3" placeholder="/home/web" :disabled="busy" /></label>
        <label class="check"><input v-model="acknowledged" type="checkbox" :disabled="busy" /><span>我已核对控制端身份和授权范围，允许其按上述权限访问本机。</span></label>
        <button type="submit" class="button button--primary" :disabled="!canGrant">保存集群管理授权</button>
      </form>
      <ul v-if="state.grants.items.length">
        <li v-for="item in state.grants.items" :key="item.controllerId">
          <div><strong>{{ controllerName(item.controllerId) }}</strong><p>{{ item.policy.write ? '资源管理' : '资源查询' }} · {{ Object.keys(item.policy.operationVersions).length }} {{ phrase('项明确授权的操作') }}</p><p v-if="item.policy.fileRoots?.length">{{ item.policy.fileRoots.join('、') }}</p><p>{{ Date.parse(item.expiresAt) <= Date.now() ? '已过期' : '到期时间' }} · {{ formatDateTime(item.expiresAt) }}</p><details><summary>查看授权操作</summary><code>{{ Object.keys(item.policy.operationVersions).sort().join('\n') }}</code></details></div>
          <button type="button" class="button button--secondary" :disabled="busy || !transportReady" @click="revoke(item.controllerId)">撤销集群管理授权</button>
        </li>
      </ul>
      <p v-else>尚未授予集群管理权限。</p>
    </template>
    <p v-if="error" role="alert">{{ phrase(error) }}</p><p v-if="notice" role="status">{{ phrase(notice) }}</p>
    <button type="button" class="button button--secondary" :disabled="busy || loading" @click="refresh">刷新集群管理授权</button>
  </details>
</template>

<style scoped>
.cluster-grants { border-top: 1px solid var(--border); padding-top: 20px; font-size: 14px; line-height: 1.6; min-width: 0; }
summary { cursor: pointer; padding: 8px 0; font-weight: 500; }
p { color: var(--text-secondary); overflow-wrap: anywhere; }
form { display: grid; gap: 16px; max-width: 720px; padding: 16px 0; }
label { display: grid; gap: 6px; }
select, textarea { padding: 8px 12px; min-height: 40px; font-size: 14px; color: var(--text); background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-sm); width: 100%; min-width: 0; box-sizing: border-box; }
fieldset { display: grid; gap: 12px; border: 1px solid var(--border); border-radius: var(--radius); padding: 12px; min-width: 0; }
.check { display: flex; gap: 10px; align-items: flex-start; }
input { width: 18px; height: 18px; flex: none; margin-top: 3px; }
button { justify-self: start; }
ul { list-style: none; padding: 0; }
li { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between; gap: 12px; border-bottom: 1px solid var(--border); padding: 16px 0; }
li > div { min-width: 0; }
code { display: block; white-space: pre-wrap; overflow-wrap: anywhere; max-height: 240px; overflow: auto; font-size: 13px; }
:focus-visible { outline: 2px solid var(--brand); outline-offset: 3px; }
[role=alert] { color: var(--danger); }
</style>
