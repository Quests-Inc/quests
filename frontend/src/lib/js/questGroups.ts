/**
 * Group listed quests by category, then by questline within each category.
 * @param {any[]} list
 * @param {{ id: number, label: string, sort_order?: number, color?: string }[]} categories
 * @param {{ id: number, title: string, category_id?: number | null, color?: string, icon?: string }[]} questlines
 */
export function groupQuestsByCategory(
  list: any[],
  categories: { id: number; label: string; sort_order?: number; color?: string }[],
  questlines: { id: number; title: string; category_id?: number | null; color?: string; icon?: string }[]
) {
  /** @type {Map<string, any[]>} */
  const buckets: Map<string, any[]> = new Map()
  for (const q of list) {
    const key = q.category_id != null ? `c${q.category_id}` : 'none'
    if (!buckets.has(key)) buckets.set(key, [])
    buckets.get(key).push(q)
  }
  const ordered = []
  const sortedCats = [...categories].sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0) || a.id - b.id)
  for (const c of sortedCats) {
    const key = `c${c.id}`
    if (!buckets.has(key)) continue
    ordered.push({
      key,
      label: c.label,
      color: c.color || '#9a9a9a',
      ...partitionByQuestline(buckets.get(key), questlines),
    })
    buckets.delete(key)
  }
  for (const [key, items] of buckets) {
    if (key === 'none') continue
    ordered.push({
      key,
      label: items[0]?.category_label || 'Раздел',
      color: items[0]?.category_color || '#9a9a9a',
      ...partitionByQuestline(items, questlines),
    })
  }
  if (buckets.has('none')) {
    ordered.push({
      key: 'none',
      label: 'Без раздела',
      color: null,
      ...partitionByQuestline(buckets.get('none'), questlines),
    })
  }
  return ordered
}

/** @param {any[]} items @param {any[]} questlines */
export function partitionByQuestline(items: any[], questlines: any[]) {
  /** @type {Map<number, any[]>} */
  const byLine: Map<number, any[]> = new Map()
  const alone = []
  for (const q of items) {
    if (q.questline_id != null) {
      const id = Number(q.questline_id)
      if (!byLine.has(id)) byLine.set(id, [])
      byLine.get(id).push(q)
    } else {
      alone.push(q)
    }
  }
  const lineById = new Map<number, any>(questlines.map((l: any) => [l.id, l]))
  const lines: any[] = []
  for (const [id, quests] of byLine) {
    const meta = lineById.get(id)
    const sorted = [...quests].sort((a, b) => {
      const ta = a.created_at || ''
      const tb = b.created_at || ''
      if (ta !== tb) return ta < tb ? -1 : 1
      return (a.id || 0) - (b.id || 0)
    })
    lines.push({
      key: `ql${id}`,
      id,
      title: meta?.title || quests[0]?.questline_title || 'Квестлайн',
      color: meta?.color || quests[0]?.questline_color || '#9a9a9a',
      icon: meta?.icon || quests[0]?.questline_icon || 'document',
      icon_url: meta?.icon_url || quests[0]?.questline_icon_url || null,
      category_id: meta?.category_id ?? quests[0]?.category_id ?? null,
      quests: sorted,
    })
  }
  lines.sort((a, b) => a.title.localeCompare(b.title, 'ru') || a.id - b.id)
  alone.sort((a, b) => {
    const ta = a.created_at || ''
    const tb = b.created_at || ''
    if (ta !== tb) return ta < tb ? -1 : 1
    return (a.id || 0) - (b.id || 0)
  })
  return { lines, alone, questCount: items.length }
}
