import { expect, test } from 'bun:test'

import { pairBackspace, pairKey } from '../../src/shared/lib/autoPair.ts'

/** Type `keys` one by one into `text` with the caret at `at` (a `|` in the string); the result with the caret as `|`. */
function type(src, keys) {
  const sel = !src.includes('|') && src.includes('[') && src.includes(']') && !src.includes('](')
  let text = sel ? src.replace('[', '').replace(']', '') : src.replace('|', '')
  let start = sel ? src.indexOf('[') : src.indexOf('|')
  let end = sel ? src.indexOf(']') - 1 : start
  for (const k of keys) {
    const e = k === 'BS' ? pairBackspace(text, start, end) : pairKey(text, start, end, k)
    if (e?.kind === 'edit') {
      text = text.slice(0, e.start) + e.text + text.slice(e.end)
      start = e.caret
      end = e.selEnd ?? e.caret
    } else if (e?.kind === 'move') {
      start = end = e.caret
    } else if (k === 'BS') {
      text = text.slice(0, start - 1) + text.slice(end)
      start = end = start - 1
    } else {
      text = text.slice(0, start) + k + text.slice(end)
      start = end = start + 1
    }
  }
  return (
    text.slice(0, start) +
    (start === end ? '|' : '[') +
    text.slice(start, end) +
    (start === end ? '' : ']') +
    text.slice(end)
  )
}

test('звёздочки в середине строки: пара, ширится, закрывающая перешагивается', () => {
  expect(type('a |', ['*'])).toBe('a *|*')
  expect(type('a |', ['*', '*'])).toBe('a **|**')
  expect(type('a |', ['*', '*', '*'])).toBe('a ***|***')
  expect(type('a |', ['*', 'x', '*'])).toBe('a *x*|')
  expect(type('a |', ['*', '*', 'x', '*', '*'])).toBe('a **x**|')
})

test('в середине слова и маркер списка в начале строки пару не создают', () => {
  expect(type('snake|', ['_'])).toBe('snake_|')
  expect(type('2|', ['*'])).toBe('2*|')
  expect(type('|', ['*', ' '])).toBe('* |')
  expect(type('|', ['*', '*'])).toBe('**|**')
})

test('скобки: пара, {{ }} и script=N{ }, закрывающая перешагивается, перед словом — просто скобка', () => {
  expect(type('|', ['{'])).toBe('{|}')
  expect(type('|', ['{', '{'])).toBe('{{|}}')
  expect(type('|', ['{', '{', 'h', 'p', '}', '}'])).toBe('{{hp}}|')
  expect(type('script=7|', ['{'])).toBe('script=7{|}')
  expect(type('|слово', ['('])).toBe('(|слово')
  expect(type('a |', ['[', 't', ']', '(', 'u', ')'])).toBe('a [t](u)|')
})

test('выделение оборачивается и остаётся выделенным: повторный знак оборачивает ещё раз', () => {
  expect(type('[abc]', ['*'])).toBe('*[abc]*')
  expect(type('[abc]', ['*', '*'])).toBe('**[abc]**')
  expect(type('[abc]', ['`'])).toBe('`[abc]`')
})

test('Backspace внутри пустой пары убирает обе половины', () => {
  expect(type('{|}', ['BS'])).toBe('|')
  expect(type('**|**', ['BS'])).toBe('*|*')
  expect(type('*a|*', ['BS'])).toBe('*|*')
  expect(type('x|', ['BS'])).toBe('|')
})
