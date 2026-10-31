import { expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import { touchValues } from '@/shared/lib/valueLive.svelte.ts'
const real = await import('@/shared/api/client.ts')
let resolveLoad: (value: any) => void
let rejectLoad: (reason: Error) => void
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  listValues: () =>
    new Promise((resolve, reject) => {
      resolveLoad = resolve
      rejectLoad = reject
    }),
}))
const { default: ValueTable } = await import('@/entities/value/ui/ValueTable.svelte')
const tick = () => new Promise((resolve) => setTimeout(resolve, 0))
const list = (value: number) => ({ own: [], inherited: [], effective: [{ name: 'hp', type: 'int', value }] })
test('refresh retains values while pending and on error, replaces after success and clears deleted values', async () => {
  const target = document.createElement('div')
  document.body.append(target)
  const seen: Record<string, string>[] = []
  const app = mount(ValueTable, {
    target,
    props: { owner: { kind: 'note', id: 259 }, variant: 'hidden', onEffective: (v) => seen.push(v) },
  })
  try {
    flushSync()
    resolveLoad!(list(20))
    await tick()
    expect(seen.at(-1)).toEqual({ hp: '20' })
    const count = seen.length
    touchValues('note:259')
    flushSync()
    expect(seen).toHaveLength(count)
    rejectLoad!(new Error('offline'))
    await tick()
    expect(seen.at(-1)).toEqual({ hp: '20' })
    touchValues('note:259')
    flushSync()
    resolveLoad!(list(12))
    await tick()
    expect(seen.at(-1)).toEqual({ hp: '12' })
    touchValues('note:259')
    flushSync()
    resolveLoad!({ own: [], inherited: [], effective: [] })
    await tick()
    expect(seen.at(-1)).toEqual({})
  } finally {
    await unmount(app)
    target.remove()
  }
})
