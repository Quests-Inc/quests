// Навигация приложения: открытая вкладка, выбранный квест / заметка / вложение, центр карты и адресная строка.
// Всё, что «открывает» сущность из интерфейса (ссылки, карта, календарь), проходит здесь, чтобы адрес и выбор не расходились.
import { initialRoute, replaceSearch, searchForView as searchForRoute } from '@/app/model/route.ts'
import { searchOfTarget, type OpenTarget } from '@/shared/lib/openTarget.ts'

export type View =
  | 'journal'
  | 'notes'
  | 'attachments'
  | 'calendar'
  | 'history'
  | 'graph'
  | 'runners'
  | 'scripts'
  | 'hooks'
  | 'runs'
export type GraphRoot = { type: 'quest' | 'note' | 'questline' | null; id: number | null }

/** Порядок вкладок в шапке: определяет, в какую сторону «уезжает» переключение. */
export const TAB_ORDER: View[] = [
  'journal',
  'notes',
  'attachments',
  'calendar',
  'history',
  'graph',
  'runners',
  'scripts',
  'hooks',
  'runs',
]

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
  let selectedId = $state<number | null>(null)
  let selectedNoteId = $state<number | null>(null)
  let selectedAttachmentId = $state<number | null>(null)
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
    const route = initialRoute()
    if (route.view) view = route.view as View
    if (route.attachmentId != null) selectedAttachmentId = route.attachmentId
    if (route.noteId != null) selectedNoteId = route.noteId
    if (route.graphRoot) graphRoot = route.graphRoot as GraphRoot
    if (route.questId != null) {
      pendingSelectId = route.questId
      selectedId = route.questId
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
    slideTo('graph')
    graphRoot = { type, id: Number(id) }
    replaceSearch(searchFor('graph'))
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
      if (pushUrl) replaceSearch(searchFor(view))
      deps.revealList('notes')
      return
    }
    selectedNoteId = n
    if (view !== 'notes') view = 'notes'
    if (pushUrl) replaceSearch({ note: n })
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
    get graphRoot() {
      return graphRoot
    },
    get pendingSelectId() {
      return pendingSelectId
    },
    set pendingSelectId(v: number | null) {
      pendingSelectId = v
    },
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
