import { expect, test } from 'bun:test'

import { createAttachmentGroup } from '@/pages/attachments/model/attachmentGroup.svelte.ts'

import { inRoot } from './root.svelte.ts'

function make() {
  const log: any[] = []
  const rows = [
    { id: 1, owner_id: 10, owner_type: 'quest', hidden: false },
    { id: 2, owner_id: 10, owner_type: 'quest', hidden: true },
    { id: 3, owner_id: 11, owner_type: 'note', hidden: false },
  ]
  let grp!: ReturnType<typeof createAttachmentGroup>
  const stop = inRoot(() => {
    grp = createAttachmentGroup({
      rows: () => rows,
      selectedId: () => null,
      download: (a) => log.push(['download', a.id]),
      setHidden: async (k, o, id, h) => void log.push(['hide', k, o, id, h]),
      remove: async (k, o, id) => {
        if (id === 3) throw new Error('нет')
        log.push(['remove', k, o, id])
      },
      onChanged: () => log.push(['changed']),
      onDeleted: (ids) => log.push(['deleted', ids]),
    })
  })
  return { grp, log, stop }
}

test('Ctrl и Shift выбирают вложения, Ctrl+A выбирает все видимые и снимает при повторе', () => {
  const t = make()
  const order = [1, 2, 3]
  expect(t.grp.click({ ctrlKey: true }, 1, order)).toBe(true)
  expect(t.grp.click({ shiftKey: true }, 3, order)).toBe(true)
  expect([...t.grp.multi.picked].sort()).toEqual([1, 2, 3])
  t.grp.multi.clear()
  t.grp.selectAll(order)
  expect(t.grp.multi.size).toBe(3)
  t.grp.selectAll(order)
  expect(t.grp.multi.size).toBe(0)
  expect(t.grp.click({}, 2, order)).toBe(false)
  t.stop()
})

test('меню группы и массовые действия: скачать, скрыть/вернуть, удалить с подтверждением', async () => {
  const t = make()
  expect(t.grp.items([1, 2]).map((i) => i.label)).toEqual([
    'Скачать (2)',
    'Скрыть (2)',
    'Снять выделение',
    undefined,
    'Удалить (2)…',
  ])
  expect(t.grp.items([2]).find((i) => i.id === 'group-hide')?.label).toBe('Вернуть (1)')
  await t.grp.run('group-download', [1, 2])
  await t.grp.run('group-hide', [1, 2])
  expect(t.log).toEqual([
    ['download', 1],
    ['download', 2],
    ['hide', 'quest', 10, 1, true],
    ['hide', 'quest', 10, 2, true],
    ['changed'],
  ])
  t.log.length = 0
  await t.grp.run('group-delete', [1, 3])
  expect(t.grp.ask).toEqual([1, 3])
  await t.grp.confirmDelete()
  expect(t.log).toEqual([['remove', 'quest', 10, 1], ['changed'], ['deleted', [1]]])
  expect(t.grp.ask).toEqual([])
  t.stop()
})
