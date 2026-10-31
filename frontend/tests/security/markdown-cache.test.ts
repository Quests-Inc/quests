import { expect, test } from 'bun:test'

import { renderMarkdown } from '@/shared/lib/markdown.ts'

test('повторный рендер того же текста отдаёт то же самое', () => {
  const a = renderMarkdown('# Привет\n\n**жирный** текст')
  expect(a).toContain('<h1')
  expect(renderMarkdown('# Привет\n\n**жирный** текст')).toBe(a)
})

test('подписи ссылок входят в ключ кеша: сменились — рендер новый', () => {
  const text = 'см. note=5'
  const one = renderMarkdown(text, { labels: { 'note:5': 'Первая' } })
  const two = renderMarkdown(text, { labels: { 'note:5': 'Вторая' } })
  expect(one).toContain('Первая')
  expect(two).toContain('Вторая')
  expect(renderMarkdown(text, { labels: { 'note:5': 'Первая' } })).toBe(one)
})

test('пустой текст — пустая строка', () => {
  expect(renderMarkdown('   ')).toBe('')
})

test('loading values use placeholders without poisoning the ready render cache; code stays literal', () => {
  const source = '{{hp}} / {{note=259.ac}} / `{{hp}}`'
  const ready = renderMarkdown(source)
  expect(ready).toContain('{{hp}}')
  const loading = renderMarkdown(source, { valuesLoading: true })
  expect(loading).toContain('data-value="hp">…</span>')
  expect(loading).toContain('data-value="note=259.ac">…</span>')
  expect(loading).toContain('<code>{{hp}}</code>')
  expect(renderMarkdown(source)).toBe(ready)
  expect(renderMarkdown('{{hp}}', { values: { hp: '12' }, valuesLoading: true })).toContain('>12</span>')
})
