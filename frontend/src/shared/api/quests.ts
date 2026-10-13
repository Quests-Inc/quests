import { toast } from '@/shared/lib/toasts.svelte.ts'

import { onUnauthorized, UnauthorizedError, BASE, request, quietQs } from './http.ts'
import type { Category, QuestRead, QuestlineRead, Tag } from './journal.gen'

export function listQuests(params: { status?: string; pinned?: boolean | null } = {}) {
  const q = new URLSearchParams()
  if (params.status) q.set('status', params.status)
  if (params.pinned != null) q.set('pinned', String(params.pinned))
  const qs = q.toString()
  return request<QuestRead[]>(`/api/quests${qs ? `?${qs}` : ''}`)
}

/** Manual web edits are quiet by default (no overlay toasts). */
export function createQuest(payload: object, { quiet = true } = {}) {
  return request(`/api/quests${quietQs(quiet)}`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateQuest(id: number, payload: object, { quiet = true } = {}) {
  return request(`/api/quests/${id}${quietQs(quiet)}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}

/** Hide a failed quest from the journal «Провалено» inbox (status stays failed). */
export function ackQuestFailure(id: number, { quiet = true } = {}) {
  return request(`/api/quests/${id}/ack-failure${quietQs(quiet)}`, {
    method: 'POST',
  })
}

export function addQuestStep(questId: number, payload: object, { quiet = true } = {}) {
  return request(`/api/quests/${questId}/steps${quietQs(quiet)}`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateQuestStep(questId: number, stepId: number, payload: object, { quiet = true } = {}) {
  return request(`/api/quests/${questId}/steps/${stepId}${quietQs(quiet)}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}

export function deleteQuestStep(questId: number, stepId: number, { quiet = true } = {}) {
  return request(`/api/quests/${questId}/steps/${stepId}${quietQs(quiet)}`, {
    method: 'DELETE',
  })
}

export function deleteQuest(id: number, { quiet = true } = {}) {
  return request(`/api/quests/${id}${quietQs(quiet)}`, { method: 'DELETE' })
}

export function listCategories() {
  return request<Category[]>('/api/categories')
}

export function listTags(q = '') {
  const qs = q ? `?q=${encodeURIComponent(q)}` : ''
  return request<Tag[]>(`/api/tags${qs}`)
}

/** Create tag; on slug conflict returns the existing tag (409 body). */
export async function createTag(payload: object) {
  const res = await fetch(`${BASE}/api/tags`, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    method: 'POST',
    body: JSON.stringify(payload),
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
      const msg = `HTTP ${res.status}: ${text.slice(0, 200)}`
      toast(msg, { kind: 'error' })
      throw new Error(msg)
    }
  }
  if (res.status === 409 && data) return data
  if (!res.ok) {
    const detail = data?.detail ?? res.statusText
    const msg = typeof detail === 'string' ? detail : JSON.stringify(detail)
    toast(msg, { kind: 'error' })
    throw new Error(msg)
  }
  return data
}

export function updateTag(id: number, payload: object) {
  return request(`/api/tags/${id}`, { method: 'PATCH', body: JSON.stringify(payload) })
}

export function deleteTag(id: number) {
  return request(`/api/tags/${id}`, { method: 'DELETE' })
}

export function setQuestTags(questId: number, tagIds: number[]) {
  return request(`/api/quests/${questId}/tags`, {
    method: 'PUT',
    body: JSON.stringify({ tag_ids: tagIds }),
  })
}

export function listQuestlines() {
  return request<QuestlineRead[]>('/api/questlines')
}

export function createQuestline(payload: object) {
  return request('/api/questlines', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateQuestline(id: number, payload: object) {
  return request(`/api/questlines/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}

export function deleteQuestline(id: number) {
  return request(`/api/questlines/${id}`, { method: 'DELETE' })
}

/** Upload custom questline image (not added to built-in SVG pool). */
export async function uploadQuestlineIcon(id: number, file: File | Blob) {
  const body = new FormData()
  body.append('file', file)
  const res = await fetch(`/api/questlines/${id}/icon`, {
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

export function clearQuestlineIcon(id: number) {
  return request(`/api/questlines/${id}/icon`, { method: 'DELETE' })
}

// --- auth ---

/**
 * Change-log feed. `params`: limit, entity_type (csv: quest|note|attachment|all),
 * entity_id, quest_id, kind (csv), questline_id, category_id, tag_id,
 * from/to (ISO), q, before_id + before_at (keyset cursor).
 */
export function getQuestLog(params: Record<string, string | number | null | undefined> = {}) {
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v != null && v !== '') q.set(k, String(v))
  }
  const qs = q.toString()
  return request(`/api/quest-log${qs ? `?${qs}` : ''}`)
}

/** One quest with its steps and tags. */
export function getQuestFull(id: number) {
  return request<QuestRead>(`/api/quests/${id}`)
}

export * from './questConsts.ts'
