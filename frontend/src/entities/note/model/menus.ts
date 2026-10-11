// Дерево заметок и его меню: что видно в боковой панели, хлебные крошки, пункты контекстных меню.
import { compareNotes, isUnder } from './tree.ts'

export interface MenuItem {
  id: string
  label?: string
  sep?: boolean
  danger?: boolean
  checked?: boolean
  children?: MenuItem[]
}

type Row = {
  id: number
  parent_id: number | null
  title?: string
  is_readme?: boolean
  pinned?: boolean
  is_private?: boolean
  is_category?: boolean
  outdated?: boolean
  sort_order?: number
}

const byId = (notes: Row[], id: number | null | undefined) =>
  id == null ? null : (notes.find((n) => n.id === id) ?? null)

/** A note is visible in the sidebar when every ancestor up to the root is expanded (or it has no parent). */
function isVisible(notes: Row[], id: number, showChildren: (id: number) => boolean): boolean {
  const n = byId(notes, id)
  if (!n) return false
  if (n.parent_id == null) return true
  if (!isVisible(notes, n.parent_id, showChildren)) return false
  return showChildren(n.parent_id)
}

/** Walking up from a hidden note, the closest ancestor that is itself visible. */
function nearestVisibleAncestor(notes: Row[], id: number, showChildren: (id: number) => boolean): number | null {
  const n = byId(notes, id)
  let cur = byId(notes, n?.parent_id)
  while (cur) {
    if (isVisible(notes, cur.id, showChildren)) return cur.id
    cur = byId(notes, cur.parent_id)
  }
  return null
}

/**
 * Notes that aren't README themselves but have a README descendant currently hidden behind a collapsed ancestor —
 * bubbled up to the deepest visible ancestor so a collapsed README stays discoverable.
 */
export function readmeBubbles(notes: Row[], showChildren: (id: number) => boolean): Set<number> {
  const out = new Set<number>()
  for (const n of notes) {
    if (!n.is_readme || isVisible(notes, n.id, showChildren)) continue
    const anc = nearestVisibleAncestor(notes, n.id, showChildren)
    if (anc != null) out.add(anc)
  }
  return out
}

/** Breadcrumbs from the root down to the note's parent (`parentId` is '' for a root note). */
export function noteCrumbs(notes: Row[], parentId: string): { id: number | null; title: string }[] {
  const out: { id: number | null; title: string }[] = [{ id: null, title: 'Корень' }]
  const pid = parentId === '' ? null : Number(parentId)
  if (pid == null || Number.isNaN(pid)) return out
  const chain: { id: number; title: string }[] = []
  let cur = byId(notes, pid)
  const seen = new Set<number>()
  while (cur && !seen.has(cur.id)) {
    seen.add(cur.id)
    chain.unshift({ id: cur.id, title: cur.title || `note=${cur.id}` })
    cur = byId(notes, cur.parent_id)
  }
  return out.concat(chain)
}

/** Where the open note can go: the root and every note that is not itself or below it. */
export function parentMenu(notes: Row[], selectedId: number | null): MenuItem[] {
  const items: MenuItem[] = [{ id: 'root', label: '— корень —' }]
  if (selectedId == null) return items
  for (const n of notes) {
    if (n.id === selectedId) continue
    if (isUnder(notes, n.id, selectedId)) continue
    items.push({ id: String(n.id), label: n.title || `note=${n.id}` })
  }
  return items
}

/**
 * "Переместить" submenu: one level up, the root, and flat neighbours — the parent's level (the note keeps its depth)
 * and its own level (it nests one deeper). Own descendants are excluded, so there is never an ambiguous re-parenting.
 */
export function moveTargetItems(note: Row | null, notes: Row[], hasChildren: (id: number) => boolean): MenuItem[] {
  const items: MenuItem[] = []
  if (note?.parent_id != null) items.push({ id: 'move-up', label: 'На уровень вверх' })
  items.push({ id: 'move:root', label: 'Корень', checked: note?.parent_id == null })
  const pid = note?.parent_id ?? null
  const parent = byId(notes, pid)
  const hasKids = (n: { id: number }) => hasChildren(n.id)
  const level = (parentOf: number | null) =>
    notes
      .filter(
        (n) =>
          (n.parent_id ?? null) === parentOf &&
          n.id !== note?.id &&
          !(note?.id != null && isUnder(notes, n.id, note.id))
      )
      .sort((a, b) => compareNotes(a, b, hasKids))
  const groups = [...(parent ? [level(parent.parent_id ?? null)] : []), level(pid)]
  let listed = false
  for (const group of groups) {
    if (!group.length) continue
    if (!listed) {
      items.push({ id: 'sep-move', sep: true })
      listed = true
    } else {
      items.push({ id: `sep-move-${items.length}`, sep: true })
    }
    for (const n of group) {
      items.push({ id: `move:${n.id}`, label: n.title || `note=${n.id}`, checked: note?.parent_id === n.id })
    }
  }
  return items
}

/** Context menu of a note row. */
export function noteMenuItems({
  noteId,
  note,
  searchActive,
  exportBusy,
  moveTargets,
}: {
  noteId: number | null
  note: Row | null
  searchActive: boolean
  exportBusy: boolean
  moveTargets: MenuItem[]
}): MenuItem[] {
  if (noteId == null) return []
  return [
    { id: 'copy-id', label: `Копировать note=${noteId}` },
    { id: 'sep-copy', sep: true },
    ...(searchActive ? [] : [{ id: 'rename', label: 'Переименовать' }]),
    {
      id: 'flags',
      label: 'Флаги',
      children: [
        { id: 'pin', label: note?.pinned ? 'Снять закреп' : 'Закрепить' },
        { id: 'readme', label: note?.is_readme ? 'Снять README' : 'Пометить README' },
        { id: 'private', label: note?.is_private ? 'Снять PRIVATE' : 'Пометить PRIVATE' },
        { id: 'category', label: note?.is_category ? 'Снять CATEGORY' : 'Пометить CATEGORY' },
        { id: 'outdated', label: note?.outdated ? 'Снять «устарела»' : 'Пометить «устарела»' },
      ],
    },
    { id: 'add-child', label: 'Добавить дочернюю' },
    { id: 'move', label: 'Переместить', children: moveTargets },
    {
      id: 'export',
      label: 'Экспорт',
      children: [
        { id: 'export-md', label: 'Markdown (.md)' },
        { id: 'export-pdf', label: exportBusy ? 'PDF…' : 'PDF' },
      ],
    },
    { id: 'sep-icon', sep: true },
    { id: 'icon', label: 'Иконка' },
    { id: 'sep-danger', sep: true },
    { id: 'delete', label: 'Удалить', danger: true },
  ]
}
