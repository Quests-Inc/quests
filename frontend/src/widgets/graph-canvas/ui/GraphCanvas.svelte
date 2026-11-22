<script lang="ts">
  import { benchmarkRaster } from '../model/rasterBenchmark.ts'
  import { planContent } from '../model/contentDwell.ts'
 import {attachmentImage} from '@/shared/lib/imageViewer.svelte.ts'
  import {imagePixels} from '../model/imageQuality.ts'
  import { GraphMinimap, GraphTools, RASTER, tintOf, viewportBounds, boxInViewport, edgeInViewport, type GraphNoteEditing, type GraphZone } from '@/entities/graph'
  import type { Snippet } from 'svelte'
  import { createClusterFolds } from '../model/folds.svelte.ts'
  import { createGraphGestures } from '../model/gestures.svelte.ts'
  import { createLinkMode } from '../model/linking.svelte.ts'
  import { createGraphStage } from '../model/stage.svelte.ts'
  import GraphCard from './GraphCard.svelte'
  import GraphEdges from './GraphEdges.svelte'
  import GraphFoot from './GraphFoot.svelte'
  import GraphGroups from './GraphGroups.svelte'
  import GraphZones from './GraphZones.svelte'

  type Stage = ReturnType<typeof createGraphStage>
  type Gest = ReturnType<typeof createGraphGestures>
  type Folds = ReturnType<typeof createClusterFolds>
  type Link = ReturnType<typeof createLinkMode>

  let {
    stage,
    textRevision = 0,
    active = true,
    gest,
    folds,
    lk,
    editing = $bindable(null),
    data,
    center,
    error,
    loading,
    switching = false,
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
    panMode = false,
    onTogglePan,
    onClear,
    onRecenter,
    resize,
    zones = [],
    zoneDrawing = false,
    onZonesChange = () => {},
    onZoneFinish = () => {},
    children,
  }: {
    zones?: GraphZone[]
    zoneDrawing?: boolean
    onZonesChange?: (zones: GraphZone[]) => void
    onZoneFinish?: () => void
    textRevision?: number
    active?: boolean
    stage: Stage
    resize?: ReturnType<typeof import('../model/resize.svelte.ts').createCardResize>
    gest: Gest
    folds: Folds
    lk: Link
    editing?: GraphNoteEditing | null
    data: { root?: string; truncated?: boolean } | null
    center: { id: number | null }
    error: string
    loading: boolean
    switching?: boolean
    canClear: boolean
    tier: Map<string, string>
    edgeActions: (edge: { draft?: string; from: string; to: string; kind: string }) => { id: string; label?: string }[]
    onCtx: (e: MouseEvent, n?: Stage['nodes'][number] | null) => void
    onGroupCtx: (e: MouseEvent, g: { type: string; owner: string }) => void
    onEdgeCtx: (e: MouseEvent, edge: { from: string; to: string; kind: string }) => void
    onOpen?: (kind: string, id: number) => void
    labels?: Record<string, string>
    onStartEdit: (n: Stage['nodes'][number], opts?: { caret?: number | null }) => void
    onFinishEdit: (saveIt: boolean) => void
    onShowChanges: (key: string) => void
    onHidePicked: () => void
    onToggleSelect: () => void
    panMode?: boolean
    onTogglePan?: () => void
    onClear: () => void
    onRecenter: () => void
    children?: Snippet
  } = $props()

  const pendingScene = $derived(((switching || loading) && !stage.view) || stage.previewing || stage.sceneHidden)

  // Markdown starts once the camera is still, two cards a frame. Photos take their address two a frame and do not wait for that.
  let warm = $state(new Set<string>())
  let dwellTick = $state(0)
  let seenCam = { x: Number.NaN, y: Number.NaN, k: Number.NaN }
  let idleSince = 0
  let clocks = new Map<string, number>()
  let queued = new Set<string>()
  let releaseToken = 0
  const mapKeys = $derived((stage.view?.nodes ?? []).map((n) => n.key).join('\n'))
  $effect(() => {
    void dwellTick
    const cam = gest.cam
    const nodes = stage.view?.nodes ?? []
    const now = performance.now()
    const camMoved = cam.x !== seenCam.x || cam.y !== seenCam.y || cam.k !== seenCam.k
    seenCam = { x: cam.x, y: cam.y, k: cam.k }
    const pad = viewportBounds(cam, stage.stageSize)
    const strict = viewportBounds(cam, stage.stageSize, 0)
    const plan = planContent({
      now,
      idleSince,
      pointer: gest.moving || !active || pendingScene,
      camMoved,
      visible: nodes.filter((n) => boxInViewport(n, strict)).map((n) => n.key),
      mounted: nodes.filter((n) => boxInViewport(n, pad)).map((n) => n.key),
      clocks,
      warm,
    })
    idleSince = plan.idleSince
    clocks = plan.clocks
    const kept = new Set<string>()
    for (const key of warm) if (plan.warm.has(key)) kept.add(key)
    if (kept.size !== warm.size) warm = kept
    if (!plan.idle) {
      releaseToken++
      queued = new Set()
    } else {
      const due = [...plan.warm].filter((key) => !warm.has(key) && !queued.has(key))
      if (due.length) {
        const token = ++releaseToken
        queued = new Set([...queued, ...due])
        let index = 0
        const pump = () => {
          if (token !== releaseToken || gest.moving) return
          const batch = due.slice(index, index + 2)
          index += batch.length
          if (!batch.length) return
          const next = new Set(warm)
          for (const key of batch) {
            queued.delete(key)
            next.add(key)
          }
          warm = next
          if (index < due.length) requestAnimationFrame(pump)
        }
        requestAnimationFrame(pump)
      }
    }
    // The frame that moved the camera is not idle. Recheck once it stops, even when the dwell is zero.
    if (plan.wait > 0 || camMoved) {
      const timer = setTimeout(() => {
        dwellTick++
      }, plan.wait > 0 ? plan.wait + 20 : 0)
      return () => clearTimeout(timer)
    }
  })

  // All cards mount during measurement. Once laid out, retain only the viewport and active editors.
  const bounds = $derived(stage.ready ? viewportBounds(gest.cam, stage.stageSize) : null)
  const renderedNodes = $derived((stage.previewing ? [] : stage.view?.nodes ?? []).filter((n) =>
    n.key === editing?.key || n.key === resize?.active || boxInViewport(n, bounds)))
  const renderedGroups = $derived(stage.groups.filter((g) => boxInViewport(g, bounds)))
  const renderedEdges = $derived(stage.drawEdges.filter((e) => edgeInViewport(e, bounds)))

  /** A step's quest: its colour tints the step. */
  const owners = $derived(
    new Map(stage.edges.filter((e) => e.kind === 'structure' && e.to.startsWith('step:') && e.from.startsWith('quest:')).map((e) => [e.to, stage.nodeMap.get(e.from)]))
  )
  const isDone = (n: { status?: string }) => n.status === 'completed' || n.status === 'done'
  const isOrphan = (n: { type: string; links: number }) => (n.type === 'note' || n.type === 'attachment') && n.links === 0
  const rasterOf = (n: { type: string; mime?: string }) =>
    n.type === 'attachment' && RASTER.has(String(n.mime || '').split(';')[0].trim())
  const galleryImages=$derived(stage.nodes.filter(rasterOf).map(node=>attachmentImage({id:node.id,filename:node.title})))
  let benchmarkStarted = false
  $effect(() => {
    if (!benchmarkStarted && active && stage.ready && !pendingScene && stage.el && new URL(location.href).searchParams.has('rasterbench')) {
      benchmarkStarted = true
      return benchmarkRaster(stage.el, () => gest.cam, c => gest.cam = c)
    }
  })
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="graph__stage"
  class:graph__stage--full={stage.full}
  class:graph__stage--pan={panMode}
  data-swipe={stage.view?.nodes?.length ? undefined : 'free'}
  bind:this={stage.el}
  onpointerdowncapture={(e) => { if (!(e.target as Element).closest('.zones, .zones__editor')) gest.onTouchDown(e) }}
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
  {#if pendingScene}
    <p class="graph__msg" role="status">Загрузка карты…</p>
  {:else if !center.id && !stage.view?.nodes?.length && !zones.length}
    <p class="graph__msg">Выберите квест или заметку в поиске выше — или откройте карту из карточки квеста. ПКМ по полю создаёт новые.</p>
  {:else if error}
    <p class="graph__msg">{error}</p>
  {/if}

  {#if !stage.sceneHidden}
  <GraphZones {zones} cam={gest.cam} drawing={zoneDrawing} worldAt={gest.worldAt} onChange={onZonesChange} onFinish={onZoneFinish} onJump={stage.focusZone}>
    {#snippet tools()}
  <GraphTools inline
    selectMode={stage.selectMode}
    pickedCount={stage.picked.size}
    full={stage.full}
    {canClear}
    {onHidePicked}
    onClearPicked={stage.clearPicked}
    {onToggleSelect}
    {panMode}
    {onTogglePan}
    onFit={stage.fit}
    onRelayout={stage.relayout}
    onToggleFull={stage.toggleFull}
    {onClear}
  />

    {/snippet}
  </GraphZones>
  {/if}

  {#if stage.previewing && !stage.sceneHidden}
    <div class="gworld" style:transform="translate({gest.cam.x}px, {gest.cam.y}px) scale({gest.cam.k})" aria-label="Геометрия карты, содержимое загружается">
      {#each (stage.view?.nodes ?? []).filter(n => boxInViewport(n, bounds)) as n (n.key)}
        <div class="graph__geometry" style:transform="translate({n.x - n.w / 2}px, {n.y - n.h / 2}px)" style:width="{n.w}px" style:height="{n.h}px"><span>{n.title}</span></div>
      {/each}
    </div>
  {/if}

  <div class="gworld" style:transform="translate({gest.cam.x}px, {gest.cam.y}px) scale({gest.cam.k})" class:gworld--hidden={!stage.ready || pendingScene}>
    <GraphEdges drawEdges={renderedEdges} lit={stage.lit} linking={lk.linking} nodeMap={stage.nodeMap} linkCursor={lk.cursor} {edgeActions} {onEdgeCtx} />
    <GraphGroups groups={renderedGroups} collapseKey={stage.collapseKey} onGroupDown={gest.onGroupDown} {onGroupCtx} onCollapse={folds.collapse} />
    {#each renderedNodes as n (n.key)}
      <GraphCard
        imageGallery={galleryImages}
        {n}
        selected={n.key === stage.selectedKey}
        lit={n.key === stage.lit?.from || n.key === stage.lit?.to}
        tint={tintOf(n, owners.get(n.key))}
        root={n.key === data?.root}
        done={isDone(n)}
        orphan={isOrphan(n)}
        dim={(!!lk.linking && n.key !== lk.linking.anchor && !lk.canPick(n)) || (!!stage.lit && n.key !== stage.lit.from && n.key !== stage.lit.to)}
        pickable={!!lk.linking && lk.canPick(n)}
        anchor={lk.linking?.anchor === n.key}
        ownerless={stage.ownerless.has(n.key)}
        editingHere={editing?.key === n.key}
        picked={stage.picked.has(n.key)}
        ready={stage.ready && !pendingScene}
        deferContent
        onContentMeasure={stage.measureContent}
        {textRevision}
        loadText={n.key === editing?.key || warm.has(n.key)}
        mapJump={{ keys: mapKeys, jump: (kind, id) => stage.focusNode(`${kind}:${id}`) }}
        raster={rasterOf(n)}
        imagePixels={imagePixels(n,gest.cam,stage.stageSize,typeof window!=='undefined'?window.devicePixelRatio||1:1)}
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
        onRevealed={(key, text) => stage.revealNote(key, text)}
        {labels}
        onOpenMore={stage.openMore}
        {onShowChanges}
        resizing={resize?.active === n.key}
        onResizeStart={panMode ? undefined : resize?.start}
        onResizeReset={resize?.reset}
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
    {#if !stage.sceneHidden && (!pendingScene || stage.previewing)}<GraphMinimap mini={stage.mini} onGo={stage.miniGo} />{/if}
  {/if}


  {@render children?.()}

  {#if data?.truncated}
    <p class="graph__trunc">Показаны не все узлы — уменьшите глубину</p>
  {/if}
</div>

{#if !pendingScene}
<GraphFoot
  selected={stage.selected}
  target={stage.selected ? stage.openTarget(stage.selected) : null}
  canRecenter={stage.canRecenter}
  {onOpen}
  {onRecenter}
/>
{/if}

<style>
  .graph__geometry {
    position: absolute;
    box-sizing: border-box;
    border: 1px solid var(--color-border, #666);
    border-radius: 10px;
    background: var(--color-bg-raised, #262626);
    color: var(--color-fg-muted, #aaa);
    pointer-events: none;
    overflow: hidden;
    padding: 10px;
  }

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

  .graph__stage--pan :global(.gcard),
  .graph__stage--pan :global(.gcard__body) {
    cursor: grab;
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
