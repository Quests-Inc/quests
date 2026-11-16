import { expect, test } from 'bun:test'

import { alignDetailScroll } from '@/widgets/quest-detail/model/alignDetailScroll.ts'

test('alignDetailScroll scrolls to top without a quest title', () => {
  const calls: number[] = []
  const scroller = {
    querySelector: () => null,
    scrollTo: ({ top }: { top: number }) => calls.push(top),
  } as unknown as HTMLElement
  alignDetailScroll(scroller, true)
  expect(calls).toEqual([0])
})
