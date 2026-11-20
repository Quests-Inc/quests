import { expect, test } from 'bun:test'

import { renderMarkdown } from '@/shared/lib/markdown.ts'
import { fragmentQuote, scrollFragment } from '@/shared/lib/markdownSpans.ts'
import { findMarkdownTarget } from '@/shared/lib/markdownTargets.ts'
import { parseRefHref } from '@/shared/lib/refs.ts'

const piece = `до <!-- span-id: light -->фраза света<!-- /span-id: light --> после

<!-- span-id: domain -->

## Свет

Жрец света.

<!-- /span-id: domain -->

\`\`\`
<!-- span-id: fake -->
не цель
<!-- /span-id: fake -->
\`\`\`
`

test('a span pair is a hidden target and a quote, a broken pair is not', () => {
  expect(fragmentQuote(piece, 'light')).toBe('фраза света')
  expect(fragmentQuote(piece, 'domain')).toBe('Свет Жрец света.')
  expect(fragmentQuote(piece, 'fake')).toBeNull()
  expect(fragmentQuote('<!-- span-id: a -->x<!-- /span-id: b -->', 'a')).toBeNull()
  expect(fragmentQuote('<!-- span-id: a --><!-- span-id: b -->x<!-- /span-id: a --><!-- /span-id: b -->', 'a')).toBeNull()
  expect(fragmentQuote('<!-- span-id: a -->x<!-- /span-id: a --><!-- span-id: a -->y<!-- /span-id: a -->', 'a')).toBeNull()
  const html = renderMarkdown(piece)
  const root = document.createElement('div')
  root.innerHTML = html
  expect(html).not.toContain('<!--')
  expect(root.querySelector('[data-md-span="light"]')?.textContent).toContain('фраза света')
  expect(root.querySelector('[data-md-span="domain"]')?.textContent).toContain('Жрец света')
  expect(root.querySelector('[data-md-span="fake"]')).toBeNull()
  expect(scrollFragment(root, 'light')).toBe(true)
  expect(scrollFragment(root, 'missing')).toBe(false)
})

test('a table-row fragment quotes the row and stays one link', () => {
  const card =
    '<!-- table: left -->\n<!-- table-id: spell -->\n| | |\n|---|---|\n| <!-- row-id: classes -->Классы | note=553 |\n'
  expect(fragmentQuote(card, 'spell/classes')).toBe('Классы · note=553')
  expect(fragmentQuote(card + card, 'spell/classes')).toBeNull()
  const html = renderMarkdown('смотри note=540#spell/classes', { labels: { 'note:540': 'Огненный шар' } })
  expect(html).toContain('href="?note=540#spell/classes"')
  expect(html).toContain('note=540#spell/classes')
  expect(html).not.toContain('Огненный шар')
  expect(parseRefHref('?note=540#spell/classes')).toEqual({ kind: 'note', id: 540, fragment: 'spell/classes' })
  const root = document.createElement('div')
  root.innerHTML = renderMarkdown(card + '\n<!-- span-id: light -->домен<!-- /span-id: light -->')
  expect(findMarkdownTarget(root, { table: 'spell', row: 'classes' })?.textContent).toContain('note=553')
  expect(root.querySelector('[data-md-span="light"]')?.textContent).toContain('домен')
  expect(scrollFragment(root, 'spell/classes')).toBe(true)
})
