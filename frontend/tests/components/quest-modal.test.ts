import { afterEach, expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

const created: any[] = []
const real = await import('@/shared/api/client.ts')
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  listCategories: async () => [],
  listQuestlines: async () => [],
  listTags: async () => [],
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
