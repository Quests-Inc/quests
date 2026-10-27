import { expect, test } from 'bun:test'

import { moveSearchItems, noteGroupMenuItems, topLevelIds } from '@/entities/note'
import { createNoteGroup } from '@/pages/notes/model/noteGroup.svelte.ts'

import { inRoot } from './root.svelte.ts'

const notes = [
  { id: 1, title: 'Корень А', parent_id: null },
  { id: 2, title: 'Дитя', parent_id: 1 },
  { id: 3, title: 'Внук', parent_id: 2, pinned: true },
  { id: 4, title: 'Другая', parent_id: null, pinned: true },
  { id: 5, title: 'Пятая', parent_id: null, is_private: true },
]

function make(selected: number | null = null, patch: Record<number, object> = {}) {
  const log: any[] = []
  let grp!: ReturnType<typeof createNoteGroup>
  const state = notes.map((n) => ({ ...n, ...patch[n.id] }))
  inRoot(() => {
    grp = createNoteGroup({
      notes: () => state,
      selectedId: () => selected,
      isDirty: (id) => id === 4,
      actions: {
        reparent: async (id, p) => void log.push(['reparent', id, p]),
        setPin: async (id, v) => void log.push(['pin', id, v]),
        setFlag: async (id, f, v) => void log.push(['flag', id, f, v]),
        setOutdated: async (id, v) => void log.push(['outdated', id, v]),
      },
      deleteNote: async (id) => {
        if (id === 2) throw new Error('boom')
        log.push(['delete', id])
      },
      afterDelete: (ids) => log.push(['after', ids]),
      onChanged: () => log.push(['changed']),
      copy: async (t) => void log.push(['copy', t]),
      downloadMarkdown: (n) => log.push(['md', n]),
      toast: (m, k, a) => log.push(['toast', k, m, a?.label ?? null]),
    })
  })
  const ev = { clientX: 1, clientY: 2 }
  const pick = (...ids: number[]) => ids.forEach((id) => grp.multi.click({ ctrlKey: true }, id, [1, 2, 3, 4, 5]))
  return { grp, log, ev, pick }
}

test('topLevelIds: потомок выделенного предка не считается отдельно', () => {
  expect(topLevelIds(notes, [1, 2, 3, 4])).toEqual([1, 4])
  expect(topLevelIds(notes, [3, 4])).toEqual([3, 4])
})

test('moveSearchItems: корень и всё вне поддеревьев переносимых; в подписи — родитель', () => {
  const items = moveSearchItems([1], notes)
  expect(items.map((i) => i.id)).toEqual(['move:root', 'move:4', 'move:5'])
  const all = moveSearchItems([5], notes)
  expect(all.find((i) => i.id === 'move:2')?.label).toBe('Дитя — Корень А')
})

test('пункты флагов зависят от того, у всех ли они уже стоят', () => {
  const both = noteGroupMenuItems({ rows: [notes[2], notes[3]], moveSearch: [] })
  expect(both.find((i) => i.id === 'flags')?.children?.[0].label).toBe('Снять закреп')
  const mixed = noteGroupMenuItems({ rows: [notes[1], notes[3]], moveSearch: [] })
  expect(mixed.find((i) => i.id === 'flags')?.children?.[0].label).toBe('Закрепить')
  expect(mixed[0].label).toBe('Копировать ссылки (2)')
  expect(mixed.find((i) => i.id === 'delete')?.label).toBe('Удалить (2)')
})

test('меню группы открывается только на выделенной заметке при выборе больше одной', () => {
  const t = make()
  expect(t.grp.openMenu(t.ev, 1)).toBe(false)
  t.pick(1, 2)
  expect(t.grp.openMenu(t.ev, 4)).toBe(false)
  expect(t.grp.openMenu(t.ev, 2)).toBe(true)
  expect(t.grp.open).toBe(true)
  expect(t.grp.items[0].label).toBe('Копировать ссылки (2)')
})

test('флаг ставится только тем, у кого его нет, список перечитывается один раз', async () => {
  const t = make()
  t.pick(3, 4, 2)
  t.grp.openMenu(t.ev, 3)
  await t.grp.select('pin')
  expect(t.log.filter((e) => e[0] === 'pin')).toEqual([['pin', 2, true]])
  expect(t.log.filter((e) => e[0] === 'changed')).toHaveLength(1)
})

test('перенос: только верхние из выделенных, не в самих себя', async () => {
  const t = make()
  t.pick(1, 2, 4)
  t.grp.openMenu(t.ev, 1)
  await t.grp.select('move:5')
  expect(t.log.filter((e) => e[0] === 'reparent')).toEqual([
    ['reparent', 1, 5],
    ['reparent', 4, 5],
  ])
  t.log.length = 0
  await t.grp.select('move:4')
  expect(t.log.filter((e) => e[0] === 'reparent')).toEqual([['reparent', 1, 4]])
})

test('PRIVATE: поставить можно сразу, снять — только после подтверждения', async () => {
  const mixed = make()
  mixed.pick(5, 4)
  mixed.grp.openMenu(mixed.ev, 5)
  await mixed.grp.select('private')
  expect(mixed.log.filter((e) => e[0] === 'flag')).toEqual([['flag', 4, 'is_private', true]])
  expect(mixed.grp.confirm).toBeNull()

  const both = make(null, { 4: { is_private: true } })
  both.pick(5, 4)
  both.grp.openMenu(both.ev, 5)
  await both.grp.select('private')
  expect(both.grp.confirm?.title).toBe('Снять приватность?')
  expect(both.log.filter((e) => e[0] === 'flag')).toEqual([])
  await both.grp.doConfirm()
  expect(both.log.filter((e) => e[0] === 'flag')).toEqual([
    ['flag', 5, 'is_private', false],
    ['flag', 4, 'is_private', false],
  ])
})

test('удаление: подтверждение с перечнем и пометкой о черновике; сбой одного не рвёт остальных', async () => {
  const t = make(4) // открыта 4: первый Ctrl-клик берёт её с собой
  t.pick(2, 3)
  t.grp.openMenu(t.ev, 3)
  await t.grp.select('delete')
  expect(t.grp.confirm?.title).toBe('Удалить заметки (3)?')
  for (const name of ['«Дитя»', '«Внук»', '«Другая»']) expect(t.grp.confirm?.message).toContain(name)
  expect(t.grp.confirm?.message).toContain('Несохранённые правки')
  await t.grp.doConfirm()
  expect(t.log.filter((e) => e[0] === 'delete')).toEqual([
    ['delete', 4],
    ['delete', 3],
  ])
  expect(t.log.find((e) => e[0] === 'after')?.[1]).toEqual([4, 3])
  // the toast of the delete carries «Отменить» for what was erased
  expect(t.log.find((e) => e[0] === 'toast' && e[1] === 'error')?.[3]).toBe('Отменить')
  expect(t.log.some((e) => e[0] === 'toast' && e[1] === 'error' && String(e[2]).includes('не удалось: 1'))).toBe(true)
  expect(t.grp.confirm).toBeNull()
  expect(t.grp.multi.size).toBe(0)
})

test('ссылки и Markdown одним файлом в порядке дерева, без вложенных повторов', async () => {
  const t = make()
  t.grp.remember([1, 2, 3, 4, 5])
  t.pick(4, 1, 2)
  t.grp.openMenu(t.ev, 1)
  await t.grp.select('copy-ids')
  expect(t.log.find((e) => e[0] === 'copy')?.[1]).toBe('note=4\nnote=1\nnote=2')
  await t.grp.select('export-md')
  const md = t.log.find((e) => e[0] === 'md')?.[1]
  expect(md.title).toBe('Заметки (2)')
  expect(md.description.indexOf('# Корень А')).toBeLessThan(md.description.indexOf('# Другая'))
})
