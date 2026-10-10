/** @typedef {{ title: string, description: string, parentId: string, pinned: boolean }} NoteDraft */

/** Unsaved note edits keyed by note id. Missing key = last saved server copy. */
export const noteDrafts = $state(/** @type {Record<string, NoteDraft>} */ ({}))

/** @param {number | string | null | undefined} id */
export function noteDraftKey(id) {
  return id == null ? '' : String(id)
}

/** @param {number | string | null | undefined} id */
export function getNoteDraft(id) {
  const key = noteDraftKey(id)
  return key ? (noteDrafts[key] ?? null) : null
}

/**
 * @param {number | string} id
 * @param {NoteDraft} draft
 */
export function putNoteDraft(id, draft) {
  noteDrafts[noteDraftKey(id)] = {
    title: draft.title,
    description: draft.description,
    parentId: draft.parentId,
    pinned: !!draft.pinned,
  }
}

/** @param {number | string | null | undefined} id */
export function clearNoteDraft(id) {
  const key = noteDraftKey(id)
  if (key) delete noteDrafts[key]
}
