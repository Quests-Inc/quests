import type { SourceRunStats } from './execution.gen'
import { request } from './http.ts'

export type Hook = {
  id: number
  name: string
  enabled: boolean
  events: string[]
  events_raw: string[]
  script_id: number
  quest_id?: number | null
  /** workspace | quest | step | note | note_tree */
  scope_kind: string
  scope_id?: number | null
  cooldown_sec: number
  max_per_hour: number
  include_private: boolean
  /** why the dispatcher switched the hook off (a storm), empty otherwise */
  disabled_reason?: string
  run_on: string
  timeout_sec: number
  script_title?: string
  needs_runner?: boolean
  created_at?: string
  updated_at?: string
  /** how its runs went lately (the newest, how many in the last hour); on the list only */
  runs?: SourceRunStats
}

export type HookEventInfo = { kind: string; family: string; title: string; fields: string[] }

export type HookEventsCatalog = {
  aliases: Record<string, string[]>
  kinds: string[]
  events?: HookEventInfo[]
}

/** The area and the limits of a hook (the part of the body that is not the old quest_id). */
export type HookArea = {
  scope_kind?: string
  scope_id?: number | null
  cooldown_sec?: number
  max_per_hour?: number
  include_private?: boolean
}

export function getHooks() {
  return request<Hook[]>('/api/hooks')
}

export function getHook(id: number) {
  return request<Hook>(`/api/hooks/${id}`)
}

export function getHookEvents() {
  return request<HookEventsCatalog>('/api/hook-events')
}

export function createHook(
  body: {
    name?: string
    enabled?: boolean
    events: string[]
    script_id: number
    quest_id?: number | null
    run_on?: string
    timeout_sec?: number
  } & HookArea
) {
  return request<Hook>('/api/hooks', { method: 'POST', body: JSON.stringify(body) })
}

export function updateHook(
  id: number,
  body: Partial<{
    name: string
    enabled: boolean
    events: string[]
    script_id: number
    quest_id: number | null
    run_on: string
    timeout_sec: number
  }> &
    HookArea
) {
  return request<Hook>(`/api/hooks/${id}`, { method: 'PATCH', body: JSON.stringify(body) })
}

export function deleteHook(id: number) {
  return request(`/api/hooks/${id}`, { method: 'DELETE' })
}
