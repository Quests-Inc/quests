import { listTemplates } from './api.js'

/**
 * Quest templates, loaded once and shared: `@` autocomplete and ref labels need
 * them everywhere (template=N), and threading a prop through every form that
 * has a MentionTextarea would be far noisier than one store.
 */
export const templateStore = $state({ list: /** @type {any[]} */ ([]), loaded: false })

let inflight = /** @type {Promise<void> | null} */ (null)

export function refreshTemplates() {
  if (inflight) return inflight
  inflight = (async () => {
    try {
      templateStore.list = await listTemplates({})
      templateStore.loaded = true
    } catch {
      /* keep the previous list; the templates modal reports its own errors */
    } finally {
      inflight = null
    }
  })()
  return inflight
}
