import { repositoryRefs } from '@/shared/lib/repositoryRefs.svelte.ts'

import { request } from './http.ts'
export type RepositoryLink = { kind: 'quest' | 'questline' | 'note'; id: number }
export type Repository = {
  id: number
  slug?: string
  clone_url?: string
  title: string
  kind: 'local' | 'github' | 'gitlab'
  url: string
  branch: string
  has_token: boolean
  bytes: number
  links: RepositoryLink[]
}
export type RepositoryView = {
  limited?: boolean
  clone_url: string
  branch: string
  branches: string[]
  commits: { id: string; title: string; author: string; date: string }[]
  tree: {
    name: string
    path: string
    kind: string
    id: string
    last_commit?: { id: string; title: string; author: string; date: string }
  }[]
  selected_commit: string
  empty: boolean
}
type Catalog = { items: Repository[]; quota_bytes: number }
let catalog: Promise<Catalog> | undefined
const views = new Map<string, Promise<RepositoryView>>()
let generation = 0
let catalogUntil = 0
export function resetRepositorySession() {
  generation++
  catalog = undefined
  views.clear()
  repositoryRefs.labels = {}
}
export function invalidateRepository(id: number) {
  catalog = undefined
  for (const key of views.keys()) if (key.startsWith(`${id}:`)) views.delete(key)
}
export function listRepositories() {
  if (!catalog || Date.now() > catalogUntil) {
    catalogUntil = Date.now() + 30_000
    const session = generation
    const pending = request<Catalog>('/api/repositories').then((result) => {
      if (session === generation)
        repositoryRefs.labels = Object.fromEntries(result.items.map((repo) => [`repo:${repo.id}`, repo.title]))
      return result
    })
    catalog = pending
    void pending.catch(() => {
      if (catalog === pending) catalog = undefined
    })
  }
  return catalog
}
async function changed<T>(operation: Promise<T>, id?: number): Promise<T> {
  const result = await operation
  catalog = undefined
  if (id !== undefined) invalidateRepository(id)
  return result
}
export const createRepository = (body: {
  title: string
  slug?: string
  kind: string
  url: string
  branch: string
  token: string
  owner?: RepositoryLink
}) => changed(request<{ id: number }>('/api/repositories', { method: 'POST', body: JSON.stringify(body) }))
export const updateRepository = (
  id: number,
  body: { url?: string; slug?: string; title?: string; branch?: string; token?: string }
) => changed(request(`/api/repositories/${id}`, { method: 'PATCH', body: JSON.stringify(body) }), id)
export const linkRepository = (id: number, owner: RepositoryLink) =>
  changed(request(`/api/repositories/${id}/links`, { method: 'POST', body: JSON.stringify(owner) }), id)
export const unlinkRepository = (id: number, owner: RepositoryLink) =>
  changed(request(`/api/repositories/${id}/links?kind=${owner.kind}&owner=${owner.id}`, { method: 'DELETE' }), id)
/** In-memory session cache: bounded, coalesced, never persisted to disk. */
export async function browseRepository(
  id: number,
  branch: string,
  path: string,
  signal?: AbortSignal,
  commit = '',
  refresh = false
) {
  if (refresh) invalidateRepository(id)
  const key = `${id}:${JSON.stringify([branch, path, commit])}`
  let pending = views.get(key)
  if (!pending) {
    pending = request<RepositoryView>(
      `/api/repositories/${id}/browse?branch=${encodeURIComponent(branch)}&path=${encodeURIComponent(path)}&commit=${encodeURIComponent(commit)}${refresh ? '&refresh=1' : ''}`
    )
    if (views.size >= 128) views.delete(views.keys().next().value!)
    views.set(key, pending)
    const current = pending
    void pending.catch(() => {
      if (views.get(key) === current) views.delete(key)
    })
  }
  // A departing consumer must not cancel a request another view is awaiting.
  const result = await pending
  if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')
  return result
}
