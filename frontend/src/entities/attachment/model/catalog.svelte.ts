// Снимок GET /api/attachments для списка на вкладке. Живые флаги WebDAV лежат отдельно, в кэше владельца.
import type { AttachmentView } from '@/shared/api/knowledge_http.gen'

export type AttachmentIndex = Record<string, Record<string, AttachmentView[]>>

export const attachmentCatalog = $state({ index: null as AttachmentIndex | null })

export function setAttachmentIndex(index: AttachmentIndex | null) {
  attachmentCatalog.index = index
}

export function clearAttachmentIndex() {
  attachmentCatalog.index = null
}
