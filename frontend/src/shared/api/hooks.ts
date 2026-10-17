import { request } from './http.ts'

export type Hook = {
  id: number
  name: string
  enabled: boolean
  events: string[]
  events_raw: string[]
  script_id: number
  quest_id?: number | null
  run_on: string
  timeout_sec: number
  script_title?: string
  needs_runner?: boolean
  created_at?: string
  updated_at?: string
}

export type HookEventsCatalog = {
  aliases: Record<string, string[]>
  kinds: string[]
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

export function createHook(body: {
  name?: string
  enabled?: boolean
  events: string[]
  script_id: number
  quest_id?: number | null
  run_on?: string
  timeout_sec?: number
}) {
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
  }>
) {
  return request<Hook>(`/api/hooks/${id}`, { method: 'PATCH', body: JSON.stringify(body) })
}

export function deleteHook(id: number) {
  return request(`/api/hooks/${id}`, { method: 'DELETE' })
}
