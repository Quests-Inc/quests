import { expect, test } from 'bun:test'

import { orderedTextOffset } from '../../src/shared/lib/orderedTextMap.ts'

test('every occurrence maps to its own sentence in both directions', () => {
  const sentence = 'Полный текст заметки. '
  const source = '# Заголовок\n\n' + sentence.repeat(300) + '\n\nКонец'
  const rendered = 'Заголовок\n' + sentence.repeat(300) + '\nКонец'
  for (const occurrence of [0, 1, 27, 150, 299]) {
    const from = rendered.indexOf(sentence) + occurrence * sentence.length + 10
    const to = source.indexOf(sentence) + occurrence * sentence.length + 10
    expect(orderedTextOffset(source, rendered, from)).toBe(to)
    expect(orderedTextOffset(source, rendered, to, true)).toBe(from)
  }
})

test('alignment resynchronizes after markdown links, code language and generated labels', () => {
  const source =
    '# Заголовок\n\n[Открыть](https://example.com/path)\n\n```javascript\nconst value = 1\n```\n\nnote=42\n\nПовтор фразы. Повтор фразы. Повтор фразы.'
  const rendered = 'Заголовок\nОткрыть\nconst value = 1\nИмя заметки из базы\nПовтор фразы. Повтор фразы. Повтор фразы.'
  const needle = 'фразы'
  const a = source.lastIndexOf(needle) + 2,
    b = rendered.lastIndexOf(needle) + 2
  expect(orderedTextOffset(source, rendered, b)).toBe(a)
  expect(orderedTextOffset(source, rendered, a, true)).toBe(b)
})

test('column alignment interpolates unmatched blocks monotonically instead of global substring fallback', () => {
  const source = 'Начало текста здесь ' + 'скрытая разметка '.repeat(20) + 'конечный видимый фрагмент'
  const rendered = 'Начало текста здесь ' + 'результат вывода '.repeat(25) + 'конечный видимый фрагмент'
  const mapped = Array.from({ length: source.length }, (_, i) => orderedTextOffset(source, rendered, i, true, true))
  for (let i = 1; i < mapped.length; i++) expect(mapped[i]!).toBeGreaterThanOrEqual(mapped[i - 1]!)
})
