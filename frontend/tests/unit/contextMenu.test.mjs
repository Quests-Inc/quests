import { expect, test } from 'bun:test'

import { contextMenuItems } from '../../src/pages/journal/model/contextMenu.ts'

const ids = (items) => items.map((i) => i.id)

test('контекстное меню квестлайна: добавить, править, удалить', () => {
  const items = contextMenuItems({ kind: 'line', lineId: 4, questId: null, stepId: null })
  expect(items[0].label).toBe('Копировать questline=4')
  expect(ids(items)).toContain('add')
  expect(items.at(-1)).toMatchObject({ id: 'delete', danger: true })
})

test('контекстное меню квеста: текущий статус отмечен, у замороженного есть «Вернуть в активные»', () => {
  const active = contextMenuItems({
    kind: 'quest',
    lineId: null,
    quest: { status: 'active' },
    questId: 7,
    stepId: null,
  })
  const status = active.find((i) => i.id === 'status')
  expect(status.children.find((c) => c.checked).id).toBe('status:active')
  expect(active.find((i) => i.id === 'delay').children.map((c) => c.id)).not.toContain('status:active')
  const frozen = contextMenuItems({
    kind: 'quest',
    lineId: null,
    quest: { status: 'frozen' },
    questId: 7,
    stepId: null,
  })
  expect(frozen.find((i) => i.id === 'delay').children.map((c) => c.id)).toContain('status:active')
})

test('контекстное меню шага: прогресс — только у многошаговых', () => {
  const single = contextMenuItems({ kind: 'step', lineId: null, questId: null, stepId: 9, step: { progress_total: 1 } })
  expect(ids(single)).not.toContain('progress')
  expect(ids(single)).toContain('complete')
  const multi = contextMenuItems({ kind: 'step', lineId: null, questId: null, stepId: 9, step: { progress_total: 5 } })
  expect(ids(multi)).toContain('progress')
  expect(contextMenuItems({ kind: 'none', lineId: null, questId: null, stepId: null })).toEqual([])
})

test('контекстное меню группы квестов: ссылки со счётчиком, статус, отсрочка, значимость; без «Редактировать» и «Удалить»', () => {
  const items = contextMenuItems({ kind: 'quests', lineId: null, questId: 3, stepId: null, count: 4 })
  expect(items[0]).toMatchObject({ id: 'copy-ids', label: 'Копировать ссылки (4)' })
  expect(ids(items)).toEqual(expect.arrayContaining(['status', 'delay', 'sig']))
  expect(ids(items)).not.toContain('edit')
  expect(ids(items)).not.toContain('delete')
  expect(items.find((i) => i.id === 'status').children.some((c) => c.checked)).toBe(false)
})
