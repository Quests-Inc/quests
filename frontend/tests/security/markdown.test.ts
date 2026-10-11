import { expect, test } from 'bun:test'

import { renderMarkdown } from '@/shared/lib/markdown.ts'

test('markdown: ссылки на сущности, картинки только из вложений, внешние ссылки безопасны, скрипты вырезаны', () => {
  const html = renderMarkdown(
    'Смотри note=7 и ![схема|center](attachment=3) [сайт](https://example.com) <script>alert(1)</script> <img src="https://evil/x.png">',
    {
      labels: { 'note:7': 'Моя заметка' },
    }
  )
  expect(html).toContain('href="?note=7"')
  expect(html).toContain('Моя заметка')
  expect(html).toContain('src="/api/attachments/3/image"')
  expect(html).toContain('md-img--center')
  expect(html).toContain('rel="noopener noreferrer"')
  expect(html).toContain('target="_blank"')
  expect(html).not.toContain('<script')
  expect(html).not.toContain('evil')
  expect(renderMarkdown('   ')).toBe('')
  expect(renderMarkdown('`note=7`')).not.toContain('href')
})

test('markdown: опасные ссылки и атрибуты не проходят', () => {
  const html = renderMarkdown(
    [
      '[xss](javascript:alert(1))',
      '<a href="javascript:alert(2)" onclick="alert(3)">x</a>',
      '<iframe src="https://evil"></iframe>',
      '<style>body{display:none}</style>',
      '![x](data:image/png;base64,AAAA)',
    ].join('\n\n')
  )
  expect(html).not.toMatch(/javascript:/i)
  expect(html).not.toMatch(/onclick/i)
  expect(html).not.toContain('<iframe')
  expect(html).not.toContain('<style')
  expect(html).not.toContain('data:image')
})
