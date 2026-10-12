import { expect, test } from 'bun:test'

import { createNoteScroll, recalledScroll, rememberScroll } from '../../src/pages/notes/model/noteScroll.ts'

test('позиция прокрутки запоминается по заметке и отдаётся обратно', () => {
  rememberScroll(7, 420)
  expect(recalledScroll(7)).toBe(420)
  expect(recalledScroll(8)).toBeNull()
})

test('возврат наверх стирает запись, а не хранит ноль', () => {
  rememberScroll(9, 100)
  rememberScroll(9, 0)
  expect(recalledScroll(9)).toBeNull()
})

test('память ограничена: самые старые записи вытесняются', () => {
  for (let i = 1000; i < 1100; i++) rememberScroll(i, i)
  expect(recalledScroll(1099)).toBe(1099)
  expect(recalledScroll(1000)).toBeNull()
})

test('createNoteScroll: после показа заметки страница возвращается на запомненное место, один раз', () => {
  const frames = []
  globalThis.requestAnimationFrame = (f) => (frames.push(f), 0)
  const page = { scrollTop: 0 }
  let shown = 5
  const sel = { id: 5 }
  const s = createNoteScroll({ shownId: () => shown, selectedId: () => sel.id, page: () => page })
  rememberScroll(5, 300)
  s.sync()
  frames.shift()()
  expect(page.scrollTop).toBe(300)
  page.scrollTop = 10
  s.sync()
  expect(frames).toHaveLength(0)
  // прокрутка пишется в память только для показанной заметки
  s.onScroll()
  expect(recalledScroll(5)).toBe(10)
  shown = 6
  page.scrollTop = 999
  s.onScroll()
  expect(recalledScroll(6)).toBeNull()
})
