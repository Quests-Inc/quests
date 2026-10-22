import { request } from './http.ts'

export type ValueType = 'int' | 'float' | 'str' | 'bool'

export type ValueRow = {
  name: string
  type: ValueType
  value: number | string | boolean | null
  formula?: string
  roll?: { script: number; arg: string }
  error?: string
  from?: string
  id?: number
  title?: string
}

export type ValueList = {
  own: ValueRow[]
  inherited: ValueRow[]
  effective: ValueRow[]
  truncated?: boolean
}

const SEG: Record<string, string> = {
  note: 'notes',
  quest: 'quests',
  step: 'steps',
  template: 'templates',
  questline: 'questlines',
}

export function valuesPath(kind: string, id = 0) {
  if (kind === 'workspace') return '/api/workspace/values'
  return `/api/${SEG[kind]}/${id}/values`
}

export function listValues(kind: string, id = 0) {
  return request<ValueList>(valuesPath(kind, id))
}

export function putValue(
  kind: string,
  id: number,
  name: string,
  type: ValueType,
  value: number | string | boolean,
  formula = ''
) {
  const body = formula.trim() ? { type, formula: formula.trim() } : { type, value }
  return request<ValueRow>(`${valuesPath(kind, id)}/${encodeURIComponent(name)}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

export function deleteValue(kind: string, id: number, name: string) {
  return request(`${valuesPath(kind, id)}/${encodeURIComponent(name)}`, { method: 'DELETE' })
}

/** The server computes the embedded script and writes the result as the value's literal. Without a body it rolls the stored source again. */
export function rollValue(kind: string, id: number, name: string, source?: { script: number; arg: string }) {
  return request<ValueRow>(`${valuesPath(kind, id)}/${encodeURIComponent(name)}/roll`, {
    method: 'POST',
    body: source ? JSON.stringify(source) : undefined,
  })
}
