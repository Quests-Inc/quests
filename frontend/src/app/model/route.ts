// Адресная строка приложения: какая вкладка и какой объект открыт, и обратно.
// Параметры `tab` / `quest=` / `note=` / `attachment=` никогда не живут в строке вместе.

const TABS = ['notes', 'attachments', 'calendar', 'history', 'graph', 'runners', 'scripts', 'hooks', 'runs'] as const
export type UrlTab = (typeof TABS)[number]

function param(name: string): string | null {
  try {
    return new URL(location.href).searchParams.get(name)
  } catch {
    return null
  }
}

function idParam(name: string): number | null {
  const raw = param(name)
  if (!raw) return null
  const id = Number(raw)
  return Number.isFinite(id) && id > 0 ? id : null
}

export const questIdFromUrl = () => idParam('quest')
export const noteIdFromUrl = () => idParam('note')
export const attachmentIdFromUrl = () => idParam('attachment')

export function tabFromUrl(): UrlTab | null {
  const raw = param('tab')
  return (TABS as readonly string[]).includes(raw ?? '') ? (raw as UrlTab) : null
}

export { replaceSearch } from '@/shared/lib/urlSearch.ts'

/** What the address bar should say for a view, given what is selected in it. */
export function searchForView(
  nextView: string,
  sel: {
    quest: number | null
    note: number | null
    attachment: number | null
    graph: { type: string | null; id: number | null }
  }
): Record<string, unknown> {
  if (nextView === 'journal') return sel.quest != null ? { quest: sel.quest } : {}
  if (nextView === 'notes') return sel.note != null ? { note: sel.note } : { tab: 'notes' }
  if (nextView === 'graph') {
    return sel.graph.id != null ? { tab: 'graph', gt: sel.graph.type, gid: sel.graph.id } : { tab: 'graph' }
  }
  if (nextView === 'attachments')
    return sel.attachment != null ? { attachment: sel.attachment } : { tab: 'attachments' }
  return { tab: nextView }
}

export type ViewName =
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

/** Where the app opens from the address: which tab, which object is selected, the map's centre. */
export function initialRoute(): {
  view?: ViewName
  attachmentId?: number
  noteId?: number
  questId?: number
  graphRoot?: { type: 'quest' | 'note' | 'questline'; id: number }
} {
  const fromNote = noteIdFromUrl()
  const fromAtt = attachmentIdFromUrl()
  const fromTab = tabFromUrl()
  if (fromAtt != null) return { view: 'attachments', attachmentId: fromAtt }
  if (fromNote != null) return { view: 'notes', noteId: fromNote }
  if (fromTab) {
    if (fromTab !== 'graph') return { view: fromTab }
    const gid = Number(param('gid'))
    const gt = param('gt')
    if (gid > 0 && (gt === 'quest' || gt === 'note' || gt === 'questline'))
      return { view: 'graph', graphRoot: { type: gt, id: gid } }
    return { view: 'graph' }
  }
  const fromQuest = questIdFromUrl()
  return fromQuest != null ? { questId: fromQuest } : {}
}

export function probeStatus(status: string) {
  if (status === 'ok') return 'ok'
  if (status === 'offline') return 'offline'
  return 'unknown'
}

export function dbStatus(status: string) {
  if (status === 'ok') return 'ok'
  if (status === 'warn') return 'warn'
  return 'unknown'
}

export interface Health {
  api: string
  overlay: string
  telegram: string
  webdav: string
  db?: string
  detail?: unknown
}

/** What the header chips show, from the server's /api/health answer. */
type ProbeStatus = { status?: string }
export function healthFrom(
  data: { api?: ProbeStatus; components?: Record<string, ProbeStatus | undefined> } | null | undefined
): Health {
  const comps = data?.components || {}
  return {
    api: data?.api?.status === 'ok' ? 'ok' : 'offline',
    overlay: comps.overlay?.status === 'ok' ? 'ok' : 'offline',
    telegram: comps.telegram?.status === 'ok' ? 'ok' : 'offline',
    webdav: probeStatus(comps.webdav?.status),
    db: dbStatus(comps.db?.status),
    detail: data,
  }
}

/** The server does not answer: only the API chip is known. */
export const healthOffline = (): Health => ({
  api: 'offline',
  overlay: 'unknown',
  telegram: 'unknown',
  webdav: 'unknown',
  db: 'unknown',
})
