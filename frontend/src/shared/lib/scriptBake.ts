// Replace `script=N{arg}?auto` with the computed value. Used when a text is exported, so the file
// shows the result without the app. Click-only embeddings stay as written. Nothing is stored.
import { evalScript } from '@/shared/api/scripts.ts'

import { escapeMdInline, MAX_AUTO_SCRIPTS, scriptEmbedRe, scriptFlags } from './scriptRef.ts'

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

async function bakeChunk(
  chunk: string,
  host: { kind: string; id: number } | undefined,
  budget: number
): Promise<[string, number]> {
  const matches = [...chunk.matchAll(scriptEmbedRe())]
  const picked = matches.filter((m) => scriptFlags(m[3]).auto).slice(0, Math.max(0, budget))
  const rendered = new Map<number, string>()
  for (const m of matches) {
    const flags = scriptFlags(m[3])
    if (flags.hide && !flags.auto) rendered.set(m.index ?? 0, '?')
  }
  await mapPool(picked, 4, async (m) => {
    const arg = m[2].trim()
    const hide = scriptFlags(m[3]).hide
    let text = hide ? '?' : `${arg} — ошибка`
    try {
      const out = await evalScript(Number(m[1]), { _: arg }, host)
      text = hide ? out.value : `${arg} = ${out.value}`
    } catch {
      /* a hidden formula stays hidden; the file is still written */
    }
    rendered.set(m.index ?? 0, escapeMdInline(text))
  })
  let text = chunk
  for (const index of [...rendered.keys()].sort((a, b) => b - a)) {
    const m = matches.find((item) => (item.index ?? 0) === index)
    if (!m) continue
    text = text.slice(0, index) + (rendered.get(index) ?? '') + text.slice(index + m[0].length)
  }
  return [text, picked.length]
}

/** Compute every `?auto` embedding outside code fences. At most 16 per text; the host is the text's owner. */
export async function bakeAutoScripts(source: string, host?: { kind: string; id: number }) {
  const parts = String(source ?? '').split(FENCE)
  let budget = MAX_AUTO_SCRIPTS
  const out: string[] = []
  for (let i = 0; i < parts.length; i++) {
    const part = parts[i] ?? ''
    if (i % 2 === 1) {
      out.push(part)
      continue
    }
    const [next, used] = await bakeChunk(part, host, budget)
    budget -= used
    out.push(next)
  }
  return out.join('')
}
