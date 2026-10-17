import { expect, test } from 'bun:test'

import { createQuestActions, postponePatch } from '@/pages/journal/model/questActions.svelte.ts'
import { toasts } from '@/shared/lib/toasts.svelte.ts'

function make(update: (id: number, patch: any) => Promise<any> = async (id, patch) => ({ id, ...patch })) {
  const calls: any[] = []
  const applied: any[] = []
  const errors: string[] = []
  const actions = createQuestActions({
    updateQuest: async (id, patch) => {
      calls.push([id, patch])
      return update(id, patch)
    },
    applyQuest: (q, opts) => applied.push([q, opts]),
    setError: (m) => errors.push(m),
    refLabels: () => ({}),
  })
  return { actions, calls, applied, errors }
}

test('postponePatch: окно не короче минуты, срок от «сейчас»', () => {
  const p = postponePatch(15, 1_000_000)
  expect(p).toEqual({
    status: 'active',
    deadline_at: new Date(1_000_000 + 900_000).toISOString(),
    duration_seconds: 900,
  })
  expect(postponePatch(0, 0).duration_seconds).toBe(60)
})

test('статус: запрос, применение, тост; ошибка очищается перед запросом', async () => {
  const { actions, calls, applied, errors } = make()
  await actions.patchStatus({ id: 4 }, 'frozen')
  expect(calls).toEqual([[4, { status: 'frozen' }]])
  expect(applied[0][0]).toMatchObject({ id: 4, status: 'frozen' })
  expect(errors).toEqual([''])
  expect(toasts.at(-1)?.message).toContain('Статус')
  expect(actions.busy).toBe(false)
})

test('значимость: та же — не отправляется', async () => {
  const { actions, calls } = make()
  await actions.patchSignificance({ id: 1, significance: 'epic' }, 'epic')
  await actions.patchSignificance({ id: 1 }, 'common')
  expect(calls).toEqual([])
  await actions.patchSignificance({ id: 1, significance: 'common' }, 'legendary')
  expect(calls).toEqual([[1, { significance: 'legendary' }]])
})

test('выполнено: переключает completed ⇄ active и не переключает выбранный квест', async () => {
  const { actions, calls, applied } = make()
  await actions.toggleCompleted({ id: 2, status: 'active' })
  await actions.toggleCompleted({ id: 2, status: 'completed' })
  expect(calls).toEqual([
    [2, { status: 'completed' }],
    [2, { status: 'active' }],
  ])
  expect(applied.every(([, opts]) => opts?.select === false)).toBe(true)
  await actions.toggleCompleted(null)
  expect(calls).toHaveLength(2)
})

test('пока идёт одно действие, остальные игнорируются; ошибка уходит в страницу и тост', async () => {
  let release: () => void = () => {}
  const slow = make(() => new Promise((r) => (release = () => r({ id: 1 }))))
  const first = slow.actions.patchStatus({ id: 1 }, 'active')
  expect(slow.actions.busy).toBe(true)
  await slow.actions.postpone({ id: 1 }, 15)
  expect(slow.calls).toHaveLength(1)
  release()
  await first
  const failing = make(async () => {
    throw new Error('сервер недоступен')
  })
  await failing.actions.postpone({ id: 1 }, 15)
  expect(failing.errors).toEqual(['', 'сервер недоступен'])
  expect(toasts.at(-1)).toMatchObject({ kind: 'error', message: 'сервер недоступен' })
  expect(failing.actions.busy).toBe(false)
})

test('группа: один тост, пропуск тех, кому менять нечего, сбой одного не рвёт остальных', async () => {
  const { actions, calls, applied } = make(async (id, patch) => {
    if (id === 2) throw new Error('boom')
    return { id, ...patch }
  })
  await actions.patchStatusMany([{ id: 1, status: 'frozen' }, { id: 2 }, { id: 3, status: 'active' }], 'frozen')
  expect(calls.map((c) => c[0])).toEqual([2, 3])
  expect(applied.map((a) => a[0].id)).toEqual([3])
  expect(applied[0][1]).toEqual({ select: false })
  expect(toasts.at(-1)?.message).toContain('не удалось: 1')
  await actions.patchSignificanceMany([{ id: 5, significance: 'epic' }, { id: 6 }], 'epic')
  expect(calls.at(-1)).toEqual([6, { significance: 'epic' }])
  await actions.postponeMany([{ id: 7 }, { id: 8 }], 30)
  expect(calls.slice(-2).map((c) => c[1].duration_seconds)).toEqual([1800, 1800])
  expect(toasts.at(-1)?.message).toContain('(2)')
})
