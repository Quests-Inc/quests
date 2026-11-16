/** Keys of cards on the keep-alive graph scene — no refetch, empty until the map was opened. */

export const graphPresence = $state({
  keys: [] as string[],
})

let focusHandler: ((key: string) => void) | null = null

export function setGraphPresence(keys: Iterable<string>, focus: (key: string) => void) {
  graphPresence.keys = [...keys]
  focusHandler = focus
}

export function clearGraphPresence() {
  graphPresence.keys = []
  focusHandler = null
}

export function focusGraphPresence(key: string) {
  focusHandler?.(key)
}
