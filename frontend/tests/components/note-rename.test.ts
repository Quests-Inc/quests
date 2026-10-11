import { expect, test } from 'bun:test'

import { createNoteRename } from '@/pages/notes/model/noteRename.svelte.ts'
import { toasts } from '@/shared/lib/toasts.svelte.ts'

function make(update: (id: number, patch: { title: string }) => Promise<unknown> = async (id, p) => ({ id, ...p })) {
  const notes = new Map([[1, { id: 1, title: 'Старое' }]])
  const saved: any[] = []
  const errors: string[] = []
  const updates: any[] = []
  const r = createNoteRename({
    noteById: (id) => notes.get(id) ?? null,
    rawTitle: (n) => `черновик:${n.title}`,
    updateNote: async (id, patch) => {
      updates.push([id, patch])
      return update(id, patch)
    },
    onSaved: (id, t, row) => saved.push([id, t, row]),
    setError: (m) => errors.push(m),
  })
  return { r, saved, errors, updates }
}

test('start: поле открывается с названием (с учётом черновика); cancel закрывает', () => {
  const { r } = make()
  r.start(1)
  expect(r.id).toBe(1)
  expect(r.value).toBe('черновик:Старое')
  r.cancel()
  expect([r.id, r.value]).toEqual([null, ''])
  r.start(99)
  expect(r.id).toBeNull()
})

test('commit: новое название уходит на сервер и в onSaved; то же или пустое — ничего', async () => {
  const { r, saved, updates } = make()
  r.start(1)
  r.value = '  Новое  '
  await r.commit()
  expect(updates).toEqual([[1, { title: 'Новое' }]])
  expect(saved).toEqual([[1, 'Новое', { id: 1, title: 'Новое' }]])
  expect(r.id).toBeNull()
  r.start(1)
  r.value = 'Старое'
  await r.commit()
  r.start(1)
  r.value = '   '
  await r.commit()
  expect(updates).toHaveLength(1)
})

test('ошибка сервера: сообщение в страницу и тост, редактирование закрыто', async () => {
  const { r, errors } = make(async () => {
    throw new Error('занято')
  })
  r.start(1)
  r.value = 'Другое'
  await r.commit()
  expect(errors).toEqual(['занято'])
  expect(toasts.at(-1)).toMatchObject({ kind: 'error', message: 'занято' })
  expect(r.id).toBeNull()
})

test('клавиши: Enter снимает фокус (коммит по blur), Esc отменяет; событие не всплывает', () => {
  const { r } = make()
  r.start(1)
  const log: string[] = []
  const ev = (key: string) =>
    ({
      key,
      preventDefault: () => log.push('prevent'),
      stopPropagation: () => log.push('stop'),
    }) as unknown as KeyboardEvent
  r.onKeydown(ev('Escape'))
  expect(r.id).toBeNull()
  expect(log).toEqual(['prevent', 'stop'])
  log.length = 0
  r.onKeydown(ev('a'))
  expect(log).toEqual(['stop'])
})
