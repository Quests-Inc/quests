// Общий список заметок. Оболочка пишет его после загрузки; журнал, карта, заметки и вложения читают отсюда.
import type { NoteListItem } from '@/shared/api/knowledge_http.gen'

export const noteCatalog = $state({ list: [] as NoteListItem[] })

export function setNotes(list: NoteListItem[]) {
  noteCatalog.list = list
}

export function clearNotes() {
  noteCatalog.list = []
}
