import type { Kind } from '@/shared/api/scriptgraph.gen.ts'

/** The order node kinds are listed in the palette. */
export const KIND_ORDER: Kind[] = ['source', 'code', 'parse', 'control', 'sink', 'custom']

export const KIND_TITLES: Record<string, string> = {
  source: 'Источники',
  code: 'Код на раннере',
  parse: 'Разбор',
  control: 'Управление',
  sink: 'Приёмники',
  custom: 'Свои ноды',
}

/** The colour of a node's header by kind. */
export const KIND_COLORS: Record<string, string> = {
  source: '#7a9e3a',
  code: '#c47a20',
  parse: '#4aa3df',
  control: '#b06ad8',
  sink: '#d85a5a',
  custom: '#3aa89a',
}

/** Parameters a node gets when it is added: a valid starting point the owner then edits. */
export function defaultParams(type: string): unknown {
  switch (type) {
    case 'code':
      return { language: '', text: '# команда: её вывод уйдёт дальше\ntrue' }
    case 'parse':
      return { schema: [{ name: 'progress', type: 'int' }] }
    case 'compare':
      return { type: 'int', op: 'ge', operand: 1 }
    case 'any_of':
      return { type: 'int' }
    case 'args':
      return { params: [{ name: 'expr', type: 'str' }] }
    case 'expr':
      return { expr: '', vars: [] }
    case 'random':
      return { kind: 'int', lo: 1, hi: 6, count: 1, aggregate: 'sum' }
    case 'regex':
      return { mode: 'match', pattern: '.*' }
    case 'sink.step_result':
      return { result: 'done' }
    case 'sink.template_error':
      return { kind: 'error' }
    case 'sink.value':
      return { name: 'value', type: 'str' }
  }
  return undefined
}

/** Operations of the Compare node per type of its input (the same table as the server's). */
export const COMPARE_OPS: Record<string, { id: string; label: string }[]> = {
  int: numericOps(),
  float: numericOps(),
  str: [
    { id: 'eq', label: 'равно' },
    { id: 'ne', label: 'не равно' },
    { id: 'contains', label: 'содержит' },
    { id: 'prefix', label: 'начинается с' },
    { id: 'matches', label: 'соответствует шаблону' },
  ],
  bool: [{ id: 'eq', label: 'равно' }],
}

function numericOps() {
  return [
    { id: 'eq', label: '=' },
    { id: 'ne', label: '≠' },
    { id: 'lt', label: '<' },
    { id: 'le', label: '≤' },
    { id: 'gt', label: '>' },
    { id: 'ge', label: '≥' },
  ]
}
