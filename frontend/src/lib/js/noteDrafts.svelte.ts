export type NoteDraft = { title: string; description: string; parentId: string; pinned: boolean }

/** Unsaved note edits keyed by note id. Missing key = last saved server copy. */
export const noteDrafts: Record<string, NoteDraft> = $state({})

export function noteDraftKey(id: number | string | null | undefined) {
  return id == null ? '' : String(id)
}

export function getNoteDraft(id: number | string | null | undefined) {
  const key = noteDraftKey(id)
  return key ? (noteDrafts[key] ?? null) : null
}

export function putNoteDraft(id: number | string, draft: NoteDraft) {
  noteDrafts[noteDraftKey(id)] = {
    title: draft.title,
    description: draft.description,
    parentId: draft.parentId,
    pinned: !!draft.pinned,
  }
}

export function clearNoteDraft(id: number | string | null | undefined) {
  const key = noteDraftKey(id)
  if (key) delete noteDrafts[key]
}
