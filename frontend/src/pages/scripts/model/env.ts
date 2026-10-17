import type { Edge } from '@/shared/api/scriptgraph.gen.ts'

/** The environment variable a link into a Code node delivers its value as (the same rule as the server's `graph.EnvName`). */
export function EnvName(e: Pick<Edge, 'from' | 'alias'>): string {
  if (e.alias) return `QUESTS_IN_${e.alias}`
  return `QUESTS_IN_N${e.from.node}_${e.from.port.toUpperCase()}`
}
