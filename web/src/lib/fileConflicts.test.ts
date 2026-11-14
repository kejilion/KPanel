// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { FileConflictBatch, cancelFileConflicts, pendingFileConflict, copyFileName } from './fileConflicts'
import { uploadConflictFile } from './fileConflictUpload'
import type { FileEntry } from '@/types/api'
const entry = (path: string, kind: FileEntry['kind'] = 'file'): FileEntry => ({path,kind,name:path.split('/').at(-1)!,sizeBytes:3,resourceVersion:'version:'+path,mode:'0644',owner:'root',group:'root',modifiedAt:'2026-11-14T00:00:00Z',editable:true,previewable:true})
afterEach(() => cancelFileConflicts())

it('serializes concurrent prompts and applies a choice to queued conflicts only in this batch', async () => {
 const batch = new FileConflictBatch('上传','host-a')
 const a = batch.resolve(entry('/a/one'),'/dest',async path=>entry(path))
 const b = batch.resolve(entry('/a/two'),'/dest',async path=>entry(path))
 await flushPromises(); expect(pendingFileConflict.value?.source.name).toBe('one')
 pendingFileConflict.value!.decide('skip',true)
 expect((await a).skipped).toBe(true); expect((await b).skipped).toBe(true)
 const another = new FileConflictBatch('上传','host-b')
 const c = another.resolve(entry('/a/three'),'/dest',async path=>entry(path))
 await flushPromises(); expect(pendingFileConflict.value?.host).toBe('host-b')
 pendingFileConflict.value!.decide('skip',false); await c
})

it('remembers overwrite but asks again for incompatible kinds and changed targets', async () => {
 const batch = new FileConflictBatch('复制','h')
 const one = batch.resolve(entry('/s/a'),'/d',async p=>entry(p)); await flushPromises()
 pendingFileConflict.value!.decide('overwrite',true); expect((await one).version).toBe('version:/d/a')
 const mismatch = batch.resolve(entry('/s/b'),'/d',async p=>entry(p,'directory'));await flushPromises()
 expect(pendingFileConflict.value!.canOverwrite).toBe(false);pendingFileConflict.value!.decide('skip',false);await mismatch
 const changed = batch.resolve(entry('/s/c'),'/d',async p=>entry(p),true);await flushPromises()
 expect(pendingFileConflict.value!.changed).toBe(true);pendingFileConflict.value!.decide('skip',false);await changed
})

it('keeps extensions and UTF-8 names bounded and rejects unreliable target lookups', async () => {
 expect(copyFileName('a.txt',1)).toBe('a (1).txt')
 expect(new TextEncoder().encode(copyFileName('中文'.repeat(100)+'.png',99)).length).toBeLessThanOrEqual(255)
 const b=new FileConflictBatch('上传','h')
 await expect(b.resolve(entry('/s/a'),'/d',async()=>{throw {status:403}})).rejects.toEqual({status:403})
})

it('cancels active and queued prompts without applying policy to another batch', async () => {
 const a=new FileConflictBatch('上传','a'), b=new FileConflictBatch('上传','b')
 const p=a.resolve(entry('/s/a'),'/d',async p=>entry(p)).catch(e=>e.name)
 const q=b.resolve(entry('/s/b'),'/d',async p=>entry(p)).catch(e=>e.name)
 await flushPromises();cancelFileConflicts();expect(await p).toBe('AbortError');expect(await q).toBe('AbortError')
 expect(pendingFileConflict.value).toBeUndefined()
})

it('binds overwrite to the shown version and re-prompts after a 409 instead of silently retrying', async () => {
 const api={entry:vi.fn(async p=>entry(p)),uploadVersion:vi.fn().mockRejectedValueOnce({status:409}).mockResolvedValue(entry('/d/a.txt'))}
 const batch=new FileConflictBatch('上传','h'); const p=uploadConflictFile(api,'/d',new File(['new'],'a.txt'),batch)
 await flushPromises();pendingFileConflict.value!.decide('overwrite',true)
 await flushPromises();expect(pendingFileConflict.value!.changed).toBe(true);pendingFileConflict.value!.decide('overwrite',false)
 expect((await p)?.path).toBe('/d/a.txt');expect(api.uploadVersion.mock.calls[0]![3]).toBe('version:/d/a.txt')
})

it('remembers file and directory choices independently', async () => {
 const batch=new FileConflictBatch('复制','h')
 const file=batch.resolve(entry('/s/a'),'/d',async p=>entry(p));await flushPromises();pendingFileConflict.value!.decide('skip',true);await file
 const folder=batch.resolve(entry('/s/folder','directory'),'/d',async p=>entry(p,'directory'));await flushPromises()
 expect(pendingFileConflict.value!.source.kind).toBe('directory');pendingFileConflict.value!.decide('overwrite',true);expect((await folder).merge).toBe(true)
 expect((await batch.resolve(entry('/s/b'),'/d',async p=>entry(p))).skipped).toBe(true)
 batch.close()
})

it('asks again when a previously confirmed destination disappears', async () => {
 const batch=new FileConflictBatch('上传','h')
 const task=batch.resolve(entry('/s/a'),'/d',async()=>{throw {status:404}},true);await flushPromises()
 expect(pendingFileConflict.value!.changed).toBe(true);pendingFileConflict.value!.decide('overwrite',false)
 expect(await task).toMatchObject({name:'a',overwrite:false});batch.close()
})
