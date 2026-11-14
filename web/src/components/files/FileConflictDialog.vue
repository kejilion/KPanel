<script setup lang="ts">
import { ref, watch } from 'vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { pendingFileConflict } from '@/lib/fileConflicts'
import { useI18n } from '@/i18n'
const i18n = useI18n()
const apply = ref(false)
watch(pendingFileConflict, () => { apply.value = false })
const size = (value?: number) => value === undefined || value < 0 ? '—' : value < 1024 ? `${value} B` : value < 1048576 ? `${(value / 1024).toFixed(1)} KiB` : `${(value / 1048576).toFixed(1)} MiB`
const date = (value?: string) => value && !value.startsWith('0001-') ? new Date(value).toLocaleString() : '—'
</script>
<template>
  <ModalDialog :open="Boolean(pendingFileConflict)" :title="i18n.t('files.conflict.title')" :description="pendingFileConflict?.operation" @close="pendingFileConflict?.cancel()">
    <div v-if="pendingFileConflict" class="file-conflict">
      <p class="file-conflict__path">{{ pendingFileConflict.host }} · {{ pendingFileConflict.target.path }}</p>
      <p v-if="pendingFileConflict.changed" role="alert">{{ i18n.t('files.conflict.changed') }}</p>
      <div class="file-conflict__compare">
        <section><h3>{{ i18n.t('files.conflict.source') }}</h3><strong>{{ pendingFileConflict.source.name }}</strong><p v-if="pendingFileConflict.source.path && !pendingFileConflict.source.path.includes('/.kpanel-')" data-i18n-ignore>{{ pendingFileConflict.source.path }}</p><p>{{ pendingFileConflict.source.kind === 'directory' ? i18n.t('files.conflict.directory') : i18n.t('files.conflict.file') }} · {{ size(pendingFileConflict.source.sizeBytes) }}</p><p>{{ date(pendingFileConflict.source.modifiedAt) }}</p></section>
        <section><h3>{{ i18n.t('files.conflict.target') }}</h3><strong>{{ pendingFileConflict.target.name }}</strong><p>{{ (pendingFileConflict.target.kind as string) === 'missing' ? i18n.t('files.conflict.missing') : pendingFileConflict.target.kind === 'directory' ? i18n.t('files.conflict.directory') : i18n.t('files.conflict.file') }} · {{ size(pendingFileConflict.target.sizeBytes) }}</p><p>{{ date(pendingFileConflict.target.modifiedAt) }}</p></section>
      </div>
      <p>{{ pendingFileConflict.canOverwrite ? pendingFileConflict.source.kind === 'directory' ? i18n.t('files.conflict.merge') : i18n.t('files.conflict.replace') : i18n.t('files.conflict.incompatible') }}</p>
      <label class="file-conflict__apply"><input v-model="apply" type="checkbox" />{{ i18n.t(pendingFileConflict.source.kind === 'directory' ? 'files.conflict.applyDirectory' : 'files.conflict.apply') }}</label>
      <p class="file-conflict__scope">{{ i18n.t('files.conflict.scope') }}</p>
    </div>
    <template #footer>
      <div class="file-conflict__actions">
        <button class="button button--ghost" @click="pendingFileConflict?.cancel()">{{ i18n.t('files.conflict.cancel') }}</button>
        <button class="button button--secondary" @click="pendingFileConflict?.decide('skip', apply)">{{ i18n.t('files.conflict.skip') }}</button>
        <button class="button button--primary" @click="pendingFileConflict?.decide('keep-both', apply)">{{ i18n.t('files.conflict.keepBoth') }}</button>
        <button class="button button--secondary" :disabled="!pendingFileConflict?.canOverwrite" @click="pendingFileConflict?.decide('overwrite', apply)">{{ i18n.t((pendingFileConflict?.target.kind as string) === 'missing' ? 'files.conflict.recreate' : pendingFileConflict?.source.kind === 'directory' ? 'files.conflict.mergeAction' : 'files.conflict.overwrite') }}</button>
      </div>
    </template>
  </ModalDialog>
</template>
<style scoped>
.file-conflict { font-size: 14px; line-height: 1.6; min-width: 0; }
.file-conflict__path, .file-conflict strong { overflow-wrap: anywhere; }
.file-conflict__compare { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.file-conflict__compare section { padding: 14px; background: var(--surface-raised); border: 1px solid var(--border); border-radius: var(--radius); }
.file-conflict h3 { font-size: 14px; margin: 0 0 8px; }
.file-conflict__compare p { margin: 6px 0 0; }
.file-conflict__apply { display: flex; align-items: center; gap: 8px; }
.file-conflict__scope { color: var(--text-secondary); }
.file-conflict__actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
.file-conflict__actions .button { font-size: 14px; }
@media (max-width: 580px) { .file-conflict__compare { grid-template-columns: 1fr; } }
</style>
