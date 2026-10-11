import type { LLMView } from './http.gen'
import { request } from './http.ts'

/** Free text → LLM action batch → dry-run preview (no writes). */
export function previewActionBatch(text: string, { noClarify = false } = {}) {
  return request('/api/llm/actions/preview', {
    method: 'POST',
    body: JSON.stringify({ text, no_clarify: noClarify }),
  })
}

/** Execute a batch previously returned by previewActionBatch() as-is (no re-generation). */
export function applyActionBatch(batch: object) {
  return request('/api/llm/actions/apply', {
    method: 'POST',
    body: JSON.stringify({ batch }),
  })
}

/** The account's assistant provider/key state (the key itself never comes back, only a hint). */
export function getMyLLM() {
  return request<LLMView>('/api/me/llm')
}

/** Save provider, address, model and (when typed) the key. An empty `api_key` keeps the stored one. */
export function saveMyLLM(form: FormData | object) {
  return request('/api/me/llm', { method: 'PUT', body: JSON.stringify(form) })
}

export function deleteMyLLM() {
  return request('/api/me/llm', { method: 'DELETE' })
}

/** Does this provider/key answer? Uses the typed key, else the stored one. */
export function testMyLLM(form: FormData | object) {
  return request('/api/me/llm/test', { method: 'POST', body: JSON.stringify(form) })
}

// --- two-factor sign-in (the account's own, session only) ---
