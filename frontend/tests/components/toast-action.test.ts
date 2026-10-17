import { afterEach, expect, test } from 'bun:test'

import { dismissToast, holdToast, releaseToast, runToastAction, toast, toasts } from '@/shared/lib/toasts.svelte.ts'
import { toastTrashed, trashUndo } from '@/shared/lib/trashUndo.ts'

const realFetch = globalThis.fetch
afterEach(() => {
  globalThis.fetch = realFetch
  for (const t of [...toasts]) dismissToast(t.id)
})

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))

test('тост с кнопкой живёт дольше обычного, а кнопка срабатывает один раз и убирает тост', async () => {
  let runs = 0
  const plain = toast('обычный')
  const id = toast('с кнопкой', { action: { label: 'Отменить', run: () => void runs++ } })
  expect(toasts.find((t) => t.id === id)?.action?.label).toBe('Отменить')
  expect(toasts.find((t) => t.id === plain)?.action).toBeUndefined()
  await Promise.all([runToastAction(id), runToastAction(id)])
  expect(runs).toBe(1)
  expect(toasts.some((t) => t.id === id)).toBe(false)
})

test('падение действия возвращается тостом-ошибкой', async () => {
  const id = toast('x', {
    action: {
      label: 'Отменить',
      run: async () => {
        throw new Error('не вышло')
      },
    },
  })
  await runToastAction(id)
  expect(toasts.some((t) => t.kind === 'error' && t.message === 'не вышло')).toBe(true)
})

test('наведение останавливает отсчёт, уход — запускает заново', async () => {
  const id = toast('держим', { ttl: 40, action: { label: 'ok', run: () => {} } })
  holdToast(id)
  await wait(90)
  expect(toasts.some((t) => t.id === id)).toBe(true)
  releaseToast(id)
  await wait(90)
  expect(toasts.some((t) => t.id === id)).toBe(false)
})

test('trashUndo: восстанавливает каждую сущность своим маршрутом и сообщает', async () => {
  const calls: string[] = []
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    calls.push(`${init?.method ?? 'GET'} ${url}`)
    return new Response(null, { status: 204 })
  }) as typeof fetch
  const action = trashUndo('note', [3, 5])
  expect(action.label).toBe('Отменить')
  await action.run()
  expect(calls).toEqual(['POST /api/trash/notes/3/restore', 'POST /api/trash/notes/5/restore'])
  expect(toasts.at(-1)?.message).toBe('Восстановлено: 2')

  const id = toastTrashed('quest', [7], 'Квест удалён')
  const t = toasts.find((x) => x.id === id)
  expect(t?.kind).toBe('success')
  await runToastAction(id)
  expect(calls.at(-1)).toBe('POST /api/trash/quests/7/restore')
  expect(toasts.at(-1)?.message).toBe('Квест восстановлен')
})
