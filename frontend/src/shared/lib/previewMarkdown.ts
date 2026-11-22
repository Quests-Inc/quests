/**
 * A markdown prefix for a hover card. Whole blocks only, and a cut never stays
 * inside a comment, a fence, a link, or `**` / `__`. The markers `table`,
 * `table-id`, `row-id` and `span-id` are kept only as a finished comment;
 * a table row whose marker would be cut does not start.
 */

const FENCE = /^( {0,3})(`{3,}|~{3,})(.*)$/
const TABLE_MARK = /^<!--\s*(?:table-id|table)\b[\s\S]*?-->\s*$/
const TABLE_ROW = /^\s*\|/

function runes(s: string) {
  return [...s]
}

function isFence(block: string) {
  return FENCE.test(block)
}

function fenceClosed(block: string) {
  const lines = block.split('\n')
  const open = lines[0]?.match(FENCE)
  if (!open) return true
  const mark = open[2][0]
  const width = open[2].length
  return lines.slice(1).some((line) => {
    const close = line.match(FENCE)
    return !!close && close[2][0] === mark && close[2].length >= width && close[3].trim() === ''
  })
}

function lastUnclosedComment(s: string) {
  const open = s.lastIndexOf('<!--')
  if (open < 0 || s.indexOf('-->', open + 4) >= 0) return -1
  return open
}

function lastUnclosedFence(s: string) {
  const lines = s.split('\n')
  let openAt = -1
  let mark = ''
  let width = 0
  let at = 0
  for (const line of lines) {
    const m = line.match(FENCE)
    if (m) {
      if (!mark) {
        mark = m[2][0]
        width = m[2].length
        openAt = at
      } else if (m[2][0] === mark && m[2].length >= width && m[3].trim() === '') {
        mark = ''
        openAt = -1
      }
    }
    at += line.length + 1
  }
  return openAt
}

function lastUnclosedLink(s: string) {
  let code = false
  let open = -1
  let expectParen = false
  for (let i = 0; i < s.length; i++) {
    const ch = s[i]
    if (ch === '`') {
      code = !code
      continue
    }
    if (code) continue
    if (ch === '[' && s[i - 1] !== '\\') {
      open = s[i - 1] === '!' ? i - 1 : i
      expectParen = false
    } else if (ch === ']' && open >= 0 && !expectParen) {
      if (s[i + 1] === '(') {
        expectParen = true
        i++
      } else open = -1
    } else if (expectParen && ch === ')') {
      open = -1
      expectParen = false
    }
  }
  return open
}

function lastUnclosedMark(s: string, mark: string) {
  let count = 0
  let last = -1
  let code = false
  for (let i = 0; i < s.length; i++) {
    if (s[i] === '`') {
      code = !code
      continue
    }
    if (code) continue
    if (s.startsWith(mark, i)) {
      count++
      last = i
      i += mark.length - 1
    }
  }
  return count % 2 === 1 ? last : -1
}

function rollback(slice: string) {
  let s = slice
  for (let n = 0; n < 8; n++) {
    const fence = lastUnclosedFence(s)
    const comment = lastUnclosedComment(s)
    const link = lastUnclosedLink(s)
    const bold = lastUnclosedMark(s, '**')
    const under = lastUnclosedMark(s, '__')
    const cut = Math.max(fence, comment, link, bold, under)
    if (cut < 0) return s
    s = s.slice(0, cut)
  }
  return s
}

function markersClosed(s: string) {
  return (
    lastUnclosedComment(s) < 0 &&
    lastUnclosedLink(s) < 0 &&
    lastUnclosedMark(s, '**') < 0 &&
    lastUnclosedMark(s, '__') < 0
  )
}

function splitBlocks(src: string) {
  const lines = src.split('\n')
  const blocks: string[] = []
  let i = 0
  while (i < lines.length) {
    if (lines[i].trim() === '') {
      i++
      continue
    }
    if (FENCE.test(lines[i])) {
      const open = lines[i].match(FENCE)!
      const mark = open[2][0]
      const width = open[2].length
      let j = i + 1
      while (j < lines.length) {
        const close = lines[j].match(FENCE)
        if (close && close[2][0] === mark && close[2].length >= width && close[3].trim() === '') {
          j++
          break
        }
        j++
      }
      blocks.push(lines.slice(i, j).join('\n'))
      i = j
      continue
    }
    if (TABLE_MARK.test(lines[i].trim()) || TABLE_ROW.test(lines[i])) {
      let j = i
      while (j < lines.length && TABLE_MARK.test(lines[j].trim())) j++
      if (j < lines.length && TABLE_ROW.test(lines[j])) {
        while (
          j < lines.length &&
          lines[j].trim() !== '' &&
          (TABLE_ROW.test(lines[j]) || /^\s*\|?\s*:?-{3,}/.test(lines[j]))
        )
          j++
        blocks.push(lines.slice(i, j).join('\n'))
        i = j
        continue
      }
    }
    let j = i + 1
    while (j < lines.length && lines[j].trim() !== '') j++
    blocks.push(lines.slice(i, j).join('\n'))
    i = j
  }
  return blocks
}

function takeTable(block: string, budget: number) {
  if (budget <= 0) return ''
  const lines = block.split('\n')
  let headerEnd = 0
  while (headerEnd < lines.length && TABLE_MARK.test(lines[headerEnd].trim())) headerEnd++
  if (headerEnd < lines.length && TABLE_ROW.test(lines[headerEnd])) {
    headerEnd++
    if (headerEnd < lines.length && /^\s*\|?\s*:?-{3,}/.test(lines[headerEnd])) headerEnd++
  }
  const header = lines.slice(0, headerEnd).join('\n')
  if (!header || !markersClosed(header) || runes(header).length > budget) return ''
  let out = header
  for (const row of lines.slice(headerEnd)) {
    const next = `${out}\n${row}`
    if (runes(next).length > budget || !markersClosed(row)) break
    out = next
  }
  return out
}

function join(out: string, block: string) {
  return out ? `${out}\n\n${block}` : block
}

/** Markdown that is safe to render as the visible start of `source`. */
export function previewMarkdown(source: string, max = 320) {
  const src = String(source ?? '')
    .replace(/\r\n/g, '\n')
    .trim()
  if (!src || runes(src).length <= max) return src
  let out = ''
  let cut = false
  for (const block of splitBlocks(src)) {
    const first = block.split('\n')[0] ?? ''
    if (isFence(block)) {
      const next = join(out, block)
      if (fenceClosed(block) && runes(next).length <= max) {
        out = next
        continue
      }
      cut = true
      break
    }
    if (TABLE_MARK.test(first.trim()) || TABLE_ROW.test(first)) {
      const room = max - runes(out ? `${out}\n\n` : '').length
      const piece = takeTable(block, room)
      if (!piece) {
        cut = true
        break
      }
      out = join(out, piece)
      if (piece !== block) cut = true
      if (piece !== block) break
      continue
    }
    const next = join(out, block)
    if (runes(next).length <= max && markersClosed(block)) {
      out = next
      continue
    }
    cut = true
    if (!out) {
      const partial = rollback(runes(block).slice(0, max).join('')).trimEnd()
      if (partial && markersClosed(partial)) out = partial
    }
    break
  }
  const trimmed = out.trim()
  if (!trimmed) return ''
  return cut ? `${trimmed}\n\n…` : trimmed
}
