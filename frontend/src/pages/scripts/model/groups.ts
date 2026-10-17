import type { Script } from '@/shared/api/execution.gen.ts'
import type { Sample } from '@/shared/api/scriptgraph.gen.ts'

export type ScriptGroup = { key: string; title: string; color: string; items: Script[] }

/** Colours of the groups, in the order of the samples; the scripts of no sample come last in a neutral one. */
const COLORS = ['#7a9e3a', '#4aa3df', '#c47a20', '#b06ad8', '#d85a5a', '#3aa89a']
const OWN = '#8a8a8a'

/**
 * The library's scripts by the sample they still follow (the server names it: same nodes and links, parameters may differ).
 * A group per sample that has scripts, in the samples' order; the scripts of no sample are the last group.
 */
export function groupScripts(items: Script[], samples: Sample[]): ScriptGroup[] {
  const groups: ScriptGroup[] = []
  samples.forEach((s, i) => {
    const mine = items.filter((x) => x.template === s.id)
    if (mine.length) groups.push({ key: s.id, title: s.title, color: COLORS[i % COLORS.length], items: mine })
  })
  const known = new Set(samples.map((s) => s.id))
  const own = items.filter((x) => !x.template || !known.has(x.template))
  if (own.length) groups.push({ key: '', title: 'Свои скрипты', color: OWN, items: own })
  return groups
}
