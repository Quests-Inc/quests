import { toast } from './toasts.svelte.js'

const BASE = ''

/** Set by App when a request comes back 401 — flips the UI to the login screen. */
let onUnauthorized = null

export function setUnauthorizedHandler(fn) {
  onUnauthorized = fn
}

/** Thrown on 401 so callers can tell "logged out" from a real failure. */
export class UnauthorizedError extends Error {
  constructor() {
    super('Требуется вход')
    this.name = 'UnauthorizedError'
  }
}

async function request(path, options = {}) {
  const res = await fetch(`${BASE}${path}`, {
    credentials: 'same-origin',
    ...options,
    headers: {
      'Content-Type': 'application/json',
      'X-Quests-Source': 'web',
      ...(options.headers || {}),
    },
  })
  if (res.status === 401) {
    if (onUnauthorized) onUnauthorized()
    throw new UnauthorizedError()
  }
  if (res.status === 204) return null

  const text = await res.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      const msg = res.ok
        ? `Ответ не JSON: ${text.slice(0, 120)}`
        : `HTTP ${res.status}: ${text.slice(0, 200)}`
      toast(msg, { kind: 'error' })
      throw new Error(msg)
    }
  }

  if (!res.ok) {
    const detail = data?.detail ?? res.statusText
    const msg = typeof detail === 'string' ? detail : JSON.stringify(detail)
    // Belt-and-suspenders: whatever called this may or may not render `msg`
    // itself (some spots do, some — like a modal confirm dialog closing
    // over its own error — quietly don't). A toast means a failure is
    // never *silent*, even where nothing else shows this specific message.
    toast(msg, { kind: 'error' })
    throw new Error(msg)
  }
  return data
}

export function listQuests(params = {}) {
  const q = new URLSearchParams()
  if (params.status) q.set('status', params.status)
  if (params.pinned != null) q.set('pinned', String(params.pinned))
  const qs = q.toString()
  return request(`/api/quests${qs ? `?${qs}` : ''}`)
}

/** Aggregated liveness: API + overlay/telegram heartbeats. */
export function fetchHealth() {
  return request('/api/health')
}

function quietQs(quiet) {
  return quiet ? '?quiet=1' : ''
}

/** Manual web edits are quiet by default (no overlay toasts). */
export function createQuest(payload, { quiet = true } = {}) {
  return request(`/api/quests${quietQs(quiet)}`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateQuest(id, payload, { quiet = true } = {}) {
  return request(`/api/quests/${id}${quietQs(quiet)}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}

export function addQuestStep(questId, payload, { quiet = true } = {}) {
  return request(`/api/quests/${questId}/steps${quietQs(quiet)}`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateQuestStep(questId, stepId, payload, { quiet = true } = {}) {
  return request(`/api/quests/${questId}/steps/${stepId}${quietQs(quiet)}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}

export function deleteQuestStep(questId, stepId, { quiet = true } = {}) {
  return request(`/api/quests/${questId}/steps/${stepId}${quietQs(quiet)}`, {
    method: 'DELETE',
  })
}

export function deleteQuest(id, { quiet = true } = {}) {
  return request(`/api/quests/${id}${quietQs(quiet)}`, { method: 'DELETE' })
}

export function listTemplates(params = {}) {
  const q = new URLSearchParams()
  if (params.enabled != null) q.set('enabled', String(params.enabled))
  const qs = q.toString()
  return request(`/api/templates${qs ? `?${qs}` : ''}`)
}

export function createTemplate(payload) {
  return request('/api/templates', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateTemplate(id, payload) {
  return request(`/api/templates/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}

export function deleteTemplate(id) {
  return request(`/api/templates/${id}`, { method: 'DELETE' })
}

export function copyTemplate(id) {
  return request(`/api/templates/${id}/copy`, { method: 'POST' })
}

/** Force-materialize one quest from a template (manual emit for testing). */
export function emitTemplate(id) {
  return request(`/api/templates/${id}/emit`, { method: 'POST' })
}

const SECRET_OWNER_PATH = {
  questline: 'questlines',
  template: 'templates',
  quest: 'quests',
  step: 'steps',
}

/** Kinds that can own secrets (a secret is inherited step > quest > template > questline). */
export const SECRET_OWNER_KINDS = Object.keys(SECRET_OWNER_PATH)

/** @param {string} kind @param {number} id */
function secretsPath(kind, id) {
  return `/api/${SECRET_OWNER_PATH[kind]}/${id}/secrets`
}

/**
 * Secret *names* for an owner — GET never returns values.
 * `keys` are set on the owner itself, `inherited` ({ key, from, id }) come from
 * a more general owner.
 * @param {string} kind @param {number} id
 * @returns {Promise<{ keys: string[], inherited: { key: string, from: string, id: number }[] }>}
 */
export async function listSecrets(kind, id) {
  const out = await request(secretsPath(kind, id))
  return { keys: out?.keys || [], inherited: out?.inherited || [] }
}

/** Write-only: sets/overwrites one secret. Never readable back through the API. */
export function setSecret(kind, id, key, value) {
  return request(`${secretsPath(kind, id)}/${encodeURIComponent(key)}`, {
    method: 'PUT',
    body: JSON.stringify({ value }),
  })
}

export function deleteSecret(kind, id, key) {
  return request(`${secretsPath(kind, id)}/${encodeURIComponent(key)}`, { method: 'DELETE' })
}

/** Streaks and close rate of a recurring template. */
export function getTemplateStreak(id) {
  return request(`/api/templates/${id}/streak`)
}

/** Free text → LLM action batch → dry-run preview (no writes). */
export function previewActionBatch(text, { noClarify = false } = {}) {
  return request('/api/llm/actions/preview', {
    method: 'POST',
    body: JSON.stringify({ text, no_clarify: noClarify }),
  })
}

/** Execute a batch previously returned by previewActionBatch() as-is (no re-generation). */
export function applyActionBatch(batch) {
  return request('/api/llm/actions/apply', {
    method: 'POST',
    body: JSON.stringify({ batch }),
  })
}

export function listCategories() {
  return request('/api/categories')
}

export function listTags(q = '') {
  const qs = q ? `?q=${encodeURIComponent(q)}` : ''
  return request(`/api/tags${qs}`)
}

/** Create tag; on slug conflict returns the existing tag (409 body). */
export async function createTag(payload) {
  const res = await fetch(`${BASE}/api/tags`, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    method: 'POST',
    body: JSON.stringify(payload),
  })
  if (res.status === 401) {
    if (onUnauthorized) onUnauthorized()
    throw new UnauthorizedError()
  }
  const text = await res.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      const msg = `HTTP ${res.status}: ${text.slice(0, 200)}`
      toast(msg, { kind: 'error' })
      throw new Error(msg)
    }
  }
  if (res.status === 409 && data) return data
  if (!res.ok) {
    const detail = data?.detail ?? res.statusText
    const msg = typeof detail === 'string' ? detail : JSON.stringify(detail)
    toast(msg, { kind: 'error' })
    throw new Error(msg)
  }
  return data
}

export function updateTag(id, payload) {
  return request(`/api/tags/${id}`, { method: 'PATCH', body: JSON.stringify(payload) })
}

export function deleteTag(id) {
  return request(`/api/tags/${id}`, { method: 'DELETE' })
}

export function setQuestTags(questId, tagIds) {
  return request(`/api/quests/${questId}/tags`, {
    method: 'PUT',
    body: JSON.stringify({ tag_ids: tagIds }),
  })
}

export function setTemplateTags(templateId, tagIds) {
  return request(`/api/templates/${templateId}/tags`, {
    method: 'PUT',
    body: JSON.stringify({ tag_ids: tagIds }),
  })
}

export function listQuestlines() {
  return request('/api/questlines')
}

export function createQuestline(payload) {
  return request('/api/questlines', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateQuestline(id, payload) {
  return request(`/api/questlines/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}

export function deleteQuestline(id) {
  return request(`/api/questlines/${id}`, { method: 'DELETE' })
}

export function listNotes() {
  return request('/api/notes')
}

export function getNote(id) {
  return request(`/api/notes/${id}`)
}

export function createNote(payload) {
  return request('/api/notes', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateNote(id, payload) {
  return request(`/api/notes/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}

export function deleteNote(id) {
  return request(`/api/notes/${id}`, { method: 'DELETE' })
}

/** Upload custom questline image (not added to built-in SVG pool). */
export async function uploadQuestlineIcon(id, file) {
  const body = new FormData()
  body.append('file', file)
  const res = await fetch(`/api/questlines/${id}/icon`, {
    method: 'POST',
    body,
  })
  const text = await res.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      throw new Error(res.ok ? `Ответ не JSON` : `HTTP ${res.status}`)
    }
  }
  if (!res.ok) {
    const detail = data?.detail ?? res.statusText
    throw new Error(typeof detail === 'string' ? detail : JSON.stringify(detail))
  }
  return data
}

export function clearQuestlineIcon(id) {
  return request(`/api/questlines/${id}/icon`, { method: 'DELETE' })
}

/** Upload custom note image (not added to built-in SVG pool). */
export async function uploadNoteIcon(id, file) {
  const body = new FormData()
  body.append('file', file)
  const res = await fetch(`/api/notes/${id}/icon`, {
    method: 'POST',
    body,
  })
  const text = await res.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      throw new Error(res.ok ? `Ответ не JSON` : `HTTP ${res.status}`)
    }
  }
  if (!res.ok) {
    const detail = data?.detail ?? res.statusText
    throw new Error(typeof detail === 'string' ? detail : JSON.stringify(detail))
  }
  return data
}

export function clearNoteIcon(id) {
  return request(`/api/notes/${id}/icon`, { method: 'DELETE' })
}

export const QUESTLINE_ICONS = ['document', 'flag', 'map', 'layers', 'target', 'scroll']

export const QUESTLINE_COLORS = [
  '#5a8a9a',
  '#8a8578',
  '#7a9e3a',
  '#6a7ab8',
  '#c47a20',
  '#c9a227',
  '#b54a3a',
  '#9a9a9a',
]

export const TEMPLATE_FREQS = ['daily', 'weekly']

export const TEMPLATE_EMIT_MODES = [
  { id: 'fixed', label: 'Обычный' },
  { id: 'surprise', label: 'Событие' },
]

export const WEEKDAY_LABELS = [
  { id: 0, label: 'Пн' },
  { id: 1, label: 'Вт' },
  { id: 2, label: 'Ср' },
  { id: 3, label: 'Чт' },
  { id: 4, label: 'Пт' },
  { id: 5, label: 'Сб' },
  { id: 6, label: 'Вс' },
]

export const QUEST_STATUSES = [
  'active',
  'expired',
  'frozen',
  'completed',
  'failed',
  'archived',
]

/** @type {Record<string, string>} */
export const QUEST_STATUS_LABELS = {
  active: 'активен',
  expired: 'просрочен',
  frozen: 'заморожен',
  completed: 'выполнен',
  failed: 'провален',
  archived: 'архив',
}

/** @type {Record<string, string>} */
export const TEMPLATE_FREQ_LABELS = {
  daily: 'ежедневно',
  weekly: 'еженедельно',
}

/** @type {{ id: string, label: string }[]} */
export const QUEST_SIGNIFICANCES = [
  { id: 'insignificant', label: 'незначительное' },
  { id: 'common', label: 'обычное' },
  { id: 'uncommon', label: 'необычное' },
  { id: 'epic', label: 'эпическое' },
  { id: 'legendary', label: 'легендарное' },
]


// --- auth ---

/** Whether this instance requires accounts, and who is signed in. */
export function fetchAuthState() {
  return request('/api/auth/state')
}

/**
 * Sign in. Deliberately bypasses `request()`: a 401 here means "wrong
 * password", not "session expired", so it must not trip the global handler.
 */
export async function login(username, password) {
  return postSignIn('/api/auth/login', { username, password })
}

/** Second step of a sign-in: the ticket from login() plus an authenticator or recovery code. */
export function loginMfa(mfaToken, code) {
  return postSignIn('/api/auth/login/mfa', { mfa_token: mfaToken, code })
}

async function postSignIn(path, payload) {
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
  return request('/api/workspaces')
}

export function createWorkspace(name) {
  return request('/api/workspaces', { method: 'POST', body: JSON.stringify({ name }) })
}

export function fetchMembers(workspaceId) {
  return request(`/api/workspaces/${workspaceId}/members`)
}

export function removeMember(workspaceId, userId) {
  return request(`/api/workspaces/${workspaceId}/members/${userId}`, { method: 'DELETE' })
}

export function createInvite(workspaceId) {
  return request(`/api/workspaces/${workspaceId}/invites`, { method: 'POST' })
}

export function fetchInvite(token) {
  return request(`/api/invites/${encodeURIComponent(token)}`)
}

/** Spend an invitation: with a session joins as that account, else `body` {username, password} signs up. */
export function acceptInvite(token, body = {}) {
  return request(`/api/invites/${encodeURIComponent(token)}/accept`, {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

/** URL of the archive download for a workspace (the browser handles the file). */
export function workspaceExportUrl(id, { secrets = false } = {}) {
  return `${BASE}/api/workspaces/${id}/export${secrets ? '?secrets=1' : ''}`
}

/** Import an archive into a NEW workspace. Not through request(): this is multipart, not JSON. */
export async function importWorkspace(file, { name = '', secrets = false } = {}) {
  const form = new FormData()
  form.append('file', file)
  if (name) form.append('name', name)
  if (secrets) form.append('secrets', '1')
  const res = await fetch(`${BASE}/api/workspaces/import`, { method: 'POST', body: form, credentials: 'same-origin' })
  if (res.status === 401 && onUnauthorized) onUnauthorized()
  const data = await res.json().catch(() => null)
  if (!res.ok) throw new Error(data?.detail || `HTTP ${res.status}`)
  return data
}

// --- API tokens of the signed-in user (HUD, MCP, Android, CLI, bot) ---

export function fetchTokens() {
  return request('/api/tokens')
}

/** The secret comes back once, in `token`. */
export function createToken(name) {
  return request('/api/tokens', { method: 'POST', body: JSON.stringify({ name }) })
}

export function revokeToken(id) {
  return request(`/api/tokens/${id}`, { method: 'DELETE' })
}

export function renameWorkspace(id, name) {
  return request(`/api/workspaces/${id}`, { method: 'PATCH', body: JSON.stringify({ name }) })
}

/** Hides the workspace now and purges it after a grace period; `confirm` must be its exact name. */
export function deleteWorkspace(id, confirm) {
  return request(`/api/workspaces/${id}`, { method: 'DELETE', body: JSON.stringify({ confirm }) })
}

export function switchWorkspace(id) {
  return request(`/api/workspaces/${id}/switch`, { method: 'POST' })
}

// --- attachments (bytes live on WebDAV; we only ever talk to the API proxy) ---

function ownerAttachmentsPath(ownerType, ownerId, attachmentId) {
  const seg =
    ownerType === 'questline' ? 'questlines' : ownerType === 'note' ? 'notes' : 'quests'
  const base = `/api/${seg}/${ownerId}/attachments`
  return attachmentId == null ? base : `${base}/${attachmentId}`
}

export function listAttachments(ownerType, ownerId, { stat = true } = {}) {
  const path = ownerAttachmentsPath(ownerType, ownerId)
  return request(stat ? path : `${path}?stat=0`)
}

/** All attachment metadata, grouped `{ quest: { [id]: [] }, questline: { … }, note: { … } }`.
 * Default skips WebDAV Stat — cheap enough to run on every journal load. */
export function listAllAttachments({ stat = false } = {}) {
  return request(stat ? '/api/attachments?stat=1' : '/api/attachments')
}

export function attachmentDownloadUrl(ownerType, ownerId, attachmentId, revision = null) {
  const base = ownerAttachmentsPath(ownerType, ownerId, attachmentId)
  if (revision == null || revision === '') return base
  return `${base}?revision=${encodeURIComponent(String(revision))}`
}

export async function uploadAttachment(ownerType, ownerId, file, comment = '') {
  const body = new FormData()
  body.append('file', file)
  if (comment) body.append('comment', comment)
  const res = await fetch(`${BASE}${ownerAttachmentsPath(ownerType, ownerId)}`, {
    method: 'POST',
    credentials: 'same-origin',
    body,
  })
  if (res.status === 401) {
    if (onUnauthorized) onUnauthorized()
    throw new UnauthorizedError()
  }
  const text = await res.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      throw new Error(res.ok ? 'Ответ не JSON' : `HTTP ${res.status}`)
    }
  }
  if (!res.ok) {
    const detail = data?.detail ?? res.statusText
    throw new Error(typeof detail === 'string' ? detail : JSON.stringify(detail))
  }
  return data
}

/** Upload a new revision of an existing attachment (becomes current). */
export async function uploadAttachmentRevision(ownerType, ownerId, attachmentId, file, comment = '') {
  const body = new FormData()
  body.append('file', file)
  if (comment) body.append('comment', comment)
  const res = await fetch(
    `${BASE}${ownerAttachmentsPath(ownerType, ownerId, attachmentId)}/revisions`,
    { method: 'POST', credentials: 'same-origin', body },
  )
  if (res.status === 401) {
    if (onUnauthorized) onUnauthorized()
    throw new UnauthorizedError()
  }
  const text = await res.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      throw new Error(res.ok ? 'Ответ не JSON' : `HTTP ${res.status}`)
    }
  }
  if (!res.ok) {
    const detail = data?.detail ?? res.statusText
    throw new Error(typeof detail === 'string' ? detail : JSON.stringify(detail))
  }
  return data
}

export function listAttachmentRevisions(ownerType, ownerId, attachmentId) {
  return request(`${ownerAttachmentsPath(ownerType, ownerId, attachmentId)}/revisions`)
}

export function setAttachmentCurrentRevision(ownerType, ownerId, attachmentId, revision) {
  return request(ownerAttachmentsPath(ownerType, ownerId, attachmentId), {
    method: 'PATCH',
    body: JSON.stringify({ current_revision: revision }),
  })
}

export function updateAttachmentComment(ownerType, ownerId, attachmentId, comment) {
  return request(ownerAttachmentsPath(ownerType, ownerId, attachmentId), {
    method: 'PATCH',
    body: JSON.stringify({ comment }),
  })
}

export function setAttachmentHidden(ownerType, ownerId, attachmentId, hidden) {
  return request(ownerAttachmentsPath(ownerType, ownerId, attachmentId), {
    method: 'PATCH',
    body: JSON.stringify({ hidden: !!hidden }),
  })
}

export function deleteAttachment(ownerType, ownerId, attachmentId) {
  return request(ownerAttachmentsPath(ownerType, ownerId, attachmentId), {
    method: 'DELETE',
  })
}

export function deleteAttachmentRevision(ownerType, ownerId, attachmentId, revision) {
  return request(
    `${ownerAttachmentsPath(ownerType, ownerId, attachmentId)}/revisions/${revision}`,
    { method: 'DELETE' },
  )
}

/**
 * Change-log feed. `params`: limit, entity_type (csv: quest|note|attachment|all),
 * entity_id, quest_id, kind (csv), questline_id, category_id, tag_id,
 * from/to (ISO), q, before_id + before_at (keyset cursor).
 * @param {Record<string, string | number | null | undefined>} [params]
 */
export function getQuestLog(params = {}) {
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v != null && v !== '') q.set(k, String(v))
  }
  const qs = q.toString()
  return request(`/api/quest-log${qs ? `?${qs}` : ''}`)
}

/**
 * Relations map around one entity (plus optional extra roots).
 * @param {string} type @param {number} id @param {number} [depth]
 * @param {{ type: string, id: number }[]} [extra]
 * @param {{ type: string, id: number }[]} [expand] cards whose direct neighbours are added whatever the depth
 */
export function getGraph(type, id, depth = 2, extra = [], expand = []) {
  const more = extra.map((r) => `${r.type}:${r.id}`).join(',')
  const open = expand.map((r) => `${r.type}:${r.id}${r.only ? `>${r.only}` : ''}`).join(',')
  return request(
    `/api/graph?${encodeURIComponent(type)}=${id}&depth=${depth}${more ? `&extra=${encodeURIComponent(more)}` : ''}${open ? `&expand=${encodeURIComponent(open)}` : ''}`,
  )
}

// --- Map draft: edits made on the relations map, kept on the server until saved.

export function getGraphDraft() {
  return request('/api/graph-draft')
}

/** @param {string} kind @param {any} payload */
export function addGraphDraftOp(kind, payload) {
  return request('/api/graph-draft/ops', { method: 'POST', body: JSON.stringify({ kind, payload }) })
}

/** @param {number} id @param {boolean} [dry] dry = only report what would go with it */
export function deleteGraphDraftOp(id, dry = false) {
  return request(`/api/graph-draft/ops/${id}${dry ? '?dry=1' : ''}`, { method: 'DELETE' })
}

/** @param {{ type: string, id: number }[]} roots */
export function putGraphDraftRoots(roots) {
  return request('/api/graph-draft/roots', { method: 'PUT', body: JSON.stringify({ roots }) })
}

export function cancelGraphDraft() {
  return request('/api/graph-draft', { method: 'DELETE' })
}

/** Throws on failure; a 422 carries `problems` in the thrown error's `problems`. */
export async function commitGraphDraft() {
  const res = await fetch(`${BASE}/api/graph-draft/commit`, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', 'X-Quests-Source': 'web' },
  })
  const data = await res.json().catch(() => null)
  if (!res.ok) {
    const err = new Error(data?.error || data?.detail || `HTTP ${res.status}`)
    // @ts-ignore
    err.problems = data?.problems || []
    // @ts-ignore
    err.failedOp = data?.failed_op ?? null
    throw err
  }
  return data
}

/** Stages a file in the draft. @param {File} file @param {string} [owner] "quest:5" etc. */
export async function uploadGraphDraftFile(file, owner = '') {
  const fd = new FormData()
  fd.append('file', file)
  if (owner) fd.append('owner', owner)
  const res = await fetch(`${BASE}/api/graph-draft/files`, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'X-Quests-Source': 'web' },
    body: fd,
  })
  const data = await res.json().catch(() => null)
  if (!res.ok) {
    const msg = typeof data?.detail === 'string' ? data.detail : `HTTP ${res.status}`
    toast(msg, { kind: 'error' })
    throw new Error(msg)
  }
  return data
}

/** One quest with its steps and tags. @param {number} id */
export function getQuestFull(id) {
  return request(`/api/quests/${id}`)
}

/** Replaces the payload of a created thing in the draft. @param {number} id @param {any} payload */
export function putGraphDraftOp(id, payload) {
  return request(`/api/graph-draft/ops/${id}`, { method: 'PUT', body: JSON.stringify({ payload }) })
}


// --- Runners: devices that execute checks (quests-runner), and their job queue.

export function getRunners() {
  return request('/api/runners')
}

/** @param {string} label @returns {Promise<{ label: string, registration_token: string, valid_hours: number }>} */
export function createRunner(label) {
  return request('/api/runners', { method: 'POST', body: JSON.stringify({ label }) })
}

/** @param {string} label */
export function deleteRunner(label) {
  return request(`/api/runners/${encodeURIComponent(label)}`, { method: 'DELETE' })
}

/** @param {{ label?: string, state?: string, limit?: number }} [f] */
export function getRunnerJobs(f = {}) {
  const qs = new URLSearchParams()
  if (f.label) qs.set('label', f.label)
  if (f.state) qs.set('state', f.state)
  if (f.limit) qs.set('limit', String(f.limit))
  const q = qs.toString()
  return request(`/api/runner-jobs${q ? `?${q}` : ''}`)
}

/** @param {number} id */
export function cancelRunnerJob(id) {
  return request(`/api/runner-jobs/${id}/cancel`, { method: 'POST' })
}

/** The account's assistant provider/key state (the key itself never comes back, only a hint). */
export function getMyLLM() {
  return request('/api/me/llm')
}

/** Save provider, address, model and (when typed) the key. An empty `api_key` keeps the stored one. */
export function saveMyLLM(form) {
  return request('/api/me/llm', { method: 'PUT', body: JSON.stringify(form) })
}

export function deleteMyLLM() {
  return request('/api/me/llm', { method: 'DELETE' })
}

/** Does this provider/key answer? Uses the typed key, else the stored one. */
export function testMyLLM(form) {
  return request('/api/me/llm/test', { method: 'POST', body: JSON.stringify(form) })
}

// --- two-factor sign-in (the account's own, session only) ---

/** Ranked note search: [{ id, title, score, where: 'title'|'body'|'fuzzy', snippet (matches in ⟪ ⟫), … }]. */
export function searchNotes(q, limit = 30) {
  return request(`/api/notes/search?q=${encodeURIComponent(q)}&limit=${limit}`)
}

export function getMyMfa() {
  return request('/api/me/2fa')
}

/** New secret + otpauth:// link to scan; nothing is on until enableMyMfa() confirms a code. */
export function setupMyMfa() {
  return request('/api/me/2fa/setup', { method: 'POST' })
}

/** Confirms the secret with a code; returns { recovery_codes } — shown once. */
export function enableMyMfa(code) {
  return request('/api/me/2fa/enable', { method: 'POST', body: JSON.stringify({ code }) })
}

export function disableMyMfa(code) {
  return request('/api/me/2fa/disable', { method: 'POST', body: JSON.stringify({ code }) })
}

/** Replaces the recovery codes (needs a valid code); returns { recovery_codes }. */
export function regenerateRecoveryCodes(code) {
  return request('/api/me/2fa/recovery', { method: 'POST', body: JSON.stringify({ code }) })
}
