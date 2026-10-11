import { expect, test } from 'bun:test'

import { clampProgress, createStepProgress, questIdOfStep, stepRef } from '@/pages/journal/model/stepProgress.svelte.ts'

const quests = [
  { id: 1, steps: [{ id: 10, title: 'a', progress_current: 1, progress_total: 3 }] },
  { id: 2, steps: [{ id: 20, progress_current: 0, progress_total: 1 }] },
]

function make(update: (q: number, s: number, p: any) => Promise<any> = async (q) => ({ id: q })) {
  const calls: any[] = []
  const applied: any[] = []
  const errors: string[] = []
  const sp = createStepProgress({
    quests: () => quests,
    selectedQuestId: () => 1,
    updateQuestStep: async (q, s, p) => {
      calls.push([q, s, p])
      return update(q, s, p)
    },
    applyQuest: (quest, opts) => applied.push([quest, opts]),
    setError: (m) => errors.push(m),
  })
  return { sp, calls, applied, errors }
}

test('clampProgress, questIdOfStep, stepRef', () => {
  expect(clampProgress('2.6', 5)).toBe(3)
  expect(clampProgress(99, 3)).toBe(3)
  expect(clampProgress(-4, 3)).toBe(0)
  expect(clampProgress('abc', 3)).toBeNaN()
  expect(questIdOfStep(quests, 20)).toBe(2)
  expect(questIdOfStep(quests, 99)).toBeNull()
  expect(stepRef(quests, 10)).toMatchObject({ questId: 1, step: { title: 'a' } })
})

test('set: ограничивает значение, шлёт запрос, применяет ответ без смены выбора', async () => {
  const { sp, calls, applied } = make()
  await sp.set(quests[0].steps[0], 99, 1)
  expect(calls).toEqual([[1, 10, { progress_current: 3 }]])
  expect(applied[0][1]).toEqual({ select: false })
  expect(sp.busyId).toBeNull()
})

test('set: то же значение — запроса нет, редактирование закрывается; bump двигает на delta', async () => {
  const { sp, calls } = make()
  const step = quests[0].steps[0]
  sp.begin(step, 1)
  expect([sp.editId, sp.editValue]).toEqual([10, '1'])
  await sp.set(step, 1, 1)
  expect(calls).toEqual([])
  expect(sp.editId).toBeNull()
  await sp.bump(step, 1, 1)
  expect(calls).toEqual([[1, 10, { progress_current: 2 }]])
})

test('клавиши и потеря фокуса: Enter сохраняет введённое, Esc отменяет, blur чужого шага игнорируется', async () => {
  const { sp, calls } = make()
  const step = quests[0].steps[0]
  sp.begin(step)
  sp.editValue = '3'
  sp.onKeydown({ key: 'Escape', preventDefault() {} }, step)
  expect(sp.editId).toBeNull()
  sp.begin(step)
  sp.editValue = '3'
  sp.onBlur({ id: 99 })
  expect(calls).toEqual([])
  sp.onKeydown({ key: 'Enter', preventDefault() {} }, step)
  await new Promise((r) => setTimeout(r, 0))
  expect(calls).toEqual([[1, 10, { progress_current: 3 }]])
})

test('ошибка сервера уходит в страницу; пока идёт запрос, второй не стартует', async () => {
  const failing = make(async () => {
    throw new Error('нет связи')
  })
  await failing.sp.set(quests[0].steps[0], 2, 1)
  expect(failing.errors).toEqual(['', 'нет связи'])
  let release: () => void = () => {}
  const slow = make(() => new Promise((r) => (release = () => r({ id: 1 }))))
  const first = slow.sp.set(quests[0].steps[0], 2, 1)
  expect(slow.sp.busyId).toBe(10)
  await slow.sp.set(quests[1].steps[0], 1, 2)
  expect(slow.calls).toHaveLength(1)
  release()
  await first
})
