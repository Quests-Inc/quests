// Состояние левых списков (журнал, заметки, скрипты): свёрнут ли каждый и общая ширина — помнится между перезагрузками.
export const SIDEBAR_MIN = 208
export const SIDEBAR_MAX = 520
export const SIDEBAR_DEFAULT = 264
/** Ниже этой ширины список и содержимое идут по одному на экран. */
export const SIDEBAR_ONE_PANE = '(max-width: 640px)'

const COLLAPSED_KEY = 'quests.sidebarCollapsed'
const WIDTH_KEY = 'quests.sidebarWidth'

const clamp = (n: number) => Math.min(SIDEBAR_MAX, Math.max(SIDEBAR_MIN, Math.round(n)))

function read<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key)
    return raw == null ? fallback : (JSON.parse(raw) as T)
  } catch {
    return fallback
  }
}

function write(key: string, value: unknown) {
  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch {
    /* private mode / quota — the state still works for this page load */
  }
}

let collapsed = $state<Record<string, boolean>>(read(COLLAPSED_KEY, {}))
let width = $state(clamp(Number(read(WIDTH_KEY, SIDEBAR_DEFAULT)) || SIDEBAR_DEFAULT))

export const sidebars = {
  isCollapsed: (key: string) => !!collapsed[key],
  setCollapsed(key: string, value: boolean) {
    if (!!collapsed[key] === value) return
    collapsed = { ...collapsed, [key]: value }
    write(COLLAPSED_KEY, collapsed)
  },
  toggle(key: string) {
    sidebars.setCollapsed(key, !collapsed[key])
  },
  get width() {
    return width
  },
  setWidth(next: number, persist = true) {
    width = clamp(next)
    if (persist) write(WIDTH_KEY, width)
  },
  resetWidth() {
    sidebars.setWidth(SIDEBAR_DEFAULT)
  },
}
