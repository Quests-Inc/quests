<script lang="ts">
  import { computeTiers } from '@/entities/graph'
  import { noteCatalog } from '@/entities/note'
  import { questCatalog } from '@/entities/quest'
  import { DraftEntityModal, QuestModal, StepModal } from '@/features/quest-edit'
  import { FindEntityModal } from '@/features/find-entity'
  import { putGraphDraftRoots } from '@/shared/api/graph.ts'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
  import { GraphDraftLog, createGraphDraft, createGraphEdits } from '@/features/graph-draft'
  import { GraphCanvas, createClusterFolds, createGraphGestures, createGraphStage, createLinkMode } from '@/widgets/graph-canvas'
  import { handleGraphKey } from '../model/keys.ts'
  import { createGraphMenu } from '../model/menu.svelte.ts'
  import { createGraphPrefs } from '../model/prefs.svelte.ts'
  import { createGraphQuery } from '../model/query.svelte.ts'
  import GraphToolbar from './GraphToolbar.svelte'

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
    questlines = [],
    labels = {},
    onCenter,
    onOpen,
  }: {
    active?: boolean
    rootType?: 'quest' | 'note' | 'questline' | null
    rootId?: number | null
    nonce?: number
    questlines?: { id: number; title?: string }[]
    labels?: Record<string, string>
    onCenter?: (type: string | null, id: number | null) => void
    onOpen?: (kind: string, id: number) => void
  } = $props()

  let quests = $derived(questCatalog.list)
  let notes = $derived(noteCatalog.list)

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

  let linking = $derived(lk.linking)
  let draft = $derived(gd.draft)
  let data = $derived(query.data)
  let center = $derived(query.center)
  let error = $derived(query.error)
  let loading = $derived(query.loading)
  let ownerless = $derived(stage.ownerless)
  let full = $derived(stage.full)
  let selectMode = $derived(stage.selectMode)
  let canClear = $derived(query.canClear)
  let filteredOut = $derived(stage.filteredOut)
  let expanded = $derived(folds.expanded)
  let loose = $derived(folds.loose)
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
  let flashOp = $derived(stage.flashOp)
  let selected = $derived(stage.selected)
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

  const onCtx = menu.onCtx
  const onGroupCtx = menu.onGroupCtx
  const onEdgeCtx = menu.onEdgeCtx
  const onCtxSelect = menu.onCtxSelect
  const onFilesPicked = menu.onFilesPicked
  const edgeActions = menu.edgeActions
  const collapseAll = folds.collapseAll
  const titleOf = stage.titleOf
  const fit = stage.fit
  const relayout = stage.relayout
  const toggleFull = stage.toggleFull
  const onFullscreenChange = stage.onFullscreenChange
  const clearMap = query.clearMap
  const addToMap = query.addToMap
  const chooseRoot = query.chooseRoot
  const recenter = query.recenter
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

  <GraphCanvas
    {stage}
    {gest}
    {folds}
    {lk}
    bind:editing={edits.editing}
    {data}
    {center}
    {error}
    {loading}
    {canClear}
    {tier}
    {edgeActions}
    {onCtx}
    {onGroupCtx}
    {onEdgeCtx}
    {onOpen}
    {labels}
    onStartEdit={startEditNote}
    onFinishEdit={finishEdit}
    onShowChanges={showChanges}
    onHidePicked={hidePicked}
    onToggleSelect={toggleSelectMode}
    onClear={clearMap}
    onRecenter={() => selected && recenter(selected)}
  >
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
  </GraphCanvas>
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

  :global(.graph__btn--on) {
    border-color: #5aa9ff;
    color: #5aa9ff;
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
  }
</style>
