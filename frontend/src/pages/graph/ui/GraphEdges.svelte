<script lang="ts">
  import { EDGES, type GraphEdgeView, type GraphNodeView } from '@/entities/graph'

  /** Слой рёбер карты: линии со стрелками, зоны клика для меню и «резинка» режима связывания. */
  let {
    drawEdges,
    linking,
    nodeMap,
    linkCursor,
    edgeActions,
    onEdgeCtx,
  }: {
    drawEdges: GraphEdgeView[]
    linking: { anchor: string; mode: string } | null
    nodeMap: Map<string, GraphNodeView>
    linkCursor: { x: number; y: number }
    edgeActions(e: GraphEdgeView): unknown[]
    onEdgeCtx: (ev: MouseEvent, e: GraphEdgeView) => void
  } = $props()
</script>

    <svg class="gedges" aria-hidden="true">
      <defs>
        {#each Object.entries(EDGES) as [k, e] (k)}
          <marker id="arrow-{k}" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
            <path d="M0 0L10 5L0 10z" fill={e.color} />
          </marker>
        {/each}
      </defs>
      {#each drawEdges as e, i (i)}
        <path
          d={e.d}
          fill="none"
          stroke={e.draft === 'remove' ? '#d23c3c' : EDGES[e.kind].color}
          stroke-width={(e.kind === 'structure' ? 1.2 : 1.8) + (e.draft === 'add' ? 1 : 0)}
          stroke-dasharray={e.draft === 'remove' ? '2 5' : EDGES[e.kind].dash}
          opacity={e.draft === 'remove' ? 0.9 : e.kind === 'structure' ? 0.6 : 0.95}
          marker-end={`url(#arrow-${e.kind})`}
        />
        {#if edgeActions(e).length}
          <path d={e.d} fill="none" stroke="transparent" stroke-width="14" class="gedge-hit" oncontextmenu={(ev) => onEdgeCtx(ev, e)} />
        {/if}
        {#if e.kind !== 'structure'}
          <circle cx={e.sx} cy={e.sy} r="3.4" fill={e.draft === 'remove' ? '#d23c3c' : EDGES[e.kind].color} />
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
