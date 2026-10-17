// Выделение нескольких квестов в списке журнала: Ctrl/Cmd — добавить или убрать, Shift — диапазон.
// Порядок строк приходит из списка в момент клика (что реально видно: свёрнутые группы не считаются).

/** Все квесты между `from` и `to` включительно по порядку `order`; если одного из них нет в списке — только `to`. */
export function pickRange(order: number[], from: number, to: number): number[] {
  const a = order.indexOf(from)
  const b = order.indexOf(to)
  if (a === -1 || b === -1) return [to]
  return order.slice(Math.min(a, b), Math.max(a, b) + 1)
}

type Mods = { ctrlKey?: boolean; metaKey?: boolean; shiftKey?: boolean }

export function createMultiSelect(deps: { current: () => number | null }) {
  let picked = $state<Set<number>>(new Set())
  let anchor: number | null = null

  function set(next: Iterable<number>) {
    picked = new Set(next)
  }

  return {
    get picked() {
      return picked
    },
    get size() {
      return picked.size
    },
    get list() {
      return [...picked]
    },
    has: (id: number) => picked.has(id),
    clear() {
      if (picked.size) picked = new Set()
      anchor = null
    },
    /** Только существующие квесты (после перезагрузки списка или удаления). */
    prune(exists: (id: number) => boolean) {
      const keep = [...picked].filter(exists)
      if (keep.length !== picked.size) set(keep)
    },
    /**
     * Клик по строке. `true` — клик ушёл в выделение (квест не открывать), `false` — обычный клик:
     * выделение сброшено, строку открывает вызывающий.
     */
    click(e: Mods, id: number, order: number[]): boolean {
      const additive = !!(e.ctrlKey || e.metaKey)
      if (e.shiftKey) {
        const from = anchor ?? deps.current() ?? id
        const range = pickRange(order, from, id)
        if (anchor == null) anchor = from
        set(additive ? [...picked, ...range] : range)
        return true
      }
      if (additive) {
        const next = new Set(picked)
        const open = deps.current()
        if (!next.size && open != null && open !== id) next.add(open)
        if (next.has(id)) next.delete(id)
        else next.add(id)
        picked = next
        anchor = id
        return true
      }
      this.clear()
      anchor = id
      return false
    },
    /** На кого действует меню, открытое на квесте `id`: на всю группу, если он в ней, иначе на него одного. */
    targets(id: number): number[] {
      return picked.size > 1 && picked.has(id) ? [...picked] : [id]
    },
  }
}
