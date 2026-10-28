import { toast } from '@/shared/lib/toasts.svelte.ts'

import type { GraphDraftView } from './graphdraft.gen'
import { BASE, request } from './http.ts'
import type { Graph } from './store.gen'

/**
 * Relations map around one entity (plus optional extra roots).
 * @param expand cards whose direct neighbours are added whatever the depth
 */
export function getGraph(
  type: string,
  id: number,
  depth: number = 2,
  extra: { type: string; id: number }[] = [],
  expand: { type: string; id: number; only?: string }[] = []
) {
  const more = extra.map((r) => `${r.type}:${r.id}`).join(',')
  const open = expand.map((r) => `${r.type}:${r.id}${r.only ? `>${r.only}` : ''}`).join(',')
  return request<Graph>(
    `/api/graph?${encodeURIComponent(type)}=${id}&depth=${depth}${more ? `&extra=${encodeURIComponent(more)}` : ''}${open ? `&expand=${encodeURIComponent(open)}` : ''}`
  )
}

// --- Map draft: edits made on the relations map, kept on the server until saved.

export function getGraphDraft() {
  return request<GraphDraftView>('/api/graph-draft')
}

export function addGraphDraftOp(kind: string, payload: any) {
  return request('/api/graph-draft/ops', { method: 'POST', body: JSON.stringify({ kind, payload }) })
}

/** @param dry dry = only report what would go with it */
export function deleteGraphDraftOp(id: number, dry: boolean = false) {
  return request(`/api/graph-draft/ops/${id}${dry ? '?dry=1' : ''}`, { method: 'DELETE' })
}

export function putGraphDraftRoots(roots: { type: string; id: number }[]) {
  return request('/api/graph-draft/roots', { method: 'PUT', body: JSON.stringify({ roots }) })
}

export function cancelGraphDraft() {
  return request('/api/graph-draft', { method: 'DELETE' })
}

/** Throws on failure; a 422 carries `problems` in the thrown error's `problems`. */
export async function commitGraphDraft() {
  const res = await fetch(`${BASE}/api/graph-draft/commit`, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', 'X-Quests-Source': 'web' },
  })
  const data = await res.json().catch((): any => null)
  if (!res.ok) {
    const err = new Error(data?.error || data?.detail || `HTTP ${res.status}`)
    // @ts-ignore
    err.problems = data?.problems || []
    // @ts-ignore
    err.failedOp = data?.failed_op ?? null
    throw err
  }
  return data
}

/**
 * Stages a file in the draft.
 * @param owner "quest:5" etc.
 */
export async function uploadGraphDraftFile(file: File, owner: string = '') {
  const fd = new FormData()
  fd.append('file', file)
  if (owner) fd.append('owner', owner)
  const res = await fetch(`${BASE}/api/graph-draft/files`, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'X-Quests-Source': 'web' },
    body: fd,
  })
  const data = await res.json().catch((): any => null)
  if (!res.ok) {
    const msg = typeof data?.detail === 'string' ? data.detail : `HTTP ${res.status}`
    toast(msg, { kind: 'error' })
    throw new Error(msg)
  }
  return data
}

/** Replaces the payload of a created thing in the draft. */
export function putGraphDraftOp(id: number, payload: any) {
  return request(`/api/graph-draft/ops/${id}`, { method: 'PUT', body: JSON.stringify({ payload }) })
}

// --- Runners: devices that execute checks (quests-runner), and their job queue.

/** A saved layout of the map (personal): the list has no `state`, one layout read by id has it. */
export type GraphLayout = { id: number; title: string; state?: unknown; created_at: string; updated_at: string }

export function listGraphLayouts() {
  return request<GraphLayout[]>('/api/graph-layouts')
}

export function getGraphLayout(id: number) {
  return request<GraphLayout>(`/api/graph-layouts/${id}`)
}

export function createGraphLayout(title: string, state: unknown) {
  return request<GraphLayout>('/api/graph-layouts', { method: 'POST', body: JSON.stringify({ title, state }) })
}

export function updateGraphLayout(id: number, patch: { title?: string; state?: unknown }) {
  return request<GraphLayout>(`/api/graph-layouts/${id}`, { method: 'PATCH', body: JSON.stringify(patch) })
}

export function deleteGraphLayout(id: number) {
  return request(`/api/graph-layouts/${id}`, { method: 'DELETE' })
}
