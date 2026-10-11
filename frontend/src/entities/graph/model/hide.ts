// Что делать со снятыми с карты карточками: скрыть (чтобы можно было вернуть), убрать из центров
// или очистить карту целиком. Чистая функция — решение принимает страница.
type N = { key: string; draft?: unknown }
type E = { from: string; to: string }
type Root = { type: string; id: number }

export interface HidePlan {
  /** Убраны все настоящие карточки вместе с центром: начать с пустой карты. */
  clearAll: boolean
  hidden: Set<string>
  roots: Root[]
  rootsChanged: boolean
  /** Карточки черновика не скрываются: их убирают из журнала. */
  skippedDraft: number
}

/**
 * Скрытыми запоминается только то, что остаётся связанным с оставшейся карточкой; карточка без связей
 * с остающимися просто исчезает. Центры карты (`roots`) снимаются из списка, а не скрываются.
 */
export function planHide(input: {
  keys: string[]
  nodes: N[]
  edges: E[]
  rootKey: string | null | undefined
  hidden: Set<string>
  roots: Root[]
}): HidePlan {
  const gone = new Set(input.keys)
  const left = input.nodes.filter((n) => !gone.has(n.key) && !n.draft)
  const plan: HidePlan = {
    clearAll: false,
    hidden: new Set(input.hidden),
    roots: [...input.roots],
    rootsChanged: false,
    skippedDraft: 0,
  }
  if (input.rootKey && gone.has(input.rootKey) && !left.length) {
    plan.clearAll = true
    return plan
  }
  const stays = new Set(input.nodes.filter((n) => !gone.has(n.key)).map((n) => n.key))
  const linked = new Set<string>()
  for (const e of input.edges) {
    if (gone.has(e.from) && stays.has(e.to)) linked.add(e.from)
    if (gone.has(e.to) && stays.has(e.from)) linked.add(e.to)
  }
  const byKey = new Map(input.nodes.map((n) => [n.key, n]))
  for (const k of gone) {
    const n = byKey.get(k)
    if (!n || k === input.rootKey) continue
    if (n.draft) {
      plan.skippedDraft += 1
      continue
    }
    const at = plan.roots.findIndex((r) => `${r.type}:${r.id}` === k)
    if (at >= 0) {
      plan.roots.splice(at, 1)
      plan.rootsChanged = true
    } else if (linked.has(k)) plan.hidden.add(k)
  }
  return plan
}
