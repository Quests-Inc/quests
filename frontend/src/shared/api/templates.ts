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

/** Force-materialize one quest from a template (manual emit for testing). */
export function emitTemplate(id: number) {
  return request(`/api/templates/${id}/emit`, { method: 'POST' })
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
  return `/api/${SECRET_OWNER_PATH[kind as keyof typeof SECRET_OWNER_PATH]}/${id}/secrets`
}

/**
 * Secret *names* for an owner — GET never returns values.
 * `keys` are set on the owner itself, `inherited` ({ key, from, id }) come from
 * a more general owner.
 */
export async function listSecrets(
  kind: string,
  id: number
): Promise<{ keys: string[]; inherited: { key: string; from: string; id: number }[] }> {
  const out = await request(secretsPath(kind, id))
  return { keys: out?.keys || [], inherited: out?.inherited || [] }
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
