import { getScripts } from '@/shared/api/client.ts'
import type { Script } from '@/shared/api/execution.gen.ts'

import { purposeOfKind } from './purpose.ts'

/** Scripts for pickers. One shared list, refreshed at most every 20 s however many forms ask for it. */
export const scriptsList = $state({
  items: [] as Script[],
  loaded: false,
})

let at = 0
let inflight: Promise<void> | null = null

export function loadScripts(force: boolean = false) {
  if (inflight) return inflight
  if (!force && scriptsList.loaded && Date.now() - at < 20000) return Promise.resolve()
  inflight = getScripts()
    .then((rows) => {
      scriptsList.items = rows ?? []
      scriptsList.loaded = true
      at = Date.now()
    })
    .catch(() => {
      /* the picker still works with the current value */
    })
    .finally(() => {
      inflight = null
    })
  return inflight
}

/**
 * The scripts offered for this kind of entity (a note script does not fit a step): those made for it, and the unsorted ones
 * whose sources read it.
 */
export function scriptsFor(kind: string): Script[] {
  const purpose = purposeOfKind(kind)
  return scriptsList.items.filter((s) =>
    s.purpose ? s.purpose === purpose : s.subjects?.length === 1 && s.subjects[0] === kind
  )
}
