import { getCustomNodes } from '@/shared/api/client.ts'
import type { CustomNode } from '@/shared/api/execution.gen.ts'

import { purposeOfKind } from './purpose.ts'

/** The workspace's custom nodes for pickers: one shared list, refreshed on demand. */
export const customNodes = $state({ items: [] as CustomNode[], loaded: false })

let inflight: Promise<void> | null = null

export function loadCustomNodes(force: boolean = false) {
  if (inflight) return inflight
  if (!force && customNodes.loaded) return Promise.resolve()
  inflight = getCustomNodes()
    .then((rows) => {
      customNodes.items = rows ?? []
      customNodes.loaded = true
    })
    .catch(() => {
      /* the pickers stay as they were */
    })
    .finally(() => {
      inflight = null
    })
  return inflight
}

/** The nodes whose sources read this kind of entity (a picker's `kind`): what a script for it can be made of. */
export function nodesFor(kind: string): CustomNode[] {
  return customNodes.items.filter(
    (n) => !n.problem && n.subjects?.some((s) => purposeOfKind(s) === purposeOfKind(kind))
  )
}
