import { afterEach, beforeEach, expect, test } from 'bun:test'

import { questExportFilename } from '@/entities/quest/model/questExport.ts'
import { buildPreviewTree, actionLabel } from '@/shared/lib/actionsPreview.ts'
import { copyText } from '@/shared/lib/clipboard.ts'
import { subscribeQuestEvents } from '@/shared/lib/live.ts'
import { mapRenderedOffsetToSource, mapSourceOffsetToRendered } from '@/shared/lib/mdCaretMap.ts'
import { DEFAULT_THEME, THEMES, applyTheme, currentThemeId, loadSavedTheme } from '@/shared/lib/theme.ts'

beforeEach(() => {
  localStorage.clear()
})

test('theme: неизвестная тема заменяется по умолчанию, выбор запоминается', () => {
  expect(loadSavedTheme()).toBe(DEFAULT_THEME)
  expect(applyTheme('nord')).toBe('nord')
  expect(document.documentElement.dataset.theme).toBe('nord')
  expect(loadSavedTheme()).toBe('nord')
  expect(applyTheme('нет-такой')).toBe(DEFAULT_THEME)
  expect(currentThemeId()).toBe(DEFAULT_THEME)
  expect(new Set(THEMES.map((t) => t.id)).size).toBe(THEMES.length)
  localStorage.setItem('quests.theme', 'мусор')
  expect(loadSavedTheme()).toBe(DEFAULT_THEME)
})

test('clipboard: пишет через Clipboard API, а без него — через скрытое поле и execCommand', async () => {
  const written: string[] = []
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText: async (v: string) => void written.push(v) },
    configurable: true,
  })
  await copyText(42)
  expect(written).toEqual(['42'])
  Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
  let copied = ''
  ;(document as any).execCommand = () => {
    copied = (document.activeElement as HTMLTextAreaElement).value
    return true
  }
  await copyText('текст')
  expect(copied).toBe('текст')
  expect(document.querySelector('textarea')).toBeNull()
  ;(document as any).execCommand = () => false
  await expect(copyText('x')).rejects.toThrow('copy failed')
  expect(document.querySelector('textarea')).toBeNull()
})

test('mdCaretMap: позиция в тексте и в отрисованном совпадают по смыслу в обе стороны', () => {
  const source = '# Заголовок\n\nПервый абзац с **жирным** текстом и ещё словами для длины.'
  const rendered = 'Заголовок\n\nПервый абзац с жирным текстом и ещё словами для длины.'
  const at = source.indexOf('текстом')
  const ren = mapSourceOffsetToRendered(source, rendered, at + 3)
  expect(rendered.slice(ren - 3, ren)).toBe(source.slice(at, at + 3))
  const back = mapRenderedOffsetToSource(source, rendered, rendered.indexOf('словами'))
  expect(source.slice(back, back + 7)).toBe('словами')
  expect(mapSourceOffsetToRendered('', 'x', 1)).toBe(0)
  expect(mapRenderedOffsetToSource('x', '', 1)).toBe(0)
})

test('actionsPreview: дерево квестлайн → квест → шаг из плоского списка действий', () => {
  expect(actionLabel('create_quest')).toBe('создать квест')
  expect(actionLabel('unknown')).toBe('unknown')
  const tree = buildPreviewTree(
    [
      { index: 0, action: 'create_questline', after: { id: -1, title: 'Новая линия' } },
      { index: 1, action: 'create_quest', after: { id: -2, title: 'Новый квест', questline_id: -1 } },
      { index: 2, action: 'add_step', after: { quest_id: -2, title: 'Шаг' } },
      {
        index: 3,
        action: 'update_quest',
        before: { title: 'Старый', status: 'active' },
        after: { id: 5, status: 'completed' },
      },
      {
        index: 4,
        action: 'delete_step',
        before: { steps: [{ id: 9, title: 'Лишний' }] },
        after: { quest_id: 5, step_id: 9 },
      },
    ],
    { quests: [{ id: 5, title: 'Старый', questline_id: null }] }
  )
  expect(tree.questlines).toHaveLength(1)
  expect(tree.questlines[0]).toMatchObject({ label: 'Новая линия', isNew: true })
  expect(tree.questlines[0].quests[0]).toMatchObject({ label: 'Новый квест', isNew: true })
  expect(tree.questlines[0].quests[0].steps[0]).toMatchObject({ label: 'Шаг', isNew: true })
  expect(tree.looseQuests).toHaveLength(1)
  expect(tree.looseQuests[0].changes).toEqual([{ field: 'статус', from: 'active', to: 'completed' }])
  expect(tree.looseQuests[0].steps[0]).toMatchObject({ label: 'Лишний', deleted: true })
})

test('questExportFilename: id, безопасное имя и расширение', () => {
  expect(questExportFilename({ id: 7, title: ' Купить хлеб! ' })).toBe('quest-7-Купить-хлеб.pdf')
  expect(questExportFilename({}, 'md')).toBe('quest-x-quest.md')
  expect(questExportFilename({ id: 1, title: '' }, 'md')).toBe('quest-1-quest.md')
})

class FakeSocket extends EventTarget {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSED = 3
  static all: FakeSocket[] = []
  readyState = 0
  sent: string[] = []
  constructor(public url: string) {
    super()
    FakeSocket.all.push(this)
  }
  send(v: string) {
    this.sent.push(v)
  }
  close() {
    this.readyState = 3
    this.dispatchEvent(new Event('close'))
  }
  open() {
    this.readyState = 1
    this.dispatchEvent(new Event('open'))
  }
  message(data: unknown) {
    this.dispatchEvent(Object.assign(new Event('message'), { data: JSON.stringify(data) }))
  }
}

const realWS = (globalThis as any).WebSocket
beforeEach(() => {
  FakeSocket.all = []
  ;(globalThis as any).WebSocket = FakeSocket
})
afterEach(() => {
  ;(globalThis as any).WebSocket = realWS
})

test('live: события доходят до подписчика, ping получает pong, статусы меняются, повторное подключение зовёт resync', async () => {
  const events: any[] = []
  const statuses: string[] = []
  let resyncs = 0
  const stop = subscribeQuestEvents((e) => events.push(e), {
    onStatus: (s) => statuses.push(s),
    onResync: () => resyncs++,
  })
  const first = FakeSocket.all[0]
  expect(statuses).toEqual(['connecting'])
  first.open()
  expect(statuses.at(-1)).toBe('live')
  expect(resyncs).toBe(0)
  first.message({ type: 'quests_changed', quest_id: 3 })
  first.message({ type: 'ping' })
  expect(events).toEqual([{ type: 'quests_changed', quest_id: 3 }])
  expect(first.sent).toEqual(['pong'])
  first.close()
  expect(statuses.at(-1)).toBe('reconnect')
  await new Promise((r) => setTimeout(r, 650))
  const second = FakeSocket.all[1]
  expect(second).toBeTruthy()
  second.open()
  expect(resyncs).toBe(1)
  stop()
  expect(statuses.at(-1)).toBe('off')
})
