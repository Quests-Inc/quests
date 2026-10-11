// Режим связывания на карте: линия тянется от карточки за курсором, клик по цели создаёт связь
// (или «прикрепляет к»), Esc / правый клик отменяют. Сами правки уходят в журнал черновика.
import { canPickTarget, hasRefEdge, parentNoteOf } from '@/entities/graph'
import { toast } from '@/shared/lib/toasts.svelte.ts'

export type LinkMode = 'to' | 'from' | 'attach'
type Node = { key: string; type?: string; x?: number; y?: number }
type Edge = { kind: string; from: string; to: string; draft?: string; op?: number }

export function createLinkMode(deps: {
  getNodeMap: () => Map<string, any>
  getEdges: () => Edge[]
  addOp: (kind: string, payload: Record<string, unknown>) => Promise<unknown>
  askDrop: (op: { id: number }) => Promise<void>
  /** Контекстное меню закрывается, когда начинается связывание. */
  onStart?: () => void
}) {
  let linking = $state<{ mode: LinkMode; anchor: string } | null>(null)
  let cursor = $state({ x: 0, y: 0 })

  /** Можно ли выбрать карточку вторым концом в текущем режиме. */
  const canPick = (n: Node) => canPickTarget(linking, n, deps.getNodeMap(), deps.getEdges())

  function start(mode: LinkMode, n: { key: string }) {
    const a = deps.getNodeMap().get(n.key)
    linking = { mode, anchor: n.key }
    cursor = a ? { x: a.x, y: a.y } : { x: 0, y: 0 }
    deps.onStart?.()
  }
  function cancel() {
    linking = null
  }

  async function finish(target: { key: string }) {
    const l = linking
    if (!l || !canPick(target)) return
    linking = null
    const map = deps.getNodeMap()
    if (l.mode === 'attach') {
      const owned = deps.getEdges().some((e) => e.kind === 'structure' && e.from === target.key && e.to === l.anchor)
      if (owned) {
        toast('Уже прикреплено сюда', { kind: 'info' })
        return
      }
      if (map.get(l.anchor)?.type === 'note') {
        await deps.addOp('set_parent', {
          id: l.anchor,
          parent: target.key,
          was: parentNoteOf(deps.getEdges(), l.anchor) ?? '',
        })
        return
      }
      await deps.addOp('attach', { item: l.anchor, owner: target.key })
      return
    }
    const from = l.mode === 'to' ? l.anchor : target.key
    const to = l.mode === 'to' ? target.key : l.anchor
    if (hasRefEdge(deps.getEdges(), from, to)) {
      toast('Такая ссылка уже есть', { kind: 'info' })
      return
    }
    await deps.addOp('add_ref', { from, to })
  }

  async function unlinkRef(n: { key: string }, targetKey: string) {
    const e = deps
      .getEdges()
      .find((x) => x.kind === 'reference' && x.from === n.key && x.to === targetKey && x.draft !== 'remove')
    // Связь, созданная в этом черновике, снимается удалением шага журнала.
    if (e?.draft === 'add' && e.op) {
      await deps.askDrop({ id: e.op })
      return
    }
    await deps.addOp('remove_ref', { from: n.key, to: targetKey })
  }

  return {
    get linking() {
      return linking
    },
    get cursor() {
      return cursor
    },
    set cursor(p: { x: number; y: number }) {
      cursor = p
    },
    canPick,
    start,
    cancel,
    finish,
    unlinkRef,
  }
}
