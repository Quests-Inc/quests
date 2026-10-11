import { afterAll, beforeAll, expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import { installFakeApi, type Fake } from './fakeApi.ts'

// Сквозная проверка оболочки: приложение целиком над фальшивым бэкендом открывает каждую вкладку без ошибок.
let fake: Fake
let App: any
let app: any
let target: HTMLElement
const errors: string[] = []
const realError = console.error

beforeAll(async () => {
  fake = installFakeApi()
  console.error = (...a: unknown[]) => void errors.push(a.map(String).join(' '))
  window.matchMedia = ((q: string) => ({
    matches: false,
    media: q,
    addEventListener() {},
    removeEventListener() {},
  })) as any
  window.scrollTo = (() => {}) as any
  ;(globalThis as any).ResizeObserver = class {
    observe() {}
    disconnect() {}
    unobserve() {}
  }
  App = (await import('@/app/App.svelte')).default
  target = document.createElement('div')
  document.body.append(target)
  app = mount(App, { target })
})

afterAll(() => {
  unmount(app)
  fake.restore()
  console.error = realError
})

const settle = async (ms = 60) => {
  await new Promise((r) => setTimeout(r, ms))
  flushSync()
}

test('журнал: грузит данные и показывает квест в боковой панели', async () => {
  await settle(200)
  expect(fake.log).toContain('GET /api/quests')
  expect(fake.log.some((l) => l.startsWith('GET /api/notes'))).toBe(true)
  expect(document.body.textContent).toContain('Первый квест')
})

test('каждая вкладка открывается и рисует своё содержимое', async () => {
  const expectations: Record<string, string> = {
    notes: 'Корневая',
    attachments: 'схема.png',
    calendar: '',
    history: '',
    graph: 'Выберите квест или заметку',
    runners: 'Раннер',
    journal: 'Первый квест',
  }
  for (const [tab, text] of Object.entries(expectations)) {
    window.questsNav!.setTab(tab)
    await settle(120)
    expect(window.questsNav!.currentTab()).toBe(tab)
    if (text) expect(document.body.textContent).toContain(text)
  }
})

/** Приложение с нуля по адресу вида `?quest=1`: как открывают ссылку из закладки или из бота. */
async function openAt(search: string, wait = 250) {
  unmount(app)
  target.remove()
  document.body.innerHTML = ''
  history.replaceState(null, '', `/${search}`)
  target = document.createElement('div')
  document.body.append(target)
  app = mount(App, { target })
  await settle(wait)
}

test('ссылка на квест открывает его с шагами', async () => {
  await openAt('?quest=1')
  expect(document.body.textContent).toContain('Шаг один')
  expect(document.body.textContent).toContain('1/3')
})

test('ссылка на заметку открывает её; ссылка в тексте подписана названием квеста; дочерняя раскрывается', async () => {
  await openAt('?note=1')
  expect(window.questsNav!.currentTab()).toBe('notes')
  const title = document.querySelector('.notes__title') as HTMLInputElement
  expect(title?.value).toBe('Корневая')
  expect(document.body.textContent).toContain('Первый квест')
  expect(document.body.textContent).not.toContain('Вложенная')
  ;(document.querySelector('.notes__fold:not(.notes__fold--spacer)') as HTMLElement).click()
  await settle(50)
  expect(document.body.textContent).toContain('Вложенная')
})

test('ссылка на карту с центром рисует карточки и рёбра', async () => {
  await openAt('?tab=graph&gt=quest&gid=1', 400)
  expect(window.questsNav!.currentTab()).toBe('graph')
  expect(document.querySelectorAll('.gcard').length).toBeGreaterThanOrEqual(2)
  expect(document.querySelector('.gedges')).not.toBeNull()
  expect(document.body.textContent).toContain('Корневая')
})

test('ссылка на вложение выбирает его на вкладке вложений', async () => {
  await openAt('?attachment=5')
  expect(window.questsNav!.currentTab()).toBe('attachments')
  expect(document.body.textContent).toContain('attachment=5')
})

test('обход вкладок не породил шторма запросов (зацикленный эффект)', () => {
  const perUrl = new Map<string, number>()
  for (const l of fake.log) perUrl.set(l, (perUrl.get(l) ?? 0) + 1)
  const noisy = [...perUrl].filter(([, n]) => n > 8)
  expect(noisy).toEqual([])
  expect(fake.log.length).toBeLessThan(120)
})

test('за время обхода в консоль не попало ошибок', () => {
  expect(errors.filter((e) => !/not implemented|deprecated/i.test(e))).toEqual([])
})
