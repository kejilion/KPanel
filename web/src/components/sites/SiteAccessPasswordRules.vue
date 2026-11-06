<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { api } from '@/lib/api'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import type { SiteAccessPasswordRule } from '@/types/api'

const props = withDefaults(defineProps<{ siteId: string; siteVersion?: string; writable?: boolean; allowAdd?: boolean; unavailableReason?: string }>(), { siteVersion: '', writable: false, allowAdd: true, unavailableReason: '' })
const emit = defineEmits<{ changed: [siteVersion: string]; busy: [value: boolean] }>()
const expanded = ref(false)
const loading = ref(false)
const error = ref(false)
const rules = ref<SiteAccessPasswordRule[]>([])
const editing = ref<{ ruleId: string; action: 'add' | 'update' | 'delete'; username: string; previousUsername: string; password: string }>()
const saving = ref(false)
const saveError = ref('')
const notice = ref('')
const ruleForm = ref<{ scope: 'server' | 'location'; path: string; username: string; password: string }>()
const deleteRule = ref<SiteAccessPasswordRule>()
const currentSiteVersion = ref(props.siteVersion)
let active: AbortController | undefined
let disposed = false

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

function sameScope(rule: SiteAccessPasswordRule): SiteAccessPasswordRule[] {
  return rules.value.filter((item) => item.scope === rule.scope && item.path === rule.path && (item.modifier || '') === (rule.modifier || ''))
}

async function refreshSiteVersion(site: string): Promise<void> {
  currentSiteVersion.value = ''
  try {
    const refreshed = await api.sites.list()
    if (!disposed && site === props.siteId) {
      currentSiteVersion.value = refreshed.items.find((item) => item.id === site)?.resourceVersion || ''
      if (currentSiteVersion.value) emit('changed', currentSiteVersion.value)
      else notice.value = phrase('网站状态刷新失败，请重新打开网站详情。')
    }
  } catch {
    if (!disposed && site === props.siteId) notice.value = phrase('网站状态刷新失败，请重新打开网站详情。')
  }
}

async function refreshAfterFailure(): Promise<void> {
  editing.value = undefined
  ruleForm.value = undefined
  deleteRule.value = undefined
  saveError.value = ''
  notice.value = ''
  await Promise.all([load(), refreshSiteVersion(props.siteId)])
}

async function load(): Promise<void> {
  active?.abort()
  const controller = new AbortController()
  active = controller
  loading.value = true
  error.value = false
  try {
    const result = await api.sites.accessPasswords(props.siteId, controller.signal)
    if (!controller.signal.aborted) rules.value = result
  } catch {
    if (!controller.signal.aborted) error.value = true
  } finally {
    if (active === controller) {
      active = undefined
      loading.value = false
    }
  }
}

function toggle(): void {
  if (saving.value) return
  expanded.value = !expanded.value
  if (expanded.value) void load()
  else { active?.abort(); editing.value = undefined; ruleForm.value = undefined; deleteRule.value = undefined; saveError.value = '' }
}

async function saveRule(): Promise<void> {
  if (!props.writable || saving.value) return
  const draft = ruleForm.value
  const deletion = deleteRule.value
  if (!draft && !deletion) return
  if (draft && (!draft.username.trim() || !draft.password || draft.scope === 'location' && !draft.path.trim())) {
    saveError.value = phrase('请填写账号和密码。')
    return
  }
  if (draft && new TextEncoder().encode(draft.password).length > 256) {
    saveError.value = phrase('访问密码不能超过 256 字节。')
    return
  }
  const site = props.siteId
  saving.value = true
  saveError.value = ''
  notice.value = ''
  try {
    const updated = await api.sites.updateAccessRule(site, deletion
      ? { action: 'delete', ruleId: deletion.id, resourceVersion: deletion.resourceVersion }
      : { action: 'add', resourceVersion: currentSiteVersion.value, scope: draft!.scope,
          path: draft!.scope === 'location' ? draft!.path : undefined,
          username: draft!.username, password: draft!.password })
    if (disposed || site !== props.siteId) return
    rules.value = updated
    ruleForm.value = undefined
    deleteRule.value = undefined
    await refreshSiteVersion(site)
  } catch {
    if (!disposed && site === props.siteId) saveError.value = phrase('操作结果未确认，请刷新查看后再重试。')
  } finally { if (!disposed) saving.value = false }
}

function begin(rule: SiteAccessPasswordRule, action: 'add' | 'update' | 'delete', username = ''): void {
  if (!props.writable || saving.value || !rule.accountsReadable) return
  editing.value = { ruleId: rule.id, action, username, previousUsername: username, password: '' }
  saveError.value = ''
  notice.value = ''
}

async function save(rule: SiteAccessPasswordRule): Promise<void> {
  const draft = editing.value
  if (!draft || draft.ruleId !== rule.id || saving.value || !props.writable) return
  if (!draft.username.trim() || (draft.action !== 'delete' && !draft.password)) {
    saveError.value = phrase('请填写账号和密码。')
    return
  }
  if (draft.action !== 'delete' && new TextEncoder().encode(draft.password).length > 256) {
    saveError.value = phrase('访问密码不能超过 256 字节。')
    return
  }
  const site = props.siteId
  saving.value = true
  saveError.value = ''
  try {
    const updated = await api.sites.updateAccessAccount(site, {
      ruleId: rule.id, resourceVersion: rule.resourceVersion, action: draft.action,
      username: draft.username, previousUsername: draft.action === 'update' ? draft.previousUsername : undefined,
      password: draft.action === 'delete' ? undefined : draft.password,
    })
    if (!disposed && site === props.siteId) { rules.value = updated; editing.value = undefined }
  } catch {
    if (!disposed && site === props.siteId) {
      saveError.value = phrase('操作结果未确认，请刷新查看后再重试。')
    }
  } finally {
    if (!disposed) saving.value = false
  }
}

watch(() => props.siteId, () => {
  active?.abort()
  expanded.value = false
  rules.value = []
  editing.value = undefined
  saveError.value = ''
  ruleForm.value = undefined
  deleteRule.value = undefined
  currentSiteVersion.value = props.siteVersion
})
watch(() => props.siteVersion, (version) => { currentSiteVersion.value = version })
watch(saving, (value) => emit('busy', value), { flush: 'sync' })
onBeforeUnmount(() => { disposed = true; active?.abort(); editing.value = undefined; ruleForm.value = undefined; emit('busy', false) })
</script>

<template>
  <section class="detail-section">
    <h3>{{ phrase('访问密码') }}</h3>
    <button class="button button--secondary button--small" type="button" :aria-expanded="expanded" :disabled="saving" @click="toggle">
      {{ phrase(expanded ? '收起' : '查看访问密码') }}
    </button>
    <template v-if="expanded">
      <p v-if="loading" role="status">{{ phrase('正在读取访问密码规则…') }}</p>
      <p v-else-if="error" role="alert">{{ phrase('读取失败，请重试。') }} <button class="button button--ghost button--small" type="button" @click="load">{{ phrase('重试') }}</button></p>
      <p v-else-if="rules.length === 0">{{ phrase('未发现可识别的访问密码规则') }}</p>
      <ul v-else class="access-password-rules">
        <li v-for="rule in rules" :key="rule.id">
          <strong>{{ rule.scope === 'server' ? phrase('站点级规则') : `${rule.modifier || ''} ${rule.path}`.trim() }}<template v-if="sameScope(rule).length > 1"> ({{ sameScope(rule).findIndex((item) => item.id === rule.id) + 1 }}/{{ sameScope(rule).length }})</template></strong>
          <button class="button button--ghost button--small button--danger-text" type="button" :disabled="!writable || saving || !rule.accountsReadable" @click="deleteRule = rule; ruleForm = undefined; editing = undefined; saveError = ''">{{ phrase('删除规则') }}</button>
          <p v-if="!rule.accountsReadable">{{ phrase('无法读取账号') }}</p>
          <template v-else>
            <div v-if="rule.usernames.length === 0">{{ phrase('没有账号') }}</div>
            <div v-for="username in rule.usernames" :key="username" class="access-password-account">
              <span>{{ username }}</span>
              <button class="button button--ghost button--small" type="button" :disabled="!writable || saving" @click="begin(rule, 'update', username)">{{ phrase('修改') }}</button>
              <button class="button button--ghost button--small button--danger-text" type="button" :disabled="!writable || saving" @click="begin(rule, 'delete', username)">{{ phrase('删除') }}</button>
            </div>
            <button class="button button--secondary button--small" type="button" :disabled="!writable || saving" @click="begin(rule, 'add')">{{ phrase('添加账号') }}</button>
            <form v-if="editing?.ruleId === rule.id" class="access-password-edit" @submit.prevent="save(rule)">
              <template v-if="editing.action !== 'delete'">
                <label class="field"><span>{{ phrase('访问账号') }}</span><input v-model="editing.username" autocomplete="off" maxlength="256" required /></label>
                <label class="field"><span>{{ phrase('访问密码') }}</span><input v-model="editing.password" type="password" autocomplete="new-password" maxlength="256" required /></label>
              </template>
              <p v-else>{{ phrase('确定删除此账号？引用同一密码文件的规则也会受到影响。') }} {{ editing.username }}</p>
              <p v-if="saveError" class="inline-alert inline-alert--danger" role="alert">{{ saveError }} <button class="button button--ghost button--small" type="button" @click="refreshAfterFailure">{{ phrase('刷新规则') }}</button></p>
              <div class="access-password-actions">
                <button class="button button--primary button--small" type="submit" :disabled="saving">{{ phrase(saving ? '正在保存…' : editing.action === 'delete' ? '确认删除' : '保存') }}</button>
                <button class="button button--secondary button--small" type="button" :disabled="saving" @click="editing = undefined">{{ phrase('取消') }}</button>
              </div>
            </form>
          </template>
        </li>
      </ul>
      <button v-if="!loading && !error" class="button button--secondary button--small" type="button" :disabled="!writable || !allowAdd || saving || !currentSiteVersion" @click="ruleForm = { scope: 'server', path: '', username: '', password: '' }; deleteRule = undefined; editing = undefined; saveError = ''">{{ phrase('添加规则') }}</button>
      <p v-if="!allowAdd">{{ phrase('跳转站点暂不支持添加访问密码。') }}</p>
      <form v-if="ruleForm" class="access-password-edit" @submit.prevent="saveRule">
        <label class="field"><span>{{ phrase('保护范围') }}</span><select v-model="ruleForm.scope"><option value="server">{{ phrase('站点级规则') }}</option><option value="location">{{ phrase('指定已有路径') }}</option></select></label>
        <label v-if="ruleForm.scope === 'location'" class="field"><span>{{ phrase('保护路径') }}</span><input v-model="ruleForm.path" placeholder="/admin/" required /><small>{{ phrase('填写网站配置中已有的路径。') }}</small></label>
        <label class="field"><span>{{ phrase('访问账号') }}</span><input v-model="ruleForm.username" autocomplete="off" required /></label>
        <label class="field"><span>{{ phrase('访问密码') }}</span><input v-model="ruleForm.password" type="password" autocomplete="new-password" maxlength="256" required /></label>
        <p>{{ phrase('认证可能影响接口、回调和证书自动续签；已有路径规则、包含配置或重定向也可能改变认证范围，请保存后测试。') }}</p>
        <p>{{ phrase('反向代理的上游端口可能仍可直接访问。') }}</p>
        <p>{{ phrase('配置的响应头可能在未认证时公开，请勿在响应头中放置敏感信息。') }}</p>
        <p v-if="saveError" class="inline-alert inline-alert--danger" role="alert">{{ saveError }} <button class="button button--ghost button--small" type="button" @click="refreshAfterFailure">{{ phrase('刷新规则') }}</button></p>
        <div class="access-password-actions"><button class="button button--primary button--small" type="submit" :disabled="saving">{{ phrase(saving ? '正在保存…' : '保存') }}</button><button class="button button--secondary button--small" type="button" :disabled="saving" @click="ruleForm = undefined">{{ phrase('取消') }}</button></div>
      </form>
      <form v-if="deleteRule" class="access-password-edit" @submit.prevent="saveRule">
        <p>{{ phrase('确定删除这条规则？密码文件会保留。') }}</p>
        <p v-if="sameScope(deleteRule).length > 1">{{ phrase('只删除这一条，其他同范围的规则会保留。') }}</p>
        <p v-if="saveError" class="inline-alert inline-alert--danger" role="alert">{{ saveError }} <button class="button button--ghost button--small" type="button" @click="refreshAfterFailure">{{ phrase('刷新规则') }}</button></p>
        <div class="access-password-actions"><button class="button button--primary button--small" type="submit" :disabled="saving">{{ phrase('确认删除') }}</button><button class="button button--secondary button--small" type="button" :disabled="saving" @click="deleteRule = undefined">{{ phrase('取消') }}</button></div>
      </form>
      <p v-if="!writable && unavailableReason">{{ unavailableReason }}</p>
      <p v-if="notice" role="status">{{ notice }}</p>
    </template>
  </section>
</template>

<style scoped>
.access-password-rules { display: grid; gap: 8px; padding: 0; margin: 12px 0 0; list-style: none; }
.access-password-rules li { display: grid; gap: 8px; }
.access-password-account { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
.access-password-edit { display: grid; gap: 10px; }
.access-password-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.access-password-rules span { color: var(--muted); overflow-wrap: anywhere; }
</style>
