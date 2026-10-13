import { expect, test } from 'bun:test'

import { durationLabel, partsToSeconds, plural, secondsToParts } from '../../src/shared/lib/duration.ts'
import { fileKind } from '../../src/shared/lib/fileKind.ts'
import { parseRefHref, parseRefs, refHref, renderRefTextHtml } from '../../src/shared/lib/refs.ts'
import { snippetHtml } from '../../src/shared/lib/searchSnippet.ts'
import {
  defaultLocalDeadlineParts,
  formatRemaining,
  localInputToUtcIso,
  parseApiDate,
  remainingFromDeadline,
  timerTone,
  toLocalInputValue,
} from '../../src/shared/lib/time.ts'

test('duration: секунды ⇄ часы и минуты, подпись, склонение', () => {
  expect(secondsToParts(5400)).toEqual({ hours: '1', minutes: '30' })
  expect(secondsToParts(0)).toEqual({ hours: '', minutes: '' })
  expect(secondsToParts(null)).toEqual({ hours: '', minutes: '' })
  expect(partsToSeconds('1', '30')).toBe(5400)
  expect(partsToSeconds('', '')).toBeNull()
  expect(partsToSeconds('-2', 'x')).toBeNull()
  expect(durationLabel('2', '')).toBe('2 ч')
  expect(durationLabel('1', '30')).toBe('1 ч 30 мин')
  expect(durationLabel('', '45')).toBe('45 мин')
  expect(durationLabel('', '')).toBe('')
  const forms = ['шаг', 'шага', 'шагов']
  expect([1, 2, 5, 11, 12, 21, 22, 25, 111].map((n) => plural(n, forms))).toEqual([
    'шаг',
    'шага',
    'шагов',
    'шагов',
    'шагов',
    'шаг',
    'шага',
    'шагов',
    'шагов',
  ])
})

test('fileKind: по MIME, затем по расширению; unknown — «прочее»', () => {
  expect(fileKind({ filename: 'a.png', content_type_detected: 'image/png' }).kind).toBe('image')
  expect(fileKind({ filename: 'a.pdf', content_type_detected: 'application/pdf' })).toMatchObject({
    kind: 'doc',
    ext: 'PDF',
  })
  expect(fileKind({ filename: 'tab.csv', content_type_detected: 'text/csv' }).kind).toBe('sheet')
  expect(fileKind({ filename: 'x.sqlite', content_type_detected: 'application/octet-stream' }).kind).toBe('db')
  expect(fileKind({ filename: 'note.md', content_type_detected: 'text/plain; charset=utf-8' }).kind).toBe('code')
  expect(fileKind({ filename: 'noext', content_type_detected: 'application/octet-stream' }).kind).toBe('other')
  expect(fileKind(null).kind).toBe('other')
})

test('refs: разбор токенов kind=id без дублей, ссылка и обратный разбор, HTML с экранированием', () => {
  expect(parseRefs('см. note=12, quest=3 и снова note=12; step=0')).toEqual([
    { kind: 'note', id: 12 },
    { kind: 'quest', id: 3 },
  ])
  expect(refHref('note', 5)).toBe('?note=5')
  expect(parseRefHref('?quest=7')).toEqual({ kind: 'quest', id: 7 })
  expect(parseRefHref('?quest=abc')).toBeNull()
  const html = renderRefTextHtml('<b> note=1', { labels: { 'note:1': 'Моя "заметка"' } })
  expect(html).toBe('&lt;b&gt; <a href="?note=1" class="md-ref">Моя &quot;заметка&quot;</a>')
  expect(renderRefTextHtml('')).toBe('')
})

test('snippetHtml: экранирует и размечает совпадения', () => {
  expect(snippetHtml("a <b> ⟪найдено⟫ & 'x'")).toBe('a &lt;b&gt; <mark>найдено</mark> &amp; &#39;x&#39;')
  expect(snippetHtml(null)).toBe('')
})

test('time: даты API без зоны — UTC, формат поля ввода, таймер', () => {
  expect(parseApiDate('2026-03-01T10:00:00').toISOString()).toBe('2026-03-01T10:00:00.000Z')
  expect(parseApiDate('2026-03-01T10:00:00+03:00').toISOString()).toBe('2026-03-01T07:00:00.000Z')
  expect(parseApiDate('мусор')).toBeNull()
  expect(parseApiDate('')).toBeNull()
  expect(localInputToUtcIso('')).toBeNull()
  const iso = localInputToUtcIso('2026-03-01T10:00')
  expect(toLocalInputValue(iso)).toBe('2026-03-01T10:00')
  expect(defaultLocalDeadlineParts(new Date(2026, 2, 1, 23, 30))).toEqual({
    date: '2026-03-02',
    hour: '00',
    minute: '00',
  })
  expect(defaultLocalDeadlineParts(new Date(2026, 2, 1, 14, 5))).toEqual({
    date: '2026-03-01',
    hour: '15',
    minute: '00',
  })
  expect(formatRemaining(75)).toBe('1:15')
  expect(formatRemaining(3725)).toBe('1:02:05')
  expect(formatRemaining(-5)).toBe('-0:05')
  expect(timerTone(900, 1000)).toBe('green')
  expect(timerTone(500, 1000)).toBe('orange')
  expect(timerTone(100, 1000)).toBe('red')
  expect(timerTone(100, 0)).toBeNull()
  expect(remainingFromDeadline('2026-03-01T10:00:10Z', Date.parse('2026-03-01T10:00:00Z'))).toBe(10)
})

import { suggestions } from '../../src/shared/lib/searchFlags.ts'

test('suggestions: пока токен не дописан — подсказки; готовый единственный вариант подсказки не просит', () => {
  const spec = {
    cat: {
      values: [
        { id: 'work', label: 'Работа', aliases: ['1'] },
        { id: 'home', label: 'Дом' },
      ],
    },
  }
  expect(suggestions('cat:', spec).items).toHaveLength(2)
  expect(suggestions('cat:wo', spec).items.map((i) => i.id)).toEqual(['work'])
  expect(suggestions('cat:work', spec)).toBeNull()
  expect(suggestions('cat:1', spec)).toBeNull()
  // токен закрыт пробелом — подсказок нет и раньше
  expect(suggestions('cat:work ', spec)).toBeNull()
  // точное совпадение, но есть и более длинный вариант: подсказка остаётся
  const two = {
    tag: {
      values: [
        { id: 'api', label: 'api' },
        { id: 'apix', label: 'apix' },
      ],
    },
  }
  expect(suggestions('tag:api', two).items).toHaveLength(2)
})
