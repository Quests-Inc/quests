import { expect, test } from 'bun:test'

import { createJournalMenu } from '@/pages/journal/model/journalMenu.svelte.ts'

import { inRoot } from './root.svelte.ts'

function make(picked: number[] = []) {
  const log: any[] = []
  const cleared: number[] = []
  const quests = [
    { id: 1, status: 'active', pinned: true },
    { id: 2, status: 'frozen', pinned: true },
    { id: 3, status: 'failed', failure_inbox: true },
  ]
  const step = { id: 10, title: 'шаг', progress_total: 4 }
  let selected: number | null = null
  let menu!: ReturnType<typeof createJournalMenu>
  const stop = inRoot(() => {
    menu = createJournalMenu({
      quests: () => quests,
      picked: (id) => (picked.length > 1 && picked.includes(id) ? picked : [id]),
      clearPicked: () => void cleared.push(1),
      copyMany: (k, ids) => log.push(['copyMany', k, ids]),
      questlines: () => [{ id: 7, title: 'Линия', category_id: 3 }],
      findStep: (id) => (id === 10 ? { step, questId: 1 } : null),
      selectQuest: (id) => (selected = id),
      selectedQuestId: () => selected,
      copyId: (k, id) => log.push(['copy', k, id]),
      openGraph: (t, id) => log.push(['graph', t, id]),
      addStep: (q) => log.push(['addStep', q]),
      askDeleteMany: (ids) => log.push(['askMany', ids]),
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
        pin: (q, v) => log.push(['pin', q.id, v]),
        pinMany: (qs, v) => log.push(['pinMany', qs.map((q) => q.id), v]),
        moveMany: (qs, l) => log.push(['moveMany', qs.map((q) => q.id), l]),
        postponeMany: (qs, m) => log.push(['postponeMany', qs.map((q) => q.id), m]),
        patchStatusMany: (qs, s) => log.push(['statusMany', qs.map((q) => q.id), s]),
        patchSignificanceMany: (qs, v) => log.push(['sigMany', qs.map((q) => q.id), v]),
      },
      step: {
        bump: (s, d, q) => log.push(['bump', s.id, d, q]),
        set: (s, v, q) => log.push(['set', s.id, v, q]),
      },
    })
  })
  const ev = { preventDefault() {}, stopPropagation() {}, clientX: 5, clientY: 6 }
  return { menu, log, cleared, ev, stop, selected: () => selected }
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

test('меню на выделенной группе: действия идут на всех, открытый квест не меняется, ПКМ мимо группы — один квест и сброс', async () => {
  const t = make([1, 2, 3])
  t.menu.openQuest(t.ev, { id: 2 })
  expect(t.selected()).toBeNull()
  expect(t.menu.items[0].label).toBe('Копировать ссылки (3)')
  await t.menu.select('copy-ids')
  await t.menu.select('status:completed')
  await t.menu.select('delay-15')
  await t.menu.select('sig:epic')
  expect(t.log).toEqual([
    ['copyMany', 'quest', [1, 2, 3]],
    ['statusMany', [1, 2, 3], 'completed'],
    ['postponeMany', [1, 2, 3], 15],
    ['sigMany', [1, 2, 3], 'epic'],
  ])
  t.menu.close()
  t.log.length = 0
  t.menu.openQuest(t.ev, { id: 9 })
  expect(t.cleared).toHaveLength(1)
  expect(t.log).toEqual([])
  expect(t.selected()).toBe(9)
  t.stop()
})

test('«Открыть на карте», «Добавить шаг», «Закрепить» у квеста, шага и квестлайна', async () => {
  const t = make()
  t.menu.openQuest(t.ev, { id: 1 })
  const ids = t.menu.items.map((i) => i.id)
  expect(ids.slice(0, 2)).toEqual(['copy-id', 'graph'])
  expect(t.menu.items.find((i) => i.id === 'pin')?.label).toBe('Снять закреп')
  await t.menu.select('graph')
  await t.menu.select('add-step')
  await t.menu.select('pin')
  t.menu.openStep(t.ev, { id: 10 })
  await t.menu.select('graph')
  await t.menu.select('add-step')
  t.menu.openLine(t.ev, { id: 7 })
  await t.menu.select('graph')
  expect(t.log).toEqual([
    ['graph', 'quest', 1],
    ['addStep', 1],
    ['pin', 1, false],
    ['graph', 'quest', 1],
    ['addStep', 1],
    ['graph', 'questline', 7],
  ])
})

test('меню группы: закрепить, переместить, удалить, снять выделение', async () => {
  const t = make([1, 2])
  t.menu.openQuest(t.ev, { id: 1 })
  const ids = t.menu.items.map((i) => i.id)
  expect(ids).toContain('delete-many')
  expect(t.menu.items.find((i) => i.id === 'delete-many')?.label).toBe('Удалить (2)')
  expect(t.menu.items.find((i) => i.id === 'pin')?.label).toBe('Снять закреп')
  expect(t.menu.items.find((i) => i.id === 'move')?.children?.map((c) => c.id)).toEqual(['move:none', 'move:7'])
  await t.menu.select('pin')
  await t.menu.select('move:7')
  await t.menu.select('move:none')
  await t.menu.select('delete-many')
  await t.menu.select('clear-picked')
  expect(t.log).toEqual([
    ['pinMany', [1, 2], false],
    ['moveMany', [1, 2], 7],
    ['moveMany', [1, 2], null],
    ['askMany', [1, 2]],
  ])
  expect(t.cleared.length).toBeGreaterThan(0)
})
