import type { CustomNode, Script, ScriptUse } from './execution.gen'
import { request } from './http.ts'
import type { Graph, NodeInfo, Port, Problem, Sample } from './scriptgraph.gen'

export type ScriptSubject = 'step' | 'note' | 'template'

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

export type EvalOut = { value: string; version: number } | { run: number; state: string; version: number }

/** Run a script with the values of `host` in scope. A script with a Code node needs `on` (a runner) and answers with a run to wait for. */
export function evalScript(id: number, args: Record<string, string>, host?: { kind: string; id: number }, on = '') {
  return request<EvalOut>(`/api/scripts/${id}/eval`, {
    method: 'POST',
    body: JSON.stringify({ args, ...(host ? { host } : {}), ...(on ? { on } : {}) }),
  })
}

export type EmbeddedRun = { state: 'waiting' | 'done' | 'failed'; value?: string; error?: string }

export function getEmbeddedRun(id: number) {
  return request<EmbeddedRun>(`/api/script-runs/${id}`)
}

const WAIT_STEP_MS = 600
const WAIT_MAX_MS = 100_000

/** The text of Result: at once for a server script, after the runner has answered for one with code. */
export async function evalScriptText(
  id: number,
  args: Record<string, string>,
  host?: { kind: string; id: number },
  on = '',
  wait: { step: number; max: number } = { step: WAIT_STEP_MS, max: WAIT_MAX_MS },
): Promise<string> {
  const out = await evalScript(id, args, host, on)
  if ('value' in out) return out.value
  const until = Date.now() + wait.max
  for (;;) {
    await new Promise((r) => setTimeout(r, wait.step))
    const run = await getEmbeddedRun(out.run)
    if (run.state === 'done') return run.value ?? ''
    if (run.state === 'failed') throw new Error(run.error || 'не вышло')
    if (Date.now() > until) throw new Error('раннер не ответил вовремя')
  }
}

export function getScript(id: number) {
  return request<ScriptFull>(`/api/scripts/${id}`)
}

export function getScriptVersion(id: number, version: number) {
  return request<{ id: number; version: number; graph: Graph }>(`/api/scripts/${id}/versions/${version}`)
}

export function createScript(body: { title: string; description?: string; purpose?: string; graph: Graph }) {
  return request<ScriptFull>('/api/scripts', { method: 'POST', body: JSON.stringify(body) })
}

/** `version` is the one the editor read; a newer one on the server answers 409. */
export function updateScript(
  id: number,
  body: { version: number; title?: string; description?: string; purpose?: string; graph?: Graph }
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

const subjectPath = (kind: ScriptSubject) => ({ step: 'steps', note: 'notes', template: 'templates' })[kind]

export function bindScript(kind: ScriptSubject, id: number, scriptId: number) {
  return request(`/api/${subjectPath(kind)}/${id}/script`, {
    method: 'PUT',
    body: JSON.stringify({ script_id: scriptId }),
  })
}

export function unbindScript(kind: ScriptSubject, id: number) {
  return request(`/api/${subjectPath(kind)}/${id}/script`, { method: 'DELETE' })
}

/** What the server takes to make or change a custom node (quest=458); `version` is the one the editor read. */
export interface CustomNodeBody {
  title: string
  description?: string
  version?: number
  inputs: CustomNode['inputs']
  outputs: CustomNode['outputs']
  fields: CustomNode['fields']
  graph: Graph
}

export function getCustomNodes() {
  return request<CustomNode[]>('/api/script-custom-nodes')
}

export function createCustomNode(body: CustomNodeBody) {
  return request<CustomNode>('/api/script-custom-nodes', { method: 'POST', body: JSON.stringify(body) })
}

export function updateCustomNode(id: number, body: CustomNodeBody) {
  return request<CustomNode>(`/api/script-custom-nodes/${id}`, { method: 'PUT', body: JSON.stringify(body) })
}

/** Install the starting library (the former samples) as custom nodes; the ones already there stay. */
export function installStarterNodes() {
  return request<{ created: CustomNode[] }>('/api/script-custom-nodes/starters', { method: 'POST', body: '{}' })
}

export function deleteCustomNode(id: number) {
  return request(`/api/script-custom-nodes/${id}`, { method: 'DELETE' })
}
