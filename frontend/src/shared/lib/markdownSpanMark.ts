// Pair markers in already-parsed HTML. No note fetch: rendering must not pull the API client.

const KEY = '[a-z][a-z0-9_-]{0,63}'

function commentMark(data: string): { close: boolean; key: string } | null {
  const open = new RegExp(`^\\s*span-id:\\s*(${KEY})\\s*$`).exec(data)
  if (open) return { close: false, key: open[1] }
  const close = new RegExp(`^\\s*/span-id:\\s*(${KEY})\\s*$`).exec(data)
  if (close) return { close: true, key: close[1] }
  return null
}

/** Wrap each valid pair. Comments themselves are removed, so preview shows only the piece. */
export function markSpanTargets(dirty: string): string {
  if (!dirty.includes('span-id:')) return dirty
  const doc = window.document.implementation.createHTMLDocument('')
  doc.body.innerHTML = dirty
  const comments: Comment[] = []
  const walk = doc.createTreeWalker(doc.body, 128)
  while (walk.nextNode()) comments.push(walk.currentNode as Comment)
  const marks = comments.flatMap((node) => {
    const mark = commentMark(node.data)
    return mark ? [{ node, ...mark }] : []
  })
  const stack: { node: Comment; key: string }[] = []
  const seen = new Set<string>()
  const bad = new Set<string>()
  const pairs: { open: Comment; close: Comment; key: string }[] = []
  for (const mark of marks) {
    if (!mark.close) {
      if (seen.has(mark.key)) bad.add(mark.key)
      seen.add(mark.key)
      stack.push(mark)
      continue
    }
    const top = stack.at(-1)
    if (top?.key === mark.key) {
      stack.pop()
      pairs.push({ open: top.node, close: mark.node, key: mark.key })
      continue
    }
    bad.add(mark.key)
    if (top) bad.add(top.key)
    const idx = stack.findIndex((open) => open.key === mark.key)
    if (idx >= 0) {
      for (const open of stack.slice(idx)) bad.add(open.key)
      stack.splice(idx)
    }
  }
  for (const open of stack) bad.add(open.key)
  for (const pair of pairs) {
    if (bad.has(pair.key)) continue
    if (pair.open.parentNode !== pair.close.parentNode || !pair.open.parentNode) continue
    const parent = pair.open.parentNode
    const inline =
      parent instanceof HTMLElement && /^(P|SPAN|A|LI|TD|TH|H[1-6]|BLOCKQUOTE|EM|STRONG|DEL)$/.test(parent.tagName)
    const box = doc.createElement(inline ? 'span' : 'div')
    box.setAttribute('data-md-span', pair.key)
    parent.insertBefore(box, pair.open)
    let node: ChildNode | null = pair.open
    while (node) {
      const next: ChildNode | null = node.nextSibling
      box.appendChild(node)
      if (node === pair.close) break
      node = next
    }
    pair.open.remove()
    pair.close.remove()
  }
  return doc.body.innerHTML
}
