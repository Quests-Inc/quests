import { expect, test } from 'bun:test'

import { foreignValueRefs, renderMarkdown } from '@/shared/lib/markdown.ts'

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

test('markdown: {{имя}} подставляется, внутри кода и скобок скрипта ссылка не рождается', () => {
  const html = renderMarkdown('Сила {{str}}, код `{{str}}`, удар script=2{1+note=4.str}.', { values: { str: '3' } })
  expect(html).toContain('class="md-value"')
  expect(html).toContain('data-value="str"')
  expect(html).toContain('>3</span>')
  expect(html).toContain('<code>{{str}}</code>')
  expect(html).not.toContain('href="?note=4"')
  expect(html).toContain('?script=2')
  const evil = renderMarkdown('{{str}}', { values: { str: '<b>x</b>' } })
  expect(evil).not.toContain('<b>')
  expect(evil).toContain('&lt;b&gt;x&lt;/b&gt;')
  expect(evil).toContain('class="md-value"')
})

test('markdown: чужое {{note=N.имя}} подставляется и не становится ссылкой', () => {
  const html = renderMarkdown('мод {{note=4.str}} и пусто {{note=9.miss}} и `{{note=4.str}}`', {
    values: { 'note=4.str': '3' },
  })
  expect(html).toContain('data-value="note=4.str"')
  expect(html).toContain('>3</span>')
  expect(html).toContain('{{note=9.miss}}')
  expect(html).not.toContain('href="?note=')
  expect(html).toContain('<code>{{note=4.str}}</code>')
  const many = Array.from({ length: 9 }, (_, i) => `{{note=${i + 1}.a}}`).join(' ')
  const refs = foreignValueRefs(`${many} {{note=1.a}} \`{{quest=1.dc}}\``)
  expect(refs).toHaveLength(8)
  expect(refs[0].key).toBe('note=1.a')
})

test('markdown: ?auto — параметр ссылки, формула со звёздочкой остаётся текстом', () => {
  const html = renderMarkdown('удар script=2{1к20+dex+pb}?auto и клик script=2{2*con}.')
  expect(html).toContain('auto=1')
  expect(html).toContain('2*con')
  expect(html).not.toContain('<em>')
  const plain = renderMarkdown('script=2{1+2}?automatic')
  expect(plain).not.toContain('auto=1')
  const code = renderMarkdown('`script=2{1}?auto`')
  expect(code).not.toContain('href="?script=')
  expect(code).toContain('script=2{1}?auto')
})

test('markdown: ?hide прячет формулу, до расчёта на ссылке знак вопроса', () => {
  const hidden = renderMarkdown('урон script=2{2к4+dex}?hide и бонус script=2{dex+pb}?auto&hide.')
  expect(hidden.match(/>\?<\/a>/g)?.length).toBe(2)
  expect(hidden).toContain('hide=1')
  expect(hidden).toContain('auto=1')
  expect(hidden).not.toContain('2к4+dex</a>')
  expect(hidden).not.toContain('dex+pb</a>')
  const word = renderMarkdown('script=2{1}?hidden')
  expect(word).not.toContain('hide=1')
  expect(word).toContain('>1</a>')
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
