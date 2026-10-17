// Поиск внутри подменю контекстного меню (например, «Переместить»): подстрока по названию, слова через пробел — все сразу.

export type SearchableItem = { id: string; label?: string; sep?: boolean }

/** Пункты, у которых в названии есть все слова запроса; разделители выпадают, результат не длиннее `limit`. */
export function filterMenuItems<T extends SearchableItem>(items: T[], query: string, limit = 40): T[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (!words.length) return items.filter((i) => !i.sep).slice(0, limit)
  const out: T[] = []
  for (const item of items) {
    if (item.sep) continue
    const label = (item.label ?? '').toLowerCase()
    if (words.every((w) => label.includes(w))) out.push(item)
    if (out.length >= limit) break
  }
  return out
}
