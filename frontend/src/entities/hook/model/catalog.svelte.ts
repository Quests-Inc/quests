// Каталог событий хуков (`GET /api/hook-events`): один на всё приложение, грузится один раз. По нему события называются
// по-русски; коды остаются в подсказках.
import { getHookEvents, type HookEventInfo } from '@/shared/api/client.ts'

export const hookCatalog = $state({
  events: [] as HookEventInfo[],
  aliases: {} as Record<string, string[]>,
  loaded: false,
})

let inflight: Promise<void> | null = null

export function loadHookCatalog(): Promise<void> {
  if (hookCatalog.loaded) return Promise.resolve()
  if (inflight) return inflight
  inflight = getHookEvents()
    .then((c) => {
      hookCatalog.events = c.events ?? []
      hookCatalog.aliases = c.aliases ?? {}
      hookCatalog.loaded = true
    })
    .catch(() => {})
    .finally(() => {
      inflight = null
    })
  return inflight
}

export const FAMILY_WORDS: Record<string, string> = {
  quest: 'Квест',
  step: 'Шаг',
  note: 'Заметка',
  value: 'Переменная',
  attachment: 'Вложение',
  template: 'Шаблон',
}

/** Event kinds with the short aliases (`complete`, `note`…) unfolded into the kinds they mean. */
export function expandEvents(events: string[], aliases: Record<string, string[]>): string[] {
  const out = new Set<string>()
  for (const e of events) for (const k of aliases[e]?.length ? aliases[e] : [e]) out.add(k)
  return [...out]
}

/** How a hook's events read: a whole family as «все события заметки», the rest by their titles. */
export function eventWords(kinds: string[], events: HookEventInfo[]): string[] {
  if (!events.length) return kinds
  const chosen = new Set(kinds)
  const out: string[] = []
  const seen = new Set<string>()
  for (const fam of Object.keys(FAMILY_WORDS)) {
    const all = events.filter((e) => e.family === fam)
    if (all.length > 1 && all.every((e) => chosen.has(e.kind))) {
      out.push(`все события: ${FAMILY_WORDS[fam].toLowerCase()}`)
      for (const e of all) seen.add(e.kind)
    }
  }
  for (const k of kinds) {
    if (seen.has(k)) continue
    out.push(events.find((e) => e.kind === k)?.title ?? k)
  }
  return out
}
