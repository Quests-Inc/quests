// Черновик карты связей: правки копятся на сервере журналом операций и применяются разом.
// Контроллер держит журнал, состояние сохранения и вопросов «сбросить?» / «убрать с зависимыми?».
import type { GraphNodeView } from '@/entities/graph'
import {
  addGraphDraftOp,
  cancelGraphDraft,
  commitGraphDraft,
  deleteGraphDraftOp,
  getGraphDraft,
  putGraphDraftRoots,
} from '@/shared/api/graph.ts'
import type { DraftOp } from '@/shared/api/store.gen'
import { toast } from '@/shared/lib/toasts.svelte.ts'

export interface DraftState {
  ops: DraftOp[]
  roots: { type: string; id: number }[]
}

export function createGraphDraft(deps: {
  getCenter: () => { type: string | null; id: number | null }
  /** Центр карты сменился (сохранение создало первую сущность на пустой карте). */
  setCenter: (c: { type: string; id: number }) => void
  getNodes: () => GraphNodeView[]
  /** Позиции, выставленные руками: сохранение переносит их на новые id. */
  getSaved: () => Map<string, { x: number; y: number }>
  resetPositions: () => void
}) {
  let draft = $state<DraftState>({ ops: [], roots: [] })
  let open = $state(true)
  let committing = $state(false)
  /** op id → message, from a refused or failed save */
  let problems = $state<Record<number, string>>({})
  let cancelAsk = $state(false)
  let dropAsk = $state<{ id: number; ids: number[] } | null>(null)
  let reloadTick = $state(0)

  async function load() {
    try {
      const d = await getGraphDraft()
      const next = { ops: d.ops ?? [], roots: Array.isArray(d.roots) ? d.roots : [] }
      // Unchanged: keep the object, so nothing is rebuilt.
      if (JSON.stringify(next) !== JSON.stringify(draft)) draft = next
    } catch {
      /* the page still works read-only */
    }
  }
  /** Adds an operation; resolves to the op, or null when the server refused it. */
  async function addOp(kind: string, payload: Record<string, unknown>) {
    try {
      const op = await addGraphDraftOp(kind, payload)
      await load()
      return op
    } catch {
      return null
    }
  }
  async function save() {
    if (committing || !draft.ops.length) return
    committing = true
    problems = {}
    try {
      const r = await commitGraphDraft()
      toast(`Сохранено: ${r.applied} ${r.applied === 1 ? 'изменение' : 'изменений'}`, { kind: 'success' })
      // An empty map has nothing to show the saved things on: centre on the first one.
      const made = !deps.getCenter().id
        ? Object.entries(r.created ?? {}).find(
            ([k]) => k.startsWith('quest:') || k.startsWith('note:') || k.startsWith('questline:')
          )
        : null
      // Saving must not rearrange the map: every card stays where it is, and
      // the things that were only in the draft keep their spot under their new ids.
      for (const n of deps.getNodes()) {
        if (!Number.isFinite(n.x) || !Number.isFinite(n.y)) continue
        const real = r.created?.[n.key]
        const key = real != null ? `${n.key.split(':')[0]}:${real}` : n.key
        if (n.type !== 'cluster') deps.getSaved().set(key, { x: n.x, y: n.y })
      }
      if (made) {
        deps.setCenter({ type: made[0].split(':')[0], id: Number(made[1]) })
      }
      // Things made on the map with no link to anything on it have nothing to
      // hang from once saved: keep them as extra centres so they stay put.
      const kept = [...draft.roots]
      for (const n of deps.getNodes()) {
        if (!n.draft || n.depth >= 0 || !['quest', 'note', 'questline'].includes(n.type)) continue
        const id = Number(r.created?.[n.key])
        if (!id || (deps.getCenter().type === n.type && deps.getCenter().id === id)) continue
        if (!kept.some((x) => x.type === n.type && x.id === id)) kept.push({ type: n.type, id })
      }
      if (kept.length !== draft.roots.length) await putGraphDraftRoots(kept)
      await load()
      reloadTick += 1
    } catch (e) {
      const found: Record<number, string> = {}
      // @ts-ignore
      for (const p of e.problems ?? []) found[p.op] = p.message
      // @ts-ignore
      if (e.failedOp != null) found[e.failedOp] = e.message
      problems = found
      open = true
      toast(e.message || 'Не удалось сохранить', { kind: 'error' })
      await load()
      reloadTick += 1
    } finally {
      committing = false
    }
  }
  async function discard() {
    cancelAsk = false
    try {
      await cancelGraphDraft()
      problems = {}
      deps.resetPositions()
      await load()
    } catch {
      /* toast already shown */
    }
  }
  /** Ask which ops would go with this one, then drop them. */
  async function askDrop(op: { id: number }) {
    try {
      const r = await deleteGraphDraftOp(op.id, true)
      const ids = r?.would_remove ?? [op.id]
      if (ids.length > 1) dropAsk = { id: op.id, ids }
      else await doDrop(op.id)
    } catch {
      /* toast already shown */
    }
  }
  async function doDrop(id: number) {
    dropAsk = null
    try {
      await deleteGraphDraftOp(id)
      await load()
    } catch {
      /* toast already shown */
    }
  }

  return {
    get draft() {
      return draft
    },
    get open() {
      return open
    },
    set open(v: boolean) {
      open = v
    },
    get committing() {
      return committing
    },
    get problems() {
      return problems
    },
    get cancelAsk() {
      return cancelAsk
    },
    set cancelAsk(v: boolean) {
      cancelAsk = v
    },
    get dropAsk() {
      return dropAsk
    },
    set dropAsk(v: { id: number; ids: number[] } | null) {
      dropAsk = v
    },
    get reloadTick() {
      return reloadTick
    },
    load,
    addOp,
    save,
    discard,
    askDrop,
    doDrop,
  }
}
