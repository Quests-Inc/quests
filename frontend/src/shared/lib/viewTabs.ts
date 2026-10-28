// Разделы приложения и их порядок в шапке: единственное место, где он задан.
// Шапка рисует вкладки отсюда, а свайп и цифровые клавиши идут по тому же `TAB_ORDER`, так что новый раздел или перестановка
// подхватываются везде сразу.

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
  | 'templates'
  | 'trash'

export type ViewTab = { view: View; label: string; icon: string }
export type ViewGroup = { id: string; label: string; icon: string; tabs: ViewTab[] }

export const VIEW_TABS: Record<View, ViewTab> = {
  journal: { view: 'journal', label: 'Журнал', icon: 'scroll' },
  notes: { view: 'notes', label: 'Заметки', icon: 'document' },
  graph: { view: 'graph', label: 'Карта', icon: 'map' },
  calendar: { view: 'calendar', label: 'Календарь', icon: 'calendar' },
  attachments: { view: 'attachments', label: 'Вложения', icon: 'attachment' },
  history: { view: 'history', label: 'История', icon: 'scroll' },
  runners: { view: 'runners', label: 'Раннеры', icon: 'layers' },
  scripts: { view: 'scripts', label: 'Скрипты', icon: 'edit' },
  hooks: { view: 'hooks', label: 'Хуки', icon: 'flag' },
  templates: { view: 'templates', label: 'Шаблоны', icon: 'repeat' },
  trash: { view: 'trash', label: 'Корзина', icon: 'delete' },
  runs: { view: 'runs', label: 'Запуски', icon: 'list' },
}

const T = VIEW_TABS

/** Вкладки шапки, всегда на виду. */
export const MAIN_TABS: ViewTab[] = [T.journal, T.notes, T.graph, T.calendar]

/** Выпадающие группы шапки, слева направо. */
export const VIEW_GROUPS: ViewGroup[] = [
  { id: 'more', label: 'Данные', icon: 'folder', tabs: [T.attachments, T.history, T.trash] },
  { id: 'auto', label: 'Автоматика', icon: 'repeat', tabs: [T.templates, T.scripts, T.hooks, T.runs, T.runners] },
]

/** Все разделы в порядке шапки: слева направо, группы — сверху вниз. */
export const TAB_ORDER: View[] = [...MAIN_TABS, ...VIEW_GROUPS.flatMap((g) => g.tabs)].map((t) => t.view)
