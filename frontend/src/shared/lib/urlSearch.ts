import { VIEW_TABS } from './viewTabs.ts'

// Адресная строка: переписать параметры целиком, не трогая историю.

/** Rewrite the query from scratch — tab / quest= / note= / attachment= never share the bar. */
export function replaceSearch(params: Record<string, unknown>) {
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

/** Workspace-local entity ids and filters must never cross a workspace boundary. */
export function searchAfterWorkspaceSwitch(search: string): Record<string, string> {
  const params = new URLSearchParams(search)
  const tab = params.has('attachment')
    ? 'attachments'
    : params.has('note')
      ? 'notes'
      : params.has('repo')
        ? 'repositories'
        : params.get('tab')
  return tab && tab !== 'journal' && Object.hasOwn(VIEW_TABS, tab) ? { tab } : {}
}
