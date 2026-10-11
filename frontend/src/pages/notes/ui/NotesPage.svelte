<script lang="ts">
  import { tick } from 'svelte'
  import {
    createNote,
    deleteNote,
    getNote,
    updateNote,
    searchNotes,
    setNoteOutdated,
  } from '@/shared/api/client.ts'
  import { createNoteActions } from '../model/noteActions.svelte.ts'
  import { createNoteEditor } from '../model/noteEditor.svelte.ts'
  import { createNoteRename } from '../model/noteRename.svelte.ts'
  import { createViewMode, VIEW_MODE_OPTIONS } from '../model/viewMode.svelte.ts'
  import { createNoteExport } from '../model/noteExport.svelte.ts'
  import { createTreeDnd } from '../model/treeDnd.svelte.ts'
  import { NOTE_SPEC, moveTargetItems, noteCrumbs, noteMenuItems, parentMenu, readmeBubbles, compareNotes, withAncestors, clearNoteDraft, noteDrafts, downloadNoteMarkdown, downloadNotePdf, NoteOutline } from '@/entities/note'
  import { parseQuery, passes } from '@/shared/lib/searchFlags.ts'
  import { copyText } from '@/shared/lib/clipboard.ts'
  import {
    caretOffsetFromPoint,
    mapRenderedOffsetToSource,
    mapSourceOffsetToRendered,
    placeTextareaCaret,
    scrollRootToTextOffset,
  } from '@/shared/lib/mdCaretMap.ts'
  import { toast } from '@/shared/lib/toasts.svelte.ts'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import { NoteIconModal } from '@/features/note-icon'
  import NoteCanvas from './NoteCanvas.svelte'
  import './noteStatus.css'
  import NotesTree from './NotesTree.svelte'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
import type { NotePayload } from '@/shared/api/knowledge_http.gen.ts'

  let {
    notes = [],
    selectedId = null,
    labels = {},
    quests = [],
    questlines = [],
    attachments = [],
    sidebarCollapsed = false,
    onSelect,
    onChanged,
    onRef,
    onOpenSecrets,
  }: {
    notes: any[],
    selectedId: number | null,
    labels?: Record<string, string>,
    quests?: any[],
    questlines?: any[],
    attachments?: any[],
    sidebarCollapsed?: boolean,
    onSelect: (id: number | null) => void,
    onChanged: () => void,
    onRef?: (kind: string, id: number) => void,
    onOpenSecrets?: (id: number) => void,
  } = $props()

  let search = $state('')
  let treeOpen: Record<string, boolean> = $state({})
  /** Ссылки/Ссылаются — свёрнуты по умолчанию. */
  let refsOpen = $state(false)
  let backlinksOpen = $state(false)
  const ed = createNoteEditor({ selectedId: () => selectedId, notes: () => notes, onChanged: () => onChanged() })
  const save = ed.save
  const revert = ed.revert
  let deleting = $state(false)
  let deleteOpen = $state(false)
  let deleteTargetId = $state((null as number | null))
  let ctxOpen = $state(false)
  let ctxX = $state(0)
  let ctxY = $state(0)
  let ctxNoteId = $state((null as number | null))
  let parentMenuOpen = $state(false)
  let parentMenuX = $state(0)
  let parentMenuY = $state(0)
  let iconModalOpen = $state(false)
  let iconModalNoteId = $state((null as number | null))
  /** Inline rename in the tree — id of the row being renamed, or null. */
  let parsedSearch = $derived(parseQuery(search, NOTE_SPEC))
  let searchText = $derived(parsedSearch.text)
  const flagOk = (n: any) => passes(parsedSearch, NOTE_SPEC, n)

  let filtered = $derived.by(() => {
    const q = searchText.toLowerCase()
    if (!q && !parsedSearch.active) return notes
    return withAncestors(
      notes,
      notes.filter((n) => flagOk(n) && (!q || `${n.title || ''} ${n.description || ''}`.toLowerCase().includes(q))),
    )
  })

  let searchActive = $derived(search.trim().length > 0)

  /** Ranked server results for the query (null = not asked / unavailable → the tree filter above stays). */
  let searchHits = $state((null as any[] | null))
  $effect(() => {
    const q = searchText
    if (q.length < 2) {
      searchHits = null
      return
    }
    let stale = false
    const timer = setTimeout(async () => {
      try {
        const r = await searchNotes(q, parsedSearch.active ? 50 : 30)
        if (!stale) searchHits = (r?.hits ?? []).filter((h: { id: number }) => flagOk(noteById(h.id) ?? h))
      } catch {
        if (!stale) searchHits = null
      }
    }, 220)
    return () => {
      stale = true
      clearTimeout(timer)
    }
  })

  /** Grouping key for the tree: the open note's own live `ed.parentId` field
   * (even before it's written into noteDrafts — that only happens once the
   * whole form differs from `ed.saved`), else its draft if one exists, else the
   * server's own `parent_id`. Lets picking a new parent move the row in the
   * sidebar immediately — parentMenuItems already excludes self/descendants,
   * so this can't introduce a cycle.
   *
   * Only trust `ed.parentId` once it's actually loaded for this note
   * (ed.loadedId === selectedId) — right after switching notes, selectedId has
   * already moved on but the async getNote() fetch hasn't landed yet, so
   * `ed.parentId` still holds the *previous* note's value. Using it anyway for
   * that one tick drew the row at a wrong/stale spot that then snapped to
   * the right one a moment later — the flash this was fixing. */
  function parentKeyOf(n: { id: number; parent_id: null }) {
    if (n.id === selectedId && ed.loadedId === selectedId) return ed.parentId === '' ? 'root' : ed.parentId
    const draft = noteDrafts[String(n.id)]
    if (draft) return draft.parentId === '' ? 'root' : draft.parentId
    return n.parent_id == null ? 'root' : String(n.parent_id)
  }

  let byParent = $derived.by(() => {
    const m: Map<string, any[]> = new Map()
    for (const n of filtered) {
      const key = parentKeyOf(n)
      if (!m.has(key)) m.set(key, [])
      m.get(key).push(n)
    }
    // Notes with children first, then leaves; pin only breaks ties inside a group.
    const hasKids = (n: { id: any }) => (m.get(String(n.id))?.length ?? 0) > 0
    for (const list of m.values()) list.sort((a, b) => compareNotes(a, b, hasKids))
    return m
  })

  let roots = $derived(byParent.get('root') || [])

  const view = createViewMode()
  const VIEW_MODES = VIEW_MODE_OPTIONS
  let viewMode = $derived(view.mode)
  const setViewMode = view.set

  function rowTitle(n: { id: number; title: any }) {
    if (n.id === rename.id) return rename.value
    return noteDrafts[String(n.id)]?.title || n.title
  }

  const rowTitleRaw = (n: { id: number; title: string }) => noteDrafts[String(n.id)]?.title || n.title

  const rename = createNoteRename({
    noteById: (id) => noteById(id),
    rawTitle: rowTitleRaw,
    updateNote,
    /** Переименование из дерева не должно затереть заголовок, который в открытой заметке сейчас правят руками. */
    onSaved: (id, t, savedRow) => {
      ed.patched(id, savedRow, { title: t }, true)
      onChanged()
    },
    setError: (message) => (ed.error = message),
  })
  const startRename = rename.start
  const cancelRename = rename.cancel
  const commitRename = rename.commit

  /** Title commits on blur/Enter, independent of the ed.description draft. */
  async function commitTitle() {
    if (selectedId == null || titleReadonly) return
    await ed.commitTitle()
  }

  function onTitleKeydown(event: { key: string; preventDefault: () => void }) {
    if (event.key === 'Enter') {
      event.preventDefault()
      titleEl?.blur()
    }
  }

  function rowDirty(n: { id: number }) {
    return n.id === selectedId ? ed.dirty : noteDrafts[String(n.id)] != null
  }

  async function addNote(parent: number | null = null) {
    ed.error = ''
    try {
      const created = await createNote({
        title: 'Новая заметка',
        description: '',
        parent_id: parent,
      })
      await onChanged()
      onSelect(created.id)
      toast('Заметка создана', { kind: 'success' })
    } catch (e) {
      ed.error = e.message || String(e)
      toast(ed.error, { kind: 'error' })
    }
  }

  async function confirmDelete() {
    const id = deleteTargetId ?? selectedId
    if (id == null || deleting) return
    deleting = true
    ed.error = ''
    try {
      await deleteNote(id)
      clearNoteDraft(id)
      deleteOpen = false
      deleteTargetId = null
      if (id === selectedId) onSelect(null)
      onChanged()
      toast('Заметка удалена', { kind: 'success' })
    } catch (e) {
      ed.error = e.message || String(e)
      toast(ed.error, { kind: 'error' })
    } finally {
      deleting = false
    }
  }

  function onKey(event: { ctrlKey: any; metaKey: any; code: string; key: string; preventDefault: () => void; stopPropagation: () => void }) {
    if (!(event.ctrlKey || event.metaKey)) return
    // code=KeyS — раскладконезависимо (ru: Ctrl+ы); key оставляем как запасной.
    const isSave =
      event.code === 'KeyS' || event.key.toLowerCase() === 's' || event.key === 'ы' || event.key === 'Ы'
    if (!isSave) return
    // Перехват браузерного «Сохранить страницу» — keydown + preventDefault.
    event.preventDefault()
    event.stopPropagation()
    if (ed.dirty && selectedId != null) save()
  }

  $effect(() => {
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  })

  let editEl = $state((null as HTMLTextAreaElement | null))
  let titleEl = $state((null as HTMLInputElement | null))
  let findOpen = $state(false)
  let noteIsOutdated = $derived(selectedId != null && !!(ed.detail?.id === selectedId ? ed.detail.outdated : noteById(selectedId)?.outdated))
  function onFindKey(e: { ctrlKey: any; metaKey: any; shiftKey: any; altKey: any; key: string; preventDefault: () => void }) {
    if ((e.ctrlKey || e.metaKey) && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 'f' && selectedId) {
      e.preventDefault()
      findOpen = true
    }
  }
  let previewEl = $state((null as HTMLDivElement | null))
  let pageEl = $state((null as HTMLElement | null))
  let titleEditing = $state(false)

  let titleReadonly = $derived(viewMode === 'formatted' && !titleEditing)

  $effect(() => {
    if (viewMode !== 'formatted') titleEditing = false
  })

  // Grow the editor to fit its text so Текст/Оба flow like Просмотр: no inner
  // scrollbar, the whole note visible, the page the only thing that scrolls.
  // CSS min-height still floors it for short notes.
  function autoGrow() {
    const ta = editEl
    if (!ta) return
    // Collapsing to 'auto' to measure shrinks the page when the textarea is
    // the tallest thing in it (Текст mode — in Оба the preview column holds
    // the height up), and the browser clamps the scroll position to the
    // shorter page; re-growing doesn't bring it back, so typing threw the
    // page to the top. Put the scroll back before anything paints.
    const scroller = pageEl
    const top = scroller?.scrollTop ?? 0
    const winTop = window.scrollY
    ta.style.height = 'auto'
    let h = ta.scrollHeight
    ta.style.height = `${h}px`
    // Pinning an explicit height can nudge a textarea's own scrollHeight up
    // (trailing-line rendering quirk), which would leave the last line or two
    // clipped under overflow:hidden — settle it in a couple of passes.
    for (let i = 0; i < 3 && ta.scrollHeight > h; i++) {
      h = ta.scrollHeight
      ta.style.height = `${h}px`
    }
    if (scroller && scroller.scrollTop !== top) scroller.scrollTop = top
    if (window.scrollY !== winTop) window.scrollTo({ top: winTop })
  }

  $effect(() => {
    // Re-measure whenever the text or the mode changes (mode switch remounts
    // the textarea, so editEl is a dependency too). A second pass next frame
    // catches the reflow when the mono webfont finishes loading.
    void ed.description
    void viewMode
    if (!editEl) return
    tick().then(() => {
      autoGrow()
      requestAnimationFrame(autoGrow)
    })
  })

  $effect(() => {
    // The mono webfont can land after the first measure and shift line height;
    // recompute once it's ready so nothing is left clipped.
    document.fonts?.ready?.then(autoGrow)
  })

  $effect(() => {
    const ta = editEl
    if (!ta) return
    // Width changes (window/split resize) reflow the text and change its
    // height; recompute on those, but ignore our own height writes to avoid a
    // feedback loop.
    let lastW = ta.clientWidth
    const ro = new ResizeObserver((entries) => {
      const w = entries[0].contentRect.width
      if (Math.abs(w - lastW) < 0.5) return
      lastW = w
      autoGrow()
    })
    ro.observe(ta)
    return () => ro.disconnect()
  })

  function previewMdRoot() {
    return previewEl?.querySelector?.('.md') ?? previewEl
  }

  async function startBodyEdit(event?: MouseEvent) {
    if (viewMode !== 'formatted') return
    event?.preventDefault?.()
    let offset = ed.description.length
    const mdRoot = previewMdRoot()
    if (event && mdRoot) {
      const rendered = mdRoot.textContent ?? ''
      const renOff = caretOffsetFromPoint(mdRoot, event.clientX, event.clientY)
      offset = mapRenderedOffsetToSource(ed.description, rendered, renOff)
    }
    setViewMode('raw')
    await tick()
    if (editEl) placeTextareaCaret(editEl, offset)
  }

  async function startTitleEdit() {
    if (!titleReadonly) return
    titleEditing = true
    await tick()
    titleEl?.focus()
    titleEl?.select()
  }

  /** Esc в сыром/оба при фокусе в textarea → режим просмотра около каретки. */
  async function onEditKeydown(event: { key: string; preventDefault: () => void }) {
    if (event.key !== 'Escape') return
    if (viewMode === 'formatted') return
    event.preventDefault()
    const caret = editEl?.selectionStart ?? 0
    const src = ed.description
    setViewMode('formatted')
    await tick()
    await tick()
    requestAnimationFrame(() => {
      const mdRoot = previewMdRoot()
      if (!mdRoot) return
      const rendered = mdRoot.textContent ?? ''
      const renOff = mapSourceOffsetToRendered(src, rendered, caret)
      scrollRootToTextOffset(mdRoot, renOff)
    })
  }

  function childrenOf(id: any) {
    return byParent.get(String(id)) || []
  }

  function hasChildren(id: any) {
    return childrenOf(id).length > 0
  }

  function isTreeOpen(id: any) {
    return treeOpen[String(id)] === true
  }

  function toggleTree(id: any, event: MouseEvent) {
    event.stopPropagation()
    event.preventDefault()
    const key = String(id)
    treeOpen = { ...treeOpen, [key]: treeOpen[key] !== true }
  }

  function showChildren(id: any) {
    if (!hasChildren(id)) return false
    if (searchActive) return true
    return isTreeOpen(id)
  }

  function noteById(id: number) {
    return notes.find((n) => n.id === id) ?? null
  }

  let readmeBubbleIds = $derived(readmeBubbles(notes, showChildren))
  let crumbs = $derived(noteCrumbs(notes, ed.parentId))
  let parentMenuItems = $derived(parentMenu(notes, selectedId))


  function openParentPicker(event: MouseEvent) {
    event.preventDefault()
    event.stopPropagation()
    const el = (event.currentTarget as HTMLElement)
    const rect = el.getBoundingClientRect()
    parentMenuX = rect.left
    parentMenuY = rect.bottom + 4
    parentMenuOpen = true
  }

  function setParentFromMenu(action: string) {
    if (selectedId == null) return
    reparentNote(selectedId, action === 'root' ? null : Number(action))
  }

  function setParentCrumb(id: number | null) {
    if (selectedId == null) return
    reparentNote(selectedId, id)
  }

  const noteExport = createNoteExport({
    downloadMarkdown: downloadNoteMarkdown,
    downloadPdf: (payload) => downloadNotePdf(payload, { labels }),
    setError: (message) => (ed.error = message),
  })
  let exportBusy = $derived(noteExport.busy)
  const openExportMenu = noteExport.openMenu
  const exportMenuItems = $derived(noteExport.items)

  /** The note as it should be exported: what is on screen for the open one, what the list knows for the others. */
  function exportNoteById(id: number, action: string) {
    if (id == null) return
    const n = noteById(id)
    const payload =
      id === selectedId
        ? { id, title: ed.title.trim() || ed.detail?.title || n?.title || `note=${id}`, description: ed.description }
        : { id, title: n?.title || `note=${id}`, description: n?.description || '' }
    return noteExport.run(payload, action)
  }

  function onExportSelect(action: string) {
    exportNoteById(selectedId, action)
  }

  let ctxNote = $derived(ctxNoteId == null ? null : noteById(ctxNoteId))

  let ctxItems = $derived(
    noteMenuItems({
      noteId: ctxNoteId,
      note: ctxNote,
      searchActive,
      exportBusy,
      moveTargets: moveTargetItems(ctxNote, notes, hasChildren),
    }),
  )

  function openNoteMenu(event: { preventDefault: () => void; stopPropagation: () => void; clientX: number; clientY: number }, note: { id: number }) {
    if (!note?.id) return
    event.preventDefault()
    event.stopPropagation()
    ctxNoteId = note.id
    ctxX = event.clientX
    ctxY = event.clientY
    ctxOpen = true
  }

  function closeNoteMenu() {
    ctxOpen = false
  }

  async function copyNoteRef(id: number) {
    if (id == null) return
    try {
      const text = `note=${id}`
      await copyText(text)
      toast(`Скопировано ${text}`, { kind: 'success', ttl: 1600 })
    } catch (e) {
      ed.error = e?.message || String(e)
      toast(ed.error, { kind: 'error' })
    }
  }

  const actions = createNoteActions({
    notes: () => notes,
    selectedId: () => selectedId,
    editor: ed,
    onChanged: () => onChanged(),
    api: { updateNote, setNoteOutdated, getNote },
  })
  const reparentNote = actions.reparent
  const togglePinNote = actions.togglePin
  const toggleOutdatedNote = actions.toggleOutdated
  const toggleReadmeNote = (id: number) => actions.toggleFlag(id, 'is_readme')
  const togglePrivateNote = (id: number) => actions.toggleFlag(id, 'is_private')
  const toggleCategoryNote = (id: number) => actions.toggleFlag(id, 'is_category')
  const moveNoteUp = actions.moveUp

  const dnd = createTreeDnd({
    notes: () => notes,
    renaming: () => rename.id != null,
    move: (id, newParentId) => reparentNote(id, newParentId),
  })

  function requestDelete(id: number) {
    if (id == null) return
    deleteTargetId = id
    deleteOpen = true
  }

  async function onCtxSelect(action: string) {
    const id = ctxNoteId
    if (action === 'copy-id') {
      await copyNoteRef(id)
      return
    }
    if (action === 'rename') {
      startRename(id)
      return
    }
    if (action === 'pin') {
      await togglePinNote(id)
      return
    }
    if (action === 'readme') {
      await toggleReadmeNote(id)
      return
    }
    if (action === 'private') {
      await togglePrivateNote(id)
      return
    }
    if (action === 'category') {
      await toggleCategoryNote(id)
      return
    }
    if (action === 'outdated') {
      await toggleOutdatedNote(id)
      return
    }
    if (action === 'add-child') {
      await addNote(id)
      return
    }
    if (action === 'move-up') {
      await moveNoteUp(id)
      return
    }
    if (action.startsWith('move:')) {
      const raw = action.slice('move:'.length)
      await reparentNote(id, raw === 'root' ? null : Number(raw))
      return
    }
    if (action === 'export-md') {
      await exportNoteById(id, 'md')
      return
    }
    if (action === 'export-pdf') {
      await exportNoteById(id, 'pdf')
      return
    }
    if (action === 'icon') {
      iconModalNoteId = id
      iconModalOpen = true
      return
    }
    if (action === 'delete') {
      requestDelete(id)
    }
  }

  let iconModalNote = $derived(
    iconModalNoteId == null ? null : noteById(iconModalNoteId),
  )

  function onIconSaved(row: NotePayload) {
    if (selectedId === row?.id) {
      ed.detail = row
    }
    onChanged()
  }

  let deleteTargetTitle = $derived(
    deleteTargetId == null ? '' : rowTitle(noteById(deleteTargetId) || { id: deleteTargetId, title: '' }),
  )
</script>

<svelte:window onkeydown={onFindKey} />

<div class="notes" class:notes--sidebar-collapsed={sidebarCollapsed}>
  <NotesTree
    bind:search
    {parsedSearch}
    {searchActive}
    {searchHits}
    {notes}
    {roots}
    {selectedId}
    dragOverKey={dnd.dragOverKey}
    renamingId={rename.id}
    bind:renamingValue={rename.value}
    bind:renameInputEl={rename.input}
    {readmeBubbleIds}
    {labels}
    {onRef}
    {onSelect}
    onAdd={() => addNote(null)}
    onMenu={openNoteMenu}
    {hasChildren}
    {showChildren}
    {childrenOf}
    onToggleTree={toggleTree}
    {rowTitle}
    {rowDirty}
    onRenameKeydown={rename.onKeydown}
    onCommitRename={commitRename}
    onListDragOver={dnd.onListDragOver}
    onListDragLeave={dnd.onListDragLeave}
    onListDrop={dnd.onListDrop}
    onRowDragStart={dnd.onRowDragStart}
    onRowDragEnd={dnd.onRowDragEnd}
    onRowDragOver={dnd.onRowDragOver}
    onRowDragLeave={dnd.onRowDragLeave}
    onRowDrop={dnd.onRowDrop}
  />

  <section class="notes__page" bind:this={pageEl}>
    <NoteCanvas
      {ed}
      {selectedId}
      bind:titleEl
      bind:editEl
      bind:previewEl
      bind:findOpen
      bind:refsOpen
      bind:backlinksOpen
      {titleReadonly}
      {noteIsOutdated}
      {exportBusy}
      {crumbs}
      {labels}
      {onRef}
      {viewMode}
      viewModes={VIEW_MODES}
      {quests}
      {questlines}
      {notes}
      {attachments}
      onOpenNoteMenu={openNoteMenu}
      onTitleDblClick={startTitleEdit}
      onTitleBlur={commitTitle}
      {onTitleKeydown}
      onTogglePin={() => selectedId != null && togglePinNote(selectedId)}
      onToggleReadme={() => selectedId != null && toggleReadmeNote(selectedId)}
      onTogglePrivate={() => selectedId != null && togglePrivateNote(selectedId)}
      onToggleCategory={() => selectedId != null && toggleCategoryNote(selectedId)}
      onToggleOutdated={() => selectedId != null && toggleOutdatedNote(selectedId)}
      onRevert={revert}
      onSave={save}
      onExport={openExportMenu}
      onDelete={() => selectedId != null && requestDelete(selectedId)}
      onCrumb={setParentCrumb}
      onParentPicker={openParentPicker}
      onViewMode={setViewMode}
      {onEditKeydown}
      onStartBodyEdit={startBodyEdit}
      {onChanged}
      {onOpenSecrets}
    />
  </section>
  <NoteOutline scroller={pageEl} content={previewEl} enabled={viewMode === 'formatted'} dep={`${selectedId}:${ed.description}`} />
</div>

<ContextMenu
  open={ctxOpen}
  x={ctxX}
  y={ctxY}
  items={ctxItems}
  onSelect={onCtxSelect}
  onClose={closeNoteMenu}
/>

<ContextMenu
  open={parentMenuOpen}
  x={parentMenuX}
  y={parentMenuY}
  items={parentMenuItems}
  onSelect={setParentFromMenu}
  onClose={() => (parentMenuOpen = false)}
/>

<ContextMenu
  open={noteExport.menuOpen}
  x={noteExport.menuX}
  y={noteExport.menuY}
  items={exportMenuItems}
  onSelect={onExportSelect}
  onClose={() => (noteExport.menuOpen = false)}
/>

<NoteIconModal
  open={iconModalOpen}
  note={iconModalNote}
  onClose={() => {
    iconModalOpen = false
    iconModalNoteId = null
  }}
  onSaved={onIconSaved}
/>

<ConfirmModal
  open={deleteOpen}
  title="Удалить заметку?"
  message={deleteTargetTitle
    ? `Удалить «${deleteTargetTitle}»? Дочерние поднимутся в корень. Ссылки note=N в других текстах останутся.`
    : 'Дочерние поднимутся в корень. Ссылки note=N в других текстах останутся.'}
  busy={deleting}
  onCancel={() => {
    if (!deleting) {
      deleteOpen = false
      deleteTargetId = null
    }
  }}
  onConfirm={confirmDelete}
/>

<style>
  .notes {
    position: relative;
    flex: 1 1 auto;
    display: grid;
    grid-template-columns: minmax(14rem, var(--sidebar-width, 16.5rem)) 1fr;
    min-height: 0;
    overflow: hidden;
  }

  .notes--sidebar-collapsed {
    grid-template-columns: 0 1fr;
  }

  .notes--sidebar-collapsed :global(.notes__tree) {
    border-right: 0;
  }

  .notes__page {
    min-height: 0;
    overflow: auto;
    padding: var(--space-5, 1.5rem) var(--space-6, 2rem) 2rem;
    display: flex;
    flex-direction: column;
    gap: var(--space-4, 1rem);
  }

  @media (max-width: 720px) {
    .notes {
      grid-template-columns: minmax(7rem, 30%) 1fr;
    }
    .notes__page {
      padding: 1rem 1rem 2rem;
    }
  }

  @media (max-width: 480px) {
    /* Too narrow for a side-by-side tree + note — go one pane at a time,
       full screen, toggled by the same sidebar-collapse state/button.
       selectNoteFromUi (App.svelte) flips it on pick/back. */
    .notes {
      grid-template-columns: 1fr;
    }

    .notes .notes__page {
      display: none;
    }

    .notes--sidebar-collapsed :global(.notes__tree) {
      display: none;
    }

    .notes--sidebar-collapsed .notes__page {
      display: flex;
    }
  }
</style>
