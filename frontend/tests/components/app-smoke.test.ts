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
    templates: 'Шаблоны',
    trash: 'Корзина',
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
  expect(document.querySelector('[aria-label="Глубина"]')).not.toBeNull()
  expect(document.querySelector('[aria-label="Раскладка"]')).not.toBeNull()
  expect(document.body.textContent).toContain('Квесты:')
  expect(document.querySelector('[aria-label="Вписать карту в окно"]')).not.toBeNull()
})

/** Кнопка шапки, не её невидимый двойник для замера ширины. */
function liveButton(label: string) {
  return [...document.querySelectorAll(`button[aria-label="${label}"]`)].find(
    (el) => !el.closest('[aria-hidden="true"]')
  ) as HTMLElement | undefined
}

test('окна оболочки открываются с шапки: шаблоны, секреты, теги, настройки, ассистент, квестлайн', async () => {
  await openAt('')
  const windows: [string, string, string][] = [
    ['Окружение', 'Окружение', 'Окружение'],
    ['Теги', 'Теги', 'Теги'],
    ['Настройки', 'Настройки', 'Настройки'],
    ['Командная строка журнала', 'Командная строка журнала', ''],
    ['Новый квестлайн', 'Новый квестлайн', 'Новый квестлайн'],
  ]
  for (const [label, title, menuLabel] of windows) {
    const btn = liveButton(label)
    if (btn) btn.click()
    else {
      liveButton('Ещё действия')!.click()
      await settle(40)
      const item = [...document.querySelectorAll('[role=menuitem]')].find((el) =>
        el.textContent?.includes(menuLabel)
      ) as HTMLElement
      expect(item, label).toBeTruthy()
      item.click()
    }
    await settle(80)
    const head = [...document.querySelectorAll('.modal__title')].find((el) => el.textContent?.includes(title))
    expect(head, title).toBeTruthy()
    const close = head!.closest('.modal')?.querySelector('button[aria-label="Закрыть"]') as HTMLElement | null
    expect(close, title).toBeTruthy()
    close!.click()
    await settle(40)
  }
})

test('ссылка на вложение выбирает его на вкладке вложений', async () => {
  await openAt('?attachment=5')
  expect(window.questsNav!.currentTab()).toBe('attachments')
  expect(document.body.textContent).toContain('attachment=5')
})

test('создание квеста: кнопка «Новый квест» → форма → POST и новый квест в списке', async () => {
  await openAt('')
  const open = liveButton('Новый квест')!
  open.click()
  await settle(80)
  const input = document.querySelector('.modal__form input[type=text]') as HTMLInputElement
  expect(input).toBeTruthy()
  input.value = 'Квест из теста'
  input.dispatchEvent(new Event('input', { bubbles: true }))
  ;(document.querySelector('.modal__form') as HTMLFormElement).dispatchEvent(
    new Event('submit', { bubbles: true, cancelable: true })
  )
  await settle(250)
  expect(fake.bodies['POST /api/quests']?.[0]).toMatchObject({ title: 'Квест из теста', status: 'active' })
  expect(document.body.textContent).toContain('Квест из теста')
})

test('шаг: кнопка +1 шлёт PATCH с новым прогрессом', async () => {
  await openAt('?quest=1')
  ;(document.querySelector('.step__btn[aria-label="Увеличить прогресс"]') as HTMLElement).click()
  await settle(150)
  expect(fake.bodies['PATCH /api/quests/1/steps/11']?.[0]).toEqual({ progress_current: 2 })
})

test('заметка: переименование в дереве шлёт PATCH и обновляет строку', async () => {
  await openAt('?note=1')
  const row = document.querySelector('.notes__row') as HTMLElement
  row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 10, clientY: 10 }))
  await settle(50)
  const item = [...document.querySelectorAll('[role=menuitem], [role=menu] button')].find((b) =>
    b.textContent?.trim().startsWith('Переименовать')
  ) as HTMLElement
  item.click()
  await settle(80)
  const input = document.querySelector('.notes__row-rename') as HTMLInputElement
  input.value = 'Новое имя'
  input.dispatchEvent(new Event('input', { bubbles: true }))
  input.dispatchEvent(new FocusEvent('blur'))
  await settle(150)
  expect(fake.bodies['PATCH /api/notes/1']?.[0]).toEqual({ title: 'Новое имя' })
  expect(document.querySelector('.notes__row')?.textContent).toContain('Новое имя')
})

test('обход всех вкладок не порождает шторма запросов (зацикленный эффект)', async () => {
  await openAt('')
  fake.log.length = 0
  for (const tab of ['notes', 'attachments', 'calendar', 'history', 'graph', 'runners', 'journal']) {
    window.questsNav!.setTab(tab)
    await settle(150)
  }
  const perUrl = new Map<string, number>()
  for (const l of fake.log) perUrl.set(l, (perUrl.get(l) ?? 0) + 1)
  expect([...perUrl].filter(([, n]) => n > 6)).toEqual([])
  expect(fake.log.length).toBeLessThan(80)
})

test('за время обхода в консоль не попало ошибок', () => {
  expect(errors.filter((e) => !/not implemented|deprecated/i.test(e))).toEqual([])
})
