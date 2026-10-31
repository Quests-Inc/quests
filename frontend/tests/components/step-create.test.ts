import { afterEach, expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'
globalThis.fetch = (async () => new Response('[]', { status: 200 })) as typeof fetch
const real = await import('@/shared/api/client.ts')
const calls: { questId: number; payload: any }[] = []
let fail = false
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  listRunners: async () => [],
  listScripts: async () => [],
  addQuestStep: async (questId: number, payload: any) => {
    calls.push({ questId, payload })
    if (fail) throw new Error('Ошибка сохранения')
    return { id: questId, steps: [{ id: 123, ...payload }] }
  },
}))
const { default: StepModal } = await import('@/features/quest-edit/ui/StepModal.svelte')
let app: ReturnType<typeof mount> | undefined
const tick = () => new Promise((resolve) => setTimeout(resolve, 0))
afterEach(async () => {
  if (app) await unmount(app)
  document.body.innerHTML = ''
  calls.length = 0
  fail = false
})
function open(onSaved: (q: any) => void, onClose: () => void) {
  const target = document.createElement('div')
  document.body.append(target)
  app = mount(StepModal, { target, props: { open: true, questId: 42, step: null, onSaved, onClose } })
  flushSync()
}
function submit() {
  const input = document.querySelector('form input[type=text]') as HTMLInputElement
  input.value = '  Новый шаг  '
  input.dispatchEvent(new Event('input', { bubbles: true }))
  flushSync()
  document.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
}
test('new step without id is saved to its quest and returned to the journal', async () => {
  let saved: any
  let closed = false
  open(
    (q) => (saved = q),
    () => (closed = true)
  )
  submit()
  await tick()
  expect(calls).toHaveLength(1)
  expect(calls[0]).toMatchObject({
    questId: 42,
    payload: { title: 'Новый шаг', progress_current: 0, progress_total: 1 },
  })
  expect(saved.steps[0].id).toBe(123)
  expect(closed).toBe(true)
})
test('failed creation stays open, shows error and can be retried', async () => {
  fail = true
  let closed = false
  open(
    () => {},
    () => (closed = true)
  )
  submit()
  await tick()
  flushSync()
  expect(document.body.textContent).toContain('Ошибка сохранения')
  expect(closed).toBe(false)
  fail = false
  submit()
  await tick()
  expect(closed).toBe(true)
})
