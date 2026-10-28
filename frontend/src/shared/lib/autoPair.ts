// Парные знаки при вводе в тексте заметки: `*` `**` `***`, `_`, `` ` ``, `~`, `(` `[` `{` (в том числе `{{ }}` и `script=N{ }`).
// Чистые функции: получают текст и выделение, отдают правку; поле само её применяет (так остаётся история отмены).

export type PairEdit =
  /** Replace text[start..end) with `text`; the caret goes to `caret`. */
  | { kind: 'edit'; start: number; end: number; text: string; caret: number; selEnd?: number }
  /** Only move the caret (typing a closing mark that is already there). */
  | { kind: 'move'; caret: number }

const CLOSE: Record<string, string> = { '(': ')', '[': ']', '{': '}' }
const CLOSERS = new Set(Object.values(CLOSE))
const MARKS = new Set(['*', '_', '`', '~'])
const wordChar = (c: string | undefined) => !!c && /[\p{L}\p{N}]/u.test(c)

/** How many `c` stand right before `at`, and right after it. */
function runs(text: string, at: number, c: string) {
  let before = 0
  while (text[at - 1 - before] === c) before++
  let after = 0
  while (text[at + after] === c) after++
  return { before, after }
}

/** A key typed in the field: the edit it should make instead of the plain character, or null to let the browser type it. */
export function pairKey(text: string, start: number, end: number, key: string): PairEdit | null {
  if (key.length !== 1) return null
  const open = key in CLOSE
  const mark = MARKS.has(key)
  const closer = CLOSERS.has(key)
  if (!open && !mark && !closer) return null
  const closing = open ? CLOSE[key] : key

  // A selection is wrapped and stays selected, so the same mark typed again wraps once more (`*a*` → `**a**` → `***a***`).
  if (start !== end && (open || mark)) {
    const inner = text.slice(start, end)
    return { kind: 'edit', start, end, text: key + inner + closing, caret: start + 1, selEnd: start + 1 + inner.length }
  }
  if (start !== end) return null

  const next = text[start]
  const prev = text[start - 1]

  if (closer && !mark) {
    // `}` before a `}` that is already there: step over it.
    return next === key ? { kind: 'move', caret: start + 1 } : null
  }

  if (open) {
    // Before a letter or digit a bracket is just a bracket (`(` in front of a word you are about to wrap by hand).
    if (wordChar(next)) return null
    return { kind: 'edit', start, end, text: key + closing, caret: start + 1 }
  }

  // A mark: `*` `_` `` ` `` `~`.
  const { before, after } = runs(text, start, key)
  // Emphasis opens at a boundary: not in the middle of a word (snake_case, 2*3).
  const boundary = !wordChar(text[start - before - 1])
  if (before > 0 && after === before && boundary) {
    // `*|*` → `**|**`: the pair widens on both sides.
    return { kind: 'edit', start, end, text: key + key, caret: start + 1 }
  }
  if (before > 0 && after === 0 && boundary && key !== '`') {
    // `*|` typed at the start of something, nothing closes it yet: open the wider pair `**|**` at once.
    return { kind: 'edit', start, end, text: key + key.repeat(before + 1), caret: start + 1 }
  }
  // A closing mark that is already there (`*abc|*`): step over it.
  if (after > 0) return { kind: 'move', caret: start + 1 }
  if (wordChar(prev) || wordChar(next)) return null
  // A bullet at the start of a line (`* item`) is not emphasis: the first `*` stays alone, a second one widens it into `**|**`.
  const lineStart = text.lastIndexOf('\n', start - 1) + 1
  if (key === '*' && /^\s*$/.test(text.slice(lineStart, start)) && (next === undefined || /\s/.test(next))) return null
  return { kind: 'edit', start, end, text: key + key, caret: start + 1 }
}

/** Backspace inside an empty pair removes both halves: `{|}` and `**|**`. */
export function pairBackspace(text: string, start: number, end: number): PairEdit | null {
  if (start !== end || start < 1) return null
  const prev = text[start - 1]
  const next = text[start]
  if (prev in CLOSE && next === CLOSE[prev])
    return { kind: 'edit', start: start - 1, end: start + 1, text: '', caret: start - 1 }
  if (MARKS.has(prev) && next === prev) {
    const { before, after } = runs(text, start, prev)
    if (before === after) return { kind: 'edit', start: start - 1, end: start + 1, text: '', caret: start - 1 }
  }
  return null
}
