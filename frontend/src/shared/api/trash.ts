import { request } from './http.ts'

/** A deleted quest or note: it lives in the trash for 30 days (`purge_at`), then the scheduler erases it. */
export type TrashItem = {
  kind: 'quest' | 'note'
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
}

/** Quests and notes in the trash, newest deleted first. */
export async function listTrash(): Promise<TrashItem[]> {
  const [quests, notes] = await Promise.all([
    request<TrashItem[]>('/api/trash/quests'),
    request<TrashItem[]>('/api/trash/notes'),
  ])
  return [...(quests ?? []), ...(notes ?? [])].sort((a, b) => (a.deleted_at < b.deleted_at ? 1 : -1))
}

export function restoreFromTrash(kind: TrashItem['kind'], id: number) {
  return request(`/api/trash/${kind}s/${id}/restore`, { method: 'POST' })
}

/** Erase one item for good (its attachments too). */
export function purgeFromTrash(kind: TrashItem['kind'], id: number) {
  return request(`/api/trash/${kind}s/${id}`, { method: 'DELETE' })
}

export async function emptyTrash() {
  await Promise.all([
    request('/api/trash/quests', { method: 'DELETE' }),
    request('/api/trash/notes', { method: 'DELETE' }),
  ])
}
