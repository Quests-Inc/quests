import { untrack } from 'svelte'

import { searchEntities, type EntitySearchRequest, type EntitySearchHit } from '@/shared/api/entitySearch.ts'

import { draftHits, type CatalogTitle } from './entitySearchDraft.ts'
import type { ParsedQuery, FlagSpec } from './searchFlags.ts'

/** Transfer the widget vocabulary, preserving AND/OR/negation semantics. */
export function searchFilters(parsed: ParsedQuery, spec: FlagSpec) {
  const out: { key: string; values: string[]; not?: boolean; all?: boolean }[] = []
  for (const term of parsed.terms) {
    if (term.neg) out.push({ key: term.key, values: [term.value], not: true })
    else {
      let group = out.find((f) => f.key === term.key && !f.not)
      if (!group) {
        group = { key: term.key, values: [], all: !!spec[term.key]?.all }
        out.push(group)
      }
      group.values.push(term.value)
    }
  }
  return out
}

/** One async lifecycle for list search and pickers. A new query keeps the previous rows until the server answers. */
export function createEntitySearch(
  input: () => EntitySearchRequest | null,
  options: { all?: boolean; delay?: number; catalog?: () => CatalogTitle[] } = {}
) {
  let hits = $state<EntitySearchHit[] | null>(null)
  let pending = $state(false)
  let error = $state('')
  let latest: EntitySearchHit[] | null = null
  let previous: EntitySearchRequest | null = null
  const cache = new Map<string, EntitySearchHit[]>()
  const requestKey = $derived(JSON.stringify(input()))
  function show(rows: EntitySearchHit[] | null) {
    latest = rows
    hits = rows
  }
  function remember(key: string, rows: EntitySearchHit[]) {
    cache.delete(key)
    cache.set(key, rows)
    while (cache.size > 64) {
      const oldest = cache.keys().next().value
      if (oldest === undefined) break
      cache.delete(oldest)
    }
  }
  $effect(() => {
    const query: EntitySearchRequest | null = JSON.parse(requestKey)
    if (!query) {
      previous = null
      show(null)
      pending = false
      error = ''
      return
    }
    const before = previous
    previous = query
    error = ''
    pending = true
    const cached = cache.get(requestKey)
    const catalog = untrack(() => options.catalog?.() ?? null)
    if (cached) show(cached)
    else show(draftHits(before, latest, query, catalog))
    const abort = new AbortController()
    const timer = setTimeout(async () => {
      try {
        let offset = 0
        const all: EntitySearchHit[] = []
        do {
          const page = await searchEntities({ ...query, offset, limit: query.limit ?? 100 }, abort.signal)
          if (abort.signal.aborted) return
          all.push(...page.hits)
          if (!options.all || !page.more || page.next <= offset) break
          offset = page.next
        } while (!abort.signal.aborted)
        if (!abort.signal.aborted) {
          show(all)
          remember(requestKey, all)
        }
      } catch (e) {
        if (!abort.signal.aborted) error = e instanceof Error ? e.message : String(e)
      } finally {
        if (!abort.signal.aborted) pending = false
      }
    }, options.delay ?? 320)
    return () => {
      clearTimeout(timer)
      abort.abort()
    }
  })
  return {
    get hits() {
      return hits
    },
    get pending() {
      return pending
    },
    get error() {
      return error
    },
    select<T extends { id: number }>(items: T[], kind: string): T[] {
      if (hits == null) return items
      const byId = new Map(items.map((item) => [item.id, item]))
      return hits
        .filter((h) => h.kind === kind)
        .flatMap((h) => {
          const item = byId.get(h.id)
          return item ? [item] : []
        })
    },
  }
}

export function searchTypes(parsed: ParsedQuery, allowed: string[]) {
  const positive = parsed.terms.filter((t) => t.key === 'type' && !t.neg).map((t) => t.value)
  const negative = parsed.terms.filter((t) => t.key === 'type' && t.neg).map((t) => t.value)
  return allowed.filter((t) => (!positive.length || positive.includes(t)) && !negative.includes(t))
}
