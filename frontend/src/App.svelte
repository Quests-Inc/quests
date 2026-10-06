<script>
  // Quests web UI — orchestrator (state, load, overlays)
  import HistoryPage from './lib/blocks/HistoryPage.svelte'
import RunnersPage from './lib/blocks/RunnersPage.svelte'
  import GraphPage from './lib/blocks/GraphPage.svelte'
  import { onMount, tick, untrack } from 'svelte'
  import { cubicOut } from 'svelte/easing'
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
  } from './lib/js/api.js'
  import { subscribeQuestEvents } from './lib/js/live.js'
  import { seedAttachmentIndex, invalidateAttachmentLiveFlags, flattenAttachmentIndex } from './lib/js/attachmentCache.js'
  import { applyTheme, loadSavedTheme } from './lib/js/theme.js'
  import { questMatchesQuery } from './lib/js/search.js'
  import { OPEN_STATUSES } from './lib/js/questFormat.js'
  import { groupQuestsByCategory } from './lib/js/questGroups.js'
  import { copyText } from './lib/js/clipboard.js'
  import { installHotkeys, registerHotkeys } from './lib/js/hotkeys.js'
  import { foldRow, moveRow } from './lib/js/listNav.js'
    import { statusToastLabel, toast, toastDone, toastProgress } from './lib/js/toasts.svelte.js'
  import { localTimeZone } from './lib/js/time.js'
  import { downloadQuestMarkdown, downloadQuestPdf } from './lib/js/questExport.js'
  import { downloadQuestPdfSimple } from './lib/js/questPdf.js'
  import ActionAssistantModal from './lib/modals/ActionAssistantModal.svelte'
  import QuestModal from './lib/modals/QuestModal.svelte'
  import StepModal from './lib/modals/StepModal.svelte'
  import QuestlineModal from './lib/modals/QuestlineModal.svelte'
  import TemplatesModal from './lib/modals/TemplatesModal.svelte'
  import SecretsModal from './lib/modals/SecretsModal.svelte'
  import { refreshTemplates, templateStore } from './lib/js/templatesStore.svelte.js'
  import TagsModal from './lib/modals/TagsModal.svelte'
  import SettingsModal from './lib/modals/SettingsModal.svelte'
  import ConfirmModal from './lib/modals/ConfirmModal.svelte'
  import ContextMenu from './lib/ui/ContextMenu.svelte'
  import ToastHost from './lib/ui/ToastHost.svelte'
  import JournalHeader from './lib/blocks/JournalHeader.svelte'
  import QuestSidebar from './lib/blocks/QuestSidebar.svelte'
  import QuestDetail from './lib/blocks/QuestDetail.svelte'
  import ActivityCalendar from './lib/blocks/ActivityCalendar.svelte'
  import TocPage from './lib/blocks/TocPage.svelte'
  import NotesPage from './lib/blocks/NotesPage.svelte'
  import AttachmentsPage from './lib/blocks/AttachmentsPage.svelte'
  import LoginScreen from './lib/blocks/LoginScreen.svelte'

  /** Auth gate: null = still checking, true/false = decided. */
  let authed = $state(null)
  /** Whether this instance uses accounts at all (false on an open local one). */
  let authRequired = $state(false)
  let username = $state('')
  /** Cleanup for the live subscription + timers, so logout can tear them down. */
  let stopApp = null

  let quests = $state([])
  let notes = $state([])
  /** @type {Record<string, any> | null} */
  let attachmentIndex = $state(null)
  let selectedId = $state(null)
  let selectedNoteId = $state(/** @type {number | null} */ (null))
  let selectedAttachmentId = $state(/** @type {number | null} */ (null))
  let loading = $state(true)
  let error = $state('')
  let liveStatus = $state('off')
  /** @type {{ api: string, overlay: string, telegram: string, webdav: string, db?: string, detail?: Record<string, any> }} */
  let health = $state({
    api: 'unknown',
    overlay: 'unknown',
    telegram: 'unknown',
    webdav: 'unknown',
    db: 'unknown',
  })
  let searchQuery = $state('')
  /** @type {'journal' | 'toc' | 'notes' | 'attachments' | 'calendar' | 'history' | 'graph' | 'runners'} */
  let view = $state('journal')
  /** Sidebar visibility per view that has one — remembered across reloads. */
  let sidebarCollapsed = $state(loadSidebarCollapsed())

  function loadSidebarCollapsed() {
    try {
      const raw = JSON.parse(localStorage.getItem('quests.sidebarCollapsed') || '{}')
      return { journal: !!raw.journal, notes: !!raw.notes }
    } catch {
      return { journal: false, notes: false }
    }
  }

  function toggleSidebar(key) {
    setSidebarCollapsed(key, !sidebarCollapsed[key])
  }

  function setSidebarCollapsed(key, value) {
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
  function revealDetailOnNarrow(key) {
    if (isNarrowViewport()) setSidebarCollapsed(key, true)
  }

  /** …and going back to the list should bring it back, not leave an empty detail pane. */
  function revealListOnNarrow(key) {
    if (isNarrowViewport()) setSidebarCollapsed(key, false)
  }
  /** @type {{ id: number, slug: string, label: string, sort_order: number, color?: string }[]} */
  /** Centre of the relations map tab; null until picked or opened from a card. */
  let graphRoot = $state(/** @type {{ type: string | null, id: number | null }} */ ({ type: null, id: null }))
  /** Bump to refresh the map after live events. */
  let graphNonce = $state(0)
  /** Bump to refresh the history feed after live events. */
  let historyNonce = $state(0)
  let categories = $state([])
  /** @type {{ id: number, title: string, category_id?: number | null, color?: string, icon?: string }[]} */
  let questlines = $state([])
  /** Open state for category subgroups: `${bucket}:${key}` → boolean (default true). */
  let categoryOpen = $state(/** @type {Record<string, boolean>} */ ({}))
  /** Open state for questline subgroups: `${bucket}:${catKey}:${lineKey}` → boolean. */
  let lineOpen = $state(/** @type {Record<string, boolean>} */ ({}))
  /** Sidebar "Заморожено (N)" tab — collapsed by default. */
  let frozenOpen = $state(false)
  function toggleFrozen() {
    frozenOpen = !frozenOpen
  }

  let modalOpen = $state(false)
  let modalMode = $state(/** @type {'create' | 'edit'} */ ('create'))
  /** Prefill when creating from questline context menu. */
  let modalDefaults = $state(
    /** @type {{ questline_id?: number | null, category_id?: number | null } | null} */ (null),
  )
  let templatesOpen = $state(false)
  let secretsOpen = $state(false)
  /** entity the secrets modal opens on (context menu → «Секреты»), or null */
  let secretsOwner = $state(/** @type {{ kind: string, id: number } | null} */ (null))
  /** template the templates modal opens for editing (a template=N link) */
  let templatesFocusId = $state(/** @type {number | null} */ (null))
  let tagsOpen = $state(false)
  let settingsOpen = $state(false)
  /** Section the settings modal scrolls to when opened (e.g. 'hotkeys'). */
  let settingsSection = $state(/** @type {string | null} */ (null))
  let assistantOpen = $state(false)
  let lineModalOpen = $state(false)
  let lineModalMode = $state(/** @type {'create' | 'edit'} */ ('create'))
  let lineModalTarget = $state(/** @type {any | null} */ (null))
  let ctxOpen = $state(false)
  let ctxX = $state(0)
  let ctxY = $state(0)
  /** @type {'line' | 'quest' | 'step' | null} */
  let ctxKind = $state(null)
  let ctxLineId = $state(/** @type {number | null} */ (null))
  let ctxQuestId = $state(/** @type {number | null} */ (null))
  let ctxStepId = $state(/** @type {number | null} */ (null))
  let lineDeleteConfirmOpen = $state(false)
  let lineDeleting = $state(false)
  let deleting = $state(false)
  let deleteConfirmOpen = $state(false)
  let stepDeleteOpen = $state(false)
  let stepDeleteBusy = $state(false)
  /** @type {{ questId: number, stepId: number, title: string } | null} */
  let stepDelete = $state(null)
  /** True while QuestDetail is playing its erase-before-delete animation. */
  let deleteAnimActive = $state(false)
  /** @type {(() => void) | null} */
  let deleteAnimResolve = null
  /** quest id → when a live event from outside the web UI touched it. The
   * event lands before the refetched data, so QuestDetail picks the mark up
   * when the text actually changes and decides to type it in. */
  const remoteEdits = new Map()
  const REMOTE_EDIT_TTL_MS = 8000
  function consumeRemoteEdit(questId) {
    const at = remoteEdits.get(questId)
    if (at == null) return false
    remoteEdits.delete(questId)
    return Date.now() - at < REMOTE_EDIT_TTL_MS
  }
  function onDeleteAnimDone() {
    deleteAnimResolve?.()
    deleteAnimResolve = null
  }
  let statusBusy = $state(false)
  let questPdfBusy = false
  let pinBusyId = $state(/** @type {number | null} */ (null))
  let stepBusyId = $state(/** @type {number | null} */ (null))
  let stepEditId = $state(/** @type {number | null} */ (null))
  let stepEditQuestId = $state(/** @type {number | null} */ (null))
  let stepEditValue = $state('')
  let stepModalOpen = $state(false)
  let stepModalQuestId = $state(/** @type {number | null} */ (null))
  let stepModalStep = $state(/** @type {any | null} */ (null))
  /** Prefer this id across in-flight load() (URL / HUD focus). */
  let pendingSelectId = $state(/** @type {number | null} */ (null))
  /** When false — only active/expired; when true — all statuses. */
  let showAllQuests = $state(false)
  /** Sidebar scope narrowing: null = all, else a category, optionally further narrowed to one of its questlines. */
  let scopeCategoryId = $state(/** @type {number | null} */ (null))
  let scopeQuestlineId = $state(/** @type {number | null} */ (null))
  let nowMs = $state(Date.now())

  function setScopeCategory(id) {
    scopeCategoryId = id
    scopeQuestlineId = null
  }
  function setScopeQuestline(id) {
    scopeQuestlineId = id
  }
  /** Questlines to narrow to under the current scope category: those whose own
   * category matches, plus any that hold quests of that category (a questline
   * can lack a category while its quests have one). Limited to ones with an
   * open quest unless "показывать завершённые" is on — a questline with
   * nothing active left isn't a live "session" worth offering to narrow to. */
  let scopeQuestlineOptions = $derived.by(() => {
    if (scopeCategoryId == null) return []
    const ids = new Set(
      questlines.filter((l) => (l.category_id ?? null) === scopeCategoryId).map((l) => l.id),
    )
    for (const q of quests) {
      if (q.questline_id != null && (q.category_id ?? null) === scopeCategoryId) {
        ids.add(q.questline_id)
      }
    }
    const inCategory = questlines.filter((l) => ids.has(l.id))
    if (showAllQuests) return inCategory
    const liveLineIds = new Set(
      quests
        .filter((q) => OPEN_STATUSES.has(q.status) && q.questline_id != null)
        .map((q) => q.questline_id),
    )
    return inCategory.filter((l) => liveLineIds.has(l.id))
  })

  let ctxItems = $derived.by(() => {
    if (ctxKind === 'line') {
      return [
        { id: 'copy-id', label: `Копировать questline=${ctxLineId}` },
        { id: 'sep-copy', sep: true },
        { id: 'add', label: 'Добавить квест' },
        { id: 'edit', label: 'Редактировать' },
        { id: 'icon', label: 'Иконка' },
        { id: 'secrets', label: 'Секреты' },
        { id: 'sep-danger', sep: true },
        { id: 'delete', label: 'Удалить', danger: true },
      ]
    }
    if (ctxKind === 'quest') {
      const quest = quests.find((q) => q.id === ctxQuestId)
      const status = quest?.status
      const significance = quest?.significance || 'common'
      /** @type {{ id: string, label?: string, sep?: boolean, checked?: boolean }[]} */
      const delayChildren = [
        { id: 'delay-15', label: '15 мин' },
        { id: 'delay-30', label: '30 мин' },
        { id: 'delay-60', label: '60 мин' },
        { id: 'delay-90', label: '90 мин' },
        { id: 'delay-120', label: '120 мин' },
      ]
      if (status === 'frozen') {
        delayChildren.push(
          { id: 'sep-undelay', sep: true },
          { id: 'status:active', label: 'Вернуть в активные' },
        )
      }
      return [
        { id: 'copy-id', label: `Копировать quest=${ctxQuestId}` },
        { id: 'sep-copy', sep: true },
        { id: 'edit', label: 'Редактировать' },
        { id: 'sep-groups', sep: true },
        {
          id: 'status',
          label: 'Изменить статус',
          children: QUEST_STATUSES.map((s) => ({
            id: `status:${s}`,
            label: QUEST_STATUS_LABELS[s] || s,
            checked: s === status,
          })),
        },
        { id: 'delay', label: 'Отложить', children: delayChildren },
        {
          id: 'sig',
          label: 'Значимость',
          children: QUEST_SIGNIFICANCES.map((s) => ({
            id: `sig:${s.id}`,
            label: s.label,
            checked: s.id === significance,
          })),
        },
        { id: 'secrets', label: 'Секреты' },
        {
          id: 'export',
          label: 'Экспорт',
          children: [
            { id: 'export-md', label: 'Markdown (.md)' },
            { id: 'export-pdf', label: 'PDF' },
            { id: 'export-pdf-simple', label: 'PDF (просто)' },
          ],
        },
        { id: 'sep-danger', sep: true },
        { id: 'delete', label: 'Удалить', danger: true },
      ]
    }
    if (ctxKind === 'step') {
      const found = findStepRef(ctxStepId)
      const total = Math.max(1, Number(found?.step?.progress_total) || 1)
      /** @type {{ id: string, label?: string, sep?: boolean, danger?: boolean, children?: { id: string, label?: string, sep?: boolean }[] }[]} */
      const items = [{ id: 'copy-id', label: `Копировать step=${ctxStepId}` }]
      if (total > 1) {
        items.push({
          id: 'progress',
          label: 'Прогресс',
          children: [
            { id: 'inc', label: '+1' },
            { id: 'dec', label: '−1' },
            { id: 'sep-done', sep: true },
            { id: 'complete', label: 'Выполнить' },
            { id: 'reset', label: 'Обнулить' },
          ],
        })
      } else {
        items.push(
          { id: 'sep-done', sep: true },
          { id: 'complete', label: 'Выполнить' },
          { id: 'reset', label: 'Обнулить' },
        )
      }
      items.push(
        { id: 'sep-edit', sep: true },
        { id: 'edit', label: 'Редактировать' },
        { id: 'secrets', label: 'Секреты' },
        { id: 'sep-danger', sep: true },
        { id: 'delete', label: 'Удалить шаг', danger: true },
      )
      return items
    }
    return []
  })

  let matchedQuests = $derived(
    quests.filter((q) => questMatchesQuery(q, searchQuery)),
  )
  let listedQuests = $derived(
    showAllQuests
      ? matchedQuests
      : matchedQuests.filter((q) => OPEN_STATUSES.has(q.status)),
  )
  let scopedQuests = $derived(
    listedQuests.filter((q) => {
      if (scopeCategoryId != null && (q.category_id ?? null) !== scopeCategoryId) return false
      if (scopeQuestlineId != null && q.questline_id !== scopeQuestlineId) return false
      return true
    }),
  )
  let byCategory = $derived(
    groupQuestsByCategory(scopedQuests, categories, questlines),
  )
  /** "Заморожено" — own sidebar tab, independent of showAllQuests/OPEN_STATUSES. */
  let frozenQuests = $derived(
    matchedQuests.filter((q) => {
      if (q.status !== 'frozen') return false
      if (scopeCategoryId != null && (q.category_id ?? null) !== scopeCategoryId) return false
      if (scopeQuestlineId != null && q.questline_id !== scopeQuestlineId) return false
      return true
    }),
  )
  let selected = $derived(quests.find((q) => q.id === selectedId) ?? null)
  /** Every card, hidden included — only the Attachments tab shows hidden ones. */
  let allAttachments = $derived(flattenAttachmentIndex(attachmentIndex))
  let attachments = $derived(allAttachments.filter((a) => !a.hidden))
  let refLabels = $derived.by(() => {
    /** @type {Record<string, string>} */
    const m = {}
    for (const n of notes) m[`note:${n.id}`] = n.title || `note=${n.id}`
    for (const q of quests) {
      m[`quest:${q.id}`] = q.title || `quest=${q.id}`
      for (const s of q.steps || []) {
        m[`step:${s.id}`] = s.title || `step=${s.id}`
      }
    }
    for (const l of questlines) m[`questline:${l.id}`] = l.title || `questline=${l.id}`
    for (const t of templateStore.list) m[`template:${t.id}`] = t.title || `template=${t.id}`
    for (const a of allAttachments) {
      m[`attachment:${a.id}`] = a.filename || `attachment=${a.id}`
    }
    return m
  })

  function isCategoryOpen(key) {
    return categoryOpen[key] !== false
  }

  function toggleCategory(key) {
    categoryOpen = { ...categoryOpen, [key]: !isCategoryOpen(key) }
  }

  function isLineOpen(catKey, lineKey) {
    const id = `${catKey}:${lineKey}`
    return lineOpen[id] !== false
  }

  function toggleLine(catKey, lineKey) {
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
        listNotes().catch(() => []),
        listAllAttachments().catch(() => null),
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

  function openCreate(defaults = null) {
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

  /** @param {{ kind: string, id: number } | null} [owner] */
  function openSecrets(owner = null) {
    secretsOwner = owner
    secretsOpen = true
  }

  function openTags() {
    tagsOpen = true
  }

  /** @param {{ section?: string } | Event | null} [opts] */
  function openSettings(opts = null) {
    settingsSection = opts && !(opts instanceof Event) ? (opts.section ?? null) : null
    settingsOpen = true
  }

  function openCreateQuestline() {
    lineModalMode = 'create'
    lineModalTarget = null
    lineModalOpen = true
  }

  function openEditQuestline(line) {
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

  function openLineContextMenu(event, line) {
    event.preventDefault()
    event.stopPropagation()
    ctxKind = 'line'
    ctxLineId = line.id
    ctxQuestId = null
    ctxStepId = null
    ctxX = event.clientX
    ctxY = event.clientY
    ctxOpen = true
  }

  function openQuestContextMenu(event, quest) {
    event.preventDefault()
    event.stopPropagation()
    ctxKind = 'quest'
    ctxQuestId = quest.id
    ctxLineId = null
    ctxStepId = null
    selectedId = quest.id
    ctxX = event.clientX
    ctxY = event.clientY
    ctxOpen = true
  }

  function openStepContextMenu(event, step) {
    event.preventDefault()
    event.stopPropagation()
    ctxKind = 'step'
    ctxStepId = step.id
    ctxQuestId = findQuestIdForStep(step.id) ?? selectedId
    ctxLineId = null
    ctxX = event.clientX
    ctxY = event.clientY
    ctxOpen = true
  }

  function closeContextMenu() {
    ctxOpen = false
    ctxKind = null
  }

  async function copyIdToClipboard(kind, id) {
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

  function onLineContextSelect(action) {
    if (action === 'copy-id') {
      copyIdToClipboard('questline', ctxLineId)
      return
    }
    if (action === 'secrets') {
      if (ctxLineId != null) openSecrets({ kind: 'questline', id: ctxLineId })
      return
    }
    const line = questlines.find((l) => l.id === ctxLineId)
    if (!line && action !== 'delete') return
    if (action === 'add') {
      openCreate({
        questline_id: ctxLineId,
        category_id: line?.category_id ?? null,
      })
      return
    }
    if (action === 'edit' || action === 'icon') {
      openEditQuestline(line)
      return
    }
    if (action === 'delete') {
      lineDeleteConfirmOpen = true
    }
  }

  async function patchQuestStatus(quest, status) {
    if (!quest || statusBusy) return
    statusBusy = true
    error = ''
    try {
      const saved = await updateQuest(quest.id, { status })
      applyQuest(saved)
      toast(`Статус: ${statusToastLabel(status)}`, { kind: 'success' })
    } catch (e) {
      error = e.message || String(e)
      toast(error, { kind: 'error' })
    } finally {
      statusBusy = false
    }
  }

  async function postponeQuest(quest, minutes) {
    if (!quest || statusBusy) return
    statusBusy = true
    error = ''
    try {
      const secs = Math.max(60, Math.round(Number(minutes) * 60))
      const deadline = new Date(Date.now() + secs * 1000).toISOString()
      const saved = await updateQuest(quest.id, {
        status: 'active',
        deadline_at: deadline,
        duration_seconds: secs,
      })
      applyQuest(saved)
      toast(`Отложен на ${minutes} мин`, { kind: 'success' })
    } catch (e) {
      error = e.message || String(e)
      toast(error, { kind: 'error' })
    } finally {
      statusBusy = false
    }
  }

  async function patchQuestSignificance(quest, significance) {
    if (!quest || statusBusy) return
    if (significance === (quest.significance || 'common')) return
    statusBusy = true
    error = ''
    try {
      const saved = await updateQuest(quest.id, { significance })
      applyQuest(saved)
      const label = QUEST_SIGNIFICANCES.find((s) => s.id === significance)?.label || significance
      toast(`Значимость: ${label}`, { kind: 'success' })
    } catch (e) {
      error = e.message || String(e)
      toast(error, { kind: 'error' })
    } finally {
      statusBusy = false
    }
  }

  async function exportQuestFromMenu(quest, mode) {
    if (!quest || questPdfBusy) return
    if (mode === 'md') {
      downloadQuestMarkdown(quest)
      toast('Markdown сохранён', { kind: 'success' })
      return
    }
    questPdfBusy = true
    const tid = 'pdf-quest'
    toastProgress(tid, 'Генерация PDF…')
    try {
      if (mode === 'simple') await downloadQuestPdfSimple(quest)
      else await downloadQuestPdf(quest, { labels: refLabels, tzLabel: localTimeZone() })
      toastDone(tid, 'PDF сохранён')
    } catch (e) {
      console.error(e)
      toastDone(tid, e?.message || 'Не удалось сохранить PDF', 'error')
    } finally {
      questPdfBusy = false
    }
  }

  function onQuestContextSelect(action) {
    if (action === 'copy-id') {
      copyIdToClipboard('quest', ctxQuestId)
      return
    }
    if (action === 'secrets') {
      if (ctxQuestId != null) openSecrets({ kind: 'quest', id: ctxQuestId })
      return
    }
    const quest = quests.find((q) => q.id === ctxQuestId)
    if (!quest) return
    if (action === 'edit') {
      openEdit(quest)
      return
    }
    if (action.startsWith('delay-')) {
      const minutes = Number(action.slice('delay-'.length))
      if (minutes > 0) postponeQuest(quest, minutes)
      return
    }
    if (action.startsWith('status:')) {
      const status = action.slice('status:'.length)
      if (status && status !== quest.status) patchQuestStatus(quest, status)
      return
    }
    if (action.startsWith('sig:')) {
      patchQuestSignificance(quest, action.slice('sig:'.length))
      return
    }
    if (action === 'export-md') {
      exportQuestFromMenu(quest, 'md')
      return
    }
    if (action === 'export-pdf') {
      exportQuestFromMenu(quest, 'rich')
      return
    }
    if (action === 'export-pdf-simple') {
      exportQuestFromMenu(quest, 'simple')
      return
    }
    if (action === 'delete') {
      selectedId = quest.id
      deleteConfirmOpen = true
    }
  }

  function onContextSelect(action) {
    if (ctxKind === 'line') {
      onLineContextSelect(action)
      return
    }
    if (ctxKind === 'quest') {
      onQuestContextSelect(action)
      return
    }
    if (ctxKind === 'step') {
      onStepContextSelect(action)
    }
  }

  async function onStepContextSelect(action) {
    if (action === 'copy-id') {
      copyIdToClipboard('step', ctxStepId)
      return
    }
    if (action === 'secrets') {
      if (ctxStepId != null) openSecrets({ kind: 'step', id: ctxStepId })
      return
    }
    const found = findStepRef(ctxStepId)
    if (!found) return
    const { step, questId } = found
    if (action === 'inc') {
      await bumpStep(step, 1, questId)
      return
    }
    if (action === 'dec') {
      await bumpStep(step, -1, questId)
      return
    }
    if (action === 'complete') {
      await setStepProgress(step, step.progress_total, questId)
      return
    }
    if (action === 'reset') {
      await setStepProgress(step, 0, questId)
      return
    }
    if (action === 'edit') {
      stepModalQuestId = questId
      stepModalStep = step
      stepModalOpen = true
      return
    }
    if (action === 'delete') {
      stepDelete = {
        questId,
        stepId: step.id,
        title: step.title || `step=${step.id}`,
      }
      stepDeleteOpen = true
    }
  }

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
    if (ctxLineId == null || lineDeleting) return
    lineDeleting = true
    error = ''
    try {
      await deleteQuestline(ctxLineId)
      lineDeleteConfirmOpen = false
      ctxLineId = null
      await load({ silent: true })
      toast('Квестлайн удалён', { kind: 'success' })
    } catch (e) {
      error = e.message || String(e)
      toast(error, { kind: 'error' })
    } finally {
      lineDeleting = false
    }
  }

  function onQuestlineSaved(_line, meta = {}) {
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

  async function toggleCompleted(quest = selected) {
    const q = quest || selected
    if (!q || statusBusy) return
    statusBusy = true
    error = ''
    try {
      const next = q.status === 'completed' ? 'active' : 'completed'
      const saved = await updateQuest(q.id, { status: next })
      applyQuest(saved, { select: false })
      toast(
        next === 'completed' ? 'Отмечен выполненным' : 'Снова активен',
        { kind: 'success' },
      )
    } catch (e) {
      error = e.message || String(e)
      toast(error, { kind: 'error' })
    } finally {
      statusBusy = false
    }
  }

  async function togglePin(quest, event) {
    event?.stopPropagation?.()
    event?.preventDefault?.()
    if (!quest || pinBusyId != null) return
    pinBusyId = quest.id
    error = ''
    try {
      const saved = await updateQuest(quest.id, { pinned: !quest.pinned })
      applyQuest(saved, { select: false })
      selectQuestFromUi(saved.id, { pushUrl: false })
    } catch (e) {
      error = e.message || String(e)
    } finally {
      pinBusyId = null
    }
  }

  async function setStepProgress(step, value, questId = null) {
    const qid = questId ?? selected?.id
    if (qid == null || stepBusyId != null) return
    const total = Math.max(0, Number(step.progress_total) || 0)
    const next = Math.max(0, Math.min(total, Math.round(Number(value))))
    if (!Number.isFinite(next) || next === step.progress_current) {
      stepEditId = null
      stepEditQuestId = null
      return
    }
    stepBusyId = step.id
    error = ''
    try {
      const saved = await updateQuestStep(qid, step.id, {
        progress_current: next,
      })
      applyQuest(saved, { select: false })
    } catch (e) {
      error = e.message || String(e)
    } finally {
      stepBusyId = null
      stepEditId = null
      stepEditQuestId = null
    }
  }

  async function bumpStep(step, delta, questId = null) {
    await setStepProgress(step, step.progress_current + delta, questId)
  }

  function beginEditStep(step, questId = null) {
    if (stepBusyId != null) return
    stepEditId = step.id
    stepEditQuestId = questId ?? findQuestIdForStep(step.id) ?? selected?.id ?? null
    stepEditValue = String(step.progress_current)
  }

  function cancelEditStep() {
    stepEditId = null
    stepEditQuestId = null
  }

  function findQuestIdForStep(stepId) {
    for (const q of quests) {
      if (q.steps?.some((s) => s.id === stepId)) return q.id
    }
    return null
  }

  function findStepRef(stepId) {
    for (const q of quests) {
      const step = q.steps?.find((s) => s.id === stepId)
      if (step) return { step, questId: q.id }
    }
    return null
  }

  function onStepEditKeydown(event, step) {
    if (event.key === 'Enter') {
      event.preventDefault()
      setStepProgress(step, stepEditValue, stepEditQuestId)
    } else if (event.key === 'Escape') {
      event.preventDefault()
      cancelEditStep()
    }
  }

  function onStepEditBlur(step) {
    if (stepEditId !== step.id) return
    setStepProgress(step, stepEditValue, stepEditQuestId)
  }

  function applyQuest(quest, { select = true } = {}) {
    const idx = quests.findIndex((q) => q.id === quest.id)
    if (idx >= 0) {
      quests = quests.map((q) => (q.id === quest.id ? quest : q))
    } else {
      load({ silent: true })
    }
    if (select) selectedId = quest.id
  }

  function onSaved(quest, meta = {}) {
    selectedId = quest.id
    load({ silent: true })
    if (meta.mode === 'create') toast('Квест создан', { kind: 'success' })
    if (view === 'graph') graphNonce += 1
  }

  function onDeleted(id) {
    if (selectedId === id) selectedId = null
    load({ silent: true })
    toast('Квест удалён', { kind: 'success' })
  }

  function questIdFromUrl() {
    try {
      const raw = new URL(location.href).searchParams.get('quest')
      if (!raw) return null
      const id = Number(raw)
      return Number.isFinite(id) && id > 0 ? id : null
    } catch {
      return null
    }
  }

  function noteIdFromUrl() {
    try {
      const raw = new URL(location.href).searchParams.get('note')
      if (!raw) return null
      const id = Number(raw)
      return Number.isFinite(id) && id > 0 ? id : null
    } catch {
      return null
    }
  }

  function attachmentIdFromUrl() {
    try {
      const raw = new URL(location.href).searchParams.get('attachment')
      if (!raw) return null
      const id = Number(raw)
      return Number.isFinite(id) && id > 0 ? id : null
    } catch {
      return null
    }
  }

  function tabFromUrl() {
    try {
      const raw = new URL(location.href).searchParams.get('tab')
      if (
        raw === 'notes' ||
        raw === 'attachments' ||
        raw === 'calendar' ||
        raw === 'history' ||
        raw === 'graph' ||
        raw === 'runners'
      ) {
        return raw
      }
    } catch {
      /* ignore */
    }
    return null
  }

  /** Rewrite the query from scratch — tab / quest= / note= / attachment= never share the bar. */
  function replaceSearch(params) {
    try {
      const qs = new URLSearchParams()
      for (const [key, value] of Object.entries(params)) {
        if (value != null && value !== '') qs.set(key, String(value))
      }
      const next = `${location.pathname}${qs.toString() ? `?${qs}` : ''}`
      if (next !== `${location.pathname}${location.search}`) {
        history.replaceState(null, '', next)
      }
    } catch {
      /* ignore */
    }
  }

  function searchForView(nextView) {
    if (nextView === 'journal') {
      return selectedId != null ? { quest: selectedId } : {}
    }
    if (nextView === 'notes') {
      return selectedNoteId != null ? { note: selectedNoteId } : { tab: 'notes' }
    }
    if (nextView === 'graph') {
      return graphRoot.id != null
        ? { tab: 'graph', gt: graphRoot.type, gid: graphRoot.id }
        : { tab: 'graph' }
    }
    if (nextView === 'attachments') {
      return selectedAttachmentId != null
        ? { attachment: selectedAttachmentId }
        : { tab: 'attachments' }
    }
    return { tab: nextView }
  }

  // Header tab order; drives which way a tab switch slides.
  // 'toc' is hidden for now (trial without it, see quest 328); the page and its
  // view branch are kept so the tab can come back by re-adding it here and in the header.
  const TAB_ORDER = ['journal', 'notes', 'attachments', 'calendar', 'history', 'graph', 'runners']
  let navDir = $state(1)

  // The map stays mounted once opened; it is only shown / hidden, with the same
  // slide the other tabs get (they are re-created per tab, the map is not).
  let graphSeen = $state(false)
  let graphOn = $state(view === 'graph')
  let graphEl = $state(/** @type {HTMLDivElement | null} */ (null))
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
  function slideTab(node, { dir, out = false }) {
    const sign = out ? -dir : dir
    return {
      duration: 220,
      easing: cubicOut,
      css: (t) => `transform: translateX(${(1 - t) * sign * 100}%)`,
    }
  }

  function setView(next) {
    const from = TAB_ORDER.indexOf(view)
    const to = TAB_ORDER.indexOf(next)
    if (from !== -1 && to !== -1 && from !== to) navDir = to > from ? 1 : -1
    view = next
    replaceSearch(searchForView(next))
  }

  /** Opens the relations map centred on one entity. */
  function openGraph(type, id) {
    const from = TAB_ORDER.indexOf(view)
    navDir = TAB_ORDER.indexOf('graph') > from ? 1 : -1
    graphRoot = { type, id: Number(id) }
    replaceSearch(searchForView('graph'))
    view = 'graph'
  }

  function onGraphCenter(type, id) {
    // (null, null) = the map was cleared.
    graphRoot = id == null ? { type: null, id: null } : { type, id: Number(id) }
    replaceSearch(searchForView('graph'))
  }

  /** Opens a map node's entity in its own tab. */
  function onGraphOpen(kind, id) {
    onJournalRef(kind, id)
  }

  /** Calendar → History for one day. The URL is set first: History reads its filters on mount. */
  function openHistoryDay(dayKey) {
    const from = TAB_ORDER.indexOf(view)
    navDir = TAB_ORDER.indexOf('history') > from ? 1 : -1
    replaceSearch({ tab: 'history', day: dayKey })
    view = 'history'
  }

  function selectQuestFromUi(id, { pushUrl = true } = {}) {
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

  function selectNoteFromUi(id, { pushUrl = true } = {}) {
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

  function selectAttachmentFromUi(id, { pushUrl = true } = {}) {
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

  function onJournalRef(kind, id) {
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
        if (q.steps?.some((s) => s.id === id)) {
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
      const data = await fetchHealth()
      const comps = data?.components || {}
      const webdav = probeStatus(comps.webdav?.status)
      if (health.webdav !== 'unknown' && health.webdav !== webdav) {
        invalidateAttachmentLiveFlags()
      }
      health = {
        api: data?.api?.status === 'ok' ? 'ok' : 'offline',
        overlay: comps.overlay?.status === 'ok' ? 'ok' : 'offline',
        telegram: comps.telegram?.status === 'ok' ? 'ok' : 'offline',
        webdav,
        db: dbStatus(comps.db?.status),
        detail: data,
      }
    } catch {
      health = {
        api: 'offline',
        overlay: 'unknown',
        telegram: 'unknown',
        webdav: 'unknown',
        db: 'unknown',
      }
    }
  }

  function probeStatus(status) {
    if (status === 'ok') return 'ok'
    if (status === 'offline') return 'offline'
    return 'unknown'
  }

  function dbStatus(status) {
    if (status === 'ok') return 'ok'
    if (status === 'warn') return 'warn'
    return 'unknown'
  }

  /** Boots data loading and live updates. Only runs once authenticated. */
  function startApp() {
    const fromNote = noteIdFromUrl()
    const fromAtt = attachmentIdFromUrl()
    const fromTab = tabFromUrl()
    if (fromAtt != null) {
      selectedAttachmentId = fromAtt
      view = 'attachments'
    } else if (fromNote != null) {
      selectedNoteId = fromNote
      view = 'notes'
    } else if (fromTab) {
      view = fromTab
      if (fromTab === 'graph') {
        try {
          const sp = new URL(location.href).searchParams
          const gid = Number(sp.get('gid'))
          const gt = sp.get('gt')
          if (gid > 0 && (gt === 'quest' || gt === 'note' || gt === 'questline')) {
            graphRoot = { type: gt, id: gid }
          }
        } catch {
          /* ignore */
        }
      }
    }
    const fromUrl = questIdFromUrl()
    if (fromUrl != null && fromNote == null && fromAtt == null && fromTab == null) {
      pendingSelectId = fromUrl
      selectedId = fromUrl
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

  function onAuthenticated(name) {
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
      setTab: (name) => setView(name),
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
  let kbStepId = $state(/** @type {number | null} */ (null))
  $effect(() => {
    selectedId
    kbStepId = null
  })

  function kbCursor() {
    const steps = selected?.steps ?? []
    if (!steps.length) return null
    return steps.find((s) => s.id === kbStepId) ?? steps.find((s) => !s.done) ?? steps[steps.length - 1]
  }

  function kbMoveStep(delta) {
    const steps = selected?.steps ?? []
    const cur = kbCursor()
    if (!cur) return
    if (kbStepId == null) {
      kbStepId = cur.id // first press only reveals the cursor
      return
    }
    const i = steps.findIndex((s) => s.id === cur.id)
    kbStepId = steps[Math.min(steps.length - 1, Math.max(0, i + delta))].id
  }

  function kbBumpStep(delta) {
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
        run: (event) => {
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

{#if authed === false}
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

  <div class="journal__stage">
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
          if (quest) openQuestContextMenu(event, quest)
        }}
      />
    </div>
  {:else if view === 'toc'}
    <div class="journal__toc">
      <TocPage
        {matchedQuests}
        {categories}
        {questlines}
        bind:searchQuery
        {nowMs}
        labels={refLabels}
        onRef={onJournalRef}
        onSelectQuest={selectQuestFromUi}
        onLineContextMenu={openLineContextMenu}
        onQuestContextMenu={openQuestContextMenu}
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
        bind:showAllQuests
        {selectedId}
        {categoryOpen}
        {lineOpen}
        {nowMs}
        onSelect={(id) => selectQuestFromUi(id)}
        onTogglePin={togglePin}
        onQuestContextMenu={openQuestContextMenu}
        onLineContextMenu={openLineContextMenu}
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
        onStepEditInput={(v) => (stepEditValue = v)}
        onQuestTitleContextMenu={openQuestContextMenu}
        onLineHeadContextMenu={(e) => {
          const line = questlines.find((l) => l.id === selected?.questline_id)
          if (line) openLineContextMenu(e, line)
        }}
        onStepContextMenu={openStepContextMenu}
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
  open={ctxOpen}
  x={ctxX}
  y={ctxY}
  items={ctxItems}
  onSelect={onContextSelect}
  onClose={closeContextMenu}
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
