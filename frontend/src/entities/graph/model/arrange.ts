// Расстановка карточек выбранной раскладкой: связанные — алгоритмом, несвязанные новые — где созданы или колонкой слева,
// руками поставленные — на свои прежние места. Меняет координаты узлов на месте и возвращает группы.
import { layoutGraph, layoutLayers, layoutRadial, layoutTree, layoutTreeDown } from './graphLayout.ts'
import { restoreHandPlaced } from './placement.ts'
import type { GraphGroupView, GraphNodeView } from './types.ts'

type Spot = { x: number; y: number }
type Node = GraphNodeView & { pos?: Spot }
type Edge = { from: string; to: string; kind: string }

export function arrangeView(
  view: { nodes: Node[]; edges: Edge[] },
  mode: string,
  opts: { loose: Set<string>; saved: Map<string, Spot> }
): GraphGroupView[] {
  const attached = view.nodes.filter((n) => n.depth >= 0)
  const floating = view.nodes.filter((n) => n.depth < 0)
  const keys = new Set(attached.map((n) => n.key))
  const edges = view.edges.filter((e) => keys.has(e.from) && keys.has(e.to))
  let groups: GraphGroupView[] = []
  if (mode !== 'tree') for (const n of view.nodes) delete n.group
  if (mode === 'tree') groups = layoutTree(attached, edges, { loose: opts.loose })
  else if (mode === 'down') groups = layoutTreeDown(attached, edges)
  else if (mode === 'radial') groups = layoutRadial(attached, edges)
  else if (mode === 'layers') groups = layoutLayers(attached)
  else layoutGraph(attached, edges, { sizeOf: (n: { w: number; h: number }) => ({ w: n.w, h: n.h }) })
  // Несвязанные новые: где созданы, иначе колонка левее карты.
  const left = attached.length ? Math.min(...attached.map((n) => n.x - n.w / 2)) : 0
  let y = attached.length ? Math.min(...attached.map((n) => n.y - n.h / 2)) : 0
  for (const n of floating) {
    if (n.pos) {
      n.x = n.pos.x
      n.y = n.pos.y
    } else {
      n.x = left - 260
      n.y = y + n.h / 2
      y += n.h + 18
    }
  }
  for (const n of attached) {
    if (n.draft && n.pos && !opts.saved.has(n.key)) {
      n.x = n.pos.x
      n.y = n.pos.y
    }
  }
  restoreHandPlaced(view.nodes, opts.saved, groups)
  return groups
}
