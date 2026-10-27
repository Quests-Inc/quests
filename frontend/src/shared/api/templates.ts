import type { TemplateRead } from './automation.gen'
import { request } from './http.ts'

export function listTemplates(params: { enabled?: boolean | null } = {}) {
  const q = new URLSearchParams()
  if (params.enabled != null) q.set('enabled', String(params.enabled))
  const qs = q.toString()
  return request<TemplateRead[]>(`/api/templates${qs ? `?${qs}` : ''}`)
}

export function createTemplate(payload: object) {
  return request<TemplateRead>('/api/templates', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateTemplate(id: number, payload: object) {
  return request<TemplateRead>(`/api/templates/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}

export function deleteTemplate(id: number) {
  return request(`/api/templates/${id}`, { method: 'DELETE' })
}

export function copyTemplate(id: number) {
  return request<TemplateRead>(`/api/templates/${id}/copy`, { method: 'POST' })
}

/**
 * Force-materialize one quest from a template. A template that makes its quest by a script waits for the device (up to a
 * minute); when it made none, the answer says so (`emitted: false`) instead of being a quest.
 */
export function emitTemplate(id: number) {
  return request<{ title?: string; emitted?: boolean; outcome?: string; detail?: string }>(
    `/api/templates/${id}/emit`,
    { method: 'POST' }
  )
}

/** What a test of a template's emit found out on its device; nothing is created. */
export interface EmitPreview {
  outcome: 'quest' | 'miss' | 'failed' | 'pending'
  title?: string
  description?: string
  steps?: { title: string; description?: string }[]
  ignored_fields?: string[]
  error?: string
}

export function emitTemplateTest(id: number) {
  return request<EmitPreview>(`/api/templates/${id}/emit-test`, { method: 'POST' })
}

const SECRET_OWNER_PATH = {
  questline: 'questlines',
  template: 'templates',
  quest: 'quests',
  step: 'steps',
  note: 'notes',
}

/** Kinds that can own secrets (a secret is inherited step > quest > template > questline). */
export const SECRET_OWNER_KINDS = Object.keys(SECRET_OWNER_PATH)

function secretsPath(kind: string, id: number) {
  if (kind === 'workspace') return '/api/workspace/secrets'
  return `/api/${SECRET_OWNER_PATH[kind as keyof typeof SECRET_OWNER_PATH]}/${id}/secrets`
}

/** Names and sources only; secret values are never readable. */
export interface SecretSource {
  key: string
  from: string
  id: number
  title?: string
}
export async function listSecrets(
  kind: string,
  id: number
): Promise<{
  keys: string[]
  own: SecretSource[]
  inherited: SecretSource[]
  effective: SecretSource[]
}> {
  const out = await request(secretsPath(kind, id))
  const keys: string[] = out?.keys ?? []
  const own = out?.own ?? keys.map((key) => ({ key, from: kind, id }))
  const inherited = out?.inherited ?? []
  return { keys, own, inherited, effective: out?.effective ?? [...own, ...inherited] }
}

/** Write-only: sets/overwrites one secret. Never readable back through the API. */
export function setSecret(kind: string, id: number, key: string, value: string) {
  return request(`${secretsPath(kind, id)}/${encodeURIComponent(key)}`, {
    method: 'PUT',
    body: JSON.stringify({ value }),
  })
}

export function deleteSecret(kind: string, id: number, key: string) {
  return request(`${secretsPath(kind, id)}/${encodeURIComponent(key)}`, { method: 'DELETE' })
}

/** Streaks and close rate of a recurring template. */
export function getTemplateStreak(id: number) {
  return request(`/api/templates/${id}/streak`)
}

export function setTemplateTags(templateId: number, tagIds: number[]) {
  return request(`/api/templates/${templateId}/tags`, {
    method: 'PUT',
    body: JSON.stringify({ tag_ids: tagIds }),
  })
}

export * from './templateConsts.ts'
