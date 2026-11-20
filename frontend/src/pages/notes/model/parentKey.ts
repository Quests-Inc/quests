/** Grouping key for the tree: the open note's live editor parent (even before
 * it is written into noteDrafts), else its draft, else the server parent.
 * Picking a new parent moves the row immediately.
 *
 * Trust the editor parent only once it is loaded for this note. Right after
 * a switch, selectedId has moved but getNote has not landed, so the editor
 * still holds the previous note and the row would flash. */
export function parentKeyOf(
  n: { id: number; parent_id: number | null },
  selectedId: number | null,
  editor: { loadedId: number | null; parentId: string },
  draftParentId: string | undefined
) {
  if (n.id === selectedId && editor.loadedId === selectedId) return editor.parentId === '' ? 'root' : editor.parentId
  if (draftParentId !== undefined) return draftParentId === '' ? 'root' : draftParentId
  return n.parent_id == null ? 'root' : String(n.parent_id)
}
