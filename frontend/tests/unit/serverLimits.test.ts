import { expect, test } from 'bun:test'

import { foreignValueRefs } from '@/shared/lib/markdown.ts'
import { autoScriptLimit } from '@/shared/lib/scriptRef.ts'
import { serverLimits } from '@/shared/lib/serverLimits.ts'

test('rendering and automatic scripts use the applied server budgets', () => {
  const old = { ...serverLimits }
  try {
    serverLimits.foreign_values = 2
    serverLimits.embed_auto = 3
    expect(foreignValueRefs('{{note=1.a}} {{note=1.b}} {{note=1.c}}')).toHaveLength(2)
    expect(foreignValueRefs('{{note=1.a}} {{note=1.b}} {{note=1.c}}', Infinity)).toHaveLength(3)
    expect(autoScriptLimit()).toBe(3)
  } finally {
    Object.assign(serverLimits, old)
  }
})
