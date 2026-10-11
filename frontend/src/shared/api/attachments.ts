import { onUnauthorized, UnauthorizedError, BASE, request } from './http.ts'
import type { AttachmentRevisions, AttachmentView } from './knowledge_http.gen'

function ownerAttachmentsPath(ownerType: string, ownerId: number, attachmentId?: number | null) {
  const seg = ownerType === 'questline' ? 'questlines' : ownerType === 'note' ? 'notes' : 'quests'
  const base = `/api/${seg}/${ownerId}/attachments`
  return attachmentId == null ? base : `${base}/${attachmentId}`
}

export function listAttachments(ownerType: string, ownerId: number, { stat = true } = {}) {
  const path = ownerAttachmentsPath(ownerType, ownerId)
  return request<AttachmentView[]>(stat ? path : `${path}?stat=0`)
}

/** All attachment metadata, grouped `{ quest: { [id]: [] }, questline: { … }, note: { … } }`.
 * Default skips WebDAV Stat — cheap enough to run on every journal load. */
export function listAllAttachments({ stat = false } = {}) {
  return request<Record<string, Record<string, AttachmentView[]>>>(
    stat ? '/api/attachments?stat=1' : '/api/attachments'
  )
}

export function attachmentDownloadUrl(
  ownerType: string,
  ownerId: number,
  attachmentId: number,
  revision: number | string | null = null
) {
  const base = ownerAttachmentsPath(ownerType, ownerId, attachmentId)
  if (revision == null || revision === '') return base
  return `${base}?revision=${encodeURIComponent(String(revision))}`
}

export async function uploadAttachment(ownerType: string, ownerId: number, file: File | Blob, comment = '') {
  const body = new FormData()
  body.append('file', file)
  if (comment) body.append('comment', comment)
  const res = await fetch(`${BASE}${ownerAttachmentsPath(ownerType, ownerId)}`, {
    method: 'POST',
    credentials: 'same-origin',
    body,
  })
  if (res.status === 401) {
    if (onUnauthorized) onUnauthorized()
    throw new UnauthorizedError()
  }
  const text = await res.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      throw new Error(res.ok ? 'Ответ не JSON' : `HTTP ${res.status}`)
    }
  }
  if (!res.ok) {
    const detail = data?.detail ?? res.statusText
    throw new Error(typeof detail === 'string' ? detail : JSON.stringify(detail))
  }
  return data
}

/** Upload a new revision of an existing attachment (becomes current). */
export async function uploadAttachmentRevision(
  ownerType: string,
  ownerId: number,
  attachmentId: number,
  file: File | Blob,
  comment = ''
) {
  const body = new FormData()
  body.append('file', file)
  if (comment) body.append('comment', comment)
  const res = await fetch(`${BASE}${ownerAttachmentsPath(ownerType, ownerId, attachmentId)}/revisions`, {
    method: 'POST',
    credentials: 'same-origin',
    body,
  })
  if (res.status === 401) {
    if (onUnauthorized) onUnauthorized()
    throw new UnauthorizedError()
  }
  const text = await res.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      throw new Error(res.ok ? 'Ответ не JSON' : `HTTP ${res.status}`)
    }
  }
  if (!res.ok) {
    const detail = data?.detail ?? res.statusText
    throw new Error(typeof detail === 'string' ? detail : JSON.stringify(detail))
  }
  return data
}

export function listAttachmentRevisions(ownerType: string, ownerId: number, attachmentId: number) {
  return request<AttachmentRevisions>(`${ownerAttachmentsPath(ownerType, ownerId, attachmentId)}/revisions`)
}

export function setAttachmentCurrentRevision(
  ownerType: string,
  ownerId: number,
  attachmentId: number,
  revision: number | string | null
) {
  return request(ownerAttachmentsPath(ownerType, ownerId, attachmentId), {
    method: 'PATCH',
    body: JSON.stringify({ current_revision: revision }),
  })
}

export function updateAttachmentComment(ownerType: string, ownerId: number, attachmentId: number, comment: string) {
  return request(ownerAttachmentsPath(ownerType, ownerId, attachmentId), {
    method: 'PATCH',
    body: JSON.stringify({ comment }),
  })
}

export function setAttachmentHidden(ownerType: string, ownerId: number, attachmentId: number, hidden: boolean) {
  return request(ownerAttachmentsPath(ownerType, ownerId, attachmentId), {
    method: 'PATCH',
    body: JSON.stringify({ hidden: !!hidden }),
  })
}

export function deleteAttachment(ownerType: string, ownerId: number, attachmentId: number) {
  return request(ownerAttachmentsPath(ownerType, ownerId, attachmentId), {
    method: 'DELETE',
  })
}

export function deleteAttachmentRevision(
  ownerType: string,
  ownerId: number,
  attachmentId: number,
  revision: number | string | null
) {
  return request(`${ownerAttachmentsPath(ownerType, ownerId, attachmentId)}/revisions/${revision}`, {
    method: 'DELETE',
  })
}
