import { request } from './http.ts'

/** A deleted quest, note, script or hook: it lives in the trash for 30 days (`purge_at`), then the scheduler erases it. */
export type TrashItem = {
  kind: 'quest' | 'note' | 'script' | 'hook' | 'template' | 'questline'
  id: number
  title: string
  deleted_at: string
  deleted_by?: number | null
  purge_at?: string | null
  /** quest */
  status?: string
  steps_total?: number
  steps_done?: number
  /** note */
  size?: number
  children?: number
  /** hook: its script, and whether that script is in the trash too (restore it first) */
  script_id?: number
  script_trashed?: boolean
}

const KINDS = ['quests', 'notes', 'scripts', 'hooks', 'templates', 'questlines'] as const

/** Everything in the trash, newest deleted first. */
export async function listTrash(): Promise<TrashItem[]> {
  const parts = await Promise.all(KINDS.map((k) => request<TrashItem[]>(`/api/trash/${k}`)))
  return parts.flatMap((p) => p ?? []).sort((a, b) => (a.deleted_at < b.deleted_at ? 1 : -1))
}

export function restoreFromTrash(kind: TrashItem['kind'], id: number) {
  return request(`/api/trash/${kind}s/${id}/restore`, { method: 'POST' })
}

/** Erase one item for good (its attachments too). */
export function purgeFromTrash(kind: TrashItem['kind'], id: number) {
  return request(`/api/trash/${kind}s/${id}`, { method: 'DELETE' })
}

/** Hooks go before scripts: erasing a script takes its trashed hooks anyway, and the order keeps both calls clean. */
export async function emptyTrash() {
  await Promise.all([
    request('/api/trash/quests', { method: 'DELETE' }),
    request('/api/trash/notes', { method: 'DELETE' }),
  ])
  await request('/api/trash/templates', { method: 'DELETE' })
  await request('/api/trash/questlines', { method: 'DELETE' })
  await request('/api/trash/hooks', { method: 'DELETE' })
  await request('/api/trash/scripts', { method: 'DELETE' })
}
