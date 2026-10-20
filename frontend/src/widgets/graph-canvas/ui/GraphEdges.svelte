<script lang="ts">
  import { arrowHead, EDGES, type GraphEdgeView, type GraphNodeView } from '@/entities/graph'

  /** Слой рёбер карты: линии со стрелками, зоны клика для меню и «резинка» режима связывания. */
  let {
    drawEdges,
    lit = null,
    linking,
    nodeMap,
    linkCursor,
    edgeActions,
    onEdgeCtx,
  }: {
    drawEdges: GraphEdgeView[]
    lit?: { from: string; to: string; kind: string } | null
    linking: { anchor: string; mode: string } | null
    nodeMap: Map<string, GraphNodeView>
    linkCursor: { x: number; y: number }
    edgeActions(e: GraphEdgeView): unknown[]
    onEdgeCtx: (ev: MouseEvent, e: GraphEdgeView) => void
  } = $props()

  type Batch = { key: string; color: string; head: string; width: number; opacity: number; lines: string[]; heads: string[]; dots: string[] }

  /**
   * Рёбра одного вида и состояния рисуются одним <path> (линии), одним (стрелки) и одним (точки выхода): на плотном графе это
   * десяток элементов вместо сотен, а стрелки — обычные треугольники без <marker>, которые браузер красит медленно.
   * Все линии сплошные: пунктир браузер красит в разы дороже (на сотне длинных рёбер ~30×), виды различаются цветом.
   */
  const litEdges = $derived(lit ? drawEdges.filter((e) => e.from === lit.from && e.to === lit.to && e.kind === lit.kind) : [])

  const batches = $derived.by(() => {
    const by = new Map<string, Batch>()
    for (const e of drawEdges) {
      const key = `${e.kind}|${e.draft ?? ''}`
      let b = by.get(key)
      if (!b) {
        const removed = e.draft === 'remove'
        b = {
          key,
          color: removed ? '#d23c3c' : EDGES[e.kind].color,
          head: EDGES[e.kind].color,
          width: (e.kind === 'structure' ? 1.2 : 1.8) + (e.draft === 'add' ? 1 : 0),
          opacity: removed ? 0.9 : e.kind === 'structure' ? 0.6 : 0.95,
          lines: [],
          heads: [],
          dots: [],
        }
        by.set(key, b)
      }
      b.lines.push(e.d)
      b.heads.push(arrowHead(e, b.width))
      if (e.kind !== 'structure') b.dots.push(`M${e.sx} ${e.sy}h0`)
    }
    return [...by.values()]
  })
</script>

    <svg class="gedges" aria-hidden="true">
      {#each litEdges as e (e.d)}
        <path d={e.d} fill="none" stroke="var(--color-accent, #c9a227)" stroke-width="8" stroke-linecap="round" opacity="0.9" />
      {/each}
      {#each batches as b (b.key)}
        <path d={b.lines.join('')} fill="none" stroke={b.color} stroke-width={b.width} opacity={b.opacity} />
        <path d={b.heads.join('')} fill={b.head} opacity={b.opacity} />
        {#if b.dots.length}
          <path d={b.dots.join('')} fill="none" stroke={b.color} stroke-width="6.8" stroke-linecap="round" />
        {/if}
      {/each}
      {#each drawEdges as e, i (i)}
        {#if edgeActions(e).length}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <path d={e.d} fill="none" stroke="transparent" stroke-width="14" class="gedge-hit" oncontextmenu={(ev) => onEdgeCtx(ev, e)} />
        {/if}
      {/each}
      {#if linking && nodeMap.get(linking.anchor)}
        {@const a = nodeMap.get(linking.anchor)}
        <line
          x1={a.x}
          y1={a.y}
          x2={linkCursor.x}
          y2={linkCursor.y}
          stroke={linking.mode === 'attach' ? EDGES.structure.color : EDGES.reference.color}
          stroke-width="2.4"
          stroke-dasharray="6 4"
          class="glink-line"
        />
        <circle cx={linkCursor.x} cy={linkCursor.y} r="4.5" fill={EDGES.reference.color} />
      {/if}
    </svg>

<style>


  .gedge-hit {
    pointer-events: stroke;
  }

  .gedges {
    position: absolute;
    left: 0;
    top: 0;
    width: 1px;
    height: 1px;
    overflow: visible;
    pointer-events: none;
  }

  .glink-line {
    pointer-events: none;
  }</style>
