import { assertUploadSize } from '@/shared/lib/uploadLimit.ts'

import { onUnauthorized, UnauthorizedError, BASE, request } from './http.ts'
import type { AttachmentRevisions, AttachmentView } from './knowledge_http.gen'

async function postForm(url: string, body: FormData) {
  try {
    return await fetch(url, { method: 'POST', credentials: 'same-origin', body })
  } catch (e) {
    if (e instanceof TypeError) {
      throw new Error('Не удалось отправить файл: соединение оборвалось', { cause: e })
    }
    throw e
  }
}

async function readUpload(res: Response) {
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

function ownerAttachmentsPath(ownerType: string, ownerId: number, attachmentId?: number | null) {
  const seg =
    ownerType === 'workspace'
      ? 'workspaces'
      : ownerType === 'questline'
        ? 'questlines'
        : ownerType === 'note'
          ? 'notes'
          : 'quests'
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
  assertUploadSize(file)
  const body = new FormData()
  body.append('file', file)
  if (comment) body.append('comment', comment)
  return readUpload(await postForm(`${BASE}${ownerAttachmentsPath(ownerType, ownerId)}`, body))
}

/** Upload a new revision of an existing attachment (becomes current). */
export async function uploadAttachmentRevision(
  ownerType: string,
  ownerId: number,
  attachmentId: number,
  file: File | Blob,
  comment = ''
) {
  assertUploadSize(file)
  const body = new FormData()
  body.append('file', file)
  if (comment) body.append('comment', comment)
  return readUpload(await postForm(`${BASE}${ownerAttachmentsPath(ownerType, ownerId, attachmentId)}/revisions`, body))
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

/** Small inert snippets; preview failures stay local to the card, without toast spam. */
export async function getAttachmentPreview(
  id: number,
  revision: number,
  signal: AbortSignal
): Promise<{ kind: string; text: string }> {
  const response = await fetch(`${BASE}/api/attachments/${id}/preview?revision=${revision}`, {
    credentials: 'same-origin',
    signal,
  })
  if (response.status === 401) onUnauthorized?.()
  if (!response.ok) throw new Error(`Preview HTTP ${response.status}`)
  return response.json()
}

/** A workspace file with no quest/note owner. It outlives the board that displays it. */
export async function uploadUnboundAttachment(file: File) {
  assertUploadSize(file)
  const body = new FormData()
  body.append('file', file)
  const response = await postForm(`${BASE}/api/attachments`, body)
  const data = await response.json()
  if (!response.ok) throw new Error(data.detail || `HTTP ${response.status}`)
  return data as AttachmentView
}
