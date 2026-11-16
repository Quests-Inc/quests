import { afterEach, expect, test } from 'bun:test'

import { flushSync, mount, tick, unmount } from 'svelte'

import QuestDetail from '@/widgets/quest-detail/ui/QuestDetail.svelte'

import { installFakeApi, type Fake } from './fakeApi.ts'

const rect = HTMLElement.prototype.getBoundingClientRect
const scrollTo = HTMLElement.prototype.scrollTo
let fake: Fake | null = null
let target: HTMLElement | null = null
let app: ReturnType<typeof mount> | null = null

afterEach(() => {
  HTMLElement.prototype.getBoundingClientRect = rect
  HTMLElement.prototype.scrollTo = scrollTo
  if (app) unmount(app)
  app = null
  target?.remove()
  target = null
  fake?.restore()
  fake = null
})

function quest(id: number, line: number | null) {
  return {
    id,
    title: `Квест ${id}`,
    description: '',
    status: 'active',
    significance: 'common',
    questline_id: line,
    questline_title: line ? 'Линия' : null,
    steps: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }
}

async function show(selected: ReturnType<typeof quest>, quests: ReturnType<typeof quest>[]) {
  const scrolled: number[] = []
  HTMLElement.prototype.getBoundingClientRect = function () {
    if (this.classList.contains('detail__title')) {
      return { top: 480, bottom: 520, left: 0, right: 200, width: 200, height: 40, x: 0, y: 480, toJSON() {} }
    }
    if (this.classList.contains('detail')) {
      return { top: 10, bottom: 400, left: 0, right: 200, width: 200, height: 390, x: 0, y: 10, toJSON() {} }
    }
    return rect.call(this)
  }
  HTMLElement.prototype.scrollTo = function (opts?: ScrollToOptions | number, y?: number) {
    const top = typeof opts === 'number' ? (y ?? 0) : (opts?.top ?? this.scrollTop)
    if (this.classList.contains('detail')) scrolled.push(top)
    this.scrollTop = top
  }
  fake = installFakeApi()
  target = document.createElement('div')
  document.body.append(target)
  app = mount(QuestDetail, {
    target,
    props: {
      selected,
      quests,
      questlines: [{ id: 7, title: 'Линия', description: 'Длинное описание квестлайна', color: '#111', icon: 'document' }],
      nowMs: Date.parse('2026-03-01T10:00:00Z'),
      statusBusy: false,
      deleting: false,
      notes: [],
    },
  })
  flushSync()
  await tick()
  await tick()
  return scrolled
}

test('квест из квестлайна проматывает ленту к заголовку квеста', async () => {
  const q = quest(1, 7)
  const scrolled = await show(q, [q, quest(2, 7)])
  expect(scrolled.at(-1)).toBe(470)
})

test('квест без квестлайна открывается с начала ленты', async () => {
  const q = quest(3, null)
  const scrolled = await show(q, [q])
  expect(scrolled.at(-1)).toBe(0)
})
