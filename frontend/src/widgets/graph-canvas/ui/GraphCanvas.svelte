<script lang="ts">
  import { GraphMinimap, GraphTools, RASTER, type GraphNoteEditing } from '@/entities/graph'
  import type { Snippet } from 'svelte'
  import { createClusterFolds } from '../model/folds.svelte.ts'
  import { createGraphGestures } from '../model/gestures.svelte.ts'
  import { createLinkMode } from '../model/linking.svelte.ts'
  import { createGraphStage } from '../model/stage.svelte.ts'
  import GraphCard from './GraphCard.svelte'
  import GraphEdges from './GraphEdges.svelte'
  import GraphFoot from './GraphFoot.svelte'
  import GraphGroups from './GraphGroups.svelte'

  type Stage = ReturnType<typeof createGraphStage>
  type Gest = ReturnType<typeof createGraphGestures>
  type Folds = ReturnType<typeof createClusterFolds>
  type Link = ReturnType<typeof createLinkMode>

  let {
    stage,
    gest,
    folds,
    lk,
    editing = $bindable(null),
    data,
    center,
    error,
    loading,
    canClear,
    tier,
    edgeActions,
    onCtx,
    onGroupCtx,
    onEdgeCtx,
    onOpen,
    labels = {},
    onStartEdit,
    onFinishEdit,
    onShowChanges,
    onHidePicked,
    onToggleSelect,
    onClear,
    onRecenter,
    children,
  }: {
    stage: Stage
    gest: Gest
    folds: Folds
    lk: Link
    editing?: GraphNoteEditing | null
    data: { root?: string; truncated?: boolean } | null
    center: { id: number | null }
    error: string
    loading: boolean
    canClear: boolean
    tier: Map<string, string>
    edgeActions: (edge: { draft?: string; from: string; to: string; kind: string }) => { id: string; label?: string }[]
    onCtx: (e: MouseEvent, n?: Stage['nodes'][number] | null) => void
    onGroupCtx: (e: MouseEvent, g: { type: string; owner: string }) => void
    onEdgeCtx: (e: MouseEvent, edge: { from: string; to: string; kind: string }) => void
    onOpen?: (kind: string, id: number) => void
    labels?: Record<string, string>
    onStartEdit: (n: { key: string }) => void
    onFinishEdit: (saveIt: boolean) => void
    onShowChanges: (key: string) => void
    onHidePicked: () => void
    onToggleSelect: () => void
    onClear: () => void
    onRecenter: () => void
    children?: Snippet
  } = $props()

  const isDone = (n: { status?: string }) => n.status === 'completed' || n.status === 'done'
  const isOrphan = (n: { type: string; links: number }) => (n.type === 'note' || n.type === 'attachment') && n.links === 0
  const rasterOf = (n: { type: string; mime?: string }) =>
    n.type === 'attachment' && RASTER.has(String(n.mime || '').split(';')[0].trim())
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="graph__stage"
  class:graph__stage--full={stage.full}
  data-swipe={stage.view?.nodes?.length ? undefined : 'free'}
  bind:this={stage.el}
  onpointerdowncapture={gest.onTouchDown}
  onpointerdown={gest.onBgDown}
  onpointermove={gest.onMove}
  onpointerup={gest.onUp}
  onpointercancel={gest.onUp}
  onwheel={gest.onWheel}
  oncontextmenu={(e) => onCtx(e)}
>
  {#if lk.linking}
    <p class="glink-hint">
      {#if lk.linking.mode === 'to'}
        Выберите, на что ссылаться
      {:else if lk.linking.mode === 'from'}
        Выберите, где должна появиться ссылка на «{stage.titleOf(lk.linking.anchor)}»
      {:else}
        Выберите, к чему прикрепить «{stage.titleOf(lk.linking.anchor)}»
      {/if}
      · ПКМ или Esc — отмена
    </p>
  {/if}
  {#if !center.id && !stage.view?.nodes?.length}
    <p class="graph__msg">Выберите квест или заметку в поиске выше — или откройте карту из карточки квеста. ПКМ по полю создаёт новые.</p>
  {:else if error}
    <p class="graph__msg">{error}</p>
  {:else if loading && !stage.view}
    <p class="graph__msg">Загрузка…</p>
  {/if}

  <div class="gworld" style:transform="translate({gest.cam.x}px, {gest.cam.y}px) scale({gest.cam.k})" class:gworld--hidden={!stage.ready}>
    <GraphEdges drawEdges={stage.drawEdges} linking={lk.linking} nodeMap={stage.nodeMap} linkCursor={lk.cursor} {edgeActions} {onEdgeCtx} />
    <GraphGroups groups={stage.groups} collapseKey={stage.collapseKey} onGroupDown={gest.onGroupDown} {onGroupCtx} onCollapse={folds.collapse} />
    {#each stage.view?.nodes ?? [] as n (n.key)}
      <GraphCard
        {n}
        selected={n.key === stage.selectedKey}
        root={n.key === data?.root}
        done={isDone(n)}
        orphan={isOrphan(n)}
        dim={!!lk.linking && n.key !== lk.linking.anchor && !lk.canPick(n)}
        pickable={!!lk.linking && lk.canPick(n)}
        anchor={lk.linking?.anchor === n.key}
        ownerless={stage.ownerless.has(n.key)}
        editingHere={editing?.key === n.key}
        picked={stage.picked.has(n.key)}
        ready={stage.ready}
        raster={rasterOf(n)}
        tier={tier.get(n.key)}
        more={stage.moreMap.get(n.key) ?? []}
        changed={stage.touched.has(n.key) && !n.draft}
        bind:editing
        z={stage.front[n.key] ?? 0}
        onRaise={stage.raise}
        onCardDown={gest.onCardDown}
        {onCtx}
        {onStartEdit}
        {onFinishEdit}
        {onOpen}
        {labels}
        onOpenMore={stage.openMore}
        {onShowChanges}
      />
    {/each}
  </div>

  {#if gest.marquee}
    <div
      class="gmarquee"
      style:left="{Math.min(gest.marquee.x0, gest.marquee.x1)}px"
      style:top="{Math.min(gest.marquee.y0, gest.marquee.y1)}px"
      style:width="{Math.abs(gest.marquee.x1 - gest.marquee.x0)}px"
      style:height="{Math.abs(gest.marquee.y1 - gest.marquee.y0)}px"
    ></div>
  {/if}

  {#if stage.mini}
    <GraphMinimap mini={stage.mini} onGo={stage.miniGo} />
  {/if}

  <GraphTools
    selectMode={stage.selectMode}
    pickedCount={stage.picked.size}
    full={stage.full}
    {canClear}
    {onHidePicked}
    onClearPicked={stage.clearPicked}
    {onToggleSelect}
    onFit={stage.fit}
    onRelayout={stage.relayout}
    onToggleFull={stage.toggleFull}
    {onClear}
  />

  {@render children?.()}

  {#if data?.truncated}
    <p class="graph__trunc">Показаны не все узлы — уменьшите глубину</p>
  {/if}
</div>

<GraphFoot
  selected={stage.selected}
  target={stage.selected ? stage.openTarget(stage.selected) : null}
  canRecenter={stage.canRecenter}
  {onOpen}
  {onRecenter}
/>

<style>
  .graph__stage {
    position: relative;
    flex: 1 1 auto;
    min-height: 16rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--pill-radius, 12px);
    background: var(--pill-bg, #0f0f0f);
    overflow: hidden;
    cursor: grab;
    touch-action: none;
    user-select: none;
  }

  .graph__stage--full {
    position: fixed;
    inset: 0;
    z-index: 40;
    min-height: 0;
    border: none;
    border-radius: 0;
  }

  .graph__msg,
  .graph__trunc {
    position: absolute;
    margin: 0;
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .graph__msg {
    inset: 0;
    display: grid;
    place-items: center;
  }

  .graph__trunc {
    left: 0.8rem;
    bottom: 0.6rem;
    font-size: var(--text-xs, 0.75rem);
  }

  .gworld {
    position: absolute;
    left: 0;
    top: 0;
    width: 0;
    height: 0;
    transform-origin: 0 0;
  }

  .gworld--hidden {
    visibility: hidden;
  }

  .gmarquee {
    position: absolute;
    z-index: 7;
    border: 1px solid #5aa9ff;
    background: color-mix(in srgb, #5aa9ff 14%, transparent);
    pointer-events: none;
  }

  .glink-hint {
    position: absolute;
    z-index: 4;
    top: 0.6rem;
    left: 50%;
    transform: translateX(-50%);
    margin: 0;
    padding: 0.35rem 0.9rem;
    border: 1px solid var(--color-accent, #c9a227);
    border-radius: 999px;
    background: var(--color-bg-raised, #1a1a1a);
    font-size: var(--text-sm, 0.875rem);
    pointer-events: none;
  }

  @media (max-width: 720px) {
    .graph__stage {
      flex: 0 0 auto;
      height: max(26rem, 68vh);
    }
  }
</style>
