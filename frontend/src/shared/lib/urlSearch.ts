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
