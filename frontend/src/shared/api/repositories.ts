import { request } from './http.ts'
export type RepositoryLink = { kind: 'quest' | 'questline' | 'note'; id: number }
export type Repository = {
  id: number
  slug?: string
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
export const listRepositories = () => request<{ items: Repository[]; quota_bytes: number }>('/api/repositories')
export const createRepository = (body: {
  title: string
  slug?: string
  kind: string
  url: string
  branch: string
  token: string
  owner?: RepositoryLink
}) => request<{ id: number }>('/api/repositories', { method: 'POST', body: JSON.stringify(body) })
export const updateRepository = (
  id: number,
  body: { url?: string; slug?: string; title?: string; branch?: string; token?: string }
) => request(`/api/repositories/${id}`, { method: 'PATCH', body: JSON.stringify(body) })
export const linkRepository = (id: number, owner: RepositoryLink) =>
  request(`/api/repositories/${id}/links`, { method: 'POST', body: JSON.stringify(owner) })
export const unlinkRepository = (id: number, owner: RepositoryLink) =>
  request(`/api/repositories/${id}/links?kind=${owner.kind}&owner=${owner.id}`, { method: 'DELETE' })
export const browseRepository = (id: number, branch: string, path: string, signal?: AbortSignal, commit = '') =>
  request<RepositoryView>(
    `/api/repositories/${id}/browse?branch=${encodeURIComponent(branch)}&path=${encodeURIComponent(path)}&commit=${encodeURIComponent(commit)}`,
    { signal }
  )
