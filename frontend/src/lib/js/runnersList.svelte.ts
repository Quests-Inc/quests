import { getRunners } from './api.ts'

/** Runners (devices that execute checks) for pickers. One shared list, refreshed
 * at most every 20 s however many forms ask for it. */
export const runnersList = $state({
  items: /** @type {{ label: string, online: boolean, registered: boolean }[]} */ ([]),
  loaded: false,
})

let at = 0
/** @type {Promise<void> | null} */
let inflight: Promise<void> | null = null

/** @param {boolean} [force] */
export function loadRunners(force: boolean = false) {
  if (inflight) return inflight
  if (!force && runnersList.loaded && Date.now() - at < 20000) return Promise.resolve()
  inflight = getRunners()
    .then((rows) => {
      runnersList.items = (rows ?? [])
        .filter((r) => !r.revoked)
        .map((r) => ({ label: r.label, online: Boolean(r.online), registered: Boolean(r.registered) }))
      runnersList.loaded = true
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
