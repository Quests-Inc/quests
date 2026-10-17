import type { RunnerJob } from './execution.gen'
import type { RunnerView } from './execution_http.gen'
import { request } from './http.ts'

export function getRunners() {
  return request<RunnerView[]>('/api/runners')
}

export function createRunner(
  label: string
): Promise<{ label: string; registration_token: string; valid_hours: number }> {
  return request('/api/runners', { method: 'POST', body: JSON.stringify({ label }) })
}

export function deleteRunner(label: string) {
  return request(`/api/runners/${encodeURIComponent(label)}`, { method: 'DELETE' })
}

export function getRunnerJobs(f: { label?: string; state?: string; limit?: number } = {}) {
  const qs = new URLSearchParams()
  if (f.label) qs.set('label', f.label)
  if (f.state) qs.set('state', f.state)
  if (f.limit) qs.set('limit', String(f.limit))
  const q = qs.toString()
  return request<RunnerJob[]>(`/api/runner-jobs${q ? `?${q}` : ''}`)
}

export function cancelRunnerJob(id: number) {
  return request(`/api/runner-jobs/${id}/cancel`, { method: 'POST' })
}
