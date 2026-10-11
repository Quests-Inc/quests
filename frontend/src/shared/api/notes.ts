import { request } from './http.ts'
import type { NoteCheckView, NoteListItem, NotePayload } from './knowledge_http.gen'

/** Set (or, with a null command, remove) a note's freshness check. `body`: { command, interval_seconds, run_on }. */
export function setNoteCheck(id: number, body: object) {
  return request<NoteCheckView>(`/api/notes/${id}/check`, { method: 'PUT', body: JSON.stringify(body) })
}

/** Ask for a run at the scheduler's next tick. */
export function runNoteCheck(id: number) {
  return request(`/api/notes/${id}/check/run`, { method: 'POST' })
}

export function listNotes() {
  return request<NoteListItem[]>('/api/notes')
}

export function getNote(id: number) {
  return request<NotePayload>(`/api/notes/${id}`)
}

export function createNote(payload: object) {
  return request<NotePayload>('/api/notes', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateNote(id: number, payload: object) {
  return request<NotePayload>(`/api/notes/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}

export function deleteNote(id: number) {
  return request(`/api/notes/${id}`, { method: 'DELETE' })
}

/** Upload custom note image (not added to built-in SVG pool). */
export async function uploadNoteIcon(id: number, file: File | Blob) {
  const body = new FormData()
  body.append('file', file)
  const res = await fetch(`/api/notes/${id}/icon`, {
    method: 'POST',
    body,
  })
  const text = await res.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      throw new Error(res.ok ? `Ответ не JSON` : `HTTP ${res.status}`)
    }
  }
  if (!res.ok) {
    const detail = data?.detail ?? res.statusText
    throw new Error(typeof detail === 'string' ? detail : JSON.stringify(detail))
  }
  return data
}

export function clearNoteIcon(id: number) {
  return request(`/api/notes/${id}/icon`, { method: 'DELETE' })
}

/** Ranked note search: [{ id, title, score, where: 'title'|'body'|'fuzzy', snippet (matches in ⟪ ⟫), … }]. */
export function searchNotes(q: string, limit = 30) {
  return request(`/api/notes/search?q=${encodeURIComponent(q)}&limit=${limit}`)
}

/** Search inside one note with the notes search's rules: { terms: matched words, passages: paragraph count }. */
export function searchInNote(id: number, q: string) {
  return request(`/api/notes/${id}/search?q=${encodeURIComponent(q)}`)
}

/** Hand-set OUTDATED flag of a note; a later check gives its own verdict. */
export function setNoteOutdated(id: number, outdated: boolean, reason = '') {
  return request(`/api/notes/${id}/outdated`, { method: 'PUT', body: JSON.stringify({ outdated, reason }) })
}
