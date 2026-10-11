import { expect, test } from 'bun:test'

import { createNoteActions } from '@/pages/notes/model/noteActions.svelte.ts'

const notes: any[] = [
  { id: 1, parent_id: null, pinned: false },
  { id: 2, parent_id: 1, pinned: true, outdated: false },
  { id: 3, parent_id: 2, is_readme: false },
]

function make(selected: number | null = 2, fail = false) {
  const calls: any[] = []
  const editor: any = {
    error: 'старая',
    detail: null,
    patched: (id: number, row: any, form: any) => calls.push(['patched', id, form]),
  }
  const a = createNoteActions({
    notes: () => notes,
    selectedId: () => selected,
    editor,
    onChanged: () => calls.push(['changed']),
    api: {
      updateNote: async (id, patch) => {
        if (fail) throw new Error('сбой')
        calls.push(['update', id, patch])
        return { id } as any
      },
      setNoteOutdated: async (id, v) => void calls.push(['outdated', id, v]),
      getNote: async (id) => ({ id, fresh: true }) as any,
    },
  })
  return { a, calls, editor }
}

test('перенос: без изменений, в себя и в потомка — ничего; допустимый — сохраняется и обновляет форму', async () => {
  const { a, calls } = make()
  await a.reparent(2, 1)
  await a.reparent(1, 1)
  await a.reparent(1, 3)
  expect(calls).toEqual([])
  await a.reparent(3, null)
  expect(calls).toEqual([['update', 3, { parent_id: null }], ['patched', 3, { parentId: '' }], ['changed']])
})

test('закрепление и флаги переключают текущее значение', async () => {
  const { a, calls } = make()
  await a.togglePin(2)
  await a.toggleFlag(3, 'is_readme')
  expect(calls).toContainEqual(['update', 2, { pinned: false }])
  expect(calls).toContainEqual(['patched', 2, { pinned: false }])
  expect(calls).toContainEqual(['update', 3, { is_readme: true }])
  expect(calls).toContainEqual(['patched', 3, { isReadme: true }])
})

test('moveUp поднимает на уровень выше; у корневой ничего не делает', async () => {
  const { a, calls } = make()
  await a.moveUp(1)
  expect(calls).toEqual([])
  await a.moveUp(3)
  expect(calls[0]).toEqual(['update', 3, { parent_id: 1 }])
})

test('ошибка сохранения кладётся в editor.error и не зовёт onChanged', async () => {
  const { a, calls, editor } = make(2, true)
  await a.togglePin(2)
  expect(editor.error).toBe('сбой')
  expect(calls).toEqual([])
})

test('toggleOutdated перечитывает открытую заметку, чужую — нет', async () => {
  const { a, editor } = make(2)
  await a.toggleOutdated(2)
  expect(editor.detail).toMatchObject({ id: 2, fresh: true })
  editor.detail = null
  await a.toggleOutdated(1)
  expect(editor.detail).toBeNull()
})

test('неизвестная заметка игнорируется', async () => {
  const { a, calls } = make()
  await a.togglePin(99)
  await a.toggleOutdated(99)
  expect(calls).toEqual([])
})
