// Правый клик на карте: меню поля, карточки, шара вложений и ребра — и что по нему делается.
import { edgeMenuItems, graphMenuItems, type GraphNodeView } from '@/entities/graph'
import { uploadGraphDraftFile } from '@/shared/api/graph.ts'

type At = { x: number; y: number }
type Root = { type: string; id: number }
type Cluster = { owner: string; key: string | null; folded: boolean; node: GraphNodeView | null }
type MenuEdge = { from: string; to: string; kind: string; draft?: string }
type Ctx = {
  open: boolean
  x: number
  y: number
  node: GraphNodeView | null
  at: At
  group: boolean
  cluster: Cluster | null
  edge: MenuEdge | null
}

const closed = (): Ctx => ({
  open: false,
  x: 0,
  y: 0,
  node: null,
  at: { x: 0, y: 0 },
  group: false,
  cluster: null,
  edge: null,
})

export function createGraphMenu(deps: {
  worldAt: (e: { clientX: number; clientY: number }) => At
  isLinking: () => boolean
  cancelLink: () => void
  selectMode: () => boolean
  pickedHas: (key: string) => boolean
  pickedSize: () => number
  setSelectedKey: (key: string) => void
  /** Клик по свёрнутому шару: раскрыть, а не просто выделить. */
  select: (n: GraphNodeView) => void
  collapseKey: (g: { type: string; owner: string }) => string | null
  getDraftRoots: () => Root[]
  getRootKey: () => string | undefined
  getNodes: () => { key: string; type: string; draft?: boolean }[]
  getExpandKeys: () => string[]
  openTarget: (n: GraphNodeView) => unknown
  ownExpand: (key: string) => unknown[]
  outgoingRefs: (n: { key: string }) => string[]
  childKeys: (n: { key: string }, type: string) => string[]
  parentNote: (n: { key: string }) => string | null
  titleOf: (key: string) => string
  hidePicked: () => Promise<void>
  toggleSelectMode: () => void
  copyRef: (n: { type: string; id: number | string }) => void
  highlightEdge: (edge: { from: string; to: string; kind: string }) => void
  getLit?: () => { from: string; to: string; kind: string } | null
  fit: () => void
  clearPicked: () => void
  hideKeys: (keys: string[]) => Promise<void>
  createNoteHere: (at: At, linkFrom: string) => Promise<void>
  openEntity: (kind: 'note' | 'step' | 'questline', at: At, extra?: { owner?: string }) => void
  startEdit: (n: GraphNodeView, at: At) => Promise<void>
  addOp: (kind: string, payload: Record<string, unknown>) => Promise<unknown>
  askDrop: (op: { id: number }) => Promise<void>
  startLink: (mode: 'to' | 'from' | 'attach', n: { key: string }) => void
  collapse: (key: string) => void
  disband: (owner: string) => void
  unlinkRef: (n: { key: string }, target: string) => Promise<void>
  setExpand: (keys: string[]) => void
  loadDraft: () => Promise<void>
  putRoots: (roots: Root[]) => Promise<unknown>
  onOpen?: (kind: string, id: number) => void
  recenter: (n: GraphNodeView) => void
  saved: Map<string, At>
  upload?: (file: File, owner: string) => Promise<{ id: number }>
}) {
  let ctx = $state<Ctx>(closed())
  let questModal = $state({ open: false, desc: '', at: { x: 0, y: 0 }, questlineId: null as number | null })
  let findOpen = $state(false)
  let fileInput = $state<HTMLInputElement | null>(null)
  let fileOwner = ''
  let fileAt: At = { x: 0, y: 0 }
  const upload = deps.upload ?? uploadGraphDraftFile

  function close() {
    ctx = { ...ctx, open: false }
  }

  function onCtx(e: MouseEvent, n: GraphNodeView | null = null) {
    e.preventDefault()
    e.stopPropagation()
    if (deps.isLinking()) {
      deps.cancelLink()
      return
    }
    if (deps.selectMode() && deps.pickedSize() && (!n || deps.pickedHas(n.key))) {
      ctx = {
        open: true,
        x: e.clientX,
        y: e.clientY,
        node: null,
        at: deps.worldAt(e),
        group: true,
        cluster: null,
        edge: null,
      }
      return
    }
    if (n?.type === 'cluster') {
      const owner = n.key.slice('cluster:'.length)
      ctx = {
        open: true,
        x: e.clientX,
        y: e.clientY,
        node: null,
        at: deps.worldAt(e),
        group: false,
        cluster: { owner, key: n.key, folded: true, node: n },
        edge: null,
      }
      return
    }
    const real = n && n.type !== 'cluster' ? n : null
    if (real) deps.setSelectedKey(real.key)
    ctx = {
      open: true,
      x: e.clientX,
      y: e.clientY,
      node: real,
      at: deps.worldAt(e),
      group: false,
      cluster: null,
      edge: null,
    }
  }

  function onGroupCtx(e: MouseEvent, g: { type: string; owner: string }) {
    e.preventDefault()
    e.stopPropagation()
    if (deps.isLinking() || g.type !== 'attachment') return
    ctx = {
      open: true,
      x: e.clientX,
      y: e.clientY,
      node: null,
      at: deps.worldAt(e),
      group: false,
      cluster: { owner: g.owner, key: deps.collapseKey(g), folded: false, node: null },
      edge: null,
    }
  }

  function onEdgeCtx(e: MouseEvent, edge: MenuEdge) {
    e.preventDefault()
    e.stopPropagation()
    if (deps.isLinking()) {
      deps.cancelLink()
      return
    }
    ctx = { open: true, x: e.clientX, y: e.clientY, node: null, at: deps.worldAt(e), group: false, cluster: null, edge }
  }

  function items() {
    return graphMenuItems(ctx, {
      selectMode: deps.selectMode(),
      pickedCount: deps.pickedSize(),
      lit: deps.getLit?.() ?? null,
      rootKey: deps.getRootKey(),
      nodes: deps.getNodes(),
      expandKeys: deps.getExpandKeys(),
      openTarget: deps.openTarget,
      isExtraRoot: (n) => !n.draft && deps.getDraftRoots().some((r) => `${r.type}:${r.id}` === n.key),
      ownExpand: deps.ownExpand,
      outgoingRefs: deps.outgoingRefs,
      childKeys: deps.childKeys,
      parentNote: deps.parentNote,
      titleOf: deps.titleOf,
    })
  }

  async function onCtxSelect(id: string) {
    const n = ctx.node
    const at = ctx.at
    if (id === 'copy-ref' && n) deps.copyRef(n)
    else if (id === 'edge-light' && ctx.edge) deps.highlightEdge(ctx.edge)
    else if (id === 'hide-picked') await deps.hidePicked()
    else if (id === 'clear-picked') deps.clearPicked()
    else if (id === 'fit') deps.fit()
    else if (id === 'select-mode') {
      close()
      deps.toggleSelectMode()
    } else if (id === 'new-quest') questModal = { open: true, desc: '', at, questlineId: null }
    else if (id === 'new-quest-ref' && n) questModal = { open: true, desc: `${n.type}=${n.id}`, at, questlineId: null }
    else if (id === 'add-quest-line' && n && typeof n.id === 'number')
      questModal = { open: true, desc: '', at, questlineId: n.id }
    else if (id === 'new-note') await deps.createNoteHere(at, '')
    else if (id === 'new-questline') deps.openEntity('questline', at)
    else if (id === 'new-step') deps.openEntity('step', at)
    else if (id === 'add-step' && n) deps.openEntity('step', at, { owner: n.key })
    else if (id === 'add-note' && n) await deps.createNoteHere(at, n.key)
    else if (id === 'edit' && n) await deps.startEdit(n, at)
    else if (id === 'copy' && n) await deps.addOp('copy', { source: n.key, pos: at })
    else if (id === 'upload' || id === 'add-file') {
      fileAt = at
      fileOwner = id === 'add-file' && n ? n.key : ''
      fileInput?.click()
    } else if (id === 'find') findOpen = true
    else if (id === 'link-to' && n) {
      close()
      deps.startLink('to', n)
    } else if (id === 'link-from' && n) {
      close()
      deps.startLink('from', n)
    } else if (id === 'attach' && n) {
      close()
      deps.startLink('attach', n)
    } else if (id === 'cl-open' && ctx.cluster?.node) deps.select(ctx.cluster.node)
    else if (id === 'cl-fold' && ctx.cluster?.key) deps.collapse(ctx.cluster.key)
    else if (id === 'cl-disband' && ctx.cluster) deps.disband(ctx.cluster.owner)
    else if (id === 'edge-unref' && ctx.edge) await deps.unlinkRef({ key: ctx.edge.from }, ctx.edge.to)
    else if (id === 'edge-nowait' && ctx.edge)
      await deps.addOp('edit_step', { id: ctx.edge.to, fields: { wait_previous: false } })
    else if (id === 'edge-unline' && ctx.edge)
      await deps.addOp('edit_quest', { id: ctx.edge.to, fields: { questline_id: null } })
    else if (id === 'edge-unparent' && ctx.edge)
      await deps.addOp('set_parent', { id: ctx.edge.to, parent: '', was: ctx.edge.from })
    else if (id === 'expand' && n)
      deps.setExpand([...deps.getExpandKeys().filter((k) => !k.startsWith(`${n.key}>`)), n.key])
    else if (id === 'collapse' && n)
      deps.setExpand(deps.getExpandKeys().filter((k) => k !== n.key && !k.startsWith(`${n.key}>`)))
    else if (id === 'drop-steps' && n) await deps.hideKeys(deps.childKeys(n, 'step'))
    else if (id === 'drop-quests' && n) await deps.hideKeys(deps.childKeys(n, 'quest'))
    else if (id === 'detach-note' && n)
      await deps.addOp('set_parent', { id: n.key, parent: '', was: deps.parentNote(n) ?? '' })
    else if (id.startsWith('unlink:') && n) await deps.unlinkRef(n, id.slice('unlink:'.length))
    else if (id === 'drop' && n?.op) await deps.askDrop({ id: n.op })
    else if (id === 'hide' && n) await deps.hideKeys([n.key])
    else if (id === 'unroot' && n) {
      await deps.putRoots(deps.getDraftRoots().filter((r) => `${r.type}:${r.id}` !== n.key))
      await deps.loadDraft()
    } else if (id === 'open' && n) {
      const target = deps.openTarget(n)
      if (Array.isArray(target) && typeof target[1] === 'number') deps.onOpen?.(String(target[0]), target[1])
    } else if (id === 'center' && n) deps.recenter(n)
  }

  async function onFilesPicked(e: Event) {
    const input = e.currentTarget
    if (!(input instanceof HTMLInputElement)) return
    const files = [...(input.files ?? [])]
    input.value = ''
    for (const [i, file] of files.entries()) {
      try {
        const op = await upload(file, fileOwner)
        deps.saved.set(`attachment:t${op.id}`, { x: fileAt.x + i * 110, y: fileAt.y })
      } catch {
        /* toast already shown */
      }
    }
    await deps.loadDraft()
  }

  return {
    get ctx() {
      return ctx
    },
    get questModal() {
      return questModal
    },
    get findOpen() {
      return findOpen
    },
    get fileInput() {
      return fileInput
    },
    set fileInput(v: HTMLInputElement | null) {
      fileInput = v
    },
    close,
    closeQuest() {
      questModal = { ...questModal, open: false }
    },
    closeFind() {
      findOpen = false
    },
    onCtx,
    onGroupCtx,
    onEdgeCtx,
    items,
    onCtxSelect,
    onFilesPicked,
    edgeActions: (e: MenuEdge) => edgeMenuItems(e, deps.getLit?.() ?? null),
  }
}
