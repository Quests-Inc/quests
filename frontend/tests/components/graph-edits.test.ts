import { beforeEach, expect, mock, test } from 'bun:test'

const puts: any[] = []
const real = await import('@/shared/api/graph.ts')
mock.module('@/shared/api/graph.ts', () => ({
  ...real,
  putGraphDraftOp: async (id: number, p: any) => void puts.push([id, p]),
}))
const realNotes = await import('@/shared/api/notes.ts')
mock.module('@/shared/api/notes.ts', () => ({
  ...realNotes,
  getNote: async (id: number) => ({ id, title: 'Полная', description: 'весь текст' }),
}))
const realQuests = await import('@/shared/api/quests.ts')
mock.module('@/shared/api/quests.ts', () => ({
  ...realQuests,
  getQuestFull: async (id: number) => ({ id, title: 'Q', steps: [{ id: 5, title: 'шаг' }] }),
}))
const { createGraphEdits } = await import('@/pages/graph/model/edits.svelte.ts')

let ops: any[] = []
const added: [string, any][] = []
let reloads = 0
let nextId = 1
let edges: any[] = []

function make() {
  return createGraphEdits({
    ops: () => ops,
    addOp: async (k, p) => {
      added.push([k, p])
      return { id: nextId++ }
    },
    reloadDraft: async () => void reloads++,
    edges: () => edges,
    noteTitles: () => ['Новая заметка'],
  })
}

beforeEach(() => {
  ops = []
  added.length = 0
  puts.length = 0
  reloads = 0
  nextId = 1
  edges = []
})

test('новая сущность: шаг с владельцем, квестлайн, заметка со ссылкой', async () => {
  const ed = make()
  ed.openEntity('step', { x: 1, y: 2 }, { owner: 'quest:3' })
  await ed.submitEntity({ title: 'A' })
  ed.openEntity('questline', { x: 0, y: 0 })
  await ed.submitEntity({ title: 'L' })
  expect(added).toEqual([
    ['create_step', { fields: { title: 'A' }, quest: 'quest:3', pos: { x: 1, y: 2 } }],
    ['create_questline', { fields: { title: 'L' }, pos: { x: 0, y: 0 } }],
  ])
  expect(ed.entityModal.open).toBe(true)
  ed.closeEntity()
  expect(ed.entityModal.open).toBe(false)
})

test('createNoteHere: заголовок уникален, ссылка с карточки-источника, сразу режим правки', async () => {
  const ed = make()
  await ed.createNoteHere({ x: 5, y: 6 }, 'quest:1')
  expect(added[0][0]).toBe('create_note')
  expect(added[0][1].fields.title).not.toBe('Новая заметка')
  expect(added[1]).toEqual(['add_ref', { from: 'quest:1', to: 'note:t1' }])
  // правка черновой заметки ищет свою операцию в журнале
  expect(ed.editing).toBeNull()
  ops = [{ id: 1, payload: { fields: { title: 'T', description: 'D' } } }]
  await ed.startEditNote({ key: 'note:t1', draft: true, op: 1, title: 'T' }, true)
  expect(ed.editing).toMatchObject({ key: 'note:t1', text: 'D', opId: 1, fresh: true })
})

test('сохранённая заметка правится по полному тексту; finishEdit без изменений ничего не пишет', async () => {
  const ed = make()
  await ed.startEditNote({ key: 'note:9', id: 9, title: 'кратко' })
  expect(ed.editing).toMatchObject({ title: 'Полная', text: 'весь текст', opId: null })
  await ed.finishEdit(true)
  expect(added).toEqual([])
  await ed.startEditNote({ key: 'note:9', id: 9, title: 'кратко' })
  ed.editing!.text = 'новый'
  await ed.finishEdit(true)
  expect(added).toEqual([['edit_note', { id: 'note:9', fields: { description: 'новый' } }]])
  expect(ed.editing).toBeNull()
})

test('черновая заметка дописывает поля в свою операцию и перечитывает журнал', async () => {
  const ed = make()
  ops = [{ id: 4, payload: { fields: { title: 'T', description: 'D' } } }]
  await ed.startEditNote({ key: 'note:t4', draft: true, op: 4, title: 'T' })
  ed.editing!.text = 'D2'
  await ed.finishEdit(true)
  expect(puts).toEqual([[4, { fields: { title: 'T', description: 'D2' } }]])
  expect(reloads).toBe(1)
})

test('отмена правки (saveIt=false) сбрасывает editing без записи', async () => {
  const ed = make()
  await ed.startEditNote({ key: 'note:9', id: 9, title: 'x' })
  await ed.finishEdit(false)
  expect(ed.editing).toBeNull()
  expect(added).toEqual([])
})

test('правка сущности из черновика открывает малую форму с заголовком по виду', async () => {
  const ed = make()
  ops = [{ id: 2, payload: { fields: { title: 'Ш', description: 'о' } } }]
  await ed.startEdit({ key: 'step:t2', type: 'step', draft: true, op: 2, title: 'Ш' }, { x: 3, y: 4 })
  expect(ed.entityModal.edit).toMatchObject({ opId: 2, title: 'Ш', heading: 'Изменить шаг' })
  await ed.submitEntity({ title: 'Ш2' })
  expect(puts).toEqual([[2, { fields: { title: 'Ш2', description: 'о' } }]])
})

test('правка сохранённых квеста и шага открывает обычные формы; шаг ищет квест по связи', async () => {
  const ed = make()
  await ed.startEdit({ key: 'quest:1', type: 'quest', id: 1, title: 'Q' }, { x: 0, y: 0 })
  expect(ed.editQuest.open).toBe(true)
  expect(ed.editQuest.quest.id).toBe(1)
  await ed.startEdit({ key: 'step:5', type: 'step', id: 5, title: 'шаг' }, { x: 0, y: 0 })
  expect(ed.editStep.open).toBe(false) // связи с квестом нет
  edges = [{ kind: 'structure', from: 'quest:1', to: 'step:5' }]
  await ed.startEdit({ key: 'step:5', type: 'step', id: 5, title: 'шаг' }, { x: 0, y: 0 })
  expect(ed.editStep).toMatchObject({ open: true, questId: 1 })
  ed.closeEditStep()
  ed.closeEditQuest()
  expect(ed.editStep.open || ed.editQuest.open).toBe(false)
})

test('draftEditQuest пишет только изменённые поля', async () => {
  const ed = make()
  await ed.startEdit({ key: 'quest:1', type: 'quest', id: 1, title: 'Q' }, { x: 0, y: 0 })
  await ed.draftEditQuest({ title: 'Q' })
  expect(added).toEqual([])
  await ed.draftEditQuest({ title: 'Другое' })
  expect(added).toEqual([['edit_quest', { id: 'quest:1', fields: { title: 'Другое' } }]])
})
