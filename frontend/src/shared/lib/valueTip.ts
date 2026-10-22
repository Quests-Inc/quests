/** What a delayed hover shows for a substituted value: type, name, and the formula or the value. */

export const VALUE_HOVER_MS = 450

const TYPE_LABEL: Record<string, string> = {
  int: 'число',
  float: 'дробное',
  str: 'строка',
  bool: 'да/нет',
}

const TYPE_COLOR: Record<string, string> = {
  int: '#4aa3df',
  float: '#5cc4c4',
  str: '#c9a227',
  bool: '#b06ad8',
}

export function valueTypeLabel(type: string) {
  return TYPE_LABEL[type] ?? type
}

/** A colour for the type's own name. Kept off the select, so options do not inherit one colour. */
export function valueTypeColor(type: string) {
  return TYPE_COLOR[type] ?? 'inherit'
}

/** The third hover line: the error, else the formula, else the text (with the source that made it). */
export function valueDetail(row: { formula?: string; text?: string; error?: string; roll?: string }) {
  if (row.error) return row.error
  if (row.formula) return row.formula
  if (row.roll) return `${row.text ?? ''} · источник ${row.roll}`
  return row.text ?? ''
}

const FOREIGN = /^(note|quest|questline|step|template)=(\d+)\.([a-z][a-z0-9_]{0,23})$/

/** `data-value` is either a local name or `note=4.str`. */
export function parseValueChip(raw: string, host?: { kind: string; id: number }) {
  const foreign = FOREIGN.exec(raw)
  if (foreign) {
    return { host: `${foreign[1]}:${foreign[2]}`, kind: foreign[1], id: Number(foreign[2]), name: foreign[3] }
  }
  if (!host || !/^[a-z][a-z0-9_]{0,23}$/.test(raw)) return null
  return { host: `${host.kind}:${host.id}`, kind: host.kind, id: host.id, name: raw }
}
