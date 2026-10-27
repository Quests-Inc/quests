import { onUnauthorized, BASE, request } from './http.ts'
import type { WorkspacesView } from './identity_http.gen'

/** Whether this instance requires accounts, and who is signed in. */
export function fetchAuthState() {
  return request('/api/auth/state')
}

/**
 * Sign in. Deliberately bypasses `request()`: a 401 here means "wrong
 * password", not "session expired", so it must not trip the global handler.
 */
export async function login(username: string, password: string) {
  return postSignIn('/api/auth/login', { username, password })
}

/** Second step of a sign-in: the ticket from login() plus an authenticator or recovery code. */
export function loginMfa(mfaToken: string, code: string) {
  return postSignIn('/api/auth/login/mfa', { mfa_token: mfaToken, code })
}

async function postSignIn(path: string, payload: object) {
  const res = await fetch(`${BASE}${path}`, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const text = await res.text()
  let data = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    /* fall through to a generic message */
  }
  if (!res.ok) {
    throw new Error(data?.detail || `Не удалось войти (HTTP ${res.status})`)
  }
  return data
}

export function logout() {
  return request('/api/auth/logout', { method: 'POST' })
}

// --- workspaces (isolated data spaces of one account) ---

export function fetchWorkspaces() {
  return request<WorkspacesView>('/api/workspaces')
}

export function createWorkspace(name: string) {
  return request('/api/workspaces', { method: 'POST', body: JSON.stringify({ name }) })
}

export function fetchMembers(workspaceId: number) {
  return request(`/api/workspaces/${workspaceId}/members`)
}

export function removeMember(workspaceId: number, userId: number) {
  return request(`/api/workspaces/${workspaceId}/members/${userId}`, { method: 'DELETE' })
}

export function createInvite(workspaceId: number) {
  return request(`/api/workspaces/${workspaceId}/invites`, { method: 'POST' })
}

export function fetchInvite(token: string) {
  return request(`/api/invites/${encodeURIComponent(token)}`)
}

/** Spend an invitation: with a session joins as that account, else `body` {username, password} signs up. */
export function acceptInvite(token: string, body = {}) {
  return request(`/api/invites/${encodeURIComponent(token)}/accept`, {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

/** URL of the archive download for a workspace (the browser handles the file). */
export function workspaceExportUrl(id: number, { secrets = false } = {}) {
  return `${BASE}/api/workspaces/${id}/export${secrets ? '?secrets=1' : ''}`
}

/** Import an archive into a NEW workspace. Not through request(): this is multipart, not JSON. */
export async function importWorkspace(file: File | Blob, { name = '', secrets = false } = {}) {
  const form = new FormData()
  form.append('file', file)
  if (name) form.append('name', name)
  if (secrets) form.append('secrets', '1')
  const res = await fetch(`${BASE}/api/workspaces/import`, { method: 'POST', body: form, credentials: 'same-origin' })
  if (res.status === 401 && onUnauthorized) onUnauthorized()
  const data = await res.json().catch((): any => null)
  if (!res.ok) throw new Error(data?.detail || `HTTP ${res.status}`)
  return data
}

// --- API tokens of the signed-in user (HUD, MCP, Android, CLI, bot) ---

export function fetchTokens() {
  return request('/api/tokens')
}

/** The secret comes back once, in `token`. */
export function createToken(name: string) {
  return request('/api/tokens', { method: 'POST', body: JSON.stringify({ name }) })
}

export function revokeToken(id: number) {
  return request(`/api/tokens/${id}`, { method: 'DELETE' })
}

export function renameWorkspace(id: number, name: string) {
  return request(`/api/workspaces/${id}`, { method: 'PATCH', body: JSON.stringify({ name }) })
}

/** Hides the workspace now and purges it after a grace period; `confirm` must be its exact name. */
export function deleteWorkspace(id: number, confirm: string) {
  return request(`/api/workspaces/${id}`, { method: 'DELETE', body: JSON.stringify({ confirm }) })
}

export function switchWorkspace(id: number) {
  return request(`/api/workspaces/${id}/switch`, { method: 'POST' })
}

// --- attachments (bytes live on WebDAV; we only ever talk to the API proxy) ---

export function getMyMfa() {
  return request('/api/me/2fa')
}

/** New secret + otpauth:// link to scan; nothing is on until enableMyMfa() confirms a code. */
export function setupMyMfa() {
  return request('/api/me/2fa/setup', { method: 'POST' })
}

/** Confirms the secret with a code; returns { recovery_codes } — shown once. */
export function enableMyMfa(code: string) {
  return request('/api/me/2fa/enable', { method: 'POST', body: JSON.stringify({ code }) })
}

export function disableMyMfa(code: string) {
  return request('/api/me/2fa/disable', { method: 'POST', body: JSON.stringify({ code }) })
}

/** Replaces the recovery codes (needs a valid code); returns { recovery_codes }. */
export function regenerateRecoveryCodes(code: string) {
  return request('/api/me/2fa/recovery', { method: 'POST', body: JSON.stringify({ code }) })
}

export function seedStarter() {
  return request<{ version: number; added: number; updated: number; preserved: number; deleted: number }>(
    '/api/workspace/starter',
    { method: 'POST' }
  )
}
export function removeStarter() {
  return request<{ trashed: number }>('/api/workspace/starter', { method: 'DELETE' })
}
