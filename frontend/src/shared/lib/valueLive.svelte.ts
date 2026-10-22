/** Effective values last published for a host, so a chip can name its type and formula without another fetch. */

export type ValueLive = {
  name: string
  type: string
  text: string
  formula: string
  error: string
  roll?: string
}

const buckets = new Map<string, Map<string, ValueLive>>()

export function hostKey(kind: string, id: number) {
  return `${kind}:${id}`
}

export function publishValues(host: string, rows: ValueLive[]) {
  const map = new Map<string, ValueLive>()
  for (const row of rows) map.set(row.name, row)
  buckets.set(host, map)
}

export function readValue(host: string, name: string) {
  return buckets.get(host)?.get(name)
}

/** Bumped after a write so the owner's table and the open texts load again. */
export const valueRev = $state({ n: 0, host: '' })

/** The host a remote change names: every table and text reads again, since values are inherited from parents and the workspace. */
export const ANY_HOST = '*'

export function touchValues(host: string) {
  valueRev.host = host
  valueRev.n += 1
}
