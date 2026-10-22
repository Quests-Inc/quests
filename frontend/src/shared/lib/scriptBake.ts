// What a text looks like in an exported file: values filled in, scripts as the view shows them, `?auto` computed.
// Nothing is stored.
import { listValues } from '@/shared/api/client.ts'
import { evalScript } from '@/shared/api/scripts.ts'

import { foreignValueRefs } from './markdown.ts'
import { escapeMdInline, labelParts, MAX_AUTO_SCRIPTS, scriptEmbedRe, scriptFlags } from './scriptRef.ts'
import { proseValue } from './valueText.ts'

const FENCE = /(```[\s\S]*?```|`[^`]+`)/

async function mapPool<T>(items: T[], limit: number, fn: (item: T) => Promise<void>) {
  if (!items.length) return
  let next = 0
  async function worker() {
    while (next < items.length) {
      const i = next++
      await fn(items[i])
    }
  }
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, () => worker()))
}

const MUSTACHE = /\{\{\s*(?:(note|quest|questline|step|template)=(\d+)\.)?([a-z][a-z0-9_]{0,23})\s*\}\}/g

/** `{{name}}` and `{{note=4.name}}` become their text; a name nobody defines stays as written. */
function fillValues(text: string, values: Record<string, string>) {
  return text.replace(MUSTACHE, (full, kind, id, name) => {
    const key = kind ? `${kind}=${id}.${name}` : name
    return Object.prototype.hasOwnProperty.call(values, key) ? escapeMdInline(values[key]) : full
  })
}

/** The formula as a reader sees it: each named value replaced by its text. */
function plainLabel(arg: string, values: Record<string, string>) {
  return escapeMdInline(labelParts(arg, values).map((part) => part.text).join(''))
}

async function bakeChunk(
  chunk: string,
  host: { kind: string; id: number } | undefined,
  values: Record<string, string>,
  budget: number
): Promise<[string, number]> {
  const matches = [...chunk.matchAll(scriptEmbedRe())]
  const picked = matches.filter((m) => scriptFlags(m[3]).auto).slice(0, Math.max(0, budget))
  const rendered = new Map<number, string>()
  for (const m of matches) {
    const flags = scriptFlags(m[3])
    // Click-only: the reader of the file has no click, so it shows what the view shows before one.
    rendered.set(m.index ?? 0, flags.hide ? '?' : plainLabel(m[2].trim(), values))
  }
  await mapPool(picked, 4, async (m) => {
    const arg = m[2].trim()
    const hide = scriptFlags(m[3]).hide
    const label = plainLabel(arg, values)
    let text = hide ? '?' : `${label} — ошибка`
    try {
      const out = await evalScript(Number(m[1]), { _: arg }, host)
      text = hide ? escapeMdInline(out.value) : `${label} = ${escapeMdInline(out.value)}`
    } catch {
      /* a hidden formula stays hidden; the file is still written */
    }
    rendered.set(m.index ?? 0, text)
  })
  let text = ''
  let pos = 0
  for (const m of matches) {
    const at = m.index ?? 0
    text += fillValues(chunk.slice(pos, at), values) + (rendered.get(at) ?? '')
    pos = at + m[0].length
  }
  text += fillValues(chunk.slice(pos), values)
  return [text, picked.length]
}

/** Effective values of the text's owner and of every `{{kind=id.name}}` it cites, by the name the text uses. */
async function exportValues(source: string, host?: { kind: string; id: number }) {
  const out: Record<string, string> = {}
  const owners = new Map<string, { kind: string; id: number }>()
  if (host && (source.includes('{{') || source.includes('script='))) owners.set(`${host.kind}:${host.id}`, host)
  for (const ref of foreignValueRefs(source)) owners.set(`${ref.kind}:${ref.id}`, { kind: ref.kind, id: ref.id })
  await Promise.all(
    [...owners.entries()].map(async ([key, owner]) => {
      try {
        const rows = (await listValues(owner.kind, owner.id)).effective ?? []
        for (const row of rows) {
          if (row.error || row.value == null) continue
          const text = proseValue(row.type, row.value)
          if (host && key === `${host.kind}:${host.id}`) out[row.name] = text
          out[`${owner.kind}=${owner.id}.${row.name}`] = text
        }
      } catch {
        /* the values stay as written; the file is still written */
      }
    })
  )
  return out
}

/**
 * A text as the file shows it, outside code fences: `{{name}}` becomes its value, `script=N{f}` the formula with its
 * values in it, `?auto` the formula and its result. At most 16 `?auto` per text; the host is the text's owner.
 */
export async function bakeAutoScripts(source: string, host?: { kind: string; id: number }) {
  source = String(source ?? '')
  const values = await exportValues(source, host)
  const parts = source.split(FENCE)
  let budget = MAX_AUTO_SCRIPTS
  const out: string[] = []
  for (let i = 0; i < parts.length; i++) {
    const part = parts[i] ?? ''
    if (i % 2 === 1) {
      out.push(part)
      continue
    }
    const [next, used] = await bakeChunk(part, host, values, budget)
    budget -= used
    out.push(next)
  }
  return out.join('')
}
