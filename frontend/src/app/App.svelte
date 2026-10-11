<script lang="ts">
  // Quests web UI — orchestrator (state, load, overlays)
  import { HistoryPage } from '@/pages/history'
import { RunnersPage } from '@/pages/runners'
  import { GraphPage } from '@/pages/graph'
  import { createNavigation, TAB_ORDER } from '@/app/model/navigation.svelte.ts'
  import { createJournalMenu } from '@/app/model/journalMenu.svelte.ts'
  import { createStepProgress, questIdOfStep, stepRef } from '@/app/model/stepProgress.svelte.ts'
  import { createQuestActions } from '@/app/model/questActions.svelte.ts'
  import { buildRefLabels } from '@/app/model/refLabels.ts'
  import {
    attachmentIdFromUrl,
    healthFrom,
    healthOffline,
    noteIdFromUrl,
    questIdFromUrl,
    replaceSearch,
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
  const nav = createNavigation({
    quests: () => quests,
    revealDetail: (tab) => revealDetailOnNarrow(tab),
    revealList: (tab) => revealListOnNarrow(tab),
    openTemplate: (id) => {
      templatesFocusId = id
      templatesOpen = true
    },
  })
  const { setView, swipeTab, openGraph, onGraphCenter, openHistoryDay, clearQuest: clearSelectedQuest, openRef: onJournalRef } = nav
  const selectQuestFromUi = nav.selectQuest
  const selectNoteFromUi = nav.selectNote
  const selectAttachmentFromUi = nav.selectAttachment
  const onGraphOpen = nav.openRef
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
  let selected = $derived(quests.find((q) => q.id === nav.selectedId) ?? null)
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
      const prefer = nav.pendingSelectId ?? nav.selectedId ?? questIdFromUrl()
      if (prefer != null && next.some((q) => q.id === prefer)) {
        nav.selectedId = prefer
        nav.pendingSelectId = null
      } else if (nav.selectedId != null && !next.some((q) => q.id === nav.selectedId)) {
        nav.selectedId = null
      }
      const preferNote = nav.selectedNoteId ?? noteIdFromUrl()
      if (preferNote != null && notes.some((n) => n.id === preferNote)) {
        nav.selectedNoteId = preferNote
      } else if (nav.selectedNoteId != null && !notes.some((n) => n.id === nav.selectedNoteId)) {
        nav.selectedNoteId = null
      }
      const flatAtt = flattenAttachmentIndex(attachmentIndex)
      const preferAtt = nav.selectedAttachmentId ?? attachmentIdFromUrl()
      if (preferAtt != null && flatAtt.some((a) => a.id === preferAtt)) {
        nav.selectedAttachmentId = preferAtt
      } else if (nav.selectedAttachmentId != null && !flatAtt.some((a) => a.id === nav.selectedAttachmentId)) {
        nav.selectedAttachmentId = null
      }
    } catch (e) {
      if (!silent) {
        error = e.message || String(e)
        quests = []
        notes = []
        attachmentIndex = null
        nav.selectedId = null
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
    nav.selectedId = q.id
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
    if (q.id !== nav.selectedId) nav.selectedId = q.id
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
    selectQuest: (id) => (nav.selectedId = id),
    selectedQuestId: () => nav.selectedId,
    copyId: (kind, id) => void copyIdToClipboard(kind, id),
    openSecrets: (owner) => openSecrets(owner),
    openCreate: (defaults) => openCreate(defaults),
    editQuest: (q) => openEdit(q),
    editLine: (line) => openEditQuestline(line),
    askDeleteLine: () => (lineDeleteConfirmOpen = true),
    askDeleteQuest: (q) => {
      nav.selectedId = q.id
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
    if (select) nav.selectedId = quest.id
  }

  function onSaved(quest: { id: any }, meta = ({} as { mode?: string })) {
    nav.selectedId = quest.id
    load({ silent: true })
    if (meta.mode === 'create') toast('Квест создан', { kind: 'success' })
    if (nav.view === 'graph') graphNonce += 1
  }

  function onDeleted(id: any) {
    if (nav.selectedId === id) nav.selectedId = null
    load({ silent: true })
    toast('Квест удалён', { kind: 'success' })
  }

  // Порядок вкладок задаёт направление слайда: см. model/navigation.svelte.ts
  let graphSeen = $state(false)
  let graphOn = $state(nav.view === 'graph')
  let graphEl = $state((null as HTMLDivElement | null))
  let prevView = nav.view
  const SLIDE_MS = 220
  const SLIDE_EASE = 'cubic-bezier(0.215, 0.61, 0.355, 1)'
  $effect(() => {
    const v = nav.view
    if (v === 'graph') graphSeen = true
    const was = prevView
    prevView = v
    if (v === was) return
    const dir = untrack(() => nav.navDir)
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
        if (nav.view !== 'graph') graphOn = false
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
    nav.start()
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
          if (nav.view === 'history') historyNonce += 1
          if (nav.view === 'graph') graphNonce += 1
        }
      },
      {
        onStatus: (s) => (liveStatus = s),
        // The socket missed events while it was down or the tab was hidden.
        onResync: () => {
          void load({ silent: true })
          void refreshTemplates()
          void refreshHealth()
          if (nav.view === 'history') historyNonce += 1
          if (nav.view === 'graph') graphNonce += 1
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
    nav.selectedId = null
    nav.selectedNoteId = null
    nav.selectedAttachmentId = null
    authed = false
  }

  onMount(() => {
    void refreshTemplates()
    // Native swipe hosts (Android HubActivity, quest=192) drive tab switches
    // through this instead of a full page reload — keeps the header mounted
    // and lets setView()'s own transition animate the change.
    window.questsNav = {
      setTab: (name: any) => setView(name),
      currentTab: () => nav.view,
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
    nav.selectedId
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
          if (TAB_ORDER[i] === nav.view) return false
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
        when: () => ['journal', 'notes'].includes(nav.view),
        run: () => moveRow(1),
      },
      {
        id: 'list-up',
        group: 'Список',
        label: 'Предыдущая строка',
        keys: '↑  K',
        codes: ['ArrowUp', 'KeyK'],
        when: () => ['journal', 'notes'].includes(nav.view),
        run: () => moveRow(-1),
      },
      {
        id: 'list-expand',
        group: 'Список',
        label: 'Развернуть группу',
        keys: '→',
        codes: ['ArrowRight'],
        when: () => ['journal', 'notes'].includes(nav.view),
        run: () => foldRow(true),
      },
      {
        id: 'list-collapse',
        group: 'Список',
        label: 'Свернуть группу',
        keys: '←',
        codes: ['ArrowLeft'],
        when: () => ['journal', 'notes'].includes(nav.view),
        run: () => foldRow(false),
      },
      {
        id: 'step-prev',
        group: 'Шаги открытого квеста',
        label: 'Предыдущий шаг',
        keys: '[',
        codes: ['BracketLeft'],
        when: () => nav.view === 'journal' && !!selected?.steps?.length,
        run: () => kbMoveStep(-1),
      },
      {
        id: 'step-next',
        group: 'Шаги открытого квеста',
        label: 'Следующий шаг',
        keys: ']',
        codes: ['BracketRight'],
        when: () => nav.view === 'journal' && !!selected?.steps?.length,
        run: () => kbMoveStep(1),
      },
      {
        id: 'step-plus',
        group: 'Шаги открытого квеста',
        label: 'Прогресс шага +1',
        keys: '+',
        codes: ['Equal', 'NumpadAdd'],
        shift: 'any',
        when: () => nav.view === 'journal' && !!selected?.steps?.length,
        run: () => kbBumpStep(1),
      },
      {
        id: 'step-minus',
        group: 'Шаги открытого квеста',
        label: 'Прогресс шага −1',
        keys: '−',
        codes: ['Minus', 'NumpadSubtract'],
        when: () => nav.view === 'journal' && !!selected?.steps?.length,
        run: () => kbBumpStep(-1),
      },
      {
        id: 'step-toggle',
        group: 'Шаги открытого квеста',
        label: 'Завершить шаг (повторно — вернуть на единицу назад)',
        keys: 'Пробел',
        codes: ['Space'],
        when: () => nav.view === 'journal' && !!selected?.steps?.length,
        run: () => kbToggleStep(),
      },
      {
        id: 'clear-selection',
        group: 'Список',
        label: 'Снять выбор квеста',
        keys: 'Esc',
        codes: ['Escape'],
        when: () => nav.view === 'journal' && nav.selectedId != null,
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
    view={nav.view}
    {liveStatus}
    {health}
    sidebarCollapsed={nav.view === 'notes' ? sidebarCollapsed.notes : sidebarCollapsed.journal}
    onToggleSidebar={() => toggleSidebar(nav.view === 'notes' ? 'notes' : 'journal')}
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
  {#key nav.view}
  <div
    class="journal__viewport"
    in:slideTab={{ dir: nav.navDir }}
    out:slideTab={{ dir: nav.navDir, out: true }}
  >
  {#if nav.view === 'calendar'}
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
  {:else if nav.view === 'notes'}
    <div class="journal__notes">
      <NotesPage
        {notes}
        {quests}
        {questlines}
        {attachments}
        selectedId={nav.selectedNoteId}
        labels={refLabels}
        sidebarCollapsed={sidebarCollapsed.notes}
        onSelect={(id) => selectNoteFromUi(id)}
        onChanged={() => load({ silent: true })}
        onRef={onJournalRef}
        onOpenSecrets={(id) => openSecrets({ kind: 'note', id })}
      />
    </div>
  {:else if nav.view === 'graph'}
    <!-- drawn below, outside the keyed viewport -->
  {:else if nav.view === 'runners'}
    <div class="journal__runners">
      <RunnersPage active={nav.view === 'runners'} onOpenQuest={(id) => selectQuestFromUi(id)} />
    </div>
  {:else if nav.view === 'history'}
    <div class="journal__history">
      <HistoryPage
        active={nav.view === 'history'}
        nonce={historyNonce}
        {categories}
        {questlines}
        onOpen={onJournalRef}
      />
    </div>
  {:else if nav.view === 'attachments'}
    <div class="journal__attachments">
      <AttachmentsPage
        attachments={allAttachments}
        {quests}
        {questlines}
        {notes}
        selectedId={nav.selectedAttachmentId}
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
        selectedId={nav.selectedId}
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
  <!-- The map stays mounted once opened (just hidden), so coming back is instant and keeps its nav.view. -->
  {#if graphSeen}
    <div class="journal__viewport journal__viewport--keep" bind:this={graphEl} style:display={graphOn ? undefined : 'none'}>
    <div class="journal__graph">
      <GraphPage
        active={nav.view === 'graph'}
        rootType={nav.graphRoot.type}
        rootId={nav.graphRoot.id}
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
