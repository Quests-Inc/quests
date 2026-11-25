import { expect, test } from 'bun:test'

import { applyTaskMarks, renderMarkdown, taskMarksFromSource } from '@/shared/lib/markdown.ts'
import { readMath } from '@/shared/lib/markdownMath.ts'
import { findMarkdownTarget, targetResult } from '@/shared/lib/markdownTargets.ts'

test('static GFM preserves quotes, headings, strikethrough and tickable task checkboxes', () => {
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
    // Ticks are viewing-only scratch state: enabled, but never a script vector.
    expect(input.disabled).toBe(false)
    expect(input.classList.contains('md-task')).toBe(true)
  }
  expect((root.querySelectorAll('input')[0] as HTMLInputElement).checked).toBe(true)
  expect((root.querySelectorAll('input')[1] as HTMLInputElement).checked).toBe(false)
  expect(html).not.toContain('onclick')
})
test('details/summary survive sanitization with markdown content inside; scripts do not', () => {
  const html = renderMarkdown(
    '<details>\n<summary>Таблица переходов</summary>\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n</details>'
  )
  expect(html).toContain('<details>')
  expect(html).toContain('<summary>Таблица переходов</summary>')
  expect(html).toContain('<table>')
  const evil = renderMarkdown(
    '<details open onclick="alert(1)"><summary>x</summary><script>alert(1)</script></details>'
  )
  expect(evil).not.toContain('onclick')
  expect(evil).not.toContain('<script')
})
test('mermaid fences stay ordinary code blocks in the sync render', () => {
  const html = renderMarkdown('```mermaid\nflowchart TD\n  a --> b\n```')
  expect(html).toContain('data-lang="mermaid"')
  expect(html).toContain('flowchart TD')
  expect(html).not.toContain('<svg')
})
test('task marks rewrite the source positionally and skip code; mismatch returns null', () => {
  const src =
    'До:\n\n- [ ] Один\n- [x] Два\n\n```md\n- [ ] не список, а текст\n```\n\n1. [ ] Три\n\nПосле `$x$` и `- [ ]` в коде.'
  expect(taskMarksFromSource(src)).toEqual([false, true, false])
  const next = applyTaskMarks(src, [true, false, true])!
  expect(next).toContain('- [x] Один')
  expect(next).toContain('- [ ] Два')
  expect(next).toContain('1. [x] Три')
  expect(next).toContain('```md\n- [ ] не список, а текст\n```')
  expect(taskMarksFromSource(next)).toEqual([true, false, true])
  expect(next).toBe(
    src.replace('- [ ] Один', '- [x] Один').replace('- [x] Два', '- [ ] Два').replace('1. [ ] Три', '1. [x] Три')
  )
  expect(applyTaskMarks(src, [true, false, true, false])).toBeNull()
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
