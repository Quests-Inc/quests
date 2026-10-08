import { fetchWorkspaces } from './api.js'

/** What the current workspace may do. `serverExec` = run check/emit commands on the
 * server; off, forms offer runner devices only (the server refuses the rest anyway).
 * Optimistic until loaded, so single-user installs never flash a restriction. */
export const wsInfo = $state({ serverExec: true, loaded: false })

/** @type {Promise<void> | null} */
let inflight = null

export function loadWorkspaceInfo() {
  if (wsInfo.loaded) return Promise.resolve()
  if (inflight) return inflight
  inflight = fetchWorkspaces()
    .then((r) => {
      wsInfo.serverExec = r?.server_exec !== false
      wsInfo.loaded = true
    })
    .catch(() => {
      /* keep the optimistic default; the server enforces regardless */
    })
    .finally(() => {
      inflight = null
    })
  return inflight
}
