// Общий список заметок. Оболочка пишет его целиком после загрузки; страницы читают отсюда.
import type { NoteListItem } from '@/shared/api/knowledge_http.gen'
import { listNotes } from '@/shared/api/notes.ts'

export const noteCatalog = $state({ list: [] as NoteListItem[] })

/** С сервера, без записи в каталог. */
export function fetchNoteList() {
  return listNotes()
}

export function setNotes(list: NoteListItem[]) {
  noteCatalog.list = list
}

export function clearNotes() {
  noteCatalog.list = []
}
