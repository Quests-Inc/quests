import { expect, test } from 'bun:test'

import { flushSync } from 'svelte'

import { createAttachmentOps } from '@/pages/attachments/model/attachmentOps.svelte.ts'

import { cell, inRoot } from './root.svelte.ts'

const att = { id: 5, owner_id: 2, owner_kind: 'quest', quest_id: 2, filename: 'a.txt', comment: 'c', hidden: false }

function make(failOn = '') {
  const log: any[] = []
  const selected = cell<any>(null)
  let changed = 0
  let ops!: ReturnType<typeof createAttachmentOps>
  const guard = (name: string, ...a: any[]) => {
    if (failOn === name) throw new Error('сбой ' + name)
    log.push([name, ...a])
  }
  const stop = inRoot(() => {
    ops = createAttachmentOps({
      selected: () => selected.value,
      selectedId: () => selected.value?.id ?? null,
      onSelect: (id) => log.push(['select', id]),
      onChanged: () => changed++,
      api: {
        listRevisions: async () => ({ revisions: [{ revision: 1 }, { revision: 2 }] }) as any,
        setHidden: async (...a) => guard('hide', ...a.slice(1)),
        setCurrent: async (...a) => guard('current', ...a.slice(1)),
        updateComment: async (...a) => guard('comment', ...a.slice(1)),
        remove: async (...a) => guard('remove', ...a.slice(1)),
        removeRevision: async (...a) => guard('removeRev', ...a.slice(1)),
        uploadRevision: async (k, o, i, f) => guard('upload', o, i, f.name),
      },
    })
  })
  const settle = async () => {
    for (let i = 0; i < 5; i++) {
      await Promise.resolve()
      flushSync()
    }
  }
  return { ops, log, selected, changed: () => changed, stop, settle }
}

test('выбор подтягивает комментарий и версии (новые первыми); сброс выбора очищает', async () => {
  const t = make()
  t.selected.value = att
  await t.settle()
  expect(t.ops.commentDraft).toBe('c')
  expect(t.ops.revs.map((r: any) => r.revision)).toEqual([2, 1])
  t.selected.value = null
  await t.settle()
  expect(t.ops.revs).toEqual([])
  t.stop()
})

test('скрытие и смена текущей версии идут на сервер и зовут onChanged', async () => {
  const t = make()
  await t.ops.toggleHidden(att as any)
  await t.ops.makeCurrent(att as any, 2)
  expect(t.log).toEqual([
    ['hide', 2, 5, true],
    ['current', 2, 5, 2],
  ])
  expect(t.changed()).toBe(2)
  expect(t.ops.busyId).toBeNull()
  t.stop()
})

test('комментарий: без изменений не сохраняется, с изменением — обрезается', async () => {
  const t = make()
  t.selected.value = att
  await t.settle()
  await t.ops.saveComment(att as any)
  expect(t.log).toEqual([])
  t.ops.commentDraft = '  новый  '
  await t.ops.saveComment(att as any)
  expect(t.log).toEqual([['comment', 2, 5, 'новый']])
  t.stop()
})

test('удаление вложения снимает выбор, версии — перечитывает; ошибка не ломает состояние', async () => {
  const t = make()
  t.selected.value = att
  await t.settle()
  t.ops.pending = { att: att as any, revision: null }
  await t.ops.confirmDelete()
  expect(t.log).toContainEqual(['remove', 2, 5])
  expect(t.log).toContainEqual(['select', null])
  expect(t.ops.pending).toBeNull()
  t.ops.pending = { att: att as any, revision: 3 }
  await t.ops.confirmDelete()
  expect(t.log).toContainEqual(['removeRev', 2, 5, 3])
  t.stop()
  const bad = make('remove')
  bad.ops.pending = { att: att as any, revision: null }
  await bad.ops.confirmDelete()
  expect(bad.ops.pending).toBeNull()
  expect(bad.ops.deleteBusy).toBe(false)
  expect(bad.changed()).toBe(0)
  bad.stop()
})

test('новая версия: файл уходит владельцу, запомненному заранее; без файла ничего', async () => {
  const t = make()
  await t.ops.uploadVersion(new File(['x'], 'v2.txt'))
  expect(t.log).toEqual([])
  t.ops.expectVersion(att as any)
  await t.ops.uploadVersion(undefined)
  t.ops.expectVersion(att as any)
  await t.ops.uploadVersion(new File(['x'], 'v2.txt'))
  expect(t.log).toEqual([['upload', 2, 5, 'v2.txt']])
  t.stop()
})
