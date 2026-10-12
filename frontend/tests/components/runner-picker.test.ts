import { expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import RunnerPicker from '@/shared/ui/RunnerPicker.svelte'

const runners = [
  { label: 'main-pc', online: true, registered: true },
  { label: 'laptop', online: false, registered: true },
  { label: 'phone', online: true, registered: true },
]

function render(value: string) {
  const target = document.createElement('div')
  document.body.appendChild(target)
  let changed = 0
  const app = mount(RunnerPicker, { target, props: { value, runners, onchange: () => changed++ } })
  const names = () =>
    [...target.querySelectorAll('.rpick__name')].map((e) => e.textContent?.trim().replace(' (офлайн)', ''))
  return { target, names, changes: () => changed, done: () => unmount(app) }
}

test('один раннер: без нумерации и без кнопок порядка', () => {
  const { target, done } = render('main-pc')
  expect(target.querySelectorAll('.rpick__item')).toHaveLength(1)
  expect(target.querySelector('.rpick__n')).toBeNull()
  expect(target.querySelector('.rpick__hint')).toBeNull()
  done()
})

test('несколько раннеров: порядок виден, оффлайн подписан, есть подсказка', () => {
  const { target, done } = render('laptop,main-pc')
  const names = [...target.querySelectorAll('.rpick__name')].map((e) => e.textContent?.trim())
  expect(names).toEqual(['laptop (офлайн)', 'main-pc'])
  expect(target.querySelectorAll('.rpick__n')).toHaveLength(2)
  expect(target.querySelector('.rpick__hint')).not.toBeNull()
  done()
})

test('неизвестная метка остаётся в списке с пометкой', () => {
  const { target, done } = render('ghost')
  expect(target.querySelector('.rpick__name')?.textContent).toContain('нет такого раннера')
  done()
})

test('убрать устройство и поменять порядок', () => {
  const { target, names, changes, done } = render('main-pc,laptop,phone')
  const items = () => target.querySelectorAll('.rpick__item')
  // «Ниже» у первого: main-pc уходит вниз
  ;(items()[0].querySelector('.rpick__btn--down') as HTMLButtonElement).click()
  flushSync()
  expect(names()).toEqual(['laptop', 'main-pc', 'phone'])
  // «Выше» у последнего поднимает его на одну позицию
  ;(items()[2].querySelector('.rpick__btn:not(.rpick__btn--down)') as HTMLButtonElement).click()
  flushSync()
  expect(names()).toEqual(['laptop', 'phone', 'main-pc'])
  // убрать первое
  const buttons = items()[0].querySelectorAll('.rpick__btn')
  ;(buttons[buttons.length - 1] as HTMLButtonElement).click()
  flushSync()
  expect(names()).toEqual(['phone', 'main-pc'])
  expect(changes()).toBe(3)
  done()
})
