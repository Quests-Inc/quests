<script lang="ts">
  import { createNoteScroll, revealOpenRow } from '../model/noteScroll.ts'
  import { createTitleEdit } from '../model/noteTitleEdit.svelte.ts'
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
  import { createNoteGroup } from '../model/noteGroup.svelte.ts'
  import { createNoteEditor } from '../model/noteEditor.svelte.ts'
  import { createNoteRename } from '../model/noteRename.svelte.ts'
  import { createViewMode, VIEW_MODE_OPTIONS } from '../model/viewMode.svelte.ts'
  import { createNoteExport } from '../model/noteExport.svelte.ts'
  import { createTreeDnd } from '../model/treeDnd.svelte.ts'
  import { flattenAttachmentIndex, attachmentCatalog } from '@/entities/attachment'
  import { NOTE_SPEC, moveSearchItems, moveTargetItems, noteCatalog, noteCrumbs, noteMenuItems, parentMenu, readmeBubbles, compareNotes, withAncestors, ancestorIds, clearNoteDraft, noteDrafts, downloadNoteMarkdown, downloadNotePdf, NoteOutline } from '@/entities/note'
  import { questCatalog } from '@/entities/quest'
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
  import { toastTrashed } from '@/shared/lib/trashUndo.ts'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import { NoteIconModal } from '@/features/note-icon'
  import NoteCanvas from './NoteCanvas.svelte'
  import './noteStatus.css'
  import NotesTree from './NotesTree.svelte'
  import SidebarLayout from '@/shared/ui/SidebarLayout.svelte'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
import type { NotePayload } from '@/shared/api/knowledge_http.gen.ts'

  let {
    selectedId = null,
    labels = {},
    questlines = [],
    onSelect,
    onChanged,
    onRef,
    onOpenSecrets,
    onOpenGraph,
  }: {
    selectedId: number | null,
    labels?: Record<string, string>,
    questlines?: any[],
    onSelect: (id: number | null) => void,
    onChanged: () => void,
    onRef?: (kind: string, id: number) => void,
    onOpenSecrets?: (id: number) => void,
    onOpenGraph?: (type: 'note', id: number) => void,
  } = $props()

  let notes = $derived(noteCatalog.list)
  let quests = $derived(questCatalog.list)
  let attachments = $derived(flattenAttachmentIndex(attachmentCatalog.index).filter((a) => !a.hidden))

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

  const titleEdit = createTitleEdit({
    selectedId: () => selectedId,
    title: () => ed.title,
    save: (t) => ed.commitTitle(t),
    input: () => titleEl,
  })
  $effect(() => {
    void selectedId
    titleEdit.cancel()
  })

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
      toastTrashed('note', [id], 'Заметка удалена')
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
  function onFindKey(e: { ctrlKey: any; metaKey: any; shiftKey: any; altKey: any; key: string; target?: any; preventDefault: () => void }) {
    if (e.key === 'Escape' && grp.multi.size > 0 && !(e.target instanceof Element && e.target.closest('input, textarea'))) grp.multi.clear()
    if ((e.ctrlKey || e.metaKey) && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 'f' && selectedId) {
      e.preventDefault()
      findOpen = true
    }
  }
  let previewEl = $state((null as HTMLDivElement | null))
  let pageEl = $state((null as HTMLElement | null))

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

  const scroll = createNoteScroll({ shownId: () => ed.loadedId, selectedId: () => selectedId, page: () => pageEl })
  $effect(() => scroll.sync())

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
    // A double click on a roll is two rolls, not an edit.
    if (event?.target instanceof Element && event.target.closest('a.md-script')) return
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

  /** After a reload (or a link) the open note must be visible: expand its ancestors once per selection. */
  let revealedFor: number | null = null
  $effect(() => {
    const id = selectedId
    if (id == null || revealedFor === id || !noteById(id)) return
    revealedFor = id
    treeOpen = { ...treeOpen, ...Object.fromEntries(ancestorIds(notes, id).map((a) => [String(a), true])) }
    void tick().then(() => revealOpenRow())
  })

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
      moveSearch: ctxNote ? moveSearchItems([ctxNote.id], notes) : [],
    }),
  )

  function openNoteMenu(event: { preventDefault: () => void; stopPropagation: () => void; clientX: number; clientY: number }, note: { id: number }) {
    if (!note?.id) return
    event.preventDefault()
    event.stopPropagation()
    if (grp.openMenu(event, note.id)) return
    grp.multi.clear()
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

  const grp = createNoteGroup({
    notes: () => notes,
    selectedId: () => selectedId,
    isDirty: (id) => (id === selectedId && ed.dirty) || !!noteDrafts[String(id)],
    actions,
    deleteNote,
    afterDelete: (ids) => {
      ids.forEach(clearNoteDraft)
      if (selectedId != null && ids.includes(selectedId)) onSelect(null)
    },
    onChanged: () => onChanged(),
    copy: copyText,
    downloadMarkdown: downloadNoteMarkdown,
    toast: (message, kind, action) => toast(message, { kind, action }),
  })

  /** A copy next to the original: same text and colour, title marked, opened right away. */
  async function duplicateNote(id: number) {
    const row = noteById(id)
    if (!row) return
    try {
      const created = await createNote({
        title: `${row.title || `note=${id}`} (копия)`,
        description: row.description ?? '',
        parent_id: row.parent_id ?? null,
        color: row.color,
      })
      await onChanged()
      onSelect(created.id)
      toast('Заметка продублирована', { kind: 'success' })
    } catch (e) {
      toast(e.message || String(e), { kind: 'error' })
    }
  }

  function requestDelete(id: number) {
    if (id == null) return
    deleteTargetId = id
    deleteOpen = true
  }

  const ctxRun: Record<string, (id: number) => unknown> = {
    'copy-id': copyNoteRef,
    rename: startRename,
    pin: togglePinNote,
    readme: toggleReadmeNote,
    private: togglePrivateNote,
    category: toggleCategoryNote,
    outdated: toggleOutdatedNote,
    'add-child': addNote,
    duplicate: duplicateNote,
    graph: (id) => onOpenGraph?.('note', id),
    'move-up': moveNoteUp,
    'export-md': (id) => exportNoteById(id, 'md'),
    'export-pdf': (id) => exportNoteById(id, 'pdf'),
    icon: (id) => ((iconModalNoteId = id), (iconModalOpen = true)),
    delete: requestDelete,
  }

  async function onCtxSelect(action: string) {
    const id = ctxNoteId
    if (id == null) return
    if (action.startsWith('move:')) {
      const raw = action.slice('move:'.length)
      await reparentNote(id, raw === 'root' ? null : Number(raw))
      return
    }
    await ctxRun[action]?.(id)
  }

  let iconModalNote = $derived(iconModalNoteId == null ? null : noteById(iconModalNoteId))

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

<SidebarLayout key="notes" class="notes">
  {#snippet sidebar()}
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
    pickedIds={grp.multi.picked}
    onRowClick={(e, id, order) => (grp.remember(order), grp.multi.click(e, id, order))}
    onClearPicked={() => grp.multi.clear()}
    onSelectAll={(ids) => grp.multi.selectAll(ids)}
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
  {/snippet}

  <section class="notes__page" bind:this={pageEl} onscroll={scroll.onScroll}>
    <NoteCanvas
      {ed}
      {selectedId}
      bind:titleEl
      bind:editEl
      bind:previewEl
      bind:findOpen
      bind:refsOpen
      bind:backlinksOpen
      titleReadonly={!titleEdit.editing}
      titleEditing={titleEdit.editing}
      bind:titleDraft={() => titleEdit.draft, (v) => (titleEdit.draft = v)}
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
      onTitleDblClick={titleEdit.start}
      onTitleBlur={titleEdit.commit}
      onTitleKeydown={titleEdit.onKeydown}
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
</SidebarLayout>

<ContextMenu
  open={ctxOpen}
  x={ctxX}
  y={ctxY}
  items={ctxItems}
  onSelect={onCtxSelect}
  onClose={closeNoteMenu}
/>

<ContextMenu open={grp.open} x={grp.x} y={grp.y} items={grp.items} onSelect={grp.select} onClose={grp.close} />

<ConfirmModal
  open={!!grp.confirm}
  title={grp.confirm?.title}
  message={grp.confirm?.message}
  confirmLabel={grp.confirm?.label}
  busy={grp.busy}
  onCancel={grp.cancelConfirm}
  onConfirm={grp.doConfirm}
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
  .notes__page {
    min-height: 0;
    overflow: auto;
    padding: var(--space-5, 1.5rem) var(--space-6, 2rem) 2rem;
    display: flex;
    flex-direction: column;
    gap: var(--space-4, 1rem);
  }

  @media (max-width: 720px) {
    .notes__page {
      padding: 1rem 1rem 2rem;
    }
  }
</style>
