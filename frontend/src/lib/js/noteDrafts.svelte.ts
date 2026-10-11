/** @typedef {{ title: string, description: string, parentId: string, pinned: boolean }} NoteDraft */
export type NoteDraft = { title: string; description: string; parentId: string; pinned: boolean }

/** Unsaved note edits keyed by note id. Missing key = last saved server copy. */
export const noteDrafts: Record<string, NoteDraft> = $state({})

/** @param {number | string | null | undefined} id */
export function noteDraftKey(id: number | string | null | undefined) {
  return id == null ? '' : String(id)
}

/** @param {number | string | null | undefined} id */
export function getNoteDraft(id: number | string | null | undefined) {
  const key = noteDraftKey(id)
  return key ? (noteDrafts[key] ?? null) : null
}

/**
 * @param {number | string} id
 * @param {NoteDraft} draft
 */
export function putNoteDraft(id: number | string, draft: NoteDraft) {
  noteDrafts[noteDraftKey(id)] = {
    title: draft.title,
    description: draft.description,
    parentId: draft.parentId,
    pinned: !!draft.pinned,
  }
}

/** @param {number | string | null | undefined} id */
export function clearNoteDraft(id: number | string | null | undefined) {
  const key = noteDraftKey(id)
  if (key) delete noteDrafts[key]
}
