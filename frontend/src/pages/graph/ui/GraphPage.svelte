<script lang="ts">
  import { computeTiers, RASTER } from '@/entities/graph'
  import { DraftEntityModal, QuestModal, StepModal } from '@/features/quest-edit'
  import { FindEntityModal } from '@/features/find-entity'
  import { putGraphDraftRoots } from '@/shared/api/graph.ts'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
  import { handleGraphKey } from '../model/keys.ts'
  import { createGraphDraft } from '../model/draft.svelte.ts'
  import { createGraphEdits } from '../model/edits.svelte.ts'
  import { createClusterFolds } from '../model/folds.svelte.ts'
  import { createGraphGestures } from '../model/gestures.svelte.ts'
  import { createLinkMode } from '../model/linking.svelte.ts'
  import { createGraphMenu } from '../model/menu.svelte.ts'
  import { createGraphPrefs } from '../model/prefs.svelte.ts'
  import { createGraphQuery } from '../model/query.svelte.ts'
  import { createGraphStage } from '../model/stage.svelte.ts'
  import GraphCard from './GraphCard.svelte'
  import GraphDraftLog from './GraphDraftLog.svelte'
  import GraphEdges from './GraphEdges.svelte'
  import GraphFoot from './GraphFoot.svelte'
  import GraphGroups from './GraphGroups.svelte'
  import GraphMinimap from './GraphMinimap.svelte'
  import GraphToolbar from './GraphToolbar.svelte'
  import GraphTools from './GraphTools.svelte'

  /**
   * Relations map tab: what a quest/note is connected to, drawn from existing
   * ownership, step order and reference tokens. Nodes are HTML cards (so
   * markdown renders and long text scrolls); the SVG layer only draws edges.
   * Состояние разнесено по контроллерам model/: загрузка, поле, жесты, черновик, меню.
   */
  let {
    active = true,
    rootType = null,
    rootId = null,
    nonce = 0,
    quests = [],
    notes = [],
    questlines = [],
    onCenter,
    onOpen,
  }: {
    active?: boolean
    rootType?: 'quest' | 'note' | 'questline' | null
    rootId?: number | null
    nonce?: number
    quests?: { id: number; status: string; title?: string }[]
    notes?: { id: number; title?: string }[]
    questlines?: { id: number; title?: string }[]
    onCenter?: (type: string | null, id: number | null) => void
    onOpen?: (kind: string, id: number) => void
  } = $props()

  const prefs = createGraphPrefs(typeof localStorage === 'undefined' ? null : localStorage)
  const saved = new Map<string, { x: number; y: number }>()
  // Поле, черновик и жесты читают друг друга только после сборки. Методы до этого не зовутся.
  let stage: ReturnType<typeof createGraphStage>
  let query: ReturnType<typeof createGraphQuery>
  let gest: ReturnType<typeof createGraphGestures>
  let gd: ReturnType<typeof createGraphDraft>
  let lk: ReturnType<typeof createLinkMode>
  let edits: ReturnType<typeof createGraphEdits>

  const folds = createClusterFolds({
    saved,
    getNodes: () => stage.nodes,
    getGroups: () => stage.groups,
  })
  gd = createGraphDraft({
    getCenter: () => query.center,
    setCenter: (c) => query.setCenter(c),
    getNodes: () => stage.nodes,
    getSaved: () => saved,
    resetPositions: () => folds.forgetPositions(),
  })
  gest = createGraphGestures({
    getStage: () => stage.el,
    nodes: () => stage.nodes,
    nodeMap: () => stage.nodeMap,
    touchView: () => stage.touch(),
    isLinking: () => !!lk.linking,
    finishLink: (n) => lk.finish(n),
    setLinkCursor: (p) => {
      lk.cursor = p
    },
    selectMode: () => stage.selectMode,
    getPicked: () => stage.picked,
    setPicked: (s) => stage.setPicked(s),
    getSaved: () => saved,
    select: (n) => stage.select(n),
  })
  lk = createLinkMode({
    getNodeMap: () => stage.nodeMap,
    getEdges: () => stage.edges,
    addOp: (kind, payload) => gd.addOp(kind, payload),
    askDrop: (op) => gd.askDrop(op),
  })
  query = createGraphQuery({
    getActive: () => active,
    getRootType: () => rootType ?? null,
    getRootId: () => rootId ?? null,
    getNonce: () => nonce,
    getExpandKeys: () => prefs.expandKeys,
    getReloadTick: () => gd.reloadTick,
    getDraftRoots: () => gd.draft.roots,
    getHiddenSize: () => prefs.hidden.size,
    getQuests: () => quests,
    getNotes: () => notes,
    getQuestlines: () => questlines,
    onCenter: (type, id) => onCenter?.(type, id),
    onRetarget: () => {
      stage.selectedKey = null
      folds.reset()
      prefs.setExpand([])
      folds.forgetPositions()
    },
    prepareFresh: () => {
      stage.selectedKey = null
      folds.forgetPositions()
      prefs.setExpand([])
    },
    forgetPositions: () => folds.forgetPositions(),
    resetFolds: () => folds.reset(),
    setExpand: (keys) => prefs.setExpand(keys),
    saveHidden: (hidden) => prefs.saveHidden(hidden),
    cancelLink: () => lk.cancel(),
    clearEditing: () => {
      edits.editing = null
    },
    clearSelection: () => {
      stage.selectedKey = null
    },
    loadDraft: () => gd.load(),
  })
  stage = createGraphStage({
    getActive: () => active,
    getData: () => query.data,
    getDraft: () => gd.draft,
    getDepth: () => query.depth,
    getQuests: () => quests,
    prefs,
    saved,
    folds,
    initialLayout: prefs.layoutMode,
    getCam: () => gest.cam,
    setCam: (cam) => {
      gest.cam = cam
    },
    clearMarquee: () => {
      gest.marquee = null
    },
    loadDraft: () => gd.load(),
  })
  edits = createGraphEdits({
    ops: () => gd.draft.ops,
    addOp: (kind, payload) => gd.addOp(kind, payload),
    reloadDraft: () => gd.load(),
    edges: () => stage.edges,
    noteTitles: () => [...notes.map((x) => x.title), ...stage.nodes.filter((x) => x.type === 'note').map((x) => x.title)],
  })

  async function hideKeys(keys: string[]) {
    if (await stage.hideKeys(keys)) await query.clearMap()
  }
  async function hidePicked() {
    if (await stage.hidePicked()) await query.clearMap()
  }
  function showChanges(key: string) {
    if (stage.showChanges(key)) gd.open = true
  }

  const menu = createGraphMenu({
    worldAt: (e) => gest.worldAt(e),
    isLinking: () => !!lk.linking,
    cancelLink: () => lk.cancel(),
    selectMode: () => stage.selectMode,
    pickedHas: (key) => stage.picked.has(key),
    pickedSize: () => stage.picked.size,
    setSelectedKey: (key) => {
      stage.selectedKey = key
    },
    select: (n) => stage.select(n),
    collapseKey: (g) => stage.collapseKey(g),
    getDraftRoots: () => gd.draft.roots,
    getRootKey: () => query.data?.root,
    getNodes: () => stage.nodes,
    getExpandKeys: () => prefs.expandKeys,
    openTarget: (n) => stage.openTarget(n),
    ownExpand: (key) => prefs.ownExpand(key),
    outgoingRefs: (n) => stage.outgoingRefs(n),
    childKeys: (n, type) => stage.childKeys(n, type),
    parentNote: (n) => stage.parentNote(n),
    titleOf: (key) => stage.titleOf(key),
    hidePicked,
    toggleSelectMode: () => stage.toggleSelectMode(),
    hideKeys,
    createNoteHere: (at, linkFrom) => edits.createNoteHere(at, linkFrom),
    openEntity: (kind, at, extra) => edits.openEntity(kind, at, extra),
    startEdit: (n, at) => edits.startEdit(n, at),
    addOp: (kind, payload) => gd.addOp(kind, payload),
    askDrop: (op) => gd.askDrop(op),
    startLink: (mode, n) => lk.start(mode, n),
    collapse: (key) => folds.collapse(key),
    disband: (owner) => folds.disband(owner),
    unlinkRef: (n, target) => lk.unlinkRef(n, target),
    setExpand: (keys) => prefs.setExpand(keys),
    loadDraft: () => gd.load(),
    putRoots: putGraphDraftRoots,
    onOpen,
    recenter: (n) => query.recenter(n),
    saved,
  })

  function toggleSelectMode() {
    stage.toggleSelectMode()
    menu.close()
  }

  let view = $derived(stage.view)
  let ready = $derived(stage.ready)
  let cam = $derived(gest.cam)
  let marquee = $derived(gest.marquee)
  let linking = $derived(lk.linking)
  let linkCursor = $derived(lk.cursor)
  let draft = $derived(gd.draft)
  let selectedKey = $derived(stage.selectedKey)
  let data = $derived(query.data)
  let center = $derived(query.center)
  let error = $derived(query.error)
  let loading = $derived(query.loading)
  let ownerless = $derived(stage.ownerless)
  let touched = $derived(stage.touched)
  let picked = $derived(stage.picked)
  let full = $derived(stage.full)
  let selectMode = $derived(stage.selectMode)
  let canClear = $derived(query.canClear)
  let filteredOut = $derived(stage.filteredOut)
  let expanded = $derived(folds.expanded)
  let loose = $derived(folds.loose)
  let drawEdges = $derived(stage.drawEdges)
  let nodeMap = $derived(stage.nodeMap)
  let groups = $derived(stage.groups)
  let prevTier = new Map<string, string>()
  let tier = $derived.by(() => {
    const camNow = gest.cam
    const next = computeTiers({
      nodes: stage.view?.nodes,
      cam: { k: camNow.k, x: camNow.x, y: camNow.y },
      stage: stage.stageSize,
      ready: stage.ready,
      editingKey: edits.editing?.key,
      prev: prevTier,
    })
    prevTier = next
    return next
  })
  let moreMap = $derived(stage.moreMap)
  let mini = $derived(stage.mini)
  let flashOp = $derived(stage.flashOp)
  let selected = $derived(stage.selected)
  let editing = $derived(edits.editing)
  let questStatuses = $derived(prefs.questStatuses)
  let statusLabel = $derived(prefs.statusLabel)
  let typesLabel = $derived(prefs.typesLabel)
  let shownTypes = $derived(prefs.shownTypes)
  let found = $derived(query.found)
  let matches = $derived(found.items)
  let rootParsed = $derived(found.parsed)
  let ctx = $derived(menu.ctx)
  let ctxItems = $derived(menu.items())
  let questModal = $derived(menu.questModal)
  let findModal = $derived(menu.findOpen)

  const { onTouchDown, onBgDown, onCardDown, onGroupDown, onMove, onUp, onWheel } = gest
  const onCtx = menu.onCtx
  const onGroupCtx = menu.onGroupCtx
  const onEdgeCtx = menu.onEdgeCtx
  const onCtxSelect = menu.onCtxSelect
  const onFilesPicked = menu.onFilesPicked
  const edgeActions = menu.edgeActions
  const collapse = folds.collapse
  const collapseAll = folds.collapseAll
  const collapseKey = stage.collapseKey
  const titleOf = stage.titleOf
  const openMore = stage.openMore
  const miniGo = stage.miniGo
  const clearPicked = stage.clearPicked
  const fit = stage.fit
  const relayout = stage.relayout
  const toggleFull = stage.toggleFull
  const onFullscreenChange = stage.onFullscreenChange
  const clearMap = query.clearMap
  const addToMap = query.addToMap
  const chooseRoot = query.chooseRoot
  const recenter = query.recenter
  const openTarget = stage.openTarget
  const canPick = lk.canPick
  const startEditNote = edits.startEditNote
  const finishEdit = edits.finishEdit
  const toggleStatus = prefs.toggleStatus
  const toggleType = prefs.toggleType
  const onChooseRoot = chooseRoot

  function onLayoutChange() {
    prefs.layoutMode = stage.layoutMode
    relayout()
  }

  function onKey(e: KeyboardEvent) {
    handleGraphKey(e, {
      active,
      linking: !!linking,
      selectMode,
      full,
      menuOpen: ctx.open,
      ownerless: ownerless.size,
      canClear,
      cancelLink: () => lk.cancel(),
      toggleSelect: () => toggleSelectMode(),
      toggleFull: () => toggleFull(),
      save: () => gd.save(),
      undo: () => {
        const last = draft.ops[draft.ops.length - 1]
        if (last) void gd.askDrop(last)
      },
      fit: () => fit(),
      relayout: () => relayout(),
      clear: () => void clearMap(),
    })
  }

  const isDone = (n: { status?: string }) => n.status === 'completed' || n.status === 'done'
  const isOrphan = (n: { type: string; links: number }) => (n.type === 'note' || n.type === 'attachment') && n.links === 0
  const rasterOf = (n: { type: string; mime?: string }) =>
    n.type === 'attachment' && RASTER.has(String(n.mime || '').split(';')[0].trim())

  $effect(() => {
    query.syncRoot()
  })
  $effect(() => {
    if (active) void gd.load()
  })
  $effect(() => {
    void query.load()
  })
  $effect(() => {
    stage.leaveIfHidden(active)
  })
  $effect(() => {
    stage.poke()
  })
  $effect(() => {
    stage.compose()
  })
  $effect(() => {
    const node = stage.el
    if (!node) return
    return stage.watchSize(node)
  })
</script>

<svelte:window
  onkeydown={onKey}
/>
<svelte:document onfullscreenchange={onFullscreenChange} />

<div class="graph">
  <GraphToolbar
    bind:rootQuery={query.rootQuery}
    bind:pickOpen={query.pickOpen}
    bind:layoutMode={stage.layoutMode}
    bind:depth={query.depth}
    bind:showEdge={stage.showEdge}
    {matches}
    {rootParsed}
    {statusLabel}
    {filteredOut}
    {questStatuses}
    {typesLabel}
    {shownTypes}
    onChooseRoot={chooseRoot}
    onToggleStatus={toggleStatus}
    onToggleType={toggleType}
    onLayoutChange={onLayoutChange}
  >
    {#if expanded.size || loose.size}
      <button type="button" class="graph__btn" onclick={collapseAll}>Свернуть вложения</button>
    {/if}
  </GraphToolbar>

  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="graph__stage"
    class:graph__stage--full={full}
    data-swipe={view?.nodes?.length ? undefined : 'free'}
    bind:this={stage.el}
    onpointerdowncapture={onTouchDown}
    onpointerdown={onBgDown}
    onpointermove={onMove}
    onpointerup={onUp}
    onpointercancel={onUp}
    onwheel={onWheel}
    oncontextmenu={(e) => onCtx(e)}
  >
    {#if linking}
      <p class="glink-hint">
        {#if linking.mode === 'to'}
          Выберите, на что ссылаться
        {:else if linking.mode === 'from'}
          Выберите, где должна появиться ссылка на «{titleOf(linking.anchor)}»
        {:else}
          Выберите, к чему прикрепить «{titleOf(linking.anchor)}»
        {/if}
        · ПКМ или Esc — отмена
      </p>
    {/if}
    {#if !center.id && !view?.nodes?.length}
      <p class="graph__msg">Выберите квест или заметку в поиске выше — или откройте карту из карточки квеста. ПКМ по полю создаёт новые.</p>
    {:else if error}
      <p class="graph__msg">{error}</p>
    {:else if loading && !view}
      <p class="graph__msg">Загрузка…</p>
    {/if}

    <div class="gworld" style:transform="translate({cam.x}px, {cam.y}px) scale({cam.k})" class:gworld--hidden={!ready}>
      <GraphEdges {drawEdges} {linking} {nodeMap} {linkCursor} {edgeActions} {onEdgeCtx} />

      <GraphGroups {groups} {collapseKey} {onGroupDown} {onGroupCtx} onCollapse={collapse} />

      {#each view?.nodes ?? [] as n (n.key)}
        <GraphCard
          {n}
          selected={n.key === selectedKey}
          root={n.key === data?.root}
          done={isDone(n)}
          orphan={isOrphan(n)}
          dim={!!linking && n.key !== linking.anchor && !canPick(n)}
          pickable={!!linking && canPick(n)}
          anchor={linking?.anchor === n.key}
          ownerless={ownerless.has(n.key)}
          editingHere={editing?.key === n.key}
          picked={picked.has(n.key)}
          {ready}
          raster={rasterOf(n)}
          tier={tier.get(n.key)}
          more={moreMap.get(n.key) ?? []}
          changed={touched.has(n.key) && !n.draft}
          bind:editing={edits.editing}
          {onCardDown}
          {onCtx}
          onStartEdit={startEditNote}
          onFinishEdit={finishEdit}
          {onOpen}
          onOpenMore={openMore}
          onShowChanges={showChanges}
        />
      {/each}
    </div>

    {#if marquee}
      <div
        class="gmarquee"
        style:left="{Math.min(marquee.x0, marquee.x1)}px"
        style:top="{Math.min(marquee.y0, marquee.y1)}px"
        style:width="{Math.abs(marquee.x1 - marquee.x0)}px"
        style:height="{Math.abs(marquee.y1 - marquee.y0)}px"
      ></div>
    {/if}

    {#if mini}
      <GraphMinimap {mini} onGo={miniGo} />
    {/if}

    <GraphTools
      {selectMode}
      pickedCount={picked.size}
      {full}
      {canClear}
      onHidePicked={hidePicked}
      onClearPicked={clearPicked}
      onToggleSelect={toggleSelectMode}
      onFit={fit}
      onRelayout={relayout}
      onToggleFull={toggleFull}
      onClear={clearMap}
    />

    {#if draft.ops.length}
      <GraphDraftLog
        ops={draft.ops}
        bind:open={gd.open}
        committing={gd.committing}
        ownerlessCount={ownerless.size}
        problems={gd.problems}
        {flashOp}
        {titleOf}
        onSave={gd.save}
        onCancelAll={() => (gd.cancelAsk = true)}
        onDrop={gd.askDrop}
      />
    {/if}

    {#if data?.truncated}
      <p class="graph__trunc">Показаны не все узлы — уменьшите глубину</p>
    {/if}
  </div>

  <GraphFoot
    {selected}
    target={selected ? openTarget(selected) : null}
    canRecenter={stage.canRecenter}
    {onOpen}
    onRecenter={() => selected && recenter(selected)}
  />
</div>

<input bind:this={menu.fileInput} type="file" multiple hidden onchange={onFilesPicked} />

<QuestModal
  open={questModal.open}
  mode="create"
  defaults={{ description: questModal.desc, questline_id: questModal.questlineId }}
  {quests}
  {notes}
  attachments={[]}
  onClose={menu.closeQuest}
  onSaved={() => {}}
  onDraft={(payload) => gd.addOp('create_quest', { fields: payload, pos: questModal.at })}
/>

<DraftEntityModal
  open={edits.entityModal.open}
  kind={edits.entityModal.kind}
  initial={edits.entityModal.edit ? { title: edits.entityModal.edit.title, description: edits.entityModal.edit.description } : null}
  heading={edits.entityModal.edit?.heading ?? ''}
  {quests}
  {notes}
  onClose={edits.closeEntity}
  onSubmit={edits.submitEntity}
/>

<QuestModal
  open={edits.editQuest.open}
  mode="edit"
  quest={edits.editQuest.quest}
  {quests}
  {notes}
  attachments={[]}
  onClose={edits.closeEditQuest}
  onSaved={() => {}}
  onDraft={edits.draftEditQuest}
/>

<StepModal
  open={edits.editStep.open}
  questId={edits.editStep.questId}
  step={edits.editStep.step}
  {quests}
  {notes}
  questlines={[]}
  attachments={[]}
  onClose={edits.closeEditStep}
  onSaved={() => {}}
  onDraft={edits.draftEditStep}
/>

<!-- Quests follow the status filter; notes and questlines are offered as they are. -->
<FindEntityModal
  open={findModal}
  types={['quest', 'note', 'questline']}
  quests={quests.filter((q) => questStatuses.has(q.status))}
  {notes}
  {questlines}
  onClose={menu.closeFind}
  onPick={addToMap}
/>

<ConfirmModal
  open={gd.cancelAsk}
  title="Отменить все изменения?"
  message={`Черновик (${draft.ops.length}) будет сброшен, загруженные в него файлы удалены. В базе ничего не изменится.`}
  confirmLabel="Отменить всё"
  onCancel={() => (gd.cancelAsk = false)}
  onConfirm={gd.discard}
/>

<ConfirmModal
  open={gd.dropAsk != null}
  title="Убрать из черновика?"
  message={`Вместе с этим уберётся то, что на него опирается: всего операций ${gd.dropAsk?.ids.length ?? 0}.`}
  confirmLabel="Убрать"
  onCancel={() => (gd.dropAsk = null)}
  onConfirm={() => gd.dropAsk && gd.doDrop(gd.dropAsk.id)}
/>

<ContextMenu
  open={ctx.open}
  x={ctx.x}
  y={ctx.y}
  items={ctxItems}
  onSelect={onCtxSelect}
  onClose={menu.close}
/>

<style>
  .graph {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    height: 100%;
    min-height: 0;
    padding: 0.75rem 1rem 1rem;
    box-sizing: border-box;
  }

  :global(.graph__btn) {
    padding: 0.25rem 0.7rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 4px;
    background: transparent;
    color: var(--color-fg, #e8e8e8);
    font: inherit;
    font-size: var(--text-xs, 0.75rem);
    cursor: pointer;
  }

  :global(.graph__btn:hover) {
    border-color: var(--color-accent, #c9a227);
  }

  .graph__stage {
    position: relative;
    flex: 1 1 auto;
    min-height: 16rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--pill-radius, 12px);
    background: var(--pill-bg, #0f0f0f);
    overflow: hidden;
  }

  /* Above the journal chrome, below modals (50) and menus (80). */
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

  .graph__stage {
    cursor: grab;
    touch-action: none;
    user-select: none;
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

  :global(.graph__btn--on) {
    border-color: #5aa9ff;
    color: #5aa9ff;
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

  :global(.graph__btn--primary) {
    background: var(--color-accent, #c9a227);
    border-color: var(--color-accent, #c9a227);
    color: var(--color-bg, #121212);
    font-weight: 600;
  }

  :global(.graph__btn--primary:disabled) {
    opacity: 0.45;
    cursor: not-allowed;
  }

  /* Narrow (portrait) screens: the controls form an even grid and the page
     scrolls, so the map field can be tall instead of squeezed under them. */
  @media (max-width: 720px) {
    .graph {
      overflow-y: auto;
      overflow-x: hidden;
    }

    .graph__stage {
      flex: 0 0 auto;
      height: max(26rem, 68vh);
    }
  }
</style>
