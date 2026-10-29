import { expect, test } from 'bun:test'

import { fileCodeLanguage, highlightCode } from '@/shared/lib/codeHighlight.ts'

test('file previews choose explicit bundled languages and leave unknown types alone', () => {
  expect(fileCodeLanguage('Пример.PY')).toBe('python')
  expect(fileCodeLanguage('data.json')).toBe('json')
  expect(fileCodeLanguage('markup.html')).toBe('xml')
  expect(fileCodeLanguage('notes.txt')).toBe('')
  expect(fileCodeLanguage('document.pdf')).toBe('')
  expect(fileCodeLanguage('Dockerfile')).toBe('dockerfile')
})
test('code gets syntax spans while markup is escaped, including unknown languages', () => {
  const python = highlightCode('def hello():\n    return "Привет"', 'python')
  expect(python).toContain('hljs-keyword')
  expect(python).toContain('Привет')
  const html = highlightCode('<script>alert(1)</script><img src=x onerror=alert(1)>', 'xml')
  expect(html).not.toContain('<script>')
  expect(html).not.toContain('<img ')
  expect(html).toContain('&lt;')
  expect(highlightCode('<b>& text</b>', '')).toBe('&lt;b&gt;&amp; text&lt;/b&gt;')
})
