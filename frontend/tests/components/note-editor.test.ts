import { beforeEach, expect, mock, test } from 'bun:test'

import { flushSync } from 'svelte'

import { cell, inRoot } from './root.svelte.ts'

const row = (over: any = {}) => ({
  id: 1,
  title: 'Заметка',
  description: 'текст',
  parent_id: null,
  pinned: false,
  is_readme: false,
  is_private: false,
  is_category: false,
  updated_at: '2026-01-01T00:00:00Z',
  ...over,
})
let server: Record<number, any> = {}
const puts: any[] = []
let failSave = false

const real = await import('@/shared/api/client.ts')
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  getNote: async (id: number) => structuredClone(server[id]),
  updateNote: async (id: number, patch: any) => {
    if (failSave) throw new Error('нет связи')
    puts.push([id, patch])
    const r = { ...server[id] }
    for (const [k, v] of Object.entries(patch)) r[k] = v
    r.updated_at = '2026-01-02T00:00:00Z'
    server[id] = r
    return structuredClone(r)
  },
}))
const { createNoteEditor } = await import('@/pages/notes/model/noteEditor.svelte.ts')
const { noteDrafts } = await import('@/entities/note')

const settle = async () => {
  for (let i = 0; i < 5; i++) {
    await Promise.resolve()
    flushSync()
  }
}

function make() {
  const selected = cell<number | null>(null)
  const list = cell<any[]>([])
  let changed = 0
  let ed!: ReturnType<typeof createNoteEditor>
  const stop = inRoot(() => {
    ed = createNoteEditor({ selectedId: () => selected.value, notes: () => list.value, onChanged: () => changed++ })
  })
  return {
    ed,
    stop,
    changed: () => changed,
    open: async (id: number | null) => {
      selected.value = id
      await settle()
    },
    setList: async (l: any[]) => {
      list.value = l
      await settle()
    },
  }
}

beforeEach(() => {
  server = { 1: row(), 2: row({ id: 2, title: 'Вторая' }) }
  puts.length = 0
  failSave = false
  for (const k of Object.keys(noteDrafts)) delete noteDrafts[k]
})

test('открытие подгружает форму; без правок dirty ложно', async () => {
  const t = make()
  await t.open(1)
  expect(t.ed.title).toBe('Заметка')
  expect(t.ed.loadedId).toBe(1)
  expect(t.ed.dirty).toBe(false)
  t.stop()
})

test('правка делает форму грязной и пишет черновик; уход и возврат его возвращают; revert стирает', async () => {
  const t = make()
  await t.open(1)
  t.ed.description = 'новое'
  await settle()
  expect(t.ed.dirty).toBe(true)
  expect(noteDrafts['1']?.description).toBe('новое')
  await t.open(2)
  await t.open(1)
  expect(t.ed.description).toBe('новое')
  t.ed.revert()
  await settle()
  expect(t.ed.description).toBe('текст')
  expect(noteDrafts['1']).toBeUndefined()
  t.stop()
})

test('save: пустой заголовок не уходит на сервер; успешное сохранение чистит форму и зовёт onChanged', async () => {
  const t = make()
  await t.open(1)
  t.ed.title = '  '
  await t.ed.save()
  expect(puts).toHaveLength(0)
  expect(t.ed.error).toBe('Нужен заголовок')
  t.ed.title = 'Новый'
  t.ed.description = 'тело'
  await t.ed.save()
  expect(puts[0][1]).toMatchObject({ title: 'Новый', description: 'тело', parent_id: null })
  expect(t.ed.dirty).toBe(false)
  expect(t.changed()).toBe(1)
  t.stop()
})

test('save при ошибке сервера оставляет правки и показывает ошибку', async () => {
  const t = make()
  await t.open(1)
  t.ed.description = 'x'
  failSave = true
  await t.ed.save()
  expect(t.ed.error).toBe('нет связи')
  expect(t.ed.dirty).toBe(true)
  expect(t.ed.saving).toBe(false)
  t.stop()
})

test('внешняя правка без локальных: форма подхватывает; с локальными — ждёт решения', async () => {
  const t = make()
  await t.open(1)
  server[1] = row({ description: 'снаружи', updated_at: '2026-01-05T00:00:00Z' })
  await t.setList([server[1]])
  await settle()
  expect(t.ed.description).toBe('снаружи')
  expect(t.ed.external).toBeNull()

  t.ed.description = 'моё'
  await settle()
  server[1] = row({ description: 'ещё снаружи', updated_at: '2026-01-09T00:00:00Z' })
  await t.setList([server[1]])
  expect(t.ed.description).toBe('моё')
  expect(t.ed.external?.description).toBe('ещё снаружи')
  await t.ed.adoptExternal(1)
  expect(t.ed.description).toBe('ещё снаружи')
  expect(t.ed.external).toBeNull()
  t.stop()
})

test('commitTitle сохраняет только заголовок; пустой откатывается', async () => {
  const t = make()
  await t.open(1)
  t.ed.title = ''
  await t.ed.commitTitle()
  expect(t.ed.title).toBe('Заметка')
  t.ed.title = 'Другой'
  await t.ed.commitTitle()
  expect(puts).toEqual([[1, { title: 'Другой' }]])
  expect(t.ed.saved.title).toBe('Другой')
  t.stop()
})

test('patched: быстрое сохранение поля обновляет форму и сохранённую копию, тронутый заголовок не затирается при keepEdited', async () => {
  const t = make()
  await t.open(1)
  t.ed.title = 'правлю руками'
  t.ed.patched(1, row({ title: 'из дерева' }), { title: 'из дерева' }, true)
  expect(t.ed.title).toBe('правлю руками')
  expect(t.ed.saved.title).toBe('из дерева')
  t.ed.patched(1, row({ pinned: true }), { pinned: true })
  expect(t.ed.pinned).toBe(true)
  expect(t.ed.saved.pinned).toBe(true)
  t.ed.patched(2, row(), { pinned: false })
  expect(t.ed.pinned).toBe(true)
  t.stop()
})
