import type { Script } from '@/shared/api/execution.gen.ts'

export type ScriptGroup = { key: string; title: string; color: string; items: Script[] }
export type PurposeInfo = { id: string; title: string; color: string }

/** The library's scripts by what they are for (set by hand), in the order of `purposes`; the unsorted ones (`none`) come last. */
export function groupScripts(items: Script[], purposes: readonly PurposeInfo[], none: PurposeInfo): ScriptGroup[] {
  const groups: ScriptGroup[] = []
  for (const p of [...purposes, none]) {
    const mine = items.filter((x) => (x.purpose ?? '') === p.id)
    if (mine.length) groups.push({ key: p.id, title: p.title, color: p.color, items: mine })
  }
  return groups
}
