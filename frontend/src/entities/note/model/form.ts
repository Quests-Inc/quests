// Форма заметки: поля, которые правятся на странице, и сравнение с сохранённым.

export interface NoteForm {
  title: string
  description: string
  parentId: string
  pinned: boolean
  isReadme: boolean
  isPrivate: boolean
  isCategory: boolean
}

type Row = {
  title?: string | null
  description?: string | null
  parent_id?: number | null
  pinned?: boolean
  is_readme?: boolean
  is_private?: boolean
  is_category?: boolean
}

/** Поля формы из заметки, пришедшей с сервера. */
export function noteFormFromRow(row: Row): NoteForm {
  return {
    title: row.title || '',
    description: row.description || '',
    parentId: row.parent_id != null ? String(row.parent_id) : '',
    pinned: !!row.pinned,
    isReadme: !!row.is_readme,
    isPrivate: !!row.is_private,
    isCategory: !!row.is_category,
  }
}

export function noteFormsEqual(a: NoteForm, b: NoteForm): boolean {
  return (
    a.title === b.title &&
    a.description === b.description &&
    a.parentId === b.parentId &&
    a.pinned === b.pinned &&
    a.isReadme === b.isReadme &&
    a.isPrivate === b.isPrivate &&
    a.isCategory === b.isCategory
  )
}
