import { request } from './http.ts'

/** Aggregated liveness: API + overlay/telegram heartbeats. */
export function fetchHealth() {
  return request('/api/health')
}
