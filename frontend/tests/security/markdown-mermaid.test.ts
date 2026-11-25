import { expect, test } from 'bun:test'

import { renderMermaidDiagrams } from '@/shared/lib/markdownMermaid.ts'

function codeBlock(code: string): HTMLElement {
  const block = document.createElement('div')
  block.className = 'md-codeblock'
  block.setAttribute('data-lang', 'mermaid')
  const pre = document.createElement('pre')
  const codeEl = document.createElement('code')
  codeEl.textContent = code
  pre.append(codeEl)
  block.append(pre)
  document.body.append(block)
  return block
}

test('mermaid pass marks every block and keeps the source on failure', async () => {
  const first = codeBlock('flowchart TD\n  a --> b')
  const second = codeBlock('flowchart TD\n  x --> y')
  await renderMermaidDiagrams(document.body)
  // In the test environment mermaid cannot lay out: the pass must degrade to the
  // source block with an explicit error marker, never leave a "pending" limbo.
  expect(first.dataset.mdMermaid).toBe('error')
  expect(second.dataset.mdMermaid).toBe('error')
  expect(first.querySelector('pre')?.textContent).toContain('flowchart TD')
  // A second pass must skip already processed blocks entirely.
  await renderMermaidDiagrams(document.body)
  expect(document.querySelectorAll('.md-mermaid')).toHaveLength(0)
})
