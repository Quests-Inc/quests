import { test } from 'bun:test'
import assert from 'node:assert/strict'

import { scrollRoom, swipeDir } from '../src/lib/js/tabSwipe.ts'

test('swipeDir: clear horizontal swipes only', () => {
  assert.equal(swipeDir(-120, 10), 1)
  assert.equal(swipeDir(120, -20), -1)
  assert.equal(swipeDir(-40, 0), 0, 'too short')
  assert.equal(swipeDir(-100, 80), 0, 'too diagonal')
})

test('scrollRoom: which way a scroller can still move', () => {
  const el = (scrollLeft) => ({ scrollLeft, scrollWidth: 500, clientWidth: 200 })
  assert.deepEqual(scrollRoom(el(0)), { next: true, prev: false })
  assert.deepEqual(scrollRoom(el(150)), { next: true, prev: true })
  assert.deepEqual(scrollRoom(el(300)), { next: false, prev: true })
})
