// `script=N{arg}` in a text. Parameters are written by the author, nothing is inferred from the formula:
// `?auto` computes when the text is shown, `?hide` hides the formula (`?` until a click, if it is not auto).

const FLAG = String.raw`auto(?:=1)?|hide(?:=1)?`
const SCRIPT_EMBED_SOURCE = String.raw`\bscript=(\d+)\{([^{}\[\]\n]{1,200})\}(\?(?:${FLAG})(?:&(?:${FLAG}))?\b)?`

/** A fresh global regex. Callers must not share one instance across overlapping scans. */
export function scriptEmbedRe() {
  return new RegExp(SCRIPT_EMBED_SOURCE, 'g')
}

/** How many `?auto` embeddings one text computes. The rest stay click links. */
export const MAX_AUTO_SCRIPTS = 16

export type ScriptFlags = { auto: boolean; hide: boolean }

/** `?auto`, `?hide`, `?auto&hide` — either order, `=1` optional. A longer word (`?automatic`) is not a flag. */
export function scriptFlags(query: string | undefined): ScriptFlags {
  const q = String(query ?? '').replace(/^\?/, '')
  const on = (name: string) => new RegExp(`(?:^|&)${name}(?:=1)?(?:&|$)`).test(q)
  return { auto: on('auto'), hide: on('hide') }
}

export function scriptHref(id: string, arg: string, flags: { auto?: boolean; hide?: boolean } = {}) {
  let href = `?script=${id}&_=${encodeURIComponent(arg)}`
  if (flags.auto) href += '&auto=1'
  if (flags.hide) href += '&hide=1'
  return href
}

export function parseScriptHref(href: string): { id: number; arg: string; auto: boolean; hide: boolean } | null {
  if (!href.startsWith('?script=')) return null
  const q = new URLSearchParams(href.slice(1))
  const id = Number(q.get('script'))
  const arg = q.get('_')
  if (!Number.isInteger(id) || id <= 0 || arg == null || arg.length > 200) return null
  return { id, arg, auto: q.get('auto') === '1', hide: q.get('hide') === '1' }
}

/** What a reader sees before a roll: the formula, or `?` when it is hidden. */
export function scriptLabel(arg: string, flags: { hide?: boolean }) {
  return flags.hide ? '?' : arg
}

/** `*` and `_` in a formula must stay characters, not markdown emphasis. */
export function escapeMdInline(s: string) {
  return s.replace(/[\\`*_{}\[\]]/g, (ch) => `\\${ch}`)
}
