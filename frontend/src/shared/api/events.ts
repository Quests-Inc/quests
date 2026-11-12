import { request } from './http.ts'
import type { EventRead } from './journal.gen'

export function listEvents() {
  return request<EventRead[]>('/api/calendar/events')
}

export function createEvent(payload: object) {
  return request<EventRead>('/api/calendar/events', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateEvent(id: number, payload: object) {
  return request<EventRead>(`/api/calendar/events/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}

export function deleteEvent(id: number) {
  return request(`/api/calendar/events/${id}`, { method: 'DELETE' })
}

export function questToEvent(id: number) {
  return request<EventRead>(`/api/quests/${id}/to-event`, { method: 'POST' })
}

/** Upload a custom event image (not added to the built-in SVG pool). */
export async function uploadEventIcon(id: number, file: File | Blob) {
  const body = new FormData()
  body.append('file', file)
  const res = await fetch(`/api/calendar/events/${id}/icon`, {
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
  return data as EventRead
}

export function clearEventIcon(id: number) {
  return request<EventRead>(`/api/calendar/events/${id}/icon`, { method: 'DELETE' })
}
