// Снимок вложений по владельцам, как его отдаёт GET /api/attachments.
import { listAllAttachments } from '@/shared/api/attachments.ts'
import type { AttachmentView } from '@/shared/api/knowledge_http.gen'

export type AttachmentIndex = Record<string, Record<string, AttachmentView[]>>

export const attachmentCatalog = $state({ index: null as AttachmentIndex | null })

/** С сервера, без записи в каталог. stat по умолчанию выключен — так грузит журнал. */
export function fetchAttachmentIndex() {
  return listAllAttachments()
}

export function setAttachmentIndex(index: AttachmentIndex | null) {
  attachmentCatalog.index = index
}

export function clearAttachmentIndex() {
  attachmentCatalog.index = null
}
