import { expect, test } from 'bun:test'

import { createJournalMenu } from '@/pages/journal/model/journalMenu.svelte.ts'

import { inRoot } from './root.svelte.ts'

function make() {
  const log: any[] = []
  const quests = [
    { id: 1, status: 'active' },
    { id: 2, status: 'frozen' },
    { id: 3, status: 'failed', failure_inbox: true },
  ]
  const step = { id: 10, title: 'шаг', progress_total: 4 }
  let selected: number | null = null
  let menu!: ReturnType<typeof createJournalMenu>
  const stop = inRoot(() => {
    menu = createJournalMenu({
      quests: () => quests,
      questlines: () => [{ id: 7, category_id: 3 }],
      findStep: (id) => (id === 10 ? { step, questId: 1 } : null),
      selectQuest: (id) => (selected = id),
      selectedQuestId: () => selected,
      copyId: (k, id) => log.push(['copy', k, id]),
      openSecrets: (o) => log.push(['secrets', o]),
      openCreate: (d) => log.push(['create', d]),
      editQuest: (q) => log.push(['editQuest', q.id]),
      editLine: (l) => log.push(['editLine', l.id]),
      askDeleteLine: () => log.push(['askLine']),
      askDeleteQuest: (q) => log.push(['askQuest', q.id]),
      editStep: (q, s) => log.push(['editStep', q, s.id]),
      askDeleteStep: (d) => log.push(['askStep', d]),
      quest: {
        postpone: (q, m) => log.push(['postpone', q.id, m]),
        patchStatus: (q, s) => log.push(['status', q.id, s]),
        patchSignificance: (q, v) => log.push(['sig', q.id, v]),
        exportQuest: (q, m) => log.push(['export', q.id, m]),
        ackFailure: (q) => log.push(['ack', q.id]),
      },
      step: {
        bump: (s, d, q) => log.push(['bump', s.id, d, q]),
        set: (s, v, q) => log.push(['set', s.id, v, q]),
      },
    })
  })
  const ev = { preventDefault() {}, stopPropagation() {}, clientX: 5, clientY: 6 }
  return { menu, log, ev, stop, selected: () => selected }
}

test('открытие по квесту выбирает его и запоминает точку; close закрывает', () => {
  const t = make()
  t.menu.openQuest(t.ev, { id: 2 })
  expect(t.menu.open).toBe(true)
  expect([t.menu.x, t.menu.y]).toEqual([5, 6])
  expect(t.selected()).toBe(2)
  expect(t.menu.items.length).toBeGreaterThan(0)
  t.menu.close()
  expect(t.menu.open).toBe(false)
  t.stop()
})

test('пункты квеста: статус меняется только на другой, отсрочка берёт минуты, удаление спрашивает', async () => {
  const t = make()
  t.menu.openQuest(t.ev, { id: 1 })
  await t.menu.select('status:active')
  await t.menu.select('status:done')
  await t.menu.select('delay-30')
  await t.menu.select('delay-0')
  await t.menu.select('export-pdf')
  await t.menu.select('delete')
  await t.menu.select('copy-id')
  expect(t.log).toEqual([
    ['status', 1, 'done'],
    ['postpone', 1, 30],
    ['export', 1, 'rich'],
    ['askQuest', 1],
    ['copy', 'quest', 1],
  ])
  t.stop()
})

test('проваленный квест в инбоксе: скрыть из провалов', async () => {
  const t = make()
  t.menu.openQuest(t.ev, { id: 3 })
  expect(t.menu.items.some((i) => i.id === 'ack-failure')).toBe(true)
  await t.menu.select('ack-failure')
  expect(t.log).toEqual([['ack', 3]])
  t.stop()
})

test('пункты шага: изменение прогресса идёт в квест шага, удаление несёт заголовок', async () => {
  const t = make()
  t.menu.openStep(t.ev, { id: 10 })
  for (const a of ['inc', 'dec', 'complete', 'reset', 'edit', 'delete']) await t.menu.select(a)
  expect(t.log).toEqual([
    ['bump', 10, 1, 1],
    ['bump', 10, -1, 1],
    ['set', 10, 4, 1],
    ['set', 10, 0, 1],
    ['editStep', 1, 10],
    ['askStep', { questId: 1, stepId: 10, title: 'шаг' }],
  ])
  t.stop()
})

test('пункты квестлайна: «добавить» переносит категорию, неизвестный квестлайн игнорируется, lineId сбрасывается', async () => {
  const t = make()
  t.menu.openLine(t.ev, { id: 7 })
  await t.menu.select('add')
  await t.menu.select('delete')
  expect(t.log).toEqual([['create', { questline_id: 7, category_id: 3 }], ['askLine']])
  expect(t.menu.lineId).toBe(7)
  t.menu.clearLine()
  expect(t.menu.lineId).toBeNull()
  t.menu.openLine(t.ev, { id: 99 })
  await t.menu.select('edit')
  expect(t.log).toHaveLength(2)
  t.stop()
})
