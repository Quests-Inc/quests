// Навигация приложения: открытая вкладка, выбранный квест / заметка / вложение, центр карты и адресная строка.
// Всё, что «открывает» сущность из интерфейса (ссылки, карта, календарь), проходит здесь, чтобы адрес и выбор не расходились.
import { initialRoute, searchForView as searchForRoute } from '@/app/model/route.ts'
import { searchOfTarget, type OpenTarget } from '@/shared/lib/openTarget.ts'
import { navigationScope, searchAfterWorkspaceSwitch } from '@/shared/lib/urlSearch.ts'
import { TAB_ORDER, type View } from '@/shared/lib/viewTabs.ts'

export { TAB_ORDER, type View }
export type GraphRoot = { type: 'quest' | 'note' | 'questline' | 'event' | 'attachment' | null; id: number | null }

const validId = (id: unknown) => {
  const n = Number(id)
  return Number.isFinite(n) && n > 0 ? n : null
}

export function createNavigation(deps: {
  quests: () => { id: number; questline_id?: number | null; steps?: { id: number }[] }[]
  revealDetail: (tab: 'journal' | 'notes') => void
  revealList: (tab: 'journal' | 'notes') => void
  openTemplate: (id: number) => void
}) {
  let view = $state<View>('journal')
  let navDir = $state(1)
  let noteHistory = $state<number[]>([])
  let noteEntries = $state<number[]>([])
  let noteHistoryIndex = $state(-1)
  let entry = 0
  function routeState() {
    return {
      ...history.state,
      questsEntry: entry,
      questsScope: navigationScope(),
      questsNotes: { ids: [...noteHistory], entries: [...noteEntries], index: noteHistoryIndex },
    }
  }
  function restoreNoteHistory() {
    if (history.state?.questsScope && history.state.questsScope !== navigationScope()) {
      noteHistory = []
      noteEntries = []
      noteHistoryIndex = -1
      entry = 0
      const qs = new URLSearchParams(searchAfterWorkspaceSwitch(location.search))
      history.replaceState(routeState(), '', `${location.pathname}${qs.size ? `?${qs}` : ''}`)
      return
    }
    entry = Number.isInteger(history.state?.questsEntry) ? history.state.questsEntry : 0
    const saved = history.state?.questsNotes
    if (
      saved &&
      Array.isArray(saved.ids) &&
      Array.isArray(saved.entries) &&
      saved.ids.length === saved.entries.length &&
      saved.entries.every((n: unknown) => Number.isInteger(n) && Number(n) >= 0) &&
      saved.ids.every((id: unknown) => Number.isInteger(id) && Number(id) > 0) &&
      Number.isInteger(saved.index) &&
      saved.index >= 0 &&
      saved.index < saved.ids.length
    ) {
      // Older entries hold a prefix. Retain known forward visits and write them
      // back on pop so reloading while looking backwards preserves the trail.
      if (!saved.ids.every((id: number, i: number) => noteHistory[i] === id) || saved.ids.length > noteHistory.length) {
        noteHistory = saved.ids
        noteEntries = saved.entries
      }
      noteHistoryIndex = saved.index
    }
  }
  function replaceSearch(params: Record<string, unknown>) {
    const qs = new URLSearchParams()
    for (const [key, value] of Object.entries(params)) if (value != null && value !== '') qs.set(key, String(value))
    const url = `${location.pathname}${qs.size ? `?${qs}` : ''}`
    if (url !== location.pathname + location.search) {
      // Preserve the current note before leaving it for another view.
      history.replaceState(routeState(), '', location.href)
      entry++
      history.pushState(routeState(), '', url)
    }
  }
  function rememberNote(id: number, push: boolean) {
    const same = location.search === `?note=${id}`
    if (push && !same) {
      history.replaceState(routeState(), '', location.href)
      entry++
    }
    if (noteHistory[noteHistoryIndex] !== id) {
      noteHistory = [...noteHistory.slice(0, noteHistoryIndex + 1), id]
      noteEntries = [...noteEntries.slice(0, noteHistoryIndex + 1), entry]
      noteHistoryIndex = noteHistory.length - 1
    } else if (push && !same) {
      noteEntries = [...noteEntries]
      noteEntries[noteHistoryIndex] = entry
    }
    const state = routeState(),
      url = `${location.pathname}?note=${id}`
    if (push && !same) history.pushState(state, '', url)
    else history.replaceState(state, '', url)
  }
  function onPopState() {
    restoreNoteHistory()
    const route = initialRoute()
    view = (route.view ?? 'journal') as View
    selectedId = route.questId ?? null
    pendingSelectId = route.questId ?? null
    selectedNoteId = route.noteId ?? null
    selectedAttachmentId = route.attachmentId ?? null
    graphRoot = route.graphRoot ?? { type: null, id: null }
    restoreNoteHistory()
    history.replaceState(routeState(), '', location.href)
    if (selectedNoteId != null) deps.revealDetail('notes')
  }
  function goNoteHistory(index: number) {
    if (index >= 0 && index < noteHistory.length) history.go(noteEntries[index] - entry)
  }
  let selectedId = $state<number | null>(null)
  let selectedNoteId = $state<number | null>(null)
  let selectedAttachmentId = $state<number | null>(null)
  let graphVisit = $state(0)
  let graphRoot = $state<GraphRoot>({ type: null, id: null })
  /** Квест, который надо выбрать, когда список дозагрузится. */
  let pendingSelectId = $state<number | null>(null)

  const searchFor = (next: string) =>
    searchForRoute(next, {
      quest: selectedId,
      note: selectedNoteId,
      attachment: selectedAttachmentId,
      graph: graphRoot,
    })

  function slideTo(next: View) {
    const from = TAB_ORDER.indexOf(view)
    const to = TAB_ORDER.indexOf(next)
    if (from !== -1 && to !== -1 && from !== to) navDir = to > from ? 1 : -1
  }

  /** Стартовое состояние из адресной строки. */
  function start() {
    restoreNoteHistory()
    const route = initialRoute()
    if (route.noteId != null) rememberNote(route.noteId, false)
    if (route.view) view = route.view as View
    if (route.attachmentId != null) selectedAttachmentId = route.attachmentId
    if (route.noteId != null) selectedNoteId = route.noteId
    if (route.graphRoot) graphRoot = route.graphRoot as GraphRoot
    if (route.questId != null) {
      pendingSelectId = route.questId
      selectedId = route.questId
    }
    if (route.eventId != null) {
      view = 'calendar'
      queueMicrotask(() => window.dispatchEvent(new CustomEvent('quests:open-event', { detail: route.eventId })))
    }
  }

  function setView(next: string) {
    slideTo(next as View)
    view = next as View
    replaceSearch(searchFor(next))
  }

  /** Свободный горизонтальный свайп листает соседнюю вкладку шапки. */
  function swipeTab(dir: number) {
    const i = TAB_ORDER.indexOf(view)
    const next = i === -1 ? null : TAB_ORDER[i + dir]
    if (next) setView(next)
  }

  /** Карта связей с центром на одной сущности. */
  function openGraph(type: GraphRoot['type'], id: number | string) {
    graphVisit++
    slideTo('graph')
    graphRoot = { type, id: Number(id) }
    replaceSearch({ tab: 'graph', gt: type, gid: Number(id) })
    view = 'graph'
  }

  /** (null, null) — карту очистили. */
  function onGraphCenter(type: GraphRoot['type'], id: number | string | null) {
    graphRoot = id == null ? { type: null, id: null } : { type, id: Number(id) }
    replaceSearch(searchFor('graph'))
  }

  /** Календарь → История за один день. Адрес пишется первым: История читает фильтры при монтировании. */
  function openHistoryDay(dayKey: string) {
    slideTo('history')
    replaceSearch({ tab: 'history', day: dayKey })
    view = 'history'
  }

  /** A tab with something picked in it (runs filtered, a script, a hook): the address first, the page reads it when it shows. */
  function openTarget(t: OpenTarget) {
    if (t.kind === 'ref') return openRef(t.ref, t.id)
    const search = searchOfTarget(t)
    if (!search) return
    const next = String(search.tab) as View
    slideTo(next)
    replaceSearch(search)
    view = next
    window.dispatchEvent(new CustomEvent('quests:route', { detail: search }))
  }

  function selectQuest(id: number, { pushUrl = true } = {}) {
    const n = validId(id)
    if (n == null) return
    pendingSelectId = n
    selectedId = n
    if (view !== 'journal') view = 'journal'
    if (pushUrl) replaceSearch({ quest: n })
    deps.revealDetail('journal')
    try {
      window.focus()
    } catch {
      /* браузеры часто запрещают фокус из фона */
    }
  }

  function selectNote(id: number | null, { pushUrl = true } = {}) {
    const n = validId(id)
    if (n == null) {
      selectedNoteId = null
      if (pushUrl) replaceSearch({ tab: 'notes' })
      deps.revealList('notes')
      return
    }
    selectedNoteId = n
    if (view !== 'notes') view = 'notes'
    if (pushUrl) rememberNote(n, true)
    deps.revealDetail('notes')
  }

  function selectAttachment(id: number | null, { pushUrl = true } = {}) {
    const n = validId(id)
    if (n == null) {
      selectedAttachmentId = null
      if (pushUrl) replaceSearch(searchFor('attachments'))
      return
    }
    selectedAttachmentId = n
    if (view !== 'attachments') view = 'attachments'
    if (pushUrl) replaceSearch({ attachment: n })
  }

  function clearQuest({ pushUrl = true } = {}) {
    selectedId = null
    pendingSelectId = null
    if (pushUrl) replaceSearch(searchFor(view))
    deps.revealList('journal')
  }

  /** Открыть сущность по ссылке вида kind=id. */
  function openRef(kind: string, id: number) {
    if (kind === 'repo') {
      slideTo('repositories')
      replaceSearch({ repo: id })
      view = 'repositories'
      window.dispatchEvent(new CustomEvent('quests:repository', { detail: id }))
      return
    }
    if (kind === 'note') return selectNote(id)
    if (kind === 'attachment') return selectAttachment(id)
    if (kind === 'quest') return selectQuest(id)
    if (kind === 'questline') {
      const first = deps.quests().find((q) => q.questline_id === id)
      if (first) selectQuest(first.id)
      else setView('journal')
      return
    }
    if (kind === 'step') {
      const owner = deps.quests().find((q) => q.steps?.some((s) => s.id === id))
      if (owner) selectQuest(owner.id)
      return
    }
    if (kind === 'template') deps.openTemplate(id)
    if (kind === 'event') {
      slideTo('calendar')
      replaceSearch({ event: id })
      view = 'calendar'
      // After the calendar tab mounts and subscribes to quests:open-event.
      queueMicrotask(() => window.dispatchEvent(new CustomEvent('quests:open-event', { detail: id })))
    }
  }

  return {
    get view() {
      return view
    },
    set view(v: View) {
      view = v
    },
    get navDir() {
      return navDir
    },
    get selectedId() {
      return selectedId
    },
    set selectedId(v: number | null) {
      selectedId = v
    },
    get selectedNoteId() {
      return selectedNoteId
    },
    set selectedNoteId(v: number | null) {
      selectedNoteId = v
    },
    get selectedAttachmentId() {
      return selectedAttachmentId
    },
    set selectedAttachmentId(v: number | null) {
      selectedAttachmentId = v
    },
    get graphVisit() {
      return graphVisit
    },
    get graphRoot() {
      return graphRoot
    },
    get pendingSelectId() {
      return pendingSelectId
    },
    set pendingSelectId(v: number | null) {
      pendingSelectId = v
    },
    get noteHistory() {
      return noteHistory
    },
    get noteHistoryIndex() {
      return noteHistoryIndex
    },
    goNoteHistory,
    onPopState,
    searchFor,
    start,
    setView,
    swipeTab,
    openGraph,
    onGraphCenter,
    openHistoryDay,
    openTarget,
    selectQuest,
    selectNote,
    selectAttachment,
    clearQuest,
    openRef,
  }
}
