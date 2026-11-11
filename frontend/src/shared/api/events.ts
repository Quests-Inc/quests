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
