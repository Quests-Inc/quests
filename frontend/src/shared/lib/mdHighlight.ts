// Подсветка исходного текста заметки по мере ввода: разметка markdown и наши вставки. Возвращает HTML для слоя под полем
// ввода: ТЕКСТ НЕ МЕНЯЕТСЯ ни на символ (только обёртки <span>), иначе слой разойдётся с полем. Незакрытая пара не красится,
// пока не закрыта.

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')

const wrap = (cls: string, s: string) => `<span class="mdh-${cls}">${esc(s)}</span>`

const REF = '(?:note|quest|questline|step|attachment|template)=\\d+\\b'
const FLAG = '(?:auto(?:=1)?|hide(?:=1)?|set=[a-z][a-z0-9_]{0,23}|on=[a-z0-9][a-z0-9_,-]{0,80})'

/** Inline tokens, first match at the earliest position wins; at one position the earlier entry of the list does. */
const INLINE: { cls: string; re: RegExp; inner?: (m: RegExpExecArray) => string }[] = [
  // Ours first: a script link and a named value stay whole even inside emphasis marks.
  {
    cls: 'script',
    re: new RegExp(String.raw`\bscript=\d+\{[^{}\[\]\n]{1,200}\}(?:\?${FLAG}(?:&${FLAG}){0,2}\b)?`, 'y'),
  },
  { cls: 'value', re: /\{\{[a-z0-9_]*\}\}/y },
  { cls: 'ref', re: new RegExp(`\\b${REF}`, 'y') },
  { cls: 'code', re: /`[^`\n]+`/y },
  { cls: 'image', re: /!\[[^\]\n]*\]\([^)\n]*\)/y },
  { cls: 'link', re: /\[[^\]\n]+\]\([^)\n]*\)/y },
  { cls: 'url', re: /\bhttps?:\/\/[^\s<>)\]]+/y },
  { cls: 'bolditalic', re: /(\*\*\*|___)(?=\S)[^\n]*?\S\1/y },
  { cls: 'bold', re: /(\*\*|__)(?=\S)[^\n]*?\S\1/y },
  { cls: 'italic', re: /(?<![*\w])(\*|_)(?=[^\s*_])[^\n]*?[^\s*_]\1(?![*\w])/y },
  { cls: 'strike', re: /~~(?=\S)[^\n]*?\S~~/y },
]

const FIRST = /[`![*_~{snqtah]/ // cheap pre-filter: the characters a token can start with

/** One line of inline text → HTML. */
function inline(text: string): string {
  let out = ''
  let plain = ''
  let i = 0
  const flush = () => {
    if (plain) out += esc(plain)
    plain = ''
  }
  outer: while (i < text.length) {
    if (FIRST.test(text[i])) {
      for (const t of INLINE) {
        t.re.lastIndex = i
        const m = t.re.exec(text)
        if (m && m.index === i && m[0].length) {
          flush()
          // Emphasis can hold our tokens: colour the inside too (the marks stay in the emphasis colour).
          out += nested(t.cls, m[0])
          i += m[0].length
          continue outer
        }
      }
    }
    plain += text[i++]
  }
  flush()
  return out
}

/** Emphasis keeps the tokens inside it coloured; the rest is wrapped whole. */
function nested(cls: string, s: string): string {
  const marks = { bolditalic: 3, bold: 2, italic: 1, strike: 2 } as Record<string, number>
  const n = marks[cls]
  if (!n) return wrap(cls, s)
  const open = s.slice(0, n)
  const close = s.slice(s.length - n)
  return `<span class="mdh-${cls}">${esc(open)}${inline(s.slice(n, s.length - n))}${esc(close)}</span>`
}

/** The whole text → HTML; always ends with a space so a final newline still takes a line in the layer. */
export function highlightMarkdown(source: string): string {
  const lines = source.split('\n')
  let fence: string | null = null
  const out: string[] = []
  for (const line of lines) {
    const f = /^\s*(```+|~~~+)/.exec(line)
    if (fence) {
      out.push(wrap('code', line))
      if (f && f[1].startsWith(fence[0]) && f[1].length >= fence.length) fence = null
      continue
    }
    if (f) {
      fence = f[1]
      out.push(wrap('code', line))
      continue
    }
    let m: RegExpExecArray | null
    if ((m = /^(#{1,6})(\s.*|)$/.exec(line))) {
      out.push(`<span class="mdh-heading"><span class="mdh-mark">${esc(m[1])}</span>${inline(m[2])}</span>`)
    } else if ((m = /^(\s*>+\s?)(.*)$/.exec(line))) {
      out.push(`<span class="mdh-mark">${esc(m[1])}</span><span class="mdh-quote">${inline(m[2])}</span>`)
    } else if (/^\s*([-*_])(\s*\1){2,}\s*$/.test(line)) {
      out.push(wrap('mark', line))
    } else if (
      /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$/.test(line) &&
      line.includes('-') &&
      line.includes('|')
    ) {
      out.push(wrap('mark', line))
    } else if ((m = /^(\s*(?:[-*+]|\d+[.)])\s+(?:\[[ xX]\]\s+)?)(.*)$/.exec(line))) {
      out.push(`<span class="mdh-mark">${esc(m[1])}</span>${inline(m[2])}`)
    } else if (line.includes('|')) {
      out.push(line.split('|').map(inline).join('<span class="mdh-mark">|</span>'))
    } else {
      out.push(inline(line))
    }
  }
  return out.join('\n') + ' '
}
