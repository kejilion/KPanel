<script setup lang="ts">
import { ref } from 'vue'
import { ShieldCheck } from '@lucide/vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import SiteSettingsSection from './SiteSettingsSection.vue'

interface SiteAccessPasswordDraft {
  enabled: boolean
  path: string
  username: string
  password: string
}

const props = withDefaults(defineProps<{ modelValue: SiteAccessPasswordDraft; writable?: boolean; unavailableReason?: string }>(), { writable: false, unavailableReason: '' })
const emit = defineEmits<{ 'update:modelValue': [value: SiteAccessPasswordDraft] }>()
const expanded = ref(false)

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

function change<K extends keyof SiteAccessPasswordDraft>(key: K, value: SiteAccessPasswordDraft[K]): void {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}
</script>

<template>
  <SiteSettingsSection :title="phrase('访问密码')" :description="phrase('为指定路径设置账号密码验证')" :expanded="expanded" @toggle="expanded = !expanded">
    <template #icon><ShieldCheck :size="16" /></template>
    <div class="access-password-fields">
      <label class="access-password-enable">
        <input type="checkbox" :checked="modelValue.enabled" :disabled="!writable && !modelValue.enabled" @change="change('enabled', ($event.target as HTMLInputElement).checked)" />
        {{ phrase('开启访问密码') }}
      </label>
      <small v-if="!writable && unavailableReason">{{ unavailableReason }}</small>
      <template v-if="modelValue.enabled && expanded">
        <label class="field">
          <span>{{ phrase('保护路径') }}</span>
          <input :value="modelValue.path" placeholder="/" required @input="change('path', ($event.target as HTMLInputElement).value)" />
          <small>{{ phrase('填写 / 保护站点，或填写网站配置中已有的路径。') }}</small>
        </label>
        <label class="field">
          <span>{{ phrase('访问账号') }}</span>
          <input :value="modelValue.username" autocomplete="off" required @input="change('username', ($event.target as HTMLInputElement).value)" />
        </label>
        <label class="field">
          <span>{{ phrase('访问密码') }}</span>
          <input :value="modelValue.password" type="password" autocomplete="new-password" maxlength="256" required @input="change('password', ($event.target as HTMLInputElement).value)" />
          <small>{{ phrase('访问密码不能超过 256 字节。') }}</small>
        </label>
        <small>{{ phrase('认证可能影响接口、回调和证书自动续签；已有路径规则、包含配置或重定向也可能改变认证范围，请保存后测试。') }} {{ phrase('建议使用 HTTPS。') }}</small>
        <small>{{ phrase('反向代理的上游端口可能仍可直接访问。') }}</small>
        <small>{{ phrase('配置的响应头可能在未认证时公开，请勿在响应头中放置敏感信息。') }}</small>
      </template>
    </div>
  </SiteSettingsSection>
</template>

<style scoped>
.access-password-fields { display: grid; gap: 12px; }
.access-password-enable { display: flex; align-items: center; gap: 8px; }
</style>
