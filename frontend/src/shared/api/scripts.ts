import type { Script, ScriptUse } from './execution.gen'
import { request } from './http.ts'
import type { Graph, NodeInfo, Port, Problem, Sample } from './scriptgraph.gen'

export type ScriptSubject = 'step' | 'note'

/** A script with its graph (a list carries none). */
export type ScriptFull = Script & { graph: Graph }

/** What the validator says about a graph without saving it. */
export interface ScriptCheck {
  ok: boolean
  problems: Problem[]
  needs_runner?: boolean
  subjects?: string[]
}

export function getScripts() {
  return request<Script[]>('/api/scripts')
}

export function getScript(id: number) {
  return request<ScriptFull>(`/api/scripts/${id}`)
}

export function getScriptVersion(id: number, version: number) {
  return request<{ id: number; version: number; graph: Graph }>(`/api/scripts/${id}/versions/${version}`)
}

export function createScript(body: { title: string; description?: string; graph: Graph }) {
  return request<ScriptFull>('/api/scripts', { method: 'POST', body: JSON.stringify(body) })
}

/** `version` is the one the editor read; a newer one on the server answers 409. */
export function updateScript(
  id: number,
  body: { version: number; title?: string; description?: string; graph?: Graph }
) {
  return request<ScriptFull>(`/api/scripts/${id}`, { method: 'PUT', body: JSON.stringify(body) })
}

export function deleteScript(id: number) {
  return request(`/api/scripts/${id}`, { method: 'DELETE' })
}

export function getScriptUses(id: number) {
  return request<ScriptUse[]>(`/api/scripts/${id}/uses`)
}

export function validateScript(graph: Graph) {
  return request<ScriptCheck>('/api/scripts/validate', { method: 'POST', body: JSON.stringify({ graph }) })
}

/** The library's examples to start a script from. */
export function getScriptSamples() {
  return request<Sample[]>('/api/script-samples')
}

export function getScriptNodes() {
  return request<NodeInfo[]>('/api/script-nodes')
}

/** The ports a node of a type gets with its parameters. */
export function getNodePorts(type: string, params?: unknown) {
  return request<{ inputs: Port[]; outputs: Port[] }>('/api/script-nodes/ports', {
    method: 'POST',
    body: JSON.stringify({ type, params }),
  })
}

const subjectPath = (kind: ScriptSubject) => (kind === 'step' ? 'steps' : 'notes')

export function bindScript(kind: ScriptSubject, id: number, scriptId: number) {
  return request(`/api/${subjectPath(kind)}/${id}/script`, {
    method: 'PUT',
    body: JSON.stringify({ script_id: scriptId }),
  })
}

export function unbindScript(kind: ScriptSubject, id: number) {
  return request(`/api/${subjectPath(kind)}/${id}/script`, { method: 'DELETE' })
}
