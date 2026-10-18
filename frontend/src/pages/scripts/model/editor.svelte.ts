// Состояние редактора скрипта: граф, порты узлов (их считает сервер по типу и параметрам), выбор, проверка, история правок, сохранение.
// Сам граф правится чистыми функциями из entities/script; здесь — то, что живёт между правками.
import {
  addNode,
  appendGraph,
  bakeSelection,
  type ChosenField,
  commitPositions,
  inlineCustom,
  customIdOf,
  connect,
  defaultParams,
  disconnect,
  KIND_COLORS,
  layout,
  moveNode,
  nextNodeId,
  nodeKey,
  type NodePorts,
  portsKey,
  pruneEdges,
  removeNode,
  replaceSelection,
  setEdge,
  setParams,
  type ScriptNodeView,
} from '@/entities/script'
import {
  createCustomNode,
  getCustomNodes,
  updateCustomNode,
  getNodePorts,
  getScriptNodes,
  updateScript,
  validateScript,
  type ScriptFull,
} from '@/shared/api/client.ts'
import type { Edge, End, Graph, NodeInfo, Problem } from '@/shared/api/scriptgraph.gen.ts'

const HISTORY = 100
const CHECK_DELAY = 350

const EMPTY: Graph = { nodes: [], edges: [] }

export function createScriptEditor() {
  let scriptId = $state<number | null>(null)
  let title = $state('')
  let description = $state('')
  let purpose = $state('')
  let version = $state(1)
  let subjects = $state<string[]>([])
  let graph = $state.raw<Graph>(EMPTY)
  let savedKey = $state('')

  let infos = $state<NodeInfo[]>([])
  // The ports each distinct (type, parameters) resolves to; `portsTick` tells readers the cache grew.
  const ports = new Map<string, NodePorts>()
  let portsTick = $state(0)

  let problems = $state<Problem[]>([])
  let checking = $state(false)
  let saving = $state(false)
  let error = $state('')

  let selectedNode = $state<number | null>(null)
  let selectedEdge = $state<number | null>(null)
  let picked = $state<Set<string>>(new Set())

  let undo: Graph[] = []
  let redo: Graph[] = []
  let historyTick = $state(0)
  let timer: ReturnType<typeof setTimeout> | null = null
  let openSeq = 0

  const stamp = (g: Graph, t = title, d = description, p = purpose) => JSON.stringify([t, d, p, g])

  /** The ports of a node; empty until the server has answered for its type and parameters. */
  function portsOf(id: number): NodePorts | undefined {
    void portsTick
    const n = graph.nodes.find((x) => x.id === id)
    return n ? ports.get(portsKey(n)) : undefined
  }

  async function ensurePorts(g: Graph): Promise<void> {
    const missing = g.nodes.filter((n) => !ports.has(portsKey(n)))
    if (!missing.length) return
    await Promise.all(
      [...new Map(missing.map((n) => [portsKey(n), n])).values()].map(async (n) => {
        try {
          const p = await getNodePorts(n.type, n.params)
          ports.set(portsKey(n), { inputs: p.inputs ?? [], outputs: p.outputs ?? [] })
        } catch (e) {
          // A wrong parameter has no ports: the node is drawn empty and the validator names the reason.
          ports.set(portsKey(n), { inputs: [], outputs: [], error: e instanceof Error ? e.message : String(e) })
        }
      })
    )
    portsTick++
  }

  function scheduleCheck(): void {
    if (timer) clearTimeout(timer)
    timer = setTimeout((): void => {
      void checkNow()
    }, CHECK_DELAY)
  }

  async function checkNow(): Promise<void> {
    if (timer) clearTimeout(timer)
    timer = null
    if (!graph.nodes.length) {
      problems = [] // an empty script is a fresh start, not a mistake
      return
    }
    const snapshot = graph
    checking = true
    try {
      const res = await validateScript(snapshot)
      if (snapshot === graph) problems = res.problems ?? []
    } catch (e) {
      problems = [{ message: e instanceof Error ? e.message : String(e) }]
    } finally {
      checking = false
    }
  }

  /** Take a new graph: remember the old one for undo, resolve the ports of what changed, drop links they no longer have. */
  async function apply(next: Graph, record = true): Promise<void> {
    if (next === graph) return
    if (record) {
      undo = [...undo.slice(-(HISTORY - 1)), graph]
      redo = []
      historyTick++
    }
    graph = next
    await ensurePorts(next)
    const pruned = pruneEdges(graph, portsOf)
    if (pruned !== graph) graph = pruned
    scheduleCheck()
  }

  async function ensureInfos() {
    if (infos.length) return
    try {
      infos = await getScriptNodes()
    } catch {
      /* the palette stays empty; the editor still opens */
    }
  }

  /** Load a script into the editor. */
  async function open(s: ScriptFull) {
    const mine = ++openSeq
    scriptId = s.id
    title = s.title
    description = s.description
    purpose = s.purpose
    version = s.version
    subjects = s.subjects ?? []
    undo = []
    redo = []
    historyTick++
    selectedNode = selectedEdge = null
    picked = new Set()
    problems = []
    error = ''
    await Promise.all([ensureInfos(), ensurePorts(s.graph)])
    if (mine !== openSeq) return
    graph = s.graph // portsOf looks nodes up in the current graph, so it must hold this one before it is laid out
    graph = layout(s.graph, portsOf)
    savedKey = stamp(graph, s.title, s.description, s.purpose)
    void checkNow()
  }

  /** Leave the script (it was deleted, or none is chosen). */
  function reset() {
    openSeq++
    scriptId = null
    title = description = purpose = ''
    graph = EMPTY
    problems = []
    undo = redo = []
    historyTick++
    selectedNode = selectedEdge = null
    picked = new Set()
    error = ''
  }

  function meta(type: string) {
    const i = infos.find((x) => x.name === type)
    return {
      title: i?.title ?? type,
      kind: i?.kind ?? 'code',
      color: KIND_COLORS[i?.kind ?? ''] ?? '#888',
      trusted: i?.trusted ?? false,
    }
  }

  const select = (id: number | null) => {
    selectedNode = id
    selectedEdge = null
  }
  const selectEdge = (index: number | null) => {
    selectedEdge = index
    selectedNode = null
  }

  async function add(type: string, x: number, y: number) {
    const { graph: next, id } = addNode(graph, type, defaultParams(type), x, y)
    await apply(next)
    select(id)
    return id
  }

  /** Load the catalogue again (a custom node was made, changed or deleted) and forget the ports it had. */
  async function refreshInfos() {
    try {
      infos = await getScriptNodes()
    } catch {
      /* the old catalogue stays */
    }
    ports.clear()
    portsTick++
    await ensurePorts(graph)
    scheduleCheck()
  }

  /** Put a custom node's parts in its place (its fields written into them) to change them; bake again to keep the change. */
  async function unfold(nodeId: number) {
    const n = graph.nodes.find((x) => x.id === nodeId)
    const id = n ? customIdOf(n.type) : null
    if (id == null) return
    const def = (await getCustomNodes()).find((x) => x.id === id)
    if (!def) throw new Error('нода не найдена')
    await apply(inlineCustom(graph, nodeId, def))
    select(null)
  }

  /** The numbers of the picked nodes. */
  const pickedIds = () => [...picked].map((k) => Number(k.slice(1)))

  /** Make a custom node of the picked nodes and put it where they were. Throws what the server refuses with. */
  async function bake(title: string, chosen: ChosenField[], replaceId?: number, whole = false) {
    const ids = whole ? graph.nodes.map((n) => n.id) : pickedIds()
    const { def, map } = bakeSelection(graph, ids, portsOf, title, chosen)
    let made
    if (replaceId) {
      // The node that is replaced keeps its number, so the scripts that use it follow; its ports are the new ones.
      const cur = (await getCustomNodes()).find((n) => n.id === replaceId)
      if (!cur) throw new Error('нода не найдена')
      made = await updateCustomNode(replaceId, { ...def, version: cur.version })
    } else {
      made = await createCustomNode(def)
    }
    await refreshInfos()
    if (whole) return made // the script itself stays as it is; the node is a copy of it
    const id = nextNodeId(graph)
    await apply(replaceSelection(graph, ids, map, made.id, id))
    picked = new Set()
    select(id)
    return made
  }

  /** Remove these nodes (a right click on a card), whatever is picked. */
  async function removeNodes(ids: number[]) {
    let next = graph
    for (const id of ids) next = removeNode(next, id)
    selectedNode = null
    picked = new Set()
    await apply(next)
  }

  /** Put another graph (a sample, another script) in with its middle on (x, y) and pick what came. */
  async function insert(extra: Graph, x: number, y: number) {
    const { graph: next, ids } = appendGraph(graph, extra, x, y)
    if (!ids.length) return
    await apply(next)
    selectedNode = null
    picked = new Set(ids.map(nodeKey))
  }

  async function remove() {
    if (selectedEdge != null) {
      const i = selectedEdge
      selectedEdge = null
      await apply(disconnect(graph, i))
      return
    }
    const ids = new Set<number>(
      picked.size ? [...picked].map((k) => Number(k.slice(1))) : selectedNode != null ? [selectedNode] : []
    )
    if (!ids.size) return
    let next = graph
    for (const id of ids) next = removeNode(next, id)
    selectedNode = null
    picked = new Set()
    await apply(next)
  }

  async function link(from: End, to: End) {
    const res = connect(graph, from, to, portsOf)
    if (res.error) {
      error = res.error
      return false
    }
    error = ''
    await apply(res.graph)
    return true
  }

  const params = (id: number, p: unknown) => apply(setParams(graph, id, p))
  const edge = (index: number, patch: Partial<Pick<Edge, 'alias' | 'optional'>>) => apply(setEdge(graph, index, patch))

  // A drag moves several nodes at once and reports each; they are one step of history.
  let moves: Pick<ScriptNodeView, 'id' | 'x' | 'y' | 'w' | 'h'>[] = []
  function moved(view: Pick<ScriptNodeView, 'id' | 'x' | 'y' | 'w' | 'h'>) {
    if (!moves.length) {
      queueMicrotask(() => {
        const batch = moves
        moves = []
        void apply(commitPositions(graph, batch))
      })
    }
    moves.push({ ...view })
  }

  function step(from: Graph[], to: Graph[]) {
    const g = from.pop()
    if (!g) return
    to.push(graph)
    graph = g
    historyTick++
    void ensurePorts(g).then(scheduleCheck)
  }
  const undoStep = () => step(undo, redo)
  const redoStep = () => step(redo, undo)

  async function save() {
    if (scriptId == null || saving) return false
    saving = true
    error = ''
    try {
      const s = await updateScript(scriptId, { version, title, description, purpose, graph })
      version = s.version
      savedKey = stamp(graph)
      return true
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      return false
    } finally {
      saving = false
    }
  }

  return {
    get scriptId() {
      return scriptId
    },
    get title() {
      return title
    },
    set title(v: string) {
      title = v
    },
    get description() {
      return description
    },
    set description(v: string) {
      description = v
    },
    get purpose() {
      return purpose
    },
    set purpose(v: string) {
      purpose = v
    },
    get version() {
      return version
    },
    get subjects() {
      return subjects
    },
    get graph() {
      return graph
    },
    get infos() {
      return infos
    },
    get problems() {
      return problems
    },
    get checking() {
      return checking
    },
    get saving() {
      return saving
    },
    get error() {
      return error
    },
    set error(v: string) {
      error = v
    },
    get dirty() {
      return scriptId != null && stamp(graph) !== savedKey
    },
    get selectedNode() {
      return selectedNode
    },
    get selectedEdge() {
      return selectedEdge
    },
    get picked() {
      return picked
    },
    set picked(v: Set<string>) {
      picked = v
    },
    get canUndo() {
      void historyTick
      return undo.length > 0
    },
    get canRedo() {
      void historyTick
      return redo.length > 0
    },
    /** Problems by node, for marking the boxes. */
    get problemNodes() {
      const m = new Map<number, string[]>()
      for (const p of problems) if (p.node) m.set(p.node, [...(m.get(p.node) ?? []), p.message])
      return m
    },
    portsOf,
    meta,
    open,
    reset,
    ensureInfos,
    refreshInfos,
    bake,
    unfold,
    insert,
    removeNodes,
    pickedIds,
    select,
    selectEdge,
    add,
    remove,
    link,
    params,
    edge,
    moved,
    moveNode: (id: number, x: number, y: number) => apply(moveNode(graph, id, x, y)),
    undo: undoStep,
    redo: redoStep,
    save,
    checkNow,
  }
}

export type ScriptEditor = ReturnType<typeof createScriptEditor>
