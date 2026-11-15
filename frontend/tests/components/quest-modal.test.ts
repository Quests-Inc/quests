import { afterEach, expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

const created: any[] = []
const updates: unknown[] = []
const stepUpdates: unknown[] = []
const real = await import('@/shared/api/client.ts')
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  updateQuest: async (id: number, body: object) => {
    updates.push(body)
    return { id, ...body }
  },
  updateQuestStep: async (id: number, sid: number, body: object) => {
    stepUpdates.push({ id, sid, body })
    return { id }
  },
  listCategories: async () => [],
  listQuestlines: async () => [],
  listTags: async () => [],
  listEvents: async () => [],
  createQuest: async (payload: any) => {
    created.push(payload)
    return { id: 7, ...payload }
  },
}))
const { default: QuestModal } = await import('@/features/quest-edit/ui/QuestModal.svelte')

let cleanup: Array<() => void> = []
afterEach(() => {
  cleanup.forEach((fn) => fn())
  cleanup = []
  updates.length = 0
  stepUpdates.length = 0
  created.length = 0
})

const tick = () => new Promise((r) => setTimeout(r, 0))

function open(props: Record<string, unknown>) {
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(QuestModal, { target, props: { open: true, mode: 'create', onClose() {}, onSaved() {}, ...props } })
  cleanup.push(() => {
    unmount(app)
    target.remove()
  })
}

function type(input: HTMLInputElement, value: string) {
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  flushSync()
}

test('создание квеста: заголовок уходит в createQuest, затем onSaved и onClose', async () => {
  const saved: any[] = []
  let closed = 0
  open({ onSaved: (q: any) => saved.push(q), onClose: () => closed++ })
  await tick()
  expect(document.body.textContent).toContain('Новый квест')
  type(document.querySelector('form input[type=text]') as HTMLInputElement, '  Купить хлеб  ')
  const form = document.querySelector('form') as HTMLFormElement
  form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  await tick()
  expect(created).toHaveLength(1)
  expect(created[0]).toMatchObject({ title: 'Купить хлеб', status: 'active', category_id: null, deadline_at: null })
  expect(saved[0].id).toBe(7)
  expect(closed).toBe(1)
})

test('пустой заголовок: createQuest не зовётся, показана ошибка', async () => {
  open({})
  await tick()
  const form = document.querySelector('form') as HTMLFormElement
  form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  flushSync()
  expect(created).toHaveLength(0)
  expect(document.body.textContent).toContain('Нужен заголовок')
})

const editQuest = () => ({
  id: 19,
  title: 'Перенос срока',
  status: 'active',
  significance: 'common',
  deadline_at: '2026-11-20T12:00:00Z',
  steps: [1, 2, 3, 4].map((id, sort_order) => ({
    id,
    title: `Шаг ${id}`,
    progress_current: 1,
    progress_total: 1,
    sort_order,
  })),
})

test('changing only the deadline never patches four unchanged completed steps', async () => {
  open({ mode: 'edit', quest: editQuest() })
  await tick()
  const section = [...document.querySelectorAll<HTMLButtonElement>('.fsec__toggle')].find(
    (b) => b.querySelector('.fsec__title')?.textContent === 'Срок'
  )!
  section.click()
  flushSync()
  ;(document.querySelector('button[aria-label="Дата срока"]') as HTMLButtonElement).click()
  flushSync()
  const date = [...document.querySelectorAll<HTMLButtonElement>('.date-input__pop button')].find(
    (b) => b.textContent?.trim() === '23'
  )
  expect(date).toBeDefined()
  date!.click()
  flushSync()
  document.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  await tick()
  expect(updates).toHaveLength(1)
  expect(stepUpdates).toHaveLength(0)
})

test('editing one step sends only the changed fields of that step', async () => {
  open({ mode: 'edit', quest: editQuest() })
  await tick()
  type(document.querySelector('[aria-label="Название шага 2"]') as HTMLInputElement, 'Новая формулировка')
  document.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  await tick()
  expect(stepUpdates).toEqual([{ id: 19, sid: 2, body: { title: 'Новая формулировка' } }])
})
