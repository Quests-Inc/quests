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
