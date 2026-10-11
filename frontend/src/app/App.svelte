<script lang="ts">
  // Quests web UI — orchestrator (state, load, overlays)
  import { HistoryPage } from '@/pages/history'
import { RunnersPage } from '@/pages/runners'
  import { GraphPage } from '@/pages/graph'
  import { createJournalMenu } from '@/app/model/journalMenu.svelte.ts'
  import { createStepProgress, questIdOfStep, stepRef } from '@/app/model/stepProgress.svelte.ts'
  import { createQuestActions } from '@/app/model/questActions.svelte.ts'
  import { buildRefLabels } from '@/app/model/refLabels.ts'
  import {
    attachmentIdFromUrl,
    healthFrom,
    healthOffline,
    initialRoute,
    noteIdFromUrl,
    questIdFromUrl,
    replaceSearch,
    searchForView as searchForRoute,
    tabFromUrl,
  } from '@/app/model/route.ts'
  import { onMount, tick, untrack } from 'svelte'
  import { cubicOut } from 'svelte/easing'
  import { tabSwipe } from '@/shared/lib/tabSwipe.ts'
  import {
    deleteQuest,
    deleteQuestStep,
    deleteQuestline,
    listCategories,
    listQuestlines,
    listQuests,
    listNotes,
    listAllAttachments,
    updateQuest,
    updateQuestStep,
    QUEST_STATUSES,
    QUEST_STATUS_LABELS,
    QUEST_SIGNIFICANCES,
    fetchHealth,
    fetchAuthState,
    logout as apiLogout,
    setUnauthorizedHandler,
  } from '@/shared/api/client.ts'
  import { subscribeQuestEvents } from '@/shared/lib/live.ts'
  import { seedAttachmentIndex, invalidateAttachmentLiveFlags, flattenAttachmentIndex } from '@/entities/attachment'
  import { applyTheme, loadSavedTheme } from '@/shared/lib/theme.ts'
  import { questMatchesQuery } from '@/shared/lib/search.ts'
  import { parseQuery, passes, tokenValues, setToken } from '@/shared/lib/searchFlags.ts'
  import { questSpec, groupQuestsByCategory, inScope, listOpenOrAll, scopeQuestlineChoices, downloadQuestMarkdown, downloadQuestPdf, downloadQuestPdfSimple } from '@/entities/quest'
  import { copyText } from '@/shared/lib/clipboard.ts'
  import { installHotkeys, registerHotkeys } from '@/shared/lib/hotkeys.ts'
  import { foldRow, moveRow } from '@/shared/lib/listNav.ts'
    import { statusToastLabel, toast, toastDone, toastProgress } from '@/shared/lib/toasts.svelte.ts'
  import { localTimeZone } from '@/shared/lib/time.ts'
  import { ActionAssistantModal } from '@/features/assistant'
  import { QuestModal, StepModal } from '@/features/quest-edit'
  import { QuestlineModal } from '@/features/questline-edit'
  import { TemplatesModal } from '@/features/templates'
  import { SecretsModal } from '@/features/secrets'
  import { refreshTemplates, templateStore } from '@/entities/template'
  import { TagsModal } from '@/features/tags'
  import { SettingsModal } from '@/features/settings'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
  import ToastHost from '@/shared/ui/ToastHost.svelte'
  import { JournalHeader } from '@/widgets/journal-header'
  import { QuestSidebar } from '@/widgets/quest-sidebar'
  import { QuestDetail } from '@/widgets/quest-detail'
  import { ActivityCalendar } from '@/widgets/activity-calendar'
  import { NotesPage } from '@/pages/notes'
  import { AttachmentsPage } from '@/pages/attachments'
  import { LoginScreen, InviteScreen, NoWorkspaceScreen } from '@/pages/auth'

  /** Auth gate: null = still checking, true/false = decided. */
  let authed = $state(null)
  /** Invitation link (?invite=…): shown instead of the journal until accepted. */
  let noWorkspace = $state(false)
  const inviteToken = new URLSearchParams(location.search).get('invite')
  /** Whether this instance uses accounts at all (false on an open local one). */
  let authRequired = $state(false)
  let username = $state('')
  /** Cleanup for the live subscription + timers, so logout can tear them down. */
  let stopApp: () => void = null

  let quests = $state([])
  let notes = $state([])
  let attachmentIndex: Record<string, any> | null = $state(null)
  let selectedId = $state(null)
  let selectedNoteId = $state((null as number | null))
  let selectedAttachmentId = $state((null as number | null))
  let loading = $state(true)
  let error = $state('')
  let liveStatus = $state('off')
  let health: { api: string, overlay: string, telegram: string, webdav: string, db?: string, detail?: Record<string, any> } = $state({
    api: 'unknown',
    overlay: 'unknown',
    telegram: 'unknown',
    webdav: 'unknown',
    db: 'unknown',
  })
  let searchQuery = $state('')
  let view = $state(
    ('journal' as 'journal' | 'notes' | 'attachments' | 'calendar' | 'history' | 'graph' | 'runners'),
  )
  /** Sidebar visibility per view that has one — remembered across reloads. */
  let sidebarCollapsed: Record<string, boolean> = $state(loadSidebarCollapsed())

  function loadSidebarCollapsed() {
    try {
      const raw = JSON.parse(localStorage.getItem('quests.sidebarCollapsed') || '{}')
      return { journal: !!raw.journal, notes: !!raw.notes }
    } catch {
      return { journal: false, notes: false }
    }
  }

  function toggleSidebar(key: string) {
    setSidebarCollapsed(key, !sidebarCollapsed[key])
  }

  function setSidebarCollapsed(key: string, value: boolean) {
    if (sidebarCollapsed[key] === value) return
    sidebarCollapsed = { ...sidebarCollapsed, [key]: value }
    try {
      localStorage.setItem('quests.sidebarCollapsed', JSON.stringify(sidebarCollapsed))
    } catch {
      /* ignore */
    }
  }

  /** Below this, the sidebar/list takes the full screen instead of a side column. */
  const NARROW_SIDEBAR_BREAKPOINT = '(max-width: 480px)'

  function isNarrowViewport() {
    try {
      return window.matchMedia(NARROW_SIDEBAR_BREAKPOINT).matches
    } catch {
      return false
    }
  }

  /** On a full-screen sidebar, picking an item should reveal it, not leave the list covering it. */
  function revealDetailOnNarrow(key: string) {
    if (isNarrowViewport()) setSidebarCollapsed(key, true)
  }

  /** …and going back to the list should bring it back, not leave an empty detail pane. */
  function revealListOnNarrow(key: string) {
    if (isNarrowViewport()) setSidebarCollapsed(key, false)
  }
  /** Centre of the relations map tab; null until picked or opened from a card. */
  let graphRoot = $state(
    ({ type: null, id: null } as { type: 'quest' | 'note' | 'questline' | null, id: number | null }),
  )
  /** Bump to refresh the map after live events. */
  let graphNonce = $state(0)
  /** Bump to refresh the history feed after live events. */
  let historyNonce = $state(0)
  let categories = $state([])
  let questlines: { id: number, title: string, category_id?: number | null, color?: string, icon?: string }[] = $state([])
  /** Open state for category subgroups: `${bucket}:${key}` → boolean (default true). */
  let categoryOpen = $state(({} as Record<string, boolean>))
  /** Open state for questline subgroups: `${bucket}:${catKey}:${lineKey}` → boolean. */
  let lineOpen = $state(({} as Record<string, boolean>))
  /** Sidebar "Заморожено (N)" tab — collapsed by default. */
  let frozenOpen = $state(false)
  function toggleFrozen() {
    frozenOpen = !frozenOpen
  }

  let modalOpen = $state(false)
  let modalMode = $state(('create' as 'create' | 'edit'))
  /** Prefill when creating from questline context menu. */
  let modalDefaults = $state(
    (null as { questline_id?: number | null, category_id?: number | null, description?: string } | null),
  )
  let templatesOpen = $state(false)
  let secretsOpen = $state(false)
  /** entity the secrets modal opens on (context menu → «Секреты»), or null */
  let secretsOwner = $state((null as { kind: string, id: number } | null))
  /** template the templates modal opens for editing (a template=N link) */
  let templatesFocusId = $state((null as number | null))
  let tagsOpen = $state(false)
  let settingsOpen = $state(false)
  /** Section the settings modal scrolls to when opened (e.g. 'hotkeys'). */
  let settingsSection = $state((null as string | null))
  let assistantOpen = $state(false)
  let lineModalOpen = $state(false)
  let lineModalMode = $state(('create' as 'create' | 'edit'))
  let lineModalTarget = $state((null as any | null))
  let lineDeleteConfirmOpen = $state(false)
  let lineDeleting = $state(false)
  let deleting = $state(false)
  let deleteConfirmOpen = $state(false)
  let stepDeleteOpen = $state(false)
  let stepDeleteBusy = $state(false)
  let stepDelete: { questId: number, stepId: number, title: string } | null = $state(null)
  /** True while QuestDetail is playing its erase-before-delete animation. */
  let deleteAnimActive = $state(false)
  let deleteAnimResolve: ((v?: unknown) => void) | null = null
  /** quest id → when a live event from outside the web UI touched it. The
   * event lands before the refetched data, so QuestDetail picks the mark up
   * when the text actually changes and decides to type it in. */
  const remoteEdits = new Map()
  const REMOTE_EDIT_TTL_MS = 8000
  function consumeRemoteEdit(questId: any) {
    const at = remoteEdits.get(questId)
    if (at == null) return false
    remoteEdits.delete(questId)
    return Date.now() - at < REMOTE_EDIT_TTL_MS
  }
  function onDeleteAnimDone() {
    deleteAnimResolve?.()
    deleteAnimResolve = null
  }
  let pinBusyId = $state((null as number | null))
  let stepModalOpen = $state(false)
  let stepModalQuestId = $state((null as number | null))
  let stepModalStep = $state((null as any | null))
  /** Prefer this id across in-flight load() (URL / HUD focus). */
  let pendingSelectId = $state((null as number | null))
  // The sidebar's search text is the single source of truth for its filters: «показывать завершённые»
  // is `status:all`, the category / questline scope are `cat:slug` / `line:id` (searchFlags.js).
  let questQuerySpec = $derived(questSpec({ categories }))
  let parsedQuestSearch = $derived(parseQuery(searchQuery, questQuerySpec))
  /** When false — only active/expired; when true — all statuses. */
  let showAllQuests = $derived(tokenValues(parsedQuestSearch, 'status').includes('all'))
  /** Sidebar scope narrowing: null = all, else a category, optionally further narrowed to one of its questlines. */
  let scopeCategoryId = $derived.by(() => {
    const slug = tokenValues(parsedQuestSearch, 'cat')[0]
    return slug ? (categories.find((c) => c.slug === slug)?.id ?? null) : null
  })
  let scopeQuestlineId = $derived.by(() => {
    const v = tokenValues(parsedQuestSearch, 'line')[0]
    if (!v) return null
    const byId = questlines.find((l) => String(l.id) === v)
    if (byId) return byId.id
    return questlines.find((l) => (l.title || '').replace(/\s+/g, '_').toLowerCase() === v.toLowerCase())?.id ?? null
  })
  let nowMs = $state(Date.now())

  function setShowAll(on: any) {
    const rest = tokenValues(parsedQuestSearch, 'status').filter((v) => v !== 'all')
    searchQuery = setToken(searchQuery, 'status', on ? [...rest, 'all'] : rest)
  }
  function setScopeCategory(id: null) {
    const slug = id == null ? null : categories.find((c) => c.id === id)?.slug
    searchQuery = setToken(setToken(searchQuery, 'line', []), 'cat', slug ? [slug] : [])
  }
  function setScopeQuestline(id: null) {
    searchQuery = setToken(searchQuery, 'line', id == null ? [] : [String(id)])
  }
  /** Questlines to narrow to under the current scope category (see scopeQuestlineOptions). */
  let scopeQuestlineOptions = $derived(scopeQuestlineChoices({ scopeCategoryId, questlines, quests, showAll: showAllQuests }))

  let matchedQuests = $derived(quests.filter((q) => questMatchesQuery(q, parsedQuestSearch.text) && passes(parsedQuestSearch, questQuerySpec, q)))
  let listedQuests = $derived(listOpenOrAll(matchedQuests, { showAll: showAllQuests, statusTokens: tokenValues(parsedQuestSearch, 'status') }))
  let scopedQuests = $derived(inScope(listedQuests, { categoryId: scopeCategoryId, questlineId: scopeQuestlineId }))
  let byCategory = $derived(groupQuestsByCategory(scopedQuests, categories, questlines))
  /** "Заморожено" — own sidebar tab, independent of showAllQuests/OPEN_STATUSES. */
  let frozenQuests = $derived(inScope(matchedQuests.filter((q) => q.status === 'frozen'), { categoryId: scopeCategoryId, questlineId: scopeQuestlineId }))
  let selected = $derived(quests.find((q) => q.id === selectedId) ?? null)
  /** Every card, hidden included — only the Attachments tab shows hidden ones. */
  let allAttachments = $derived(flattenAttachmentIndex(attachmentIndex))
  let attachments = $derived(allAttachments.filter((a) => !a.hidden))
  let refLabels = $derived(buildRefLabels({ notes, quests, questlines, templates: templateStore.list, attachments: allAttachments }))

  function isCategoryOpen(key: string|number) {
    return categoryOpen[key] !== false
  }

  function toggleCategory(key: any) {
    categoryOpen = { ...categoryOpen, [key]: !isCategoryOpen(key) }
  }

  function isLineOpen(catKey: any, lineKey: any) {
    const id = `${catKey}:${lineKey}`
    return lineOpen[id] !== false
  }

  function toggleLine(catKey: any, lineKey: any) {
    const id = `${catKey}:${lineKey}`
    lineOpen = { ...lineOpen, [id]: !isLineOpen(catKey, lineKey) }
  }

  async function load({ silent = false } = {}) {
    if (!silent) loading = true
    if (!silent) error = ''
    try {
      const [next, cats, lines, nextNotes, attIndex] = await Promise.all([
        listQuests({}),
        listCategories(),
        listQuestlines(),
        listNotes().catch((): any[] => []),
        listAllAttachments().catch((): any => null),
      ])
      quests = next
      notes = Array.isArray(nextNotes) ? nextNotes : []
      attachmentIndex = attIndex && typeof attIndex === 'object' ? attIndex : null
      categories = Array.isArray(cats) ? cats : []
      questlines = Array.isArray(lines) ? lines : []
      if (attIndex) {
        seedAttachmentIndex(attIndex, {
          questIds: next.map((q) => q.id),
          questlineIds: questlines.map((l) => l.id),
          noteIds: notes.map((n) => n.id),
        })
      }
      const prefer = pendingSelectId ?? selectedId ?? questIdFromUrl()
      if (prefer != null && next.some((q) => q.id === prefer)) {
        selectedId = prefer
        pendingSelectId = null
      } else if (selectedId != null && !next.some((q) => q.id === selectedId)) {
        selectedId = null
      }
      const preferNote = selectedNoteId ?? noteIdFromUrl()
      if (preferNote != null && notes.some((n) => n.id === preferNote)) {
        selectedNoteId = preferNote
      } else if (selectedNoteId != null && !notes.some((n) => n.id === selectedNoteId)) {
        selectedNoteId = null
      }
      const flatAtt = flattenAttachmentIndex(attachmentIndex)
      const preferAtt = selectedAttachmentId ?? attachmentIdFromUrl()
      if (preferAtt != null && flatAtt.some((a) => a.id === preferAtt)) {
        selectedAttachmentId = preferAtt
      } else if (selectedAttachmentId != null && !flatAtt.some((a) => a.id === selectedAttachmentId)) {
        selectedAttachmentId = null
      }
    } catch (e) {
      if (!silent) {
        error = e.message || String(e)
        quests = []
        notes = []
        attachmentIndex = null
        selectedId = null
      }
    } finally {
      if (!silent) loading = false
    }
  }

  function openCreate(defaults: any = null) {
    modalMode = 'create'
    modalDefaults =
      defaults && !(defaults instanceof Event)
        ? {
            questline_id: defaults.questline_id ?? null,
            category_id: defaults.category_id ?? null,
            description: defaults.description ?? '',
          }
        : null
    modalOpen = true
  }

  function openTemplates() {
    templatesOpen = true
  }

  function openSecrets(owner: { kind: string, id: number } | null = null) {
    secretsOwner = owner
    secretsOpen = true
  }

  function openTags() {
    tagsOpen = true
  }

  function openSettings(opts: { section?: string } | Event | null = null) {
    settingsSection = opts && !(opts instanceof Event) ? (opts.section ?? null) : null
    settingsOpen = true
  }

  function openCreateQuestline() {
    lineModalMode = 'create'
    lineModalTarget = null
    lineModalOpen = true
  }

  function openEditQuestline(line: any) {
    const row =
      typeof line === 'number' ? questlines.find((l) => l.id === line) : line
    if (!row) return
    lineModalMode = 'edit'
    lineModalTarget = row
    lineModalOpen = true
  }

  function openEdit(quest = selected) {
    const q = quest instanceof Event ? selected : quest || selected
    if (!q?.id) return
    selectedId = q.id
    modalMode = 'edit'
    modalDefaults = null
    modalOpen = true
  }

  async function copyIdToClipboard(kind: string, id: number) {
    if (id == null) return
    try {
      const text = `${kind}=${id}`
      await copyText(text)
      toast(`Скопировано ${text}`, { kind: 'success', ttl: 1600 })
    } catch (e) {
      error = e?.message || String(e)
      toast(error, { kind: 'error' })
    }
  }

  const questActions = createQuestActions({
    updateQuest,
    applyQuest: (quest, opts) => applyQuest(quest, opts),
    setError: (message) => (error = message),
    refLabels: () => refLabels,
  })
  let statusBusy = $derived(questActions.busy)

  async function confirmDeleteStep() {
    if (!stepDelete || stepDeleteBusy) return
    stepDeleteBusy = true
    error = ''
    try {
      await deleteQuestStep(stepDelete.questId, stepDelete.stepId)
      stepDeleteOpen = false
      stepDelete = null
      await load({ silent: true })
      toast('Шаг удалён', { kind: 'success' })
    } catch (e) {
      error = e.message || String(e)
      toast(error, { kind: 'error' })
    } finally {
      stepDeleteBusy = false
    }
  }

  async function confirmDeleteQuestline() {
    if (menu.lineId == null || lineDeleting) return
    lineDeleting = true
    error = ''
    try {
      await deleteQuestline(menu.lineId)
      lineDeleteConfirmOpen = false
      menu.clearLine()
      await load({ silent: true })
      toast('Квестлайн удалён', { kind: 'success' })
    } catch (e) {
      error = e.message || String(e)
      toast(error, { kind: 'error' })
    } finally {
      lineDeleting = false
    }
  }

  function onQuestlineSaved(_line: any, meta = ({} as { mode?: string })) {
    load({ silent: true })
    if (meta.mode === 'create') toast('Квестлайн создан', { kind: 'success' })
  }

  function onQuestlineDeleted() {
    load({ silent: true })
    toast('Квестлайн удалён', { kind: 'success' })
  }

  function requestDeleteSelected(quest = selected) {
    const q = quest || selected
    if (!q || deleting) return
    if (q.id !== selectedId) selectedId = q.id
    deleteConfirmOpen = true
  }

  async function confirmDeleteSelected() {
    if (!selected || deleting) return
    deleting = true
    error = ''
    try {
      const id = selected.id
      deleteAnimActive = true
      await new Promise((resolve) => {
        deleteAnimResolve = resolve
      })
      deleteAnimActive = false
      await deleteQuest(id)
      deleteConfirmOpen = false
      onDeleted(id)
    } catch (e) {
      error = e.message || String(e)
      toast(error, { kind: 'error' })
    } finally {
      deleting = false
      deleteAnimActive = false
    }
  }

  const toggleCompleted = (quest = selected) => questActions.toggleCompleted(quest || selected)

  async function togglePin(quest: { id: number; pinned: any; overlay_hidden: any }, event: { stopPropagation: () => void; preventDefault: () => void }) {
    event?.stopPropagation?.()
    event?.preventDefault?.()
    if (!quest || pinBusyId != null) return
    pinBusyId = quest.id
    error = ''
    try {
      // default → always in the HUD → never in the HUD → default
      const patch = quest.pinned
        ? { overlay_hidden: true }
        : quest.overlay_hidden
          ? { overlay_hidden: false }
          : { pinned: true }
      const saved = await updateQuest(quest.id, patch)
      applyQuest(saved, { select: false })
      selectQuestFromUi(saved.id, { pushUrl: false })
    } catch (e) {
      error = e.message || String(e)
    } finally {
      pinBusyId = null
    }
  }

  const steps = createStepProgress({
    quests: () => quests,
    selectedQuestId: () => selected?.id ?? null,
    updateQuestStep,
    applyQuest: (quest, opts) => applyQuest(quest, opts),
    setError: (message) => (error = message),
  })
  let stepBusyId = $derived(steps.busyId)
  let stepEditId = $derived(steps.editId)
  let stepEditValue = $derived(steps.editValue)
  const { set: setStepProgress, bump: bumpStep, begin: beginEditStep, onKeydown: onStepEditKeydown, onBlur: onStepEditBlur } = steps
  function findQuestIdForStep(stepId: number) {
    return questIdOfStep(quests, stepId)
  }
  function findStepRef(stepId: number) {
    return stepRef(quests, stepId)
  }

  const menu = createJournalMenu({
    quests: () => quests,
    questlines: () => questlines,
    findStep: findStepRef,
    selectQuest: (id) => (selectedId = id),
    selectedQuestId: () => selectedId,
    copyId: (kind, id) => void copyIdToClipboard(kind, id),
    openSecrets: (owner) => openSecrets(owner),
    openCreate: (defaults) => openCreate(defaults),
    editQuest: (q) => openEdit(q),
    editLine: (line) => openEditQuestline(line),
    askDeleteLine: () => (lineDeleteConfirmOpen = true),
    askDeleteQuest: (q) => {
      selectedId = q.id
      deleteConfirmOpen = true
    },
    editStep: (questId, step) => {
      stepModalQuestId = questId
      stepModalStep = step
      stepModalOpen = true
    },
    askDeleteStep: (d) => {
      stepDelete = d
      stepDeleteOpen = true
    },
    quest: questActions,
    step: { bump: (s, d, q) => bumpStep(s, d, q), set: (s, v, q) => setStepProgress(s, v, q) },
  })

  function applyQuest(quest: { id: any }, { select = true } = {}) {
    const idx = quests.findIndex((q) => q.id === quest.id)
    if (idx >= 0) {
      quests = quests.map((q) => (q.id === quest.id ? quest : q))
    } else {
      load({ silent: true })
    }
    if (select) selectedId = quest.id
  }

  function onSaved(quest: { id: any }, meta = ({} as { mode?: string })) {
    selectedId = quest.id
    load({ silent: true })
    if (meta.mode === 'create') toast('Квест создан', { kind: 'success' })
    if (view === 'graph') graphNonce += 1
  }

  function onDeleted(id: any) {
    if (selectedId === id) selectedId = null
    load({ silent: true })
    toast('Квест удалён', { kind: 'success' })
  }

  const searchForView = (nextView: string) =>
    searchForRoute(nextView, { quest: selectedId, note: selectedNoteId, attachment: selectedAttachmentId, graph: graphRoot })

  // Header tab order; drives which way a tab switch slides.
  const TAB_ORDER = ['journal', 'notes', 'attachments', 'calendar', 'history', 'graph', 'runners']
  let navDir = $state(1)

  // The map stays mounted once opened; it is only shown / hidden, with the same
  // slide the other tabs get (they are re-created per tab, the map is not).
  let graphSeen = $state(false)
  let graphOn = $state(view === 'graph')
  let graphEl = $state((null as HTMLDivElement | null))
  let prevView = view
  const SLIDE_MS = 220
  const SLIDE_EASE = 'cubic-bezier(0.215, 0.61, 0.355, 1)'
  $effect(() => {
    const v = view
    if (v === 'graph') graphSeen = true
    const was = prevView
    prevView = v
    if (v === was) return
    const dir = untrack(() => navDir)
    if (v === 'graph') {
      graphOn = true
      void tick().then(() => {
        graphEl?.animate([{ transform: `translateX(${dir * 100}%)` }, { transform: 'none' }], { duration: SLIDE_MS, easing: SLIDE_EASE })
      })
    } else if (was === 'graph' && graphEl) {
      const anim = graphEl.animate([{ transform: 'none' }, { transform: `translateX(${-dir * 100}%)` }], {
        duration: SLIDE_MS,
        easing: SLIDE_EASE,
      })
      anim.onfinish = () => {
        if (view !== 'graph') graphOn = false
      }
    }
  })

  /** Horizontal slide for tab switches: forward enters from the right, back from the left. */
  function slideTab(node: any, { dir, out = false }: any) {
    const sign = out ? -dir : dir
    return {
      duration: 220,
      easing: cubicOut,
      css: (t: number) => `transform: translateX(${(1 - t) * sign * 100}%)`,
    }
  }

  function setView(next: string) {
    const from = TAB_ORDER.indexOf(view)
    const to = TAB_ORDER.indexOf(next)
    if (from !== -1 && to !== -1 && from !== to) navDir = to > from ? 1 : -1
    view = next as typeof view
    replaceSearch(searchForView(next))
  }

  /** A free horizontal swipe on the page steps to the neighbouring header tab. */
  function swipeTab(dir: number) {
    const i = TAB_ORDER.indexOf(view)
    const next = i === -1 ? null : TAB_ORDER[i + dir]
    if (next) setView(next)
  }

  /** Opens the relations map centred on one entity. */
  function openGraph(type: any, id: any) {
    const from = TAB_ORDER.indexOf(view)
    navDir = TAB_ORDER.indexOf('graph') > from ? 1 : -1
    graphRoot = { type, id: Number(id) }
    replaceSearch(searchForView('graph'))
    view = 'graph'
  }

  function onGraphCenter(type: any, id: null) {
    // (null, null) = the map was cleared.
    graphRoot = id == null ? { type: null, id: null } : { type, id: Number(id) }
    replaceSearch(searchForView('graph'))
  }

  /** Opens a map node's entity in its own tab. */
  function onGraphOpen(kind: string, id: number) {
    onJournalRef(kind, id)
  }

  /** Calendar → History for one day. The URL is set first: History reads its filters on mount. */
  function openHistoryDay(dayKey: any) {
    const from = TAB_ORDER.indexOf(view)
    navDir = TAB_ORDER.indexOf('history') > from ? 1 : -1
    replaceSearch({ tab: 'history', day: dayKey })
    view = 'history'
  }

  function selectQuestFromUi(id: number, { pushUrl = true } = {}) {
    const n = Number(id)
    if (!Number.isFinite(n) || n <= 0) return
    pendingSelectId = n
    selectedId = n
    if (view !== 'journal') view = 'journal'
    if (pushUrl) replaceSearch({ quest: n })
    revealDetailOnNarrow('journal')
    try {
      window.focus()
    } catch {
      /* browsers often block focus from background */
    }
  }

  function selectNoteFromUi(id: number, { pushUrl = true } = {}) {
    const n = Number(id)
    if (!Number.isFinite(n) || n <= 0) {
      selectedNoteId = null
      if (pushUrl) replaceSearch(searchForView(view))
      revealListOnNarrow('notes')
      return
    }
    selectedNoteId = n
    if (view !== 'notes') view = 'notes'
    if (pushUrl) replaceSearch({ note: n })
    revealDetailOnNarrow('notes')
  }

  function selectAttachmentFromUi(id: number, { pushUrl = true } = {}) {
    const n = Number(id)
    if (!Number.isFinite(n) || n <= 0) {
      selectedAttachmentId = null
      if (pushUrl) replaceSearch(searchForView('attachments'))
      return
    }
    selectedAttachmentId = n
    if (view !== 'attachments') view = 'attachments'
    if (pushUrl) replaceSearch({ attachment: n })
  }

  function onJournalRef(kind: string, id: number) {
    if (kind === 'note') {
      selectNoteFromUi(id)
      return
    }
    if (kind === 'attachment') {
      selectAttachmentFromUi(id)
      return
    }
    if (kind === 'quest') {
      selectQuestFromUi(id)
      return
    }
    if (kind === 'questline') {
      const first = quests.find((q) => q.questline_id === id)
      if (first) selectQuestFromUi(first.id)
      else setView('journal')
      return
    }
    if (kind === 'step') {
      for (const q of quests) {
        if (q.steps?.some((s: { id: any }) => s.id === id)) {
          selectQuestFromUi(q.id)
          return
        }
      }
      return
    }
    if (kind === 'template') {
      templatesFocusId = id
      templatesOpen = true
    }
  }

  function clearSelectedQuest({ pushUrl = true } = {}) {
    selectedId = null
    pendingSelectId = null
    if (pushUrl) replaceSearch(searchForView(view))
    revealListOnNarrow('journal')
  }

  async function refreshHealth() {
    try {
      const next = healthFrom(await fetchHealth())
      if (health.webdav !== 'unknown' && health.webdav !== next.webdav) invalidateAttachmentLiveFlags()
      health = next
    } catch {
      health = healthOffline()
    }
  }

  /** Boots data loading and live updates. Only runs once authenticated. */
  function startApp() {
    const start = initialRoute()
    if (start.view) view = start.view
    if (start.attachmentId != null) selectedAttachmentId = start.attachmentId
    if (start.noteId != null) selectedNoteId = start.noteId
    if (start.graphRoot) graphRoot = start.graphRoot
    if (start.questId != null) {
      pendingSelectId = start.questId
      selectedId = start.questId
    }
    load()
    refreshHealth()
    const tick = setInterval(() => {
      nowMs = Date.now()
    }, 1000)
    const healthTick = setInterval(refreshHealth, 5000)
    const stop = subscribeQuestEvents(
      (msg) => {
        if (msg?.type === 'hello' && msg.pending_focus != null) {
          selectQuestFromUi(msg.pending_focus)
          return
        }
        if (msg?.type === 'ui_focus_quest' && msg.quest_id != null) {
          selectQuestFromUi(msg.quest_id)
          return
        }
        if (msg?.type === 'quests_changed') {
          if (msg.quest_id != null && msg.source && msg.source !== 'web') {
            remoteEdits.set(msg.quest_id, Date.now())
          }
          load({ silent: true })
          if (view === 'history') historyNonce += 1
          if (view === 'graph') graphNonce += 1
        }
      },
      {
        onStatus: (s) => (liveStatus = s),
        // The socket missed events while it was down or the tab was hidden.
        onResync: () => {
          void load({ silent: true })
          void refreshTemplates()
          void refreshHealth()
          if (view === 'history') historyNonce += 1
          if (view === 'graph') graphNonce += 1
        },
      },
    )
    return () => {
      clearInterval(tick)
      clearInterval(healthTick)
      stop()
    }
  }

  function onAuthenticated(name: string) {
    username = name
    authed = true
    stopApp = startApp()
  }

  async function doLogout() {
    try {
      await apiLogout()
    } catch {
      /* dropping the session locally is what matters */
    }
    if (stopApp) stopApp()
    stopApp = null
    quests = []
    notes = []
    attachmentIndex = null
    selectedId = null
    selectedNoteId = null
    selectedAttachmentId = null
    authed = false
  }

  onMount(() => {
    void refreshTemplates()
    // Native swipe hosts (Android HubActivity, quest=192) drive tab switches
    // through this instead of a full page reload — keeps the header mounted
    // and lets setView()'s own transition animate the change.
    window.questsNav = {
      setTab: (name: any) => setView(name),
      currentTab: () => view,
    }

    applyTheme(loadSavedTheme())
    // A 401 from any later call means the session lapsed — fall back to login.
    setUnauthorizedHandler(() => {
      if (authed === false) return
      if (stopApp) stopApp()
      stopApp = null
      authed = false
    })

    let cancelled = false
    ;(async () => {
      if (inviteToken) return // the invitation screen owns the page
      let state
      try {
        state = await fetchAuthState()
      } catch {
        // Auth state is unreachable; treat as open so a local instance with a
        // temporarily unhappy API still renders its usual error banner.
        state = { auth_required: false, authenticated: true }
      }
      if (cancelled) return
      authRequired = !!state.auth_required
      if (state.auth_required && !state.authenticated) {
        authed = false
        loading = false
        return
      }
      username = state.username ?? ''
      authed = true
      if (state.auth_required && state.workspace_id === 0) {
        noWorkspace = true
        loading = false
        return
      }
      stopApp = startApp()
    })()

    return () => {
      cancelled = true
      if (stopApp) stopApp()
      stopApp = null
    }
  })

  // Keyboard cursor on the steps of the open quest. Invisible until a step key
  // is used (kbStepId stays null for mouse users); then it follows [ ] and +/-.
  let kbStepId = $state((null as number | null))
  $effect(() => {
    selectedId
    kbStepId = null
  })

  function kbCursor() {
    const steps = selected?.steps ?? []
    if (!steps.length) return null
    return steps.find((s: { id: number }) => s.id === kbStepId) ?? steps.find((s: { done: any }) => !s.done) ?? steps[steps.length - 1]
  }

  function kbMoveStep(delta: number) {
    const steps = selected?.steps ?? []
    const cur = kbCursor()
    if (!cur) return
    if (kbStepId == null) {
      kbStepId = cur.id // first press only reveals the cursor
      return
    }
    const i = steps.findIndex((s: { id: any }) => s.id === cur.id)
    kbStepId = steps[Math.min(steps.length - 1, Math.max(0, i + delta))].id
  }

  function kbBumpStep(delta: number) {
    const cur = kbCursor()
    if (!cur || !selected) return false
    kbStepId = cur.id
    void bumpStep(cur, delta, selected.id)
  }

  function kbToggleStep() {
    const cur = kbCursor()
    if (!cur || !selected) return false
    kbStepId = cur.id
    const total = Math.max(0, Number(cur.progress_total) || 0)
    const done = cur.progress_current >= total
    void setStepProgress(cur, done ? Math.max(0, total - 1) : total, selected.id)
  }

  // Keyboard control (design: note=76). The dispatcher itself stays silent in
  // text fields, modals and context menus, so these only run on the bare page.
  $effect(() => {
    const unregister = registerHotkeys([
      {
        id: 'tabs',
        group: 'Вкладки',
        label: 'Журнал, заметки, вложения, календарь, история, карта',
        keys: '1–6',
        codes: TAB_ORDER.flatMap((_, i) => [`Digit${i + 1}`, `Numpad${i + 1}`]),
        run: (event: { code: string|any[] }) => {
          const i = Number(event.code.slice(-1)) - 1
          if (TAB_ORDER[i] === view) return false
          setView(TAB_ORDER[i])
        },
      },
      {
        id: 'open-quest',
        group: 'Окна',
        label: 'Новый квест',
        keys: 'N',
        codes: ['KeyN'],
        run: () => openCreate(),
      },
      {
        id: 'open-questline',
        group: 'Окна',
        label: 'Новый квестлайн',
        keys: 'L',
        codes: ['KeyL'],
        run: () => openCreateQuestline(),
      },
      {
        id: 'open-templates',
        group: 'Окна',
        label: 'Шаблоны',
        keys: 'T',
        codes: ['KeyT'],
        run: () => openTemplates(),
      },
      {
        id: 'open-assistant',
        group: 'Окна',
        label: 'Ассистент',
        keys: 'A',
        codes: ['KeyA'],
        run: () => (assistantOpen = true),
      },
      {
        id: 'open-settings',
        group: 'Окна',
        label: 'Настройки',
        keys: ',',
        codes: ['Comma'],
        run: () => openSettings(),
      },
      {
        id: 'open-help',
        group: 'Окна',
        label: 'Справка по клавишам',
        keys: '?',
        codes: ['Slash'],
        shift: true,
        run: () => openSettings({ section: 'hotkeys' }),
      },
      {
        id: 'list-down',
        group: 'Список',
        label: 'Следующая строка (квест, группа, заметка)',
        keys: '↓  J',
        codes: ['ArrowDown', 'KeyJ'],
        when: () => ['journal', 'notes'].includes(view),
        run: () => moveRow(1),
      },
      {
        id: 'list-up',
        group: 'Список',
        label: 'Предыдущая строка',
        keys: '↑  K',
        codes: ['ArrowUp', 'KeyK'],
        when: () => ['journal', 'notes'].includes(view),
        run: () => moveRow(-1),
      },
      {
        id: 'list-expand',
        group: 'Список',
        label: 'Развернуть группу',
        keys: '→',
        codes: ['ArrowRight'],
        when: () => ['journal', 'notes'].includes(view),
        run: () => foldRow(true),
      },
      {
        id: 'list-collapse',
        group: 'Список',
        label: 'Свернуть группу',
        keys: '←',
        codes: ['ArrowLeft'],
        when: () => ['journal', 'notes'].includes(view),
        run: () => foldRow(false),
      },
      {
        id: 'step-prev',
        group: 'Шаги открытого квеста',
        label: 'Предыдущий шаг',
        keys: '[',
        codes: ['BracketLeft'],
        when: () => view === 'journal' && !!selected?.steps?.length,
        run: () => kbMoveStep(-1),
      },
      {
        id: 'step-next',
        group: 'Шаги открытого квеста',
        label: 'Следующий шаг',
        keys: ']',
        codes: ['BracketRight'],
        when: () => view === 'journal' && !!selected?.steps?.length,
        run: () => kbMoveStep(1),
      },
      {
        id: 'step-plus',
        group: 'Шаги открытого квеста',
        label: 'Прогресс шага +1',
        keys: '+',
        codes: ['Equal', 'NumpadAdd'],
        shift: 'any',
        when: () => view === 'journal' && !!selected?.steps?.length,
        run: () => kbBumpStep(1),
      },
      {
        id: 'step-minus',
        group: 'Шаги открытого квеста',
        label: 'Прогресс шага −1',
        keys: '−',
        codes: ['Minus', 'NumpadSubtract'],
        when: () => view === 'journal' && !!selected?.steps?.length,
        run: () => kbBumpStep(-1),
      },
      {
        id: 'step-toggle',
        group: 'Шаги открытого квеста',
        label: 'Завершить шаг (повторно — вернуть на единицу назад)',
        keys: 'Пробел',
        codes: ['Space'],
        when: () => view === 'journal' && !!selected?.steps?.length,
        run: () => kbToggleStep(),
      },
      {
        id: 'clear-selection',
        group: 'Список',
        label: 'Снять выбор квеста',
        keys: 'Esc',
        codes: ['Escape'],
        when: () => view === 'journal' && selectedId != null,
        run: () => clearSelectedQuest(),
      },
    ])
    const detach = installHotkeys()
    return () => {
      detach()
      unregister()
    }
  })
</script>

{#if inviteToken}
  <InviteScreen token={inviteToken} />
{:else if noWorkspace}
  <NoWorkspaceScreen />
{:else if authed === false}
  <LoginScreen {onAuthenticated} />
{:else if authed === null}
  <div class="boot-gate"></div>
{:else}
<div class="journal">
  <JournalHeader
    {view}
    {liveStatus}
    {health}
    sidebarCollapsed={view === 'notes' ? sidebarCollapsed.notes : sidebarCollapsed.journal}
    onToggleSidebar={() => toggleSidebar(view === 'notes' ? 'notes' : 'journal')}
    onViewChange={(v) => setView(v)}
    onOpenSettings={openSettings}
    onOpenTemplates={openTemplates}
    onOpenSecrets={() => openSecrets()}
    onOpenTags={openTags}
    onOpenCreateQuestline={openCreateQuestline}
    onOpenCreateQuest={() => openCreate()}
    onOpenAssistant={() => (assistantOpen = true)}
  />

  {#if error}
    <p class="banner-error" role="alert">{error}</p>
  {/if}

  <ToastHost />

  <div class="journal__stage" use:tabSwipe={swipeTab}>
  {#key view}
  <div
    class="journal__viewport"
    in:slideTab={{ dir: navDir }}
    out:slideTab={{ dir: navDir, out: true }}
  >
  {#if view === 'calendar'}
    <div class="journal__calendar">
      <ActivityCalendar
        {quests}
        labels={refLabels}
        onRef={onJournalRef}
        onSelectQuest={selectQuestFromUi}
        onOpenHistory={openHistoryDay}
        onQuestContextMenu={(event, id) => {
          const quest = quests.find((q) => q.id === id)
          if (quest) menu.openQuest(event, quest)
        }}
      />
    </div>
  {:else if view === 'notes'}
    <div class="journal__notes">
      <NotesPage
        {notes}
        {quests}
        {questlines}
        {attachments}
        selectedId={selectedNoteId}
        labels={refLabels}
        sidebarCollapsed={sidebarCollapsed.notes}
        onSelect={(id) => selectNoteFromUi(id)}
        onChanged={() => load({ silent: true })}
        onRef={onJournalRef}
        onOpenSecrets={(id) => openSecrets({ kind: 'note', id })}
      />
    </div>
  {:else if view === 'graph'}
    <!-- drawn below, outside the keyed viewport -->
  {:else if view === 'runners'}
    <div class="journal__runners">
      <RunnersPage active={view === 'runners'} onOpenQuest={(id) => selectQuestFromUi(id)} />
    </div>
  {:else if view === 'history'}
    <div class="journal__history">
      <HistoryPage
        active={view === 'history'}
        nonce={historyNonce}
        {categories}
        {questlines}
        onOpen={onJournalRef}
      />
    </div>
  {:else if view === 'attachments'}
    <div class="journal__attachments">
      <AttachmentsPage
        attachments={allAttachments}
        {quests}
        {questlines}
        {notes}
        selectedId={selectedAttachmentId}
        onSelect={(id) => selectAttachmentFromUi(id)}
        onOpenOwner={onJournalRef}
        onChanged={() => load({ silent: true })}
      />
    </div>
  {:else}
    <div class="journal__body" class:journal__body--sidebar-collapsed={sidebarCollapsed.journal}>
      <QuestSidebar
        {loading}
        {quests}
        {matchedQuests}
        {listedQuests}
        {byCategory}
        {frozenQuests}
        {frozenOpen}
        onToggleFrozen={toggleFrozen}
        {categories}
        {scopeCategoryId}
        {scopeQuestlineId}
        {scopeQuestlineOptions}
        onScopeCategory={setScopeCategory}
        onScopeQuestline={setScopeQuestline}
        bind:searchQuery
        {showAllQuests}
        onShowAll={setShowAll}
        querySpec={questQuerySpec}
        parsedSearch={parsedQuestSearch}
        {selectedId}
        {categoryOpen}
        {lineOpen}
        {nowMs}
        onSelect={(id) => selectQuestFromUi(id)}
        onTogglePin={togglePin}
        onQuestContextMenu={menu.openQuest}
        onLineContextMenu={menu.openLine}
        onToggleCategory={toggleCategory}
        onToggleLine={toggleLine}
        labels={refLabels}
        onRef={onJournalRef}
      />
      <QuestDetail
        onOpenGraph={openGraph}
        {selected}
        {kbStepId}
        {quests}
        {showAllQuests}
        {nowMs}
        {statusBusy}
        {deleting}
        {deleteAnimActive}
        {consumeRemoteEdit}
        onDeleteAnimDone={onDeleteAnimDone}
        {stepBusyId}
        {stepEditId}
        {stepEditValue}
        onToggleCompleted={toggleCompleted}
        onOpenEdit={(q) => openEdit(q)}
        onRequestDelete={requestDeleteSelected}
        onBumpStep={bumpStep}
        onBeginEditStep={beginEditStep}
        onStepEditKeydown={onStepEditKeydown}
        onStepEditBlur={onStepEditBlur}
        onStepEditInput={(v) => (steps.editValue = v)}
        onQuestTitleContextMenu={menu.openQuest}
        onLineHeadContextMenu={(e) => {
          const line = questlines.find((l) => l.id === selected?.questline_id)
          if (line) menu.openLine(e, line)
        }}
        onStepContextMenu={menu.openStep}
        onSelectQuest={(id) => selectQuestFromUi(id)}
        labels={refLabels}
        onRef={onJournalRef}
        notes={notes}
        {questlines}
      />
    </div>
  {/if}
  </div>
  {/key}
  <!-- The map stays mounted once opened (just hidden), so coming back is instant and keeps its view. -->
  {#if graphSeen}
    <div class="journal__viewport journal__viewport--keep" bind:this={graphEl} style:display={graphOn ? undefined : 'none'}>
    <div class="journal__graph">
      <GraphPage
        active={view === 'graph'}
        rootType={graphRoot.type}
        rootId={graphRoot.id}
        nonce={graphNonce}
        {quests}
        {notes}
        {questlines}
        onCenter={onGraphCenter}
        onOpen={onGraphOpen}
      />
    </div>
    </div>
  {/if}

  </div>
</div>

<QuestModal
  open={modalOpen}
  mode={modalMode}
  quest={modalMode === 'edit' ? selected : null}
  defaults={modalMode === 'create' ? modalDefaults : null}
  {quests}
  {notes}
  {attachments}
  onClose={() => {
    modalOpen = false
    modalDefaults = null
  }}
  onSaved={onSaved}
  onDeleted={onDeleted}
/>

<StepModal
  open={stepModalOpen}
  questId={stepModalQuestId}
  step={stepModalStep}
  {quests}
  {questlines}
  {notes}
  {attachments}
  onClose={() => {
    stepModalOpen = false
    stepModalStep = null
    stepModalQuestId = null
  }}
  onSaved={(q) => applyQuest(q, { select: false })}
/>

<QuestlineModal
  open={lineModalOpen}
  mode={lineModalMode}
  line={lineModalMode === 'edit' ? lineModalTarget : null}
  {quests}
  {questlines}
  {notes}
  {attachments}
  onClose={() => {
    lineModalOpen = false
    lineModalTarget = null
  }}
  onSaved={onQuestlineSaved}
  onDeleted={onQuestlineDeleted}
/>

<TemplatesModal
  open={templatesOpen}
  {quests}
  {notes}
  {attachments}
  focusId={templatesFocusId}
  onClose={() => {
    templatesOpen = false
    templatesFocusId = null
  }}
  onChanged={() => {
    void load({ silent: true })
    void refreshTemplates()
  }}
  onOpenSecrets={(id) => openSecrets({ kind: 'template', id })}
/>

<SecretsModal
  open={secretsOpen}
  owner={secretsOwner}
  {quests}
  {questlines}
  labels={refLabels}
  onClose={() => (secretsOpen = false)}
/>

<TagsModal open={tagsOpen} onClose={() => (tagsOpen = false)} />

<SettingsModal
  open={settingsOpen}
  onClose={() => (settingsOpen = false)}
  {health}
  {liveStatus}
  {username}
  section={settingsSection}
  onLogout={authRequired ? doLogout : null}
/>

<ActionAssistantModal
  open={assistantOpen}
  {quests}
  {questlines}
  notes={notes}
  {attachments}
  onClose={() => (assistantOpen = false)}
  onApplied={() => load({ silent: true })}
/>

<ContextMenu
  open={menu.open}
  x={menu.x}
  y={menu.y}
  items={menu.items}
  onSelect={menu.select}
  onClose={menu.close}
/>

<ConfirmModal
  open={deleteConfirmOpen}
  title="Удалить квест?"
  message={selected ? `Удалить квест «${selected.title}»?` : ''}
  busy={deleting}
  onCancel={() => {
    if (!deleting) deleteConfirmOpen = false
  }}
  onConfirm={confirmDeleteSelected}
/>

<ConfirmModal
  open={stepDeleteOpen}
  title="Удалить шаг?"
  message={stepDelete ? `Удалить шаг «${stepDelete.title}»?` : ''}
  busy={stepDeleteBusy}
  onCancel={() => {
    if (!stepDeleteBusy) {
      stepDeleteOpen = false
      stepDelete = null
    }
  }}
  onConfirm={confirmDeleteStep}
/>

<ConfirmModal
  open={lineDeleteConfirmOpen}
  title="Удалить квестлайн?"
  message="Квесты останутся, но отвяжутся от линии."
  busy={lineDeleting}
  onCancel={() => {
    if (!lineDeleting) lineDeleteConfirmOpen = false
  }}
  onConfirm={confirmDeleteQuestline}
/>
{/if}

<style>
  /* Blank canvas while the auth probe is in flight — avoids flashing either
     the login card or the journal before we know which one is right. */
  .boot-gate {
    min-height: 100vh;
    background: var(--color-bg);
  }
</style>
