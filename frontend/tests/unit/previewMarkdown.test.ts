import { expect, test } from 'bun:test'

import { previewMarkdown } from '@/shared/lib/previewMarkdown.ts'

test('a short note is kept whole', () => {
  expect(previewMarkdown('Обычный абзац и note=3.')).toBe('Обычный абзац и note=3.')
})

test('a cut inside a comment, a link, bold or a fence rolls back', () => {
  expect(previewMarkdown('видно <!-- table-id: spell', 18)).toBe('видно\n\n…')
  expect(previewMarkdown('см. [слово](http://example.com/очень-длинная-ссылка) хвост', 24)).toBe('см.\n\n…')
  expect(previewMarkdown('начало **жирный хвост который не влезает', 16)).toBe('начало\n\n…')
  const fence = '```\n' + 'код\n'.repeat(40) + '```\n\nпосле'
  expect(previewMarkdown(fence, 30)).toBe('')
})

test('a table row starts only with a whole row-id', () => {
  const table = [
    '<!-- table: left -->',
    '<!-- table-id: spell -->',
    '| Имя | Класс |',
    '|---|---|',
    '| <!-- row-id: mage --> Волшебник | маг |',
    '| <!-- row-id: rogue --> Плут | плут |',
  ].join('\n')
  const cut = previewMarkdown(table, 120)
  expect(cut).toContain('<!-- table-id: spell -->')
  expect(cut).toContain('row-id: mage')
  expect(cut).not.toContain('row-id: rogue')
  expect(cut).not.toContain('<!-- row-id: ro')
  expect(cut.endsWith('…')).toBe(true)
})

test('span-id is kept only as a finished comment', () => {
  const src = 'до <!-- span-id: light --> кусок <!-- /span-id: light --> после'
  expect(previewMarkdown(src, 12)).toBe('до\n\n…')
  expect(previewMarkdown('до <!-- span-id: light --> кусок', 40)).toContain('<!-- span-id: light -->')
})
