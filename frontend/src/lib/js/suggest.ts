/** Questline / section suggestion for new quests and templates (quest=205).
 *
 * TF-IDF over words + character 3-grams of title (×2) and description,
 * cosine similarity, weighted vote of the k nearest existing quests. Runs
 * entirely in the browser over the quests the page already has — no server
 * round-trips. Parameters come from an offline evaluation on real data
 * (chronological, template duplicates excluded): for the questline ~40–48%
 * of quests get a suggestion and ~97% of those are right.
 */

export const SUGGEST_DEFAULTS = {
  k: 3,
  /** The leader's share of the neighbours' summed similarity. */
  share: 0.7,
  /** Similarity of the best neighbour; below it nothing is "similar". */
  minSim: 0.1,
}

/** Looser bounds that keep an already-shown suggestion on screen, so it
 * doesn't blink out and back on every keystroke near the threshold. */
const KEEP = { share: 0.55, minSim: 0.07 }

const TITLE_WEIGHT = 2

function tokens(text) {
  const words =
    String(text || '')
      .toLowerCase()
      .replaceAll('ё', 'е')
      .match(/[a-zа-я0-9]+/g) || []
  const out = [...words]
  for (const w of words) {
    const p = ` ${w} `
    for (let i = 0; i + 3 <= p.length; i++) out.push(p.slice(i, i + 3))
  }
  return out
}

function termCounts(title, description) {
  const tf = new Map()
  for (const t of tokens(title)) tf.set(t, (tf.get(t) || 0) + TITLE_WEIGHT)
  for (const t of tokens(description)) tf.set(t, (tf.get(t) || 0) + 1)
  return tf
}

function vectorize(tf, df, n) {
  const v = new Map()
  let norm = 0
  for (const [t, c] of tf) {
    const w = (1 + Math.log(c)) * Math.log((n + 1) / ((df.get(t) || 0) + 1) + 1)
    v.set(t, w)
    norm += w * w
  }
  norm = Math.sqrt(norm) || 1
  for (const [t, w] of v) v.set(t, w / norm)
  return v
}

function cosine(a, b) {
  if (a.size > b.size) [a, b] = [b, a]
  let s = 0
  for (const [t, w] of a) {
    const x = b.get(t)
    if (x) s += w * x
  }
  return s
}

/**
 * @param {any[]} quests — anything with title/description/questline_id/category_id/tags
 */
export function buildSuggestIndex(quests: any[]) {
  const docs = (quests || []).map((q) => ({
    questline: q.questline_id ?? null,
    category: q.category_id ?? null,
    tags: Array.isArray(q.tags) ? q.tags : [],
    tf: termCounts(q.title, q.description),
  }))
  const df = new Map()
  for (const d of docs) for (const t of d.tf.keys()) df.set(t, (df.get(t) || 0) + 1)
  for (const d of docs) d.vec = vectorize(d.tf, df, docs.length)
  return { docs, df, n: docs.length }
}

/**
 * Vote of the nearest quests for `target`. Returns the leader, or null.
 * For 'category' only questline-free quests vote: elsewhere the section is
 * inherited from the questline and says nothing about the text.
 * @param {ReturnType<typeof buildSuggestIndex>} index
 * @param {{ title: string, description?: string }} query
 * @param {'questline' | 'category'} target
 */
export function nearestVote(
  index: ReturnType<typeof buildSuggestIndex>,
  query: { title: string; description?: string },
  target: 'questline' | 'category',
  k = SUGGEST_DEFAULTS.k
) {
  if (!index?.n) return null
  const q = vectorize(termCounts(query.title, query.description), index.df, index.n)
  if (!q.size) return null
  const pool = target === 'category' ? index.docs.filter((d) => d.questline == null) : index.docs
  const top = pool
    .map((d) => ({ d, s: cosine(q, d.vec) }))
    .sort((a, b) => b.s - a.s)
    .slice(0, k)
  if (!top.length) return null
  const votes = new Map()
  let total = 0
  for (const { d, s } of top) {
    const label = d[target]
    votes.set(label, (votes.get(label) || 0) + s)
    total += s
  }
  let leader = null
  let weight = -1
  for (const [label, w] of votes) {
    if (w > weight) [leader, weight] = [label, w]
  }
  return { id: leader, share: total ? weight / total : 0, sim: top[0].s }
}

/**
 * Rank tags from k nearest quests. Returns [{tag, score}] above minSim, excluding
 * already-selected slugs. Score = sum of neighbour similarities carrying that tag.
 */
export function suggestTags(
  index,
  query,
  { excludeSlugs = [], k = SUGGEST_DEFAULTS.k, minSim = SUGGEST_DEFAULTS.minSim, limit = 5 } = {}
) {
  if (!index?.n) return []
  const q = vectorize(termCounts(query.title, query.description), index.df, index.n)
  if (!q.size) return []
  const top = index.docs
    .map((d) => ({ d, s: cosine(q, d.vec) }))
    .filter((x) => x.s >= minSim)
    .sort((a, b) => b.s - a.s)
    .slice(0, k)
  if (!top.length) return []
  const ex = new Set((excludeSlugs || []).map((s) => String(s).toLowerCase()))
  /** @type {Map<number, { tag: any, score: number }>} */
  const votes: Map<number, { tag: any; score: number }> = new Map()
  for (const { d, s } of top) {
    for (const tag of d.tags || []) {
      if (!tag?.id || ex.has(String(tag.slug || '').toLowerCase())) continue
      const cur = votes.get(tag.id)
      if (cur) cur.score += s
      else votes.set(tag.id, { tag, score: s })
    }
  }
  return [...votes.values()].sort((a, b) => b.score - a.score).slice(0, limit)
}

/**
 * Decide what to show. `current` is the id on screen right now (or null):
 * it stays while it's still the leader within the looser KEEP bounds.
 * @returns {number | null}
 */
export function pickSuggestion(vote, current = null, opts = SUGGEST_DEFAULTS): number | null {
  if (!vote || vote.id == null) return null
  if (current != null && vote.id === current && vote.share >= KEEP.share && vote.sim >= KEEP.minSim) {
    return current
  }
  return vote.share >= opts.share && vote.sim >= opts.minSim ? vote.id : null
}
