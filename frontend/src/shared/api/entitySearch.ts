import { request } from './http.ts'

export type SearchFilter = { kind?: string; key: string; values: string[]; not?: boolean; all?: boolean }
export type EntitySearchRequest = {
  q: string
  types: string[]
  filters?: SearchFilter[]
  allowed?: Record<string, number[]>
  excluded?: Record<string, number[]>
  limit?: number
  offset?: number
}
export type EntitySearchHit = {
  kind: string
  id: number
  title: string
  where: string
  snippet: string
  score: number
  parent_kind?: string
  parent_id?: number | null
}
export type EntitySearchPage = { hits: EntitySearchHit[]; more: boolean; next: number }
export function searchEntities(query: EntitySearchRequest, signal?: AbortSignal) {
  return request<EntitySearchPage>('/api/search', { method: 'POST', body: JSON.stringify(query), signal })
}
