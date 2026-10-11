import { expect, test } from 'bun:test'

import { flushSync } from 'svelte'

import { createDeletions } from '@/app/model/deletions.svelte.ts'

function make(over: Partial<Parameters<typeof createDeletions>[0]['api']> = {}) {
  const log: any[] = []
  let selected: { id: number } | null = { id: 1 }
  let lineId: number | null = 7
  const d = createDeletions({
    selected: () => selected,
    selectQuest: (id) => {
      selected = { id }
      log.push(['select', id])
    },
    lineId: () => lineId,
    clearLine: () => {
      lineId = null
      log.push(['clearLine'])
    },
    reload: async () => void log.push(['reload']),
    onQuestDeleted: (id) => log.push(['deleted', id]),
    setError: (m) => log.push(['error', m]),
    api: {
      deleteQuest: async (id) => void log.push(['api.quest', id]),
      deleteQuestStep: async (q, s) => void log.push(['api.step', q, s]),
      deleteQuestline: async (id) => void log.push(['api.line', id]),
      ...over,
    },
  })
  return { d, log }
}
const tick = async () => {
  for (let i = 0; i < 4; i++) {
    await Promise.resolve()
    flushSync()
  }
}

test('квест: вопрос выбирает чужой квест; удаление ждёт анимацию и только потом идёт на сервер', async () => {
  const { d, log } = make()
  d.askQuest({ id: 5 })
  expect(d.questAsk).toBe(true)
  expect(log).toContainEqual(['select', 5])
  const p = d.confirmQuest()
  await tick()
  expect(d.animActive).toBe(true)
  expect(log.some((e) => e[0] === 'api.quest')).toBe(false)
  d.animDone()
  await p
  expect(log).toContainEqual(['api.quest', 5])
  expect(log.at(-1)).toEqual(['deleted', 5])
  expect(d.questAsk).toBe(false)
  expect(d.animActive).toBe(false)
  expect(d.questBusy).toBe(false)
})

test('квест: ошибка сервера оставляет вопрос открытым и пишет ошибку; cancel во время работы игнорируется', async () => {
  const { d, log } = make({
    deleteQuest: async () => {
      throw new Error('нет прав')
    },
  })
  d.askQuest()
  const p = d.confirmQuest()
  await tick()
  d.cancelQuest()
  expect(d.questAsk).toBe(true)
  d.animDone()
  await p
  expect(log).toContainEqual(['error', 'нет прав'])
  expect(d.questAsk).toBe(true)
  d.cancelQuest()
  expect(d.questAsk).toBe(false)
})

test('шаг: удаление перечитывает список и закрывает вопрос; отмена сбрасывает шаг', async () => {
  const { d, log } = make()
  d.askStep({ questId: 1, stepId: 9, title: 'ш' })
  expect(d.stepOpen).toBe(true)
  await d.confirmStep()
  expect(log).toEqual([['error', ''], ['api.step', 1, 9], ['reload']])
  expect(d.stepOpen).toBe(false)
  expect(d.step).toBeNull()
  d.askStep({ questId: 1, stepId: 9, title: 'ш' })
  d.cancelStep()
  expect(d.step).toBeNull()
})

test('квестлайн: берёт id у меню, после удаления сбрасывает его; без id ничего не делает', async () => {
  const { d, log } = make()
  d.askLine()
  expect(d.lineAsk).toBe(true)
  await d.confirmLine()
  expect(log).toContainEqual(['api.line', 7])
  expect(log).toContainEqual(['clearLine'])
  expect(d.lineAsk).toBe(false)
  const n = log.length
  await d.confirmLine()
  expect(log).toHaveLength(n)
})
