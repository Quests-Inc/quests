<script lang="ts">
  import { untrack, tick } from 'svelte'
  import {
    createNote,
    deleteNote,
    getNote,
    updateNote,
    searchNotes,
    setNoteOutdated,
  } from '../js/api.ts'
  import { snippetHtml, WHERE_LABEL } from '../js/searchSnippet.ts'
  import NoteCheck from '../ui/NoteCheck.svelte'
  import NoteFind from '../ui/NoteFind.svelte'
  import { parseQuery, passes, helpLine } from '../js/searchFlags.ts'
  import FlagHints from '../ui/FlagHints.svelte'
  import { clearNoteDraft, getNoteDraft, noteDrafts, putNoteDraft, type NoteDraft } from '../js/noteDrafts.svelte.ts'
  type NoteForm = NoteDraft & { isReadme?: boolean; isPrivate?: boolean; isCategory?: boolean }
  import { copyText } from '../js/clipboard.ts'
  import {
    caretOffsetFromPoint,
    mapRenderedOffsetToSource,
    mapSourceOffsetToRendered,
    placeTextareaCaret,
    scrollRootToTextOffset,
  } from '../js/mdCaretMap.ts'
  import { downloadNoteMarkdown, downloadNotePdf } from '../js/noteExport.ts'
  import { toast, toastDone, toastProgress } from '../js/toasts.svelte.ts'
  import Icon from '../ui/Icon.svelte'
  import MarkdownBody from '../ui/MarkdownBody.svelte'
  import NoteOutline from '../ui/NoteOutline.svelte'
  import RefText from '../ui/RefText.svelte'
  import MentionTextarea from '../ui/MentionTextarea.svelte'
  import AttachmentsBlock from './AttachmentsBlock.svelte'
  import EntityHistory from './EntityHistory.svelte'
  import ConfirmModal from '../modals/ConfirmModal.svelte'
  import NoteIconModal from '../modals/NoteIconModal.svelte'
  import ContextMenu from '../ui/ContextMenu.svelte'
  import QuestlineIcon from '../ui/QuestlineIcon.svelte'
import type { NotePayload } from '../api/http.gen.ts'

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
  let detail = $state((null as any | null))
  let title = $state('')
  let description = $state('')
  let parentId = $state('')
  let pinned = $state(false)
  let isReadme = $state(false)
  let isPrivate = $state(false)
  let isCategory = $state(false)
  /** Private note: description stays collapsed until explicitly revealed. */
  let privateRevealed = $state(false)
  let saving = $state(false)
  /** The open note was changed from outside (agent, another tab, the bot)
   * while it has unsaved edits here — the incoming version waits for the
   * user's say instead of replacing their text. */
  let external = $state((null as ReturnType<typeof fromRow> | null))
  let flashing = $state(false)
  let deleting = $state(false)
  let deleteOpen = $state(false)
  let deleteTargetId = $state((null as number | null))
  let error = $state('')
  let saved = $state({
    title: '',
    description: '',
    parentId: '',
    pinned: false,
    isReadme: false,
    isPrivate: false,
    isCategory: false,
  })
  let viewMode: 'raw' | 'combined' | 'formatted' = $state(loadViewMode())
  let ctxOpen = $state(false)
  let ctxX = $state(0)
  let ctxY = $state(0)
  let ctxNoteId = $state((null as number | null))
  let parentMenuOpen = $state(false)
  let parentMenuX = $state(0)
  let parentMenuY = $state(0)
  let exportMenuOpen = $state(false)
  let exportMenuX = $state(0)
  let exportMenuY = $state(0)
  let exportBusy = $state(false)
  let iconModalOpen = $state(false)
  let iconModalNoteId = $state((null as number | null))
  /** Inline rename in the tree — id of the row being renamed, or null. */
  let renamingId = $state((null as number | null))
  let renamingValue = $state('')
  let renameInputEl = $state((null as HTMLInputElement | null))
  /** Drag-n-drop reparenting in the tree — only moves a note between parents,
   * never touches sort order (that stays children → pinned → sort_order → id,
   * same as byParent's own sort above). */
  let dragId = $state((null as number | null))
  /** String(id) of the row under the cursor, or 'root' for the list's empty
   * background, or null. */
  let dragOverKey = $state((null as string | null))

  let dirty = $derived(
    title !== saved.title ||
      description !== saved.description ||
      parentId !== saved.parentId ||
      pinned !== saved.pinned ||
      isReadme !== saved.isReadme ||
      isPrivate !== saved.isPrivate ||
      isCategory !== saved.isCategory,
  )

  /** Filters typed into the search box: `flag:name` (`-flag:name` = without it). */
  const NOTE_SPEC = {
    flag: {
      label: 'флаг',
      all: true,
      values: [
        { id: 'outdated', label: 'устарела', aliases: ['stale'] },
        { id: 'readme', label: 'README' },
        { id: 'private', label: 'PRIVATE' },
        { id: 'category', label: 'CATEGORY' },
        { id: 'pinned', label: 'закреплена', aliases: ['pin'] },
        { id: 'check', label: 'есть проверка актуальности' },
      ],
      test: (n: { outdated: any; is_readme: any; is_private: any; is_category: any; pinned: any; has_check: any }, v: string|number) =>
        ({ outdated: !!n.outdated, readme: !!n.is_readme, private: !!n.is_private, category: !!n.is_category, pinned: !!n.pinned, check: !!n.has_check })[v],
    },
  }
  let parsedSearch = $derived(parseQuery(search, NOTE_SPEC))
  let searchText = $derived(parsedSearch.text)
  const flagOk = (n: any) => passes(parsedSearch, NOTE_SPEC, n)

  let filtered = $derived.by(() => {
    const q = searchText.toLowerCase()
    if (!q && !parsedSearch.active) return notes
    return withAncestors(
      notes.filter((n) => flagOk(n) && (!q || `${n.title || ''} ${n.description || ''}`.toLowerCase().includes(q))),
    )
  })

  /** The given notes and the chain of their parents, in the original order. */
  function withAncestors(list: any[]) {
    const byId = new Map(notes.map((n) => [n.id, n]))
    const keep = new Set()
    for (const n of list) {
      let cur = n
      keep.add(cur.id)
      while (cur.parent_id != null) {
        const parent = byId.get(cur.parent_id)
        if (!parent) break
        keep.add(parent.id)
        cur = parent
      }
    }
    return notes.filter((n) => keep.has(n.id))
  }

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

  /** Grouping key for the tree: the open note's own live `parentId` field
   * (even before it's written into noteDrafts — that only happens once the
   * whole form differs from `saved`), else its draft if one exists, else the
   * server's own `parent_id`. Lets picking a new parent move the row in the
   * sidebar immediately — parentMenuItems already excludes self/descendants,
   * so this can't introduce a cycle.
   *
   * Only trust `parentId` once it's actually loaded for this note
   * (loadedId === selectedId) — right after switching notes, selectedId has
   * already moved on but the async getNote() fetch hasn't landed yet, so
   * `parentId` still holds the *previous* note's value. Using it anyway for
   * that one tick drew the row at a wrong/stale spot that then snapped to
   * the right one a moment later — the flash this was fixing. */
  function parentKeyOf(n: { id: number; parent_id: null }) {
    if (n.id === selectedId && loadedId === selectedId) return parentId === '' ? 'root' : parentId
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

  /** Tree order: with children → pinned → sort_order → id. */
  function compareNotes(a: any, b: any, hasKids: (n: any) => boolean) {
    return (
      Number(hasKids(b)) - Number(hasKids(a)) ||
      Number(b.pinned) - Number(a.pinned) ||
      (a.sort_order || 0) - (b.sort_order || 0) ||
      a.id - b.id
    )
  }

  let roots = $derived(byParent.get('root') || [])

  let loadedId = $state((null as number | null))

  const VIEW_MODES = ([
    { id: 'raw', label: 'Текст' },
    { id: 'combined', label: 'Оба' },
    { id: 'formatted', label: 'Просмотр' },
  ] as const)

  function loadViewMode() {
    try {
      const v = localStorage.getItem('quests.notes.viewMode')
      if (v === 'raw' || v === 'combined' || v === 'formatted') return v
    } catch {
      /* ignore */
    }
    return 'combined'
  }

  function setViewMode(mode: 'raw' | 'combined' | 'formatted') {
    viewMode = mode
    try {
      localStorage.setItem('quests.notes.viewMode', mode)
    } catch {
      /* ignore */
    }
  }

  // "Оба" needs a side-by-side split that doesn't fit the full-screen single
  // pane below 480px — drop out of it there (the option itself is hidden by
  // CSS too) so a note never opens into a mode with no toggle back to it.
  $effect(() => {
    const mq = window.matchMedia('(max-width: 480px)')
    const apply = () => {
      if (mq.matches && viewMode === 'combined') setViewMode('formatted')
    }
    apply()
    mq.addEventListener('change', apply)
    return () => mq.removeEventListener('change', apply)
  })

  function fromRow(row: NotePayload) {
    return {
      title: row.title || '',
      description: row.description || '',
      parentId: row.parent_id != null ? String(row.parent_id) : '',
      pinned: !!row.pinned,
      isReadme: !!row.is_readme,
      isPrivate: !!row.is_private,
      isCategory: !!row.is_category,
    }
  }

  function applyForm(src: NoteForm) {
    title = src.title
    description = src.description
    parentId = src.parentId
    pinned = src.pinned
    isReadme = src.isReadme
    isPrivate = src.isPrivate
    isCategory = src.isCategory
    privateRevealed = false
  }

  function formDraft() {
    return { title, description, parentId, pinned, isReadme, isPrivate, isCategory }
  }

  function draftsEqual(a: { title: any; description: any; parentId: any; pinned: any; isReadme: any; isPrivate: any; isCategory: any }, b: { title: any; description: any; parentId: any; pinned: any; isReadme: any; isPrivate: any; isCategory: any }) {
    return (
      a.title === b.title &&
      a.description === b.description &&
      a.parentId === b.parentId &&
      a.pinned === b.pinned &&
      a.isReadme === b.isReadme &&
      a.isPrivate === b.isPrivate &&
      a.isCategory === b.isCategory
    )
  }

  function syncDraft(id: number) {
    if (id == null) return
    const form = formDraft()
    if (draftsEqual(form, saved)) clearNoteDraft(id)
    else putNoteDraft(id, form)
  }

  function revert() {
    external = null
    applyForm(saved)
    if (selectedId != null) clearNoteDraft(selectedId)
  }

  function rowTitle(n: { id: number; title: any }) {
    if (n.id === renamingId) return renamingValue
    return noteDrafts[String(n.id)]?.title || n.title
  }

  function startRename(id: number) {
    const n = noteById(id)
    if (!n) return
    renamingId = id
    renamingValue = rowTitleRaw(n)
    tick().then(() => {
      renameInputEl?.focus()
      renameInputEl?.select()
    })
  }

  function rowTitleRaw(n: { id: any; title: any }) {
    return noteDrafts[String(n.id)]?.title || n.title
  }

  function cancelRename() {
    renamingId = null
    renamingValue = ''
  }

  async function commitRename() {
    const id = renamingId
    if (id == null) return
    const t = renamingValue.trim()
    const n = noteById(id)
    cancelRename()
    if (!t || !n || t === n.title) return
    try {
      const savedRow = await updateNote(id, { title: t })
      if (id === selectedId) {
        // Only overwrite the open editor's own title field if it wasn't
        // independently mid-edit — otherwise a rename from the tree would
        // clobber text the user is still typing in the detail pane.
        const titleWasUntouched = title === saved.title
        detail = savedRow
        saved = { ...saved, title: t }
        if (titleWasUntouched) title = t
        const draft = getNoteDraft(id)
        if (draft) putNoteDraft(id, { ...draft, title: t })
      }
      onChanged()
    } catch (e) {
      error = e.message || String(e)
      toast(error, { kind: 'error' })
    }
  }

  /** Title commits on blur/Enter, independent of the description draft. */
  async function commitTitle() {
    const id = selectedId
    if (id == null || titleReadonly) return
    const t = title.trim()
    if (t === saved.title) {
      title = saved.title
      return
    }
    if (!t) {
      title = saved.title
      return
    }
    try {
      detail = await updateNote(id, { title: t })
      saved = { ...saved, title: t }
      title = t
      onChanged()
    } catch (e) {
      error = e.message || String(e)
      toast(error, { kind: 'error' })
    }
  }

  function onTitleKeydown(event: { key: string; preventDefault: () => void }) {
    if (event.key === 'Enter') {
      event.preventDefault()
      titleEl?.blur()
    }
  }

  function onRenameKeydown(event: { key: string; preventDefault: () => void; stopPropagation: () => void }) {
    if (event.key === 'Enter') {
      event.preventDefault()
      renameInputEl?.blur()
    } else if (event.key === 'Escape') {
      event.preventDefault()
      cancelRename()
    }
    event.stopPropagation()
  }

  function rowDirty(n: { id: number }) {
    return n.id === selectedId ? dirty : noteDrafts[String(n.id)] != null
  }

  $effect(() => {
    const id = selectedId
    untrack(() => {
      if (loadedId != null && loadedId !== id) syncDraft(loadedId)
    })
    if (id == null) {
      detail = null
      const empty = {
        title: '',
        description: '',
        parentId: '',
        pinned: false,
        isReadme: false,
        isPrivate: false,
        isCategory: false,
      }
      applyForm(empty)
      saved = empty
      loadedId = null
      return
    }
    let cancelled = false
    getNote(id)
      .then((row) => {
        if (cancelled) return
        detail = row
        const next = fromRow(row)
        const draft = getNoteDraft(id)
        applyForm(draft || next)
        saved = next
        loadedId = id
        error = ''
      })
      .catch((e) => {
        if (cancelled) return
        error = e.message || String(e)
      })
    return () => {
      cancelled = true
      untrack(() => syncDraft(id))
    }
  })

  $effect(() => {
    const id = selectedId
    const loaded = loadedId
    if (id == null || loaded !== id) return
    const form = { title, description, parentId, pinned, isReadme, isPrivate, isCategory }
    untrack(() => {
      if (draftsEqual(form, saved)) clearNoteDraft(id)
      else putNoteDraft(id, form)
    })
  })

  /** Brief highlight of the body when a live update lands. */
  async function flashBody() {
    flashing = false
    await tick()
    flashing = true
    setTimeout(() => (flashing = false), 1500)
  }

  /** Take the server's current version of the open note as the form. */
  async function adoptExternal(id: number) {
    try {
      const row = await getNote(id)
      if (selectedId !== id) return
      detail = row
      const next = fromRow(row)
      applyForm(next)
      saved = next
      clearNoteDraft(id)
      external = null
      void flashBody()
    } catch (e) {
      error = e.message || String(e)
    }
  }

  // Live updates: the journal reloads the notes list on every server event, so
  // the open note is compared with its row there. A newer row that differs
  // from what was loaded means someone else edited it.
  $effect(() => {
    const id = loadedId
    const row = id != null ? notes.find((n) => n.id === id) : null
    if (!row) return
    untrack(() => {
      if (saving) return
      // A list fetched before our own save landed carries an older row.
      if (detail?.updated_at && row.updated_at <= detail.updated_at) return
      const next = fromRow(row)
      if (draftsEqual(next, saved)) return
      if (draftsEqual(formDraft(), saved)) void adoptExternal(id)
      else external = next
    })
  })

  async function save() {
    if (selectedId == null || saving) return
    const t = title.trim()
    if (!t) {
      error = 'Нужен заголовок'
      toast(error, { kind: 'error' })
      return
    }
    saving = true
    error = ''
    try {
      const savedRow = await updateNote(selectedId, {
        title: t,
        description,
        pinned,
        is_readme: isReadme,
        is_private: isPrivate,
        is_category: isCategory,
        parent_id: parentId === '' ? null : Number(parentId),
      })
      detail = savedRow
      saved = fromRow(savedRow)
      applyForm(saved)
      clearNoteDraft(selectedId)
      external = null
      onChanged()
      toast('Сохранено', { kind: 'success', ttl: 1400 })
    } catch (e) {
      error = e.message || String(e)
      toast(error, { kind: 'error' })
    } finally {
      saving = false
    }
  }

  async function addNote(parent: number | null = null) {
    error = ''
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
      error = e.message || String(e)
      toast(error, { kind: 'error' })
    }
  }

  async function confirmDelete() {
    const id = deleteTargetId ?? selectedId
    if (id == null || deleting) return
    deleting = true
    error = ''
    try {
      await deleteNote(id)
      clearNoteDraft(id)
      deleteOpen = false
      deleteTargetId = null
      if (id === selectedId) onSelect(null)
      onChanged()
      toast('Заметка удалена', { kind: 'success' })
    } catch (e) {
      error = e.message || String(e)
      toast(error, { kind: 'error' })
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
    if (dirty && selectedId != null) save()
  }

  $effect(() => {
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  })

  let editEl = $state((null as HTMLTextAreaElement | null))
  let titleEl = $state((null as HTMLInputElement | null))
  let findOpen = $state(false)
  let noteIsOutdated = $derived(selectedId != null && !!(detail?.id === selectedId ? detail.outdated : noteById(selectedId)?.outdated))
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
    void description
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
    let offset = description.length
    const mdRoot = previewMdRoot()
    if (event && mdRoot) {
      const rendered = mdRoot.textContent ?? ''
      const renOff = caretOffsetFromPoint(mdRoot, event.clientX, event.clientY)
      offset = mapRenderedOffsetToSource(description, rendered, renOff)
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
    const src = description
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

  /** candidate is under ancestor (cannot become its parent). */
  function isUnder(candidateId: number, ancestorId: number) {
    if (candidateId == null || ancestorId == null) return false
    let cur = noteById(candidateId)
    const seen = new Set()
    while (cur?.parent_id != null && !seen.has(cur.id)) {
      seen.add(cur.id)
      if (cur.parent_id === ancestorId) return true
      cur = noteById(cur.parent_id)
    }
    return false
  }

  /** A note is visible in the sidebar when every ancestor up to the root is
   * expanded (or it has no parent). */
  function isNoteVisible(id: any) {
    const n = noteById(id)
    if (!n) return false
    if (n.parent_id == null) return true
    if (!isNoteVisible(n.parent_id)) return false
    return showChildren(n.parent_id)
  }

  /** Walking up from a hidden note, the closest ancestor that is itself
   * visible — the row where a "there's a README below" hint should land. */
  function nearestVisibleAncestor(id: any) {
    const n = noteById(id)
    let cur = n?.parent_id != null ? noteById(n.parent_id) : null
    while (cur) {
      if (isNoteVisible(cur.id)) return cur.id
      cur = cur.parent_id != null ? noteById(cur.parent_id) : null
    }
    return null
  }

  /** Notes that aren't README themselves but have a README descendant
   * currently hidden behind a collapsed ancestor — bubbled up to the
   * deepest visible ancestor so a collapsed README stays discoverable.
   * Rendered in yellow (not red) so it isn't mistaken for the note itself
   * being the README. */
  let readmeBubbleIds = $derived.by(() => {
    const out = new Set()
    for (const n of notes) {
      if (!n.is_readme || isNoteVisible(n.id)) continue
      const anc = nearestVisibleAncestor(n.id)
      if (anc != null) out.add(anc)
    }
    return out
  })

  let crumbs = $derived.by(() => {
    const out: { id: number | null, title: string }[] = [{ id: null, title: 'Корень' }]
    const pid = parentId === '' ? null : Number(parentId)
    if (pid == null || Number.isNaN(pid)) return out
    const chain: { id: number, title: string }[] = []
    let cur = noteById(pid)
    const seen = new Set()
    while (cur && !seen.has(cur.id)) {
      seen.add(cur.id)
      chain.unshift({ id: cur.id, title: cur.title || `note=${cur.id}` })
      cur = cur.parent_id != null ? noteById(cur.parent_id) : null
    }
    return out.concat(chain)
  })

  let parentMenuItems = $derived.by(() => {
    const items: { id: string, label: string }[] = [{ id: 'root', label: '— корень —' }]
    if (selectedId == null) return items
    for (const n of notes) {
      if (n.id === selectedId) continue
      if (isUnder(n.id, selectedId)) continue
      items.push({ id: String(n.id), label: n.title || `note=${n.id}` })
    }
    return items
  })

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

  function openExportMenu(event: MouseEvent) {
    event.preventDefault()
    event.stopPropagation()
    const el = (event.currentTarget as HTMLElement)
    const rect = el.getBoundingClientRect()
    exportMenuX = rect.left
    exportMenuY = rect.bottom + 4
    exportMenuOpen = true
  }

  async function exportNoteById(id: number, action: string) {
    if (id == null || exportBusy) return
    const n = noteById(id)
    const payload =
      id === selectedId
        ? {
            id,
            title: title.trim() || detail?.title || n?.title || `note=${id}`,
            description,
          }
        : {
            id,
            title: n?.title || `note=${id}`,
            description: n?.description || '',
          }
    exportBusy = true
    error = ''
    try {
      if (action === 'md') {
        downloadNoteMarkdown(payload)
        toast('Markdown сохранён', { kind: 'success' })
      } else if (action === 'pdf') {
        const tid = 'pdf-note'
        toastProgress(tid, 'Генерация PDF…')
        try {
          await downloadNotePdf(payload, { labels })
          toastDone(tid, 'PDF сохранён')
        } catch (e) {
          toastDone(tid, e?.message || 'Не удалось сохранить PDF', 'error')
          throw e
        }
      }
    } catch (e) {
      if (action !== 'pdf') {
        error = e?.message || String(e)
        toast(error, { kind: 'error' })
      } else {
        error = e?.message || String(e)
      }
    } finally {
      exportBusy = false
    }
  }

  function onExportSelect(action: any) {
    exportNoteById(selectedId, action)
  }

  let exportMenuItems = $derived([
    { id: 'md', label: 'Markdown (.md)' },
    { id: 'pdf', label: exportBusy ? 'PDF…' : 'PDF' },
  ])

  let ctxNote = $derived(ctxNoteId == null ? null : noteById(ctxNoteId))

  function moveTargets(note: { parent_id: null; id: null }) {
    const items: { id: string, label?: string, sep?: boolean, checked?: boolean }[] = []
    if (note?.parent_id != null) {
      items.push({ id: 'move-up', label: 'На уровень вверх' })
    }
    items.push({
      id: 'move:root',
      label: 'Корень',
      checked: note?.parent_id == null,
    })
    // Only flat neighbours: the parent's level (parent + its siblings — the note
    // keeps its depth) and the note's own level (siblings — it nests one deeper).
    // Own descendants are excluded, so there is never an ambiguous re-parenting.
    const pid = note?.parent_id ?? null
    const parent = pid != null ? noteById(pid) : null
    const hasKids = (n: { id: any }) => hasChildren(n.id)
    const level = (parentOf: any) =>
      notes
        .filter(
          (n) =>
            (n.parent_id ?? null) === parentOf &&
            n.id !== note?.id &&
            !(note?.id != null && isUnder(n.id, note.id)),
        )
        .sort((a, b) => compareNotes(a, b, hasKids))
    const groups = [
      ...(parent ? [level(parent.parent_id ?? null)] : []),
      level(pid),
    ]
    let listed = false
    for (const group of groups) {
      if (!group.length) continue
      if (!listed) {
        items.push({ id: 'sep-move', sep: true })
        listed = true
      } else {
        items.push({ id: `sep-move-${items.length}`, sep: true })
      }
      for (const n of group) {
        items.push({
          id: `move:${n.id}`,
          label: n.title || `note=${n.id}`,
          checked: note?.parent_id === n.id,
        })
      }
    }
    return items
  }

  let ctxItems = $derived.by(() => {
    if (ctxNoteId == null) return []
    return [
      { id: 'copy-id', label: `Копировать note=${ctxNoteId}` },
      { id: 'sep-copy', sep: true },
      ...(searchActive ? [] : [{ id: 'rename', label: 'Переименовать' }]),
      {
        id: 'flags',
        label: 'Флаги',
        children: [
          { id: 'pin', label: ctxNote?.pinned ? 'Снять закреп' : 'Закрепить' },
          { id: 'readme', label: ctxNote?.is_readme ? 'Снять README' : 'Пометить README' },
          { id: 'private', label: ctxNote?.is_private ? 'Снять PRIVATE' : 'Пометить PRIVATE' },
          { id: 'category', label: ctxNote?.is_category ? 'Снять CATEGORY' : 'Пометить CATEGORY' },
          { id: 'outdated', label: ctxNote?.outdated ? 'Снять «устарела»' : 'Пометить «устарела»' },
        ],
      },
      { id: 'add-child', label: 'Добавить дочернюю' },
      { id: 'move', label: 'Переместить', children: moveTargets(ctxNote) },
      {
        id: 'export',
        label: 'Экспорт',
        children: [
          { id: 'export-md', label: 'Markdown (.md)' },
          { id: 'export-pdf', label: exportBusy ? 'PDF…' : 'PDF' },
        ],
      },
      { id: 'sep-icon', sep: true },
      { id: 'icon', label: 'Иконка' },
      { id: 'sep-danger', sep: true },
      { id: 'delete', label: 'Удалить', danger: true },
    ]
  })

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
      error = e?.message || String(e)
      toast(error, { kind: 'error' })
    }
  }

  async function reparentNote(id: number, nextParent: number) {
    const n = noteById(id)
    if (!n) return
    const current = n.parent_id ?? null
    if (current === nextParent) return
    if (nextParent != null && (nextParent === id || isUnder(nextParent, id))) return
    error = ''
    try {
      const savedRow = await updateNote(id, { parent_id: nextParent })
      const pid = nextParent == null ? '' : String(nextParent)
      if (id === selectedId) {
        saved = { ...saved, parentId: pid }
        parentId = pid
        detail = savedRow
        const draft = getNoteDraft(id)
        if (draft) putNoteDraft(id, { ...draft, parentId: pid })
      }
      onChanged()
    } catch (e) {
      error = e.message || String(e)
    }
  }

  async function togglePinNote(id: number) {
    const n = noteById(id)
    if (!n) return
    const next = !n.pinned
    error = ''
    try {
      const savedRow = await updateNote(id, { pinned: next })
      if (id === selectedId) {
        saved = { ...saved, pinned: next }
        pinned = next
        detail = savedRow
        const draft = getNoteDraft(id)
        if (draft) putNoteDraft(id, { ...draft, pinned: next })
      }
      onChanged()
    } catch (e) {
      error = e.message || String(e)
    }
  }

  /** Shared body for the three status toggles below — same immediate-save
   * shape as togglePinNote, just parameterised over which field flips. */
  async function toggleNoteFlag(id: number, apiField: string, stateVar: string, setState: { (v: boolean): boolean; (v: boolean): boolean; (v: boolean): boolean; (arg0: boolean): void }) {
    const n = noteById(id)
    if (!n) return
    const current = apiField === 'is_readme' ? n.is_readme : apiField === 'is_private' ? n.is_private : n.is_category
    const next = !current
    error = ''
    try {
      const savedRow = await updateNote(id, { [apiField]: next })
      if (id === selectedId) {
        saved = { ...saved, [stateVar]: next }
        setState(next)
        detail = savedRow
        const draft = getNoteDraft(id)
        if (draft) putNoteDraft(id, { ...draft, [stateVar]: next })
      }
      onChanged()
    } catch (e) {
      error = e.message || String(e)
    }
  }

  async function toggleOutdatedNote(id: number) {
    const n = noteById(id)
    if (!n) return
    error = ''
    try {
      await setNoteOutdated(id, !n.outdated)
      if (id === selectedId) detail = await getNote(id)
      onChanged()
    } catch (e) {
      error = e.message || String(e)
    }
  }

  function toggleReadmeNote(id: number) {
    return toggleNoteFlag(id, 'is_readme', 'isReadme', (v: boolean) => (isReadme = v))
  }

  function togglePrivateNote(id: number) {
    return toggleNoteFlag(id, 'is_private', 'isPrivate', (v: boolean) => (isPrivate = v))
  }

  function toggleCategoryNote(id: number) {
    return toggleNoteFlag(id, 'is_category', 'isCategory', (v: boolean) => (isCategory = v))
  }

  async function moveNoteUp(id: number) {
    const n = noteById(id)
    if (!n?.parent_id) return
    const parent = noteById(n.parent_id)
    await reparentNote(id, parent?.parent_id ?? null)
  }

  /** @param targetId — null means "make it a root note" */
  function canDropOn(targetId: number | null, draggedId = dragId) {
    if (draggedId == null) return false
    if (targetId === draggedId) return false
    // target is a descendant of the dragged note — moving there would cycle it.
    if (targetId != null && isUnder(targetId, draggedId)) return false
    return true
  }

  /** Reparent from a tree drag — saved immediately, like the breadcrumb picker. */
  function moveNoteTo(id: number, newParentId: number | null) {
    return reparentNote(id, newParentId)
  }

  function onRowDragStart(event: { preventDefault: () => void; dataTransfer: { effectAllowed: string; setData: (arg0: string,arg1: string) => void } }, id: number) {
    if (renamingId != null) {
      event.preventDefault()
      return
    }
    dragId = id
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', String(id))
  }

  function onRowDragEnd() {
    dragId = null
    dragOverKey = null
  }

  function onRowDragOver(event: { preventDefault: () => void; stopPropagation: () => void; dataTransfer: { dropEffect: string } }, id: number) {
    if (!canDropOn(id)) return
    event.preventDefault()
    event.stopPropagation()
    event.dataTransfer.dropEffect = 'move'
    dragOverKey = String(id)
  }

  function onRowDragLeave(event: any, id: any) {
    if (dragOverKey === String(id)) dragOverKey = null
  }

  function onRowDrop(event: { preventDefault: () => void; stopPropagation: () => void }, id: number) {
    event.preventDefault()
    event.stopPropagation()
    const draggedId = dragId
    dragId = null
    dragOverKey = null
    if (draggedId == null || !canDropOn(id, draggedId)) return
    moveNoteTo(draggedId, id)
  }

  function onListDragOver(event: { preventDefault: () => void; dataTransfer: { dropEffect: string } }) {
    if (!canDropOn(null)) return
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
    dragOverKey = 'root'
  }

  function onListDragLeave(event: any) {
    if (dragOverKey === 'root') dragOverKey = null
  }

  function onListDrop(event: { preventDefault: () => void }) {
    event.preventDefault()
    const draggedId = dragId
    dragId = null
    dragOverKey = null
    if (draggedId == null || !canDropOn(null, draggedId)) return
    moveNoteTo(draggedId, null)
  }

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

  function onIconSaved(row: { id: number }) {
    if (selectedId === row?.id) {
      detail = row
    }
    onChanged()
  }

  let deleteTargetTitle = $derived(
    deleteTargetId == null ? '' : rowTitle(noteById(deleteTargetId) || { id: deleteTargetId, title: '' }),
  )
</script>

<svelte:window onkeydown={onFindKey} />

<div class="notes" class:notes--sidebar-collapsed={sidebarCollapsed}>
  <aside class="notes__tree">
    <div class="notes__tools">
      <input
        class="search"
        type="search"
        placeholder="Поиск…"
        title={`Фильтры: ${helpLine(NOTE_SPEC)}; -flag:… — без флага`}
        bind:value={search}
      />
      <button
        type="button"
        class="btn btn--accent notes__add"
        aria-label="Новая заметка"
        onclick={() => addNote(null)}
      >
        +
      </button>
    </div>
    <div class="notes__flaghints"><FlagHints value={search} spec={NOTE_SPEC} parsed={parsedSearch} onPick={(v) => (search = v)} /></div>
    <div
      class="notes__list"
      data-nav-root
      class:notes__list--dragover={dragOverKey === 'root'}
      ondragover={onListDragOver}
      ondragleave={onListDragLeave}
      ondrop={onListDrop}
    >
      {#if searchActive && searchHits}
        {#if searchHits.length === 0}
          <p class="empty">Ничего не найдено</p>
        {:else}
          <ul class="hits">
            {#each searchHits as h (h.id)}
              <li>
                <button type="button" data-nav class="hit" class:hit--on={h.id === selectedId} onclick={() => onSelect(h.id)} oncontextmenu={(e) => openNoteMenu(e, h)}>
                  <span class="hit__title">{h.title || 'Без названия'}{#if h.outdated}<span class="notes__status-dot notes__status-dot--outdated notes__hit-flag" title={h.outdated_reason || 'Устарела'}><Icon name="clock-alert" size={12} /></span>{/if}</span>
                  <span class="hit__where">{WHERE_LABEL[h.where] ?? ''}</span>
                  {#if h.snippet}<span class="hit__snippet">{@html snippetHtml(h.snippet)}</span>{/if}
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      {:else if notes.length === 0}
        <p class="empty">Пока пусто — это хранилище знания, не список дел.</p>
      {:else if roots.length === 0 && search}
        <p class="empty">Ничего не найдено</p>
      {:else}
        {#snippet tree(nodes: any[], depth: number)}
          {#each nodes as n (n.id)}
            <div class="notes__node" data-nav-node style:--notes-depth="{depth}">
              {#if hasChildren(n.id)}
                <button
                  type="button"
                  class="notes__fold"
                  aria-expanded={showChildren(n.id)}
                  aria-label={showChildren(n.id) ? 'Свернуть' : 'Развернуть'}
                  onclick={(e) => toggleTree(n.id, e)}
                >
                  <Icon
                    name={showChildren(n.id) ? 'chevron-down' : 'chevron-right'}
                    size={12}
                  />
                </button>
              {:else}
                <span class="notes__fold notes__fold--spacer" aria-hidden="true"></span>
              {/if}
              <button
                type="button"
                data-nav
                class="notes__row"
                class:notes__row--on={n.id === selectedId}
                class:notes__row--pin={n.pinned}
                class:notes__row--dragover={dragOverKey === String(n.id)}
                draggable={n.id !== renamingId}
                onclick={() => onSelect(n.id)}
                oncontextmenu={(e) => openNoteMenu(e, n)}
                ondragstart={(e) => onRowDragStart(e, n.id)}
                ondragend={onRowDragEnd}
                ondragover={(e) => onRowDragOver(e, n.id)}
                ondragleave={(e) => onRowDragLeave(e, n.id)}
                ondrop={(e) => onRowDrop(e, n.id)}
              >
                <span class="notes__row-icon" style="--line-color: {n.color || '#9a9a9a'}">
                  <QuestlineIcon
                    icon={n.icon || 'document'}
                    iconUrl={n.icon_url}
                    size="sm"
                  />
                </span>
                <span class="notes__row-label">
                  {#if n.id === renamingId}
                    <input
                      bind:this={renameInputEl}
                      class="notes__row-rename"
                      type="text"
                      bind:value={renamingValue}
                      onclick={(e) => e.stopPropagation()}
                      onkeydown={onRenameKeydown}
                      onblur={commitRename}
                    />
                  {:else}
                    <RefText class="notes__row-title" source={rowTitle(n)} {labels} {onRef} />{#if rowDirty(n)}<span
                        class="notes__unsaved"
                        title="Несохранено">*</span>{/if}
                  {/if}
                </span>
                {#if n.is_readme || n.is_private || n.is_category || n.outdated || readmeBubbleIds.has(n.id)}
                  <span class="notes__row-status" aria-hidden="true">
                    {#if n.is_readme}<span class="notes__status-dot notes__status-dot--readme"><Icon name="alert" size={12} /></span>{:else if readmeBubbleIds.has(n.id)}<span class="notes__status-dot notes__status-dot--readme-bubble" title="Ниже есть README"><Icon name="alert" size={12} /></span>{/if}
                    {#if n.is_private}<span class="notes__status-dot notes__status-dot--private"><Icon name="lock" size={12} /></span>{/if}
                    {#if n.is_category}<span class="notes__status-dot notes__status-dot--category"><Icon name="folder" size={12} /></span>{/if}
                    {#if n.outdated}<span class="notes__status-dot notes__status-dot--outdated" title={n.outdated_reason || 'Устарела'}><Icon name="clock-alert" size={12} /></span>{/if}
                  </span>
                {/if}
              </button>
            </div>
            {#if showChildren(n.id)}
              {@render tree(childrenOf(n.id), depth + 1)}
            {/if}
          {/each}
        {/snippet}
        {@render tree(roots, 0)}
      {/if}
    </div>
  </aside>

  <section class="notes__page" bind:this={pageEl}>
    {#if error}
      <p class="notes__error">{error}</p>
    {/if}
    {#if selectedId == null}
      <p class="notes__empty">Выберите заметку или создайте новую</p>
    {:else}
      {#if external}
        <div class="notes__external" role="status">
          <span>Заметку изменили снаружи, а у вас есть несохранённые правки.</span>
          <button type="button" class="notes__external-btn" onclick={() => adoptExternal(selectedId)}>
            Загрузить новую версию
          </button>
          <button type="button" class="notes__external-btn" onclick={() => (external = null)}>
            Оставить мою
          </button>
        </div>
      {/if}
      <header
        class="notes__head"
        oncontextmenu={(e) => {
          const n = noteById(selectedId)
          if (n) openNoteMenu(e, n)
        }}
      >
        <div class="notes__head-row">
          <div class="notes__title-cluster">
            <span class="notes__title-grow">
              <span class="notes__title-sizer" aria-hidden="true">{title || '\u00a0'}</span>
              <input
                class="notes__title"
                class:notes__title--locked={titleReadonly}
                size="1"
                bind:this={titleEl}
                bind:value={title}
                readonly={titleReadonly}
                title={titleReadonly ? 'Двойной клик — править заголовок' : undefined}
                ondblclick={startTitleEdit}
                onblur={commitTitle}
                onkeydown={onTitleKeydown}
              />
            </span>
            {#if dirty}<span class="notes__unsaved" title="Несохранено">*</span>{/if}
          </div>
          <div class="detail__actions notes__actions">
            <button
              type="button"
              class="btn btn--icon"
              class:notes__pin--on={pinned}
              onclick={() => selectedId != null && togglePinNote(selectedId)}
              title={pinned ? 'Открепить' : 'Закрепить'}
              aria-label={pinned ? 'Открепить' : 'Закрепить'}
              aria-pressed={pinned}
            >
              <Icon name={pinned ? 'pin-filled' : 'pin'} />
            </button>
            <button
              type="button"
              class="btn btn--icon notes__status-btn notes__status-btn--readme"
              class:notes__status-btn--on={isReadme}
              onclick={() => selectedId != null && toggleReadmeNote(selectedId)}
              title={isReadme ? 'Снять README (прочитано)' : 'Пометить README (прочитать)'}
              aria-label={isReadme ? 'Снять README' : 'Пометить README'}
              aria-pressed={isReadme}
            >
              <Icon name="alert" />
            </button>
            <button
              type="button"
              class="btn btn--icon notes__status-btn notes__status-btn--private"
              class:notes__status-btn--on={isPrivate}
              onclick={() => selectedId != null && togglePrivateNote(selectedId)}
              title={isPrivate ? 'Снять PRIVATE' : 'Пометить PRIVATE (скрыть от MCP)'}
              aria-label={isPrivate ? 'Снять PRIVATE' : 'Пометить PRIVATE'}
              aria-pressed={isPrivate}
            >
              <Icon name="lock" />
            </button>
            <button
              type="button"
              class="btn btn--icon notes__status-btn notes__status-btn--category"
              class:notes__status-btn--on={isCategory}
              onclick={() => selectedId != null && toggleCategoryNote(selectedId)}
              title={isCategory ? 'Снять CATEGORY' : 'Пометить CATEGORY (папка-агрегатор)'}
              aria-label={isCategory ? 'Снять CATEGORY' : 'Пометить CATEGORY'}
              aria-pressed={isCategory}
            >
              <Icon name="folder" />
            </button>
            <button
              type="button"
              class="btn btn--icon notes__status-btn notes__status-btn--outdated"
              class:notes__status-btn--on={noteIsOutdated}
              onclick={() => selectedId != null && toggleOutdatedNote(selectedId)}
              title={noteIsOutdated ? `Снять «устарела»${detail?.outdated_reason ? ': ' + detail.outdated_reason : ''}` : 'Пометить «устарела» (содержимое расходится с действительностью)'}
              aria-label={noteIsOutdated ? 'Снять «устарела»' : 'Пометить «устарела»'}
              aria-pressed={noteIsOutdated}
            >
              <Icon name="clock-alert" />
            </button>
            {#if dirty}
              <button
                type="button"
                class="btn btn--icon"
                onclick={revert}
                title="Отменить правки"
                aria-label="Отменить правки"
              >
                <Icon name="undo" />
              </button>
            {/if}
            <button
              type="button"
              class="btn btn--icon"
              disabled={saving || !dirty}
              onclick={save}
              title={saving ? 'Сохранение…' : 'Сохранить (Ctrl+S)'}
              aria-label={saving ? 'Сохранение…' : 'Сохранить'}
            >
              {#if saving}…{:else}<Icon name="save" />{/if}
            </button>
            <button
              type="button"
              class="btn btn--icon"
              disabled={exportBusy}
              onclick={openExportMenu}
              title={exportBusy ? 'Выгрузка…' : 'Скачать'}
              aria-label={exportBusy ? 'Выгрузка…' : 'Скачать заметку'}
              aria-haspopup="menu"
            >
              {#if exportBusy}…{:else}<Icon name="file-download" />{/if}
            </button>
            <button
              type="button"
              class="btn btn--icon btn--danger"
              onclick={() => requestDelete(selectedId)}
              title="Удалить"
              aria-label="Удалить"
            >
              <Icon name="delete" />
            </button>
          </div>
        </div>
        <div class="notes__colophon">
          <nav class="notes__crumbs" aria-label="Родитель">
            {#each crumbs as c, i (c.id ?? 'root')}
              {#if i > 0}<span class="notes__crumb-sep" aria-hidden="true">/</span>{/if}
              <button
                type="button"
                class="notes__crumb"
                class:notes__crumb--here={i === crumbs.length - 1}
                onclick={(e) => {
                  if (i === crumbs.length - 1) openParentPicker(e)
                  else setParentCrumb(c.id)
                }}
                title={i === crumbs.length - 1 ? 'Сменить родителя' : `Вложить в «${c.title}»`}
              >
                <RefText source={c.title} {labels} {onRef} />
              </button>
            {/each}
          </nav>
          <div class="notes__modes" role="radiogroup" aria-label="Режим просмотра">
            <button type="button" class="notes__mode" class:notes__mode--on={findOpen} aria-pressed={findOpen} title="Поиск в заметке (Ctrl+F)" onclick={() => (findOpen = !findOpen)}>Найти</button>
            {#each VIEW_MODES as mode (mode.id)}
              <button
                type="button"
                class="notes__mode"
                class:notes__mode--on={viewMode === mode.id}
                data-mode={mode.id}
                role="radio"
                aria-checked={viewMode === mode.id}
                onclick={() => setViewMode(mode.id)}
              >
                {mode.label}
              </button>
            {/each}
          </div>
        </div>
      </header>
      {#if isPrivate && !privateRevealed}
        <div class="notes__private-gate">
          <Icon name="lock" size={20} />
          <p>Приватная заметка — содержимое скрыто от случайного взгляда.</p>
          <button type="button" class="btn" onclick={() => (privateRevealed = true)}>
            Показать
          </button>
        </div>
      {:else}
      {#if findOpen}
        <NoteFind noteId={selectedId} container={viewMode === 'raw' ? null : previewEl} dep={description} onClose={() => (findOpen = false)} />
      {/if}
      <div
        class="notes__split"
        class:notes__split--raw={viewMode === 'raw'}
        class:notes__split--combined={viewMode === 'combined'}
        class:notes__split--formatted={viewMode === 'formatted'}
        class:notes__split--flash={flashing}
      >
        {#if viewMode !== 'formatted'}
          <MentionTextarea
            class="notes__edit"
            placement="inside"
            bind:value={description}
            bind:el={editEl}
            {quests}
            {questlines}
            {notes}
            {attachments}
            rows={16}
            placeholder="Markdown. @название — ссылка. Код и конфиг — в блоках ``` … ```"
            onkeydown={onEditKeydown}
          />
        {/if}
        {#if viewMode !== 'raw'}
          <div
            class="notes__preview block--prose"
            class:notes__preview--doc={viewMode === 'formatted'}
            bind:this={previewEl}
            title={viewMode === 'formatted' ? 'Двойной клик — править' : undefined}
            ondblclick={startBodyEdit}
          >
            <MarkdownBody source={description} {labels} {onRef} />
          </div>
        {/if}
      </div>
      {/if}
      {#if detail?.refs?.length}
        <div class="block notes__links notes__links--first">
          <button
            type="button"
            class="block__label block__label--toggle"
            onclick={() => (refsOpen = !refsOpen)}
            aria-expanded={refsOpen}
          >
            Ссылки ({detail.refs.length})
            <Icon name={refsOpen ? 'chevron-down' : 'chevron-right'} size={12} />
          </button>
          {#if refsOpen}
            <ul>
              {#each detail.refs as ref (`${ref.kind}-${ref.id}`)}
                <li>
                  <button type="button" class="notes__link" onclick={() => onRef?.(ref.kind, ref.id)}>
                    {ref.title || `${ref.kind}=${ref.id}`}
                    <span class="notes__kind">{ref.kind}={ref.id}</span>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}
      {#if detail?.backlinks?.length}
        <div class="block notes__links" class:notes__links--first={!detail?.refs?.length}>
          <button
            type="button"
            class="block__label block__label--toggle"
            onclick={() => (backlinksOpen = !backlinksOpen)}
            aria-expanded={backlinksOpen}
          >
            Ссылаются ({detail.backlinks.length})
            <Icon name={backlinksOpen ? 'chevron-down' : 'chevron-right'} size={12} />
          </button>
          {#if backlinksOpen}
            <ul>
              {#each detail.backlinks as ref (`b-${ref.kind}-${ref.id}`)}
                <li>
                  <button type="button" class="notes__link" onclick={() => onRef?.(ref.kind, ref.id)}>
                    {ref.title || `${ref.kind}=${ref.id}`}
                    <span class="notes__kind">{ref.kind}={ref.id}</span>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}
      {#if selectedId}
        <div class="block notes__attach">
          <NoteCheck noteId={selectedId} onChanged={onChanged} onOpenSecrets={() => onOpenSecrets?.(selectedId)} />
        </div>
        <div class="block notes__attach">
          <AttachmentsBlock ownerType="note" ownerId={selectedId} onOpenOwner={onRef} />
        </div>
        <div class="block notes__attach">
          <EntityHistory entityType="note" entityId={selectedId} version={detail?.updated_at} />
        </div>
      {/if}
    {/if}
  </section>
  <NoteOutline scroller={pageEl} content={previewEl} enabled={viewMode === 'formatted'} dep={`${selectedId}:${description}`} />
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
  open={exportMenuOpen}
  x={exportMenuX}
  y={exportMenuY}
  items={exportMenuItems}
  onSelect={onExportSelect}
  onClose={() => (exportMenuOpen = false)}
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

  .notes--sidebar-collapsed .notes__tree {
    border-right: 0;
  }

  .notes__tree {
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
    border-right: 1px solid var(--color-border, #333);
    background: var(--color-bg-raised, #1a1a1a);
  }

  .notes__tools {
    display: flex;
    gap: 0.4rem;
    padding: 0.55rem 0.6rem;
    border-bottom: 1px solid var(--color-border, #333);
  }

  .notes__flaghints:empty {
    display: none;
  }

  .hits {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 2px;
  }
  .hit {
    display: grid;
    gap: 0.15rem;
    width: 100%;
    padding: 0.45rem 0.6rem;
    text-align: left;
    color: inherit;
    background: transparent;
    border: 0;
    border-radius: var(--radius-sm, 6px);
    cursor: pointer;
  }
  .hit:hover,
  .hit--on {
    background: var(--color-bg-hover, #2a2a2a);
  }
  .hit__title {
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg);
  }
  .hit__where {
    font-size: 0.68rem;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--color-fg-subtle, #6e6e6e);
  }
  .hit__snippet {
    font-size: 0.78rem;
    line-height: 1.35;
    color: var(--color-fg-muted, #9a9a9a);
    overflow-wrap: anywhere;
  }
  .hit__snippet :global(mark) {
    background: color-mix(in srgb, var(--color-accent, #c9a227) 35%, transparent);
    color: var(--color-fg);
    border-radius: 2px;
  }

  .notes__tools .search {
    flex: 1 1 auto;
    min-width: 0;
  }

  .notes__add {
    flex: 0 0 auto;
    min-width: 2rem;
    padding-inline: 0.55rem;
    font-size: 1.1rem;
    line-height: 1;
  }

  .notes__list {
    flex: 1 1 auto;
    overflow: auto;
    padding: 0.25rem 0;
  }

  .notes__node {
    display: flex;
    align-items: stretch;
    padding-left: calc(0.35rem + var(--notes-depth, 0) * 0.85rem);
  }

  .notes__fold {
    flex: 0 0 1.35rem;
    display: flex;
    align-items: center;
    justify-content: center;
    border: 0;
    padding: 0;
    margin: 0;
    background: transparent;
    color: color-mix(in srgb, var(--color-fg, #e8e8e8) 55%, transparent);
    cursor: pointer;
    border-radius: 2px;
  }

  .notes__fold:hover {
    color: var(--color-fg, #e8e8e8);
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 6%, transparent);
  }

  .notes__fold--spacer {
    pointer-events: none;
  }

  .notes__row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex: 1 1 auto;
    min-width: 0;
    border: 0;
    border-left: 2px solid transparent;
    background: transparent;
    color: inherit;
    text-align: left;
    padding: 0.28rem 0.45rem 0.28rem 0.35rem;
    cursor: pointer;
    font-family: var(--font-body, Georgia, serif);
    font-size: var(--text-sm, 0.875rem);
  }

  .notes__row:hover {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 4%, transparent);
  }

  .notes__row--on {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 6%, var(--color-bg-raised, #1a1a1a));
    border-left-color: var(--color-fg, #e8e8e8);
  }

  .notes__row--dragover {
    background: color-mix(in srgb, var(--color-accent, #c9a227) 16%, transparent);
    border-left-color: var(--color-accent, #c9a227);
  }

  .notes__list--dragover {
    box-shadow: inset 0 0 0 1px var(--color-accent, #c9a227);
  }

  .notes__row-icon {
    flex-shrink: 0;
    display: inline-flex;
    color: var(--line-color, #9a9a9a);
  }

  .notes__row-label {
    display: inline-flex;
    align-items: baseline;
    flex: 1 1 auto;
    min-width: 0;
    max-width: 100%;
  }

  :global(.notes__row-title) {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .notes__row-rename {
    width: 100%;
    min-width: 0;
    padding: 0.1rem 0.3rem;
    border: 1px solid var(--color-accent, #c9a227);
    border-radius: var(--radius-sm, 2px);
    background: var(--color-bg, #121212);
    color: var(--color-fg, #e8e8e8);
    font: inherit;
  }

  .notes__unsaved {
    flex-shrink: 0;
    color: var(--color-accent, #c9a227);
    font-weight: 700;
    line-height: 1;
    animation: notes-unsaved-glow 1.8s cubic-bezier(0.37, 0, 0.63, 1) infinite;
  }

  .notes__row-status {
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    gap: 0.15rem;
  }

  /* Status toggles in the open note's header, and their matching sidebar badges. */
  .notes__status-btn {
    opacity: 0.35;
  }

  .notes__status-btn--on {
    opacity: 1;
  }

  .notes__status-btn--readme.notes__status-btn--on,
  .notes__status-dot--readme {
    color: #d14343;
    animation: notes-readme-blink 1.4s steps(1, end) infinite;
  }

  /* Same alert icon, but on a note whose README is a hidden (collapsed)
   * descendant, not the note itself — yellow keeps it from being mistaken
   * for "this note is the README". */
  .notes__status-dot--readme-bubble {
    color: #d1a33a;
    animation: notes-readme-blink 1.4s steps(1, end) infinite;
  }

  .notes__status-btn--private.notes__status-btn--on,
  .notes__status-dot--private {
    color: #9a9a9a;
  }

  .notes__status-btn--category.notes__status-btn--on,
  .notes__status-dot--category {
    color: #b5651d;
  }

  .notes__status-btn--outdated.notes__status-btn--on,
  .notes__status-dot--outdated {
    color: #d14343;
  }

  .notes__hit-flag {
    margin-left: 0.35rem;
    vertical-align: middle;
  }

  .notes__status-dot {
    display: inline-flex;
  }

  @keyframes notes-readme-blink {
    0%,
    49% {
      opacity: 1;
    }
    50%,
    100% {
      opacity: 0.25;
    }
  }

  .notes__private-gate {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-2, 0.5rem);
    padding: var(--space-5, 2rem) var(--space-3, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
    text-align: center;
  }

  .notes__private-gate p {
    margin: 0;
    font-size: 0.9rem;
  }

  @keyframes notes-unsaved-glow {
    0%,
    100% {
      opacity: 0.28;
      text-shadow: 0 0 0.05em color-mix(in srgb, var(--color-accent, #c9a227) 15%, transparent);
    }
    50% {
      opacity: 1;
      text-shadow:
        0 0 0.12em var(--color-accent, #c9a227),
        0 0 0.45em color-mix(in srgb, var(--color-accent, #c9a227) 85%, transparent);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .notes__unsaved {
      animation: none;
      opacity: 1;
    }
  }

  .notes__page {
    min-height: 0;
    overflow: auto;
    padding: var(--space-5, 1.5rem) var(--space-6, 2rem) 2rem;
    display: flex;
    flex-direction: column;
    gap: var(--space-4, 1rem);
  }

  .notes__empty,
  .empty {
    color: var(--color-fg-muted, #9a9a9a);
    padding: 1rem;
  }

  .notes__error {
    margin: 0;
    color: var(--color-danger, #b54a3a);
  }

  .notes__external {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.4rem 0.75rem;
    margin: 0;
    padding: 0.4rem 0.6rem;
    border-left: 3px solid var(--color-accent, #c9a227);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 12%, transparent);
    font-size: 0.85rem;
  }

  .notes__external-btn {
    padding: 0;
    border: 0;
    background: none;
    color: var(--color-accent, #c9a227);
    font: inherit;
    text-decoration: underline;
    cursor: pointer;
  }

  .notes__split--flash {
    animation: notes-flash 1.5s ease-out;
  }

  @keyframes notes-flash {
    from {
      background: color-mix(in srgb, var(--color-accent, #c9a227) 22%, transparent);
    }
    to {
      background: transparent;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .notes__split--flash {
      animation: none;
    }
  }

  .notes__head {
    /* No bottom margin: the page's flex gap alone sets the distance from the
       header divider to the note body, matching the gap below the body so the
       body sits evenly between its two dividers. */
    margin: 0;
    padding: 0 0 var(--space-3, 0.75rem);
    border-bottom: 1px solid var(--color-border, #333);
  }

  .notes__head-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3, 0.75rem);
  }

  .notes__actions {
    flex-shrink: 0;
  }

  /* On a phone the four action buttons leave the title ~40% of the row;
     give the title the full line and drop the actions under it. */
  @media (max-width: 480px) {
    .notes__head-row {
      flex-wrap: wrap;
      row-gap: var(--space-2, 0.5rem);
    }

    .notes__title-cluster {
      flex-basis: 100%;
    }

    .notes__actions {
      margin-left: auto;
    }
  }

  .notes__pin--on {
    color: var(--color-accent, #c9a227);
  }

  .notes__title-cluster {
    display: inline-flex;
    align-items: baseline;
    flex: 1 1 auto;
    min-width: 0;
    max-width: 100%;
    font-family: var(--font-display, Georgia, serif);
    font-size: clamp(1.35rem, 2.4vw, 1.85rem);
    font-weight: 700;
    line-height: 1.15;
  }

  .notes__title-grow {
    position: relative;
    display: inline-grid;
    /* Flex items default to min-width:auto (= their content's min-content),
       and the sizer's white-space:pre makes that the full title width —
       without an explicit 0 here a long title refuses to shrink and
       overlaps the pin/save buttons instead of clipping. */
    min-width: 0;
    max-width: 100%;
    overflow: hidden;
  }

  .notes__title-sizer,
  .notes__title {
    grid-area: 1 / 1;
    /* Same min-width:auto trap applies to grid items sizing their track —
       without this the track still grows to the sizer's full-text
       min-content width even though the container above is now capped. */
    min-width: 0;
    padding: 0.15rem 0;
    font: inherit;
  }

  .notes__title-sizer {
    visibility: hidden;
    white-space: pre;
    pointer-events: none;
  }

  .notes__title {
    width: 100%;
    min-width: 0;
    border: 0;
    background: transparent;
    color: inherit;
    box-sizing: border-box;
  }

  .notes__title--locked {
    cursor: default;
    caret-color: transparent;
  }

  .notes__title--locked:focus {
    outline: none;
  }

  .notes__title-cluster .notes__unsaved {
    flex-shrink: 0;
    padding: 0.15rem 0;
  }

  .notes__colophon {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.35rem 0.85rem;
    margin: 0.45rem 0 0;
    font-family: var(--font-ui, sans-serif);
    font-size: 0.72rem;
    letter-spacing: 0.02em;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .notes__crumbs {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.25rem 0.35rem;
    min-width: 0;
  }

  .notes__crumb {
    border: 0;
    padding: 0;
    margin: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    cursor: pointer;
    max-width: 12rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .notes__crumb:hover {
    color: var(--color-fg, #e8e8e8);
  }

  .notes__crumb--here {
    color: var(--color-fg, #e8e8e8);
    text-decoration: underline;
    text-underline-offset: 0.2em;
  }

  .notes__crumb-sep {
    opacity: 0.45;
    user-select: none;
  }

  .notes__modes {
    margin-left: auto;
    display: flex;
    flex-wrap: wrap;
    gap: 0.65rem;
    align-items: baseline;
  }

  .notes__mode {
    border: 0;
    padding: 0;
    margin: 0;
    background: transparent;
    color: var(--color-fg-subtle, #6e6e6e);
    font: inherit;
    letter-spacing: 0.02em;
    cursor: pointer;
  }

  .notes__mode:hover {
    color: var(--color-fg-muted, #9a9a9a);
  }

  .notes__mode--on {
    color: var(--color-fg, #e8e8e8);
    box-shadow: 0 1px 0 currentColor;
  }

  .notes__split {
    display: grid;
    gap: 0;
    /* Content-driven in every mode: the body flows full-length and the page
       scrolls, nothing is boxed to the viewport. The editor is kept from
       being squashed not by a fixed height but by auto-growing to its text
       (autoGrow below), so opening ссылки/ссылаются can't eat into it. */
    min-height: 0;
    flex: 0 0 auto;
  }

  .notes__split--combined {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: 0 1.1rem;
    background: linear-gradient(
      to right,
      transparent calc(50% - 0.5px),
      var(--color-border, #333) calc(50% - 0.5px),
      var(--color-border, #333) calc(50% + 0.5px),
      transparent calc(50% + 0.5px)
    );
  }

  .notes__split--combined :global(textarea.notes__edit) {
    border-right: 0;
    padding-right: 0.25rem;
  }

  .notes__split--combined .notes__preview {
    padding-left: 0.25rem;
  }

  /* minmax(0, …), not a bare 1fr: 1fr's floor is the content's min-content
     width, so one unbreakable line (a path, a URL, a long inline code) grew
     the track — and the whole note — to thousands of px on a phone. */
  .notes__split--raw,
  .notes__split--formatted {
    grid-template-columns: minmax(0, 1fr);
  }

  /* Reading mode: preview flows with its content instead of scrolling inside
     its own box, so the body ends where the text ends. */
  .notes__split--formatted .notes__preview {
    min-height: 0;
    overflow: visible;
  }

  .notes__split :global(textarea.notes__edit) {
    width: 100%;
    min-height: 18rem;
    /* Height is set from content by autoGrow so the editor flows like the
       reading view — no inner scroll, the page scrolls instead. */
    resize: none;
    overflow: hidden;
    padding: 0.15rem 0.1rem;
    border: 0;
    border-radius: 0;
    background: transparent;
    color: inherit;
    font-family: var(--font-mono, ui-monospace, monospace);
    font-size: 0.85rem;
    line-height: 1.5;
  }

  .notes__preview {
    /* block--prose carries margin-bottom: 2rem for typography; here the flex
       gap already spaces the body, so drop it — otherwise it re-opens the
       bottom gap once the box grows with content in reading mode. */
    margin: 0;
    padding: 0.15rem 0.25rem 0.15rem 0.85rem;
    overflow: auto;
    min-height: 18rem;
    min-width: 0;
    overflow-wrap: break-word;
  }

  .notes__split--formatted .notes__preview {
    padding-left: 0.1rem;
  }

  .notes__preview--doc {
    cursor: text;
  }

  .notes__split--raw :global(textarea.notes__edit) {
    padding-left: 0.1rem;
  }

  .notes__links {
    margin: 0;
    padding: 0;
  }

  .notes__links--first {
    /* Sit the divider a flex-gap below the body (no extra margin), then pad
       the label the same flex-gap below the divider. Both the body above and
       the links group below then clear each divider by the same amount. */
    margin-top: 0;
    padding-top: var(--space-4, 1rem);
    border-top: 1px solid var(--color-border, #333);
  }

  .block__label--toggle {
    /* Block-level flex, not inline-flex: an inline box would sit on the
       parent's text baseline and carry its line-height strut as dead space. */
    display: flex;
    width: fit-content;
    align-items: center;
    gap: 0.3rem;
    border: 0;
    background: transparent;
    /* .block__label is written for an <h3> and carries margin-bottom 0.5rem —
       8px of invisible space under the label that made it look glued to the
       divider above. As a toggle row its box must be exactly the label. */
    margin: 0;
    padding: 0;
    line-height: 1;
    cursor: pointer;
  }

  .block__label--toggle:hover {
    color: var(--color-fg-muted, #9a9a9a);
  }

  .notes__links ul {
    list-style: none;
    margin: var(--space-2, 0.5rem) 0 0;
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .notes__link {
    border: 0;
    border-bottom: 1px solid color-mix(in srgb, var(--color-border, #333) 80%, transparent);
    background: transparent;
    color: inherit;
    padding: 0.15rem 0;
    cursor: pointer;
    font-family: var(--font-ui, sans-serif);
    font-size: var(--text-sm, 0.875rem);
  }

  .notes__link:hover {
    border-bottom-color: var(--color-fg-muted, #9a9a9a);
  }

  .notes__kind {
    margin-left: 0.35rem;
    color: var(--color-fg-subtle, #6e6e6e);
    font-family: var(--font-mono, monospace);
    font-size: 0.7rem;
  }

  .notes__attach {
    /* Same rhythm as the links divider: one flex-gap above the divider, one
       flex-gap of padding below it, so «Вложения» clears its divider by the
       same amount the links label clears the divider above. */
    margin-top: 0;
    margin-bottom: 0;
    padding-top: var(--space-4, 1rem);
    border-top: 1px solid var(--color-border, #333);
  }

  @media (max-width: 720px) {
    /* Stay side-by-side and just shrink — no stacking, which pushed the
       note detail below a full-height tree and made it feel lost. */
    .notes {
      grid-template-columns: minmax(7rem, 30%) 1fr;
    }
    .notes__page {
      padding: 1rem 1rem 2rem;
    }
    .notes__split--combined {
      grid-template-columns: 1fr;
      gap: 0.85rem;
      background: none;
    }
    .notes__split--combined :global(textarea.notes__edit) {
      border-bottom: 1px solid var(--color-border, #333);
      padding-bottom: 0.85rem;
      padding-right: 0.1rem;
    }
    .notes__split--combined .notes__preview {
      padding-left: 0.1rem;
    }
    .notes__modes {
      margin-left: 0;
      width: 100%;
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

    .notes--sidebar-collapsed .notes__tree {
      display: none;
    }

    .notes--sidebar-collapsed .notes__page {
      display: flex;
    }

    /* No room for a side-by-side split; the $effect above steers away from
       it too, so this is just for anyone still mid-toggle on resize. */
    .notes__mode[data-mode="combined"] {
      display: none;
    }
  }
</style>
