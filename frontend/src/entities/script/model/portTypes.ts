import type { PortType } from '@/shared/api/scriptgraph.gen.ts'

const SCALARS: PortType[] = ['int', 'float', 'str', 'bool']

const isScalar = (t: PortType) => SCALARS.includes(t)

/**
 * Whether an output of type `from` may feed an input of type `to` (the same rule as the server's `graph.Assignable`):
 * equal types, int → float, a scalar → doc, anything but secrets → any, a scalar or doc → value.
 */
export function assignable(from: PortType, to: PortType): boolean {
  if (from === to) return true
  switch (to) {
    case 'any':
      return from !== 'secrets'
    case 'float':
      return from === 'int'
    case 'doc':
      return isScalar(from)
    case 'value':
      return isScalar(from) || from === 'doc'
  }
  return false
}

const COLORS: Record<string, string> = {
  int: '#4aa3df',
  float: '#5cc4c4',
  str: '#c9a227',
  bool: '#b06ad8',
  doc: '#e08a3c',
  any: '#8a8578',
  secrets: '#d85a5a',
  value: '#8a8578',
}

/** A colour for a port type, so a wire's end tells what it carries. References share one colour. */
export function portColor(t: PortType): string {
  if (t.startsWith('ref<')) return '#7a9e3a'
  return COLORS[t] ?? '#8a8578'
}
