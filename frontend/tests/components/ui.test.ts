import { afterEach, expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
import OptionPills from '@/shared/ui/OptionPills.svelte'
import Toggle from '@/shared/ui/Toggle.svelte'

let cleanup: Array<() => void> = []

function render(component: any, props: Record<string, unknown>) {
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(component, { target, props })
  cleanup.push(() => {
    unmount(app)
    target.remove()
  })
  return target
}

afterEach(() => {
  cleanup.forEach((fn) => fn())
  cleanup = []
})

test('Toggle: подпись и переключение по клику', () => {
  const state = { checked: false }
  const el = render(Toggle, {
    label: 'Включено',
    get checked() {
      return state.checked
    },
    set checked(v: boolean) {
      state.checked = v
    },
  })
  expect(el.textContent).toContain('Включено')
  const input = el.querySelector('input') as HTMLInputElement
  expect(input.checked).toBe(false)
  input.click()
  flushSync()
  expect(state.checked).toBe(true)
})

test('OptionPills: клик по варианту отдаёт его id', () => {
  const picked: string[] = []
  const el = render(OptionPills, {
    options: [
      { id: 'a', label: 'Первый' },
      { id: 'b', label: 'Второй' },
    ],
    selected: new Set<string>(),
    onToggle: (id: string) => picked.push(id),
  })
  const second = [...el.querySelectorAll('button')].find((b) => b.textContent?.includes('Второй')) as HTMLElement
  second.click()
  flushSync()
  expect(picked).toEqual(['b'])
})

test('ConfirmModal: открыт — показывает сообщение и зовёт onConfirm / onCancel', () => {
  const calls: string[] = []
  render(ConfirmModal, {
    open: true,
    message: 'Удалить квест?',
    confirmLabel: 'Да',
    cancelLabel: 'Нет',
    onConfirm: () => calls.push('confirm'),
    onCancel: () => calls.push('cancel'),
  })
  expect(document.body.textContent).toContain('Удалить квест?')
  const btn = (label: string) =>
    [...document.querySelectorAll('button')].find((b) => b.textContent?.trim() === label) as HTMLElement
  btn('Да').click()
  btn('Нет').click()
  flushSync()
  expect(calls).toEqual(['confirm', 'cancel'])
})

test('ConfirmModal: закрыт — ничего не рисует', () => {
  render(ConfirmModal, { open: false, message: 'Спрятано' })
  expect(document.body.textContent).not.toContain('Спрятано')
})
