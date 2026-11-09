import { expect, test } from 'bun:test'

import { renderMarkdown } from '@/shared/lib/markdown.ts'
import { readMath } from '@/shared/lib/markdownMath.ts'
import { findMarkdownTarget, targetResult } from '@/shared/lib/markdownTargets.ts'

test('static GFM preserves quotes, headings, strikethrough and disabled checkboxes', () => {
  const html = renderMarkdown(
    '##### Пятый\n\n###### Шестой\n\n> Пояснение **важное**\n\n~~было~~\n\n- [x] Готово\n- [ ] Нет\n\n<input type="text"><input type="checkbox" onclick="alert(1)">'
  )
  expect(html).toContain('<h5>')
  expect(html).toContain('<h6>')
  expect(html).toContain('<blockquote>')
  expect(html).toContain('<del>было</del>')
  const root = document.createElement('div')
  root.innerHTML = html
  expect(root.querySelectorAll('input')).toHaveLength(3)
  for (const input of root.querySelectorAll('input')) {
    expect(input.type).toBe('checkbox')
    expect(input.disabled).toBe(true)
  }
  expect(html).not.toContain('onclick')
})
test('math supports note137 and multiline display; code and currency stay literal', () => {
  const html = renderMarkdown(
    'Стоимость $\\Phi_{ср}$ и $\\frac{4384}{8926{,}58}$.\n\n$$\n\\mathbf{8926{,}58}\\ \\text{тыс. руб.}\n$$\n\n`$a_b$`\n\n```tex\n$$x$$\n```\n\nЦена $10 и $20; \\$5.'
  )
  expect(html).toContain('katex')
  expect(html).toContain('md-math--display')
  expect(html).not.toContain('md-math--error')
  expect(html).toContain('<code>$a_b$</code>')
  expect(html).toContain('Цена $10 и $20')
  expect(html).not.toContain('data-math=')
  const root = document.createElement('div')
  root.innerHTML = html
  expect(root.querySelectorAll('.md-math')).toHaveLength(3)
})
test('math is isolated from refs and cannot inject markup or load external images', () => {
  const html = renderMarkdown(
    '$\\text{note=7}$\n\n$\\includegraphics{https://evil/a.png}$\n\n$\\htmlClass{evil}{x}$\n\n$\\unknown{<script>alert(1)</script>}$'
  )
  expect(html).not.toContain('href="?note=')
  expect(html).not.toContain('<img')
  expect(html).not.toContain('<script')
  expect(html).toContain('md-math--error')
})
test('metadata stays hidden and targets survive row reordering; duplicates are ambiguous', () => {
  const root = document.createElement('div')
  root.innerHTML = renderMarkdown(
    '<!-- table: left -->\n<!-- table-id: diagnostics -->\n| | |\n|---|---|\n| <!-- row-id: ok -->Норма | 0 |\n| <!-- row-id: fail -->Сбой | 1 |'
  )
  const result = targetResult(
    '{"kind":"markdown-target","version":1,"text":"Сбой","target":{"table":"diagnostics","row":"fail"}}'
  )!
  expect(result.text).toBe('Сбой')
  expect(findMarkdownTarget(root, result.target)?.textContent).toContain('Сбой')
  expect(root.innerHTML).not.toContain('<!--')
  expect(root.querySelectorAll('[scope="row"]')).toHaveLength(2)
  const row = findMarkdownTarget(root, result.target)!
  row.parentElement!.prepend(row)
  expect(findMarkdownTarget(root, result.target)).toBe(row)
  row.parentElement!.append(row.cloneNode(true))
  expect(findMarkdownTarget(root, result.target)).toBeNull()
  expect(targetResult('{"answer":5}')).toBeNull()
  expect(targetResult('{"kind":"markdown-target","version":2}')).toBeNull()
})

test('math delimiters distinguish display, inline and currency', () => {
  expect(readMath('$x$')?.display).toBe(false)
  expect(readMath('$$x$$', true)?.display).toBe(true)
  expect(readMath('$ x $')).toBeUndefined()
  expect(readMath('$10 and $20')).toBeUndefined()
})
