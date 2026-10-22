// `script=N{arg}` in a text. Parameters are written by the author, nothing is inferred from the formula:
// `?auto` computes when the text is shown, `?hide` hides the formula (`?` until a click, if it is not auto),
// `?on=label` runs a script with code on that runner and waits for it (never on show: the code runs only on a click),
// `?set=name` writes the result of a click into the value `name` of the text's owner (never on show, so it beats `?auto`).

const FLAG = String.raw`auto(?:=1)?|hide(?:=1)?|set=[a-z][a-z0-9_]{0,23}|on=[a-z0-9][a-z0-9_-]{0,31}`
const SCRIPT_EMBED_SOURCE = String.raw`\bscript=(\d+)\{([^{}\[\]\n]{1,200})\}(\?(?:${FLAG})(?:&(?:${FLAG})){0,2}\b)?`

/** A fresh global regex. Callers must not share one instance across overlapping scans. */
export function scriptEmbedRe() {
  return new RegExp(SCRIPT_EMBED_SOURCE, 'g')
}

/** How many `?auto` embeddings one text computes. The rest stay click links. */
export const MAX_AUTO_SCRIPTS = 16

export type ScriptFlags = { auto: boolean; hide: boolean; set: string; on: string }

/** `?auto`, `?hide`, `?auto&hide` — either order, `=1` optional. A longer word (`?automatic`) is not a flag. */
export function scriptFlags(query: string | undefined): ScriptFlags {
  const q = String(query ?? '').replace(/^\?/, '')
  const on = (name: string) => new RegExp(`(?:^|&)${name}(?:=1)?(?:&|$)`).test(q)
  const set = /(?:^|&)set=([a-z][a-z0-9_]{0,23})(?:&|$)/.exec(q)?.[1] ?? ''
  const runner = /(?:^|&)on=([a-z0-9][a-z0-9_-]{0,31})(?:&|$)/.exec(q)?.[1] ?? ''
  return { auto: on('auto') && !set && !runner, hide: on('hide'), set, on: runner }
}

export function scriptHref(id: string, arg: string, flags: { auto?: boolean; hide?: boolean; set?: string; on?: string } = {}) {
  let href = `?script=${id}&_=${encodeURIComponent(arg)}`
  if (flags.auto) href += '&auto=1'
  if (flags.hide) href += '&hide=1'
  if (flags.set) href += `&set=${flags.set}`
  if (flags.on) href += `&on=${flags.on}`
  return href
}

export function parseScriptHref(href: string): { id: number; arg: string; auto: boolean; hide: boolean; set: string; on: string } | null {
  if (!href.startsWith('?script=')) return null
  const q = new URLSearchParams(href.slice(1))
  const id = Number(q.get('script'))
  const arg = q.get('_')
  if (!Number.isInteger(id) || id <= 0 || arg == null || arg.length > 200) return null
  const set = q.get('set') ?? ''
  if (set && !/^[a-z][a-z0-9_]{0,23}$/.test(set)) return null
  const on = q.get('on') ?? ''
  if (on && !/^[a-z0-9][a-z0-9_-]{0,31}$/.test(on)) return null
  return { id, arg, auto: q.get('auto') === '1' && !set && !on, hide: q.get('hide') === '1', set, on }
}

/** What a reader sees before a roll: the formula, or `?` when it is hidden. */
export function scriptLabel(arg: string, flags: { hide?: boolean }) {
  return flags.hide ? '?' : arg
}

/** `*` and `_` in a formula must stay characters, not markdown emphasis. */
export function escapeMdInline(s: string) {
  return s.replace(/[\\`*_{}\[\]]/g, (ch) => `\\${ch}`)
}

export type LabelPart = { text: string; name?: string }

/** A formula split into plain text and the named values it reads. `2к4+bite_dmg` → `2к4+`, then the value of `bite_dmg`. */
export function labelParts(arg: string, values: Record<string, string>): LabelPart[] {
  const out: LabelPart[] = []
  let last = 0
  for (const m of arg.matchAll(/(?<![a-z0-9_])[a-z][a-z0-9_]*/g)) {
    const name = m[0]
    if (!Object.prototype.hasOwnProperty.call(values, name)) continue
    const at = m.index ?? 0
    if (at > last) out.push({ text: arg.slice(last, at) })
    out.push({ text: values[name], name })
    last = at + name.length
  }
  if (last < arg.length) out.push({ text: arg.slice(last) })
  return out
}
