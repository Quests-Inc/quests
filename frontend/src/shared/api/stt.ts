import { request } from './http.ts'

export type SttConfig = { base_url: string; model: string; has_key: boolean; enabled: boolean }

export function getStt() {
  return request<SttConfig>('/api/stt')
}

export function saveStt(body: { base_url: string; model: string; api_key?: string }) {
  return request<SttConfig>('/api/stt', { method: 'PUT', body: JSON.stringify(body) })
}

async function readJson(
  res: Response
): Promise<{ detail?: string; error?: { message?: string }; text?: string } | null> {
  return res.json().catch((): null => null)
}

/** The server forwards the file to the configured recognizer. */
export async function transcribeFile(file: File, language = 'ru') {
  const body = new FormData()
  body.append('file', file)
  body.append('language', language)
  const res = await fetch('/api/stt/transcriptions', {
    method: 'POST',
    body,
    credentials: 'same-origin',
    headers: { 'X-Quests-Source': 'web' },
  })
  const data = await readJson(res)
  if (!res.ok) throw new Error(data?.detail || data?.error?.message || `STT HTTP ${res.status}`)
  return String(data?.text || '').trim()
}

export async function fetchAttachmentAudio(id: number) {
  const res = await fetch(`/api/attachments/${id}/audio`, { credentials: 'same-origin' })
  if (!res.ok) throw new Error(`Вложение HTTP ${res.status}`)
  return res.blob()
}

/** HEAD the audio route: image markdown is reused for recordings, and only audio answers here. */
export async function attachmentIsAudio(id: string) {
  const head = await fetch(`/api/attachments/${id}/audio`, { method: 'HEAD', credentials: 'same-origin' })
  return head.ok && (head.headers.get('content-type') || '').startsWith('audio/')
}
