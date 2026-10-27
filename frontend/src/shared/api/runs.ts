// Журнал запусков скриптов (quest=489): список, один запуск с нодами и проверка из редактора.
import type { RunnerJob, RunSummary, SourceRunStats } from './execution.gen'
import { request } from './http.ts'
import type { Edge, Graph } from './scriptgraph.gen'

export type RunSourceKind = 'step' | 'note' | 'template' | 'hook' | 'embed' | 'test'

export interface RunFilter {
  script_id?: number
  source_kind?: RunSourceKind | ''
  source_id?: number
  subject_type?: string
  subject_id?: number
  state?: 'waiting' | 'done' | 'failed' | ''
  before?: number
  limit?: number
}

/** A node of the graph a run executed, with what the run did with it. */
export interface RunNode {
  id: number
  type: string
  params?: unknown
  state?: 'ran' | 'skipped' | 'failed' | 'waiting'
  failure?: string
  outputs?: Record<string, unknown>
  /** Outputs that carried an exit signal: nothing went that way. */
  exits?: string[]
}

export interface RunDetail {
  id: number
  graph_ref: string
  script_id?: number
  subject_type: string
  subject_id: number
  state: 'waiting' | 'done' | 'failed'
  error?: string
  result?: string
  created_at: string
  finished_at?: string
  source_kind: RunSourceKind
  source_id: number
  dry?: boolean
  nodes: RunNode[]
  edges: Edge[]
  jobs: RunnerJob[]
  graph_error?: string
}

export function listRuns(f: RunFilter = {}) {
  const qs = new URLSearchParams()
  for (const [k, v] of Object.entries(f)) if (v !== undefined && v !== '' && v !== 0) qs.set(k, String(v))
  const q = qs.toString()
  return request<RunSummary[]>(`/api/runs${q ? `?${q}` : ''}`)
}

export function getRun(id: number) {
  return request<RunDetail>(`/api/runs/${id}`)
}

/** The newest run of every source of a kind, by its id (as a string key). */
export function getRunStats(kind: 'step' | 'note' | 'template' | 'hook') {
  return request<Record<string, SourceRunStats>>(`/api/run-stats?kind=${kind}`)
}

export interface TryBody {
  /** The graph as the editor holds it, when it is not saved yet. */
  graph?: Graph
  subject?: { type: string; id: number }
  event?: string
  args?: Record<string, string>
  live?: boolean
  on?: string
}

/** A test run («Запустить на…»): its id, read with getRun. */
export function tryScript(id: number, body: TryBody) {
  return request<{ run: number }>(`/api/scripts/${id}/try`, { method: 'POST', body: JSON.stringify(body) })
}
