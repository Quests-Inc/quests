import { expect, test } from 'bun:test'

import { renderMarkdown } from '@/shared/lib/markdown.ts'

const BODY = '| **КД** | 13 |\n| **Хиты** | 9 |\n'

test('table: the header is on top by default', () => {
  const html = renderMarkdown('| Имя | Значение |\n|---|---|\n' + BODY)
  expect(html).toContain('<thead>')
  expect(html).toContain('scope="col"')
  expect(html).not.toContain('scope="row"')
})

test('table: an empty header row means the first column holds the headers', () => {
  const html = renderMarkdown('| | |\n|---|---|\n' + BODY)
  expect(html).not.toContain('<thead>')
  expect(html.match(/<th scope="row"/g)).toHaveLength(2)
  expect(html).toContain('<td>13</td>')
})

test('table: a line above chooses the side; the top row becomes an ordinary row when the headers are on the left', () => {
  const left = renderMarkdown('<!-- table: left -->\n| Класс | Волк |\n|---|---|\n' + BODY)
  expect(left).not.toContain('<thead>')
  expect(left.match(/<th scope="row"/g)).toHaveLength(3)
  expect(left).not.toContain('table:')

  const both = renderMarkdown('<!-- table: both -->\n| | Значение |\n|---|---|\n' + BODY)
  expect(both).toContain('<thead>')
  expect(both.match(/<th scope="row"/g)).toHaveLength(2)

  const none = renderMarkdown('<!-- table: none -->\n| Имя | Значение |\n|---|---|\n' + BODY)
  expect(none).not.toContain('<th')
  expect(none).toContain('<td>Имя</td>')

  const top = renderMarkdown('<!-- table: top -->\n| | |\n|---|---|\n' + BODY)
  expect(top).toContain('<thead>')
})

test('table: chips and links inside cells survive', () => {
  const html = renderMarkdown('| | |\n|---|---|\n| **КД** | {{ac}} |\n', { values: { ac: '13' } })
  expect(html).toContain('class="md-value"')
  expect(html).toContain('scope="row"')
})
