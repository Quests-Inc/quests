import { afterEach, expect, test } from 'bun:test'

import { filterFromUrl, isLong, shortValue } from '@/pages/runs/model/runWords.ts'
import { openTarget, searchOfTarget, setOpenTarget, type OpenTarget } from '@/shared/lib/openTarget.ts'
import { replaceSearch } from '@/shared/lib/urlSearch.ts'

afterEach(() => setOpenTarget(null))

test('a value on a link reads as one line, long ones are cut and unfold', () => {
  expect(shortValue('ok')).toBe('ok')
  expect(shortValue({ a: 1 })).toBe('{"a":1}')
  expect(shortValue('x'.repeat(200), 10)).toBe(`${'x'.repeat(10)}…`)
  expect(shortValue(undefined)).toBe('—')
  expect(isLong({ a: 1 })).toBe(true)
  expect(isLong('a\nb')).toBe(true)
  expect(isLong(7)).toBe(false)
})

test('the run log reads its filter from the address', () => {
  expect(filterFromUrl('http://x/?tab=runs&run=5&src=hook:3&state=failed')).toEqual({
    run: 5,
    script: undefined,
    source: { kind: 'hook', id: 3 },
    state: 'failed',
  })
  expect(filterFromUrl('http://x/?tab=runs&script=9&state=bogus&src=hook:x')).toEqual({
    run: undefined,
    script: 9,
    source: undefined,
    state: '',
  })
})

test('a target becomes the query of the tab it opens, and goes to the registered handler', () => {
  expect(searchOfTarget({ kind: 'runs', run: 4, source: { kind: 'step', id: 2 } })).toEqual({
    tab: 'runs',
    run: 4,
    script: undefined,
    src: 'step:2',
    state: undefined,
  })
  expect(searchOfTarget({ kind: 'script', id: 7 })).toEqual({ tab: 'scripts', script: 7 })
  expect(searchOfTarget({ kind: 'hook', id: 8 })).toEqual({ tab: 'hooks', hook: 8 })
  expect(searchOfTarget({ kind: 'ref', ref: 'note', id: 1 })).toBeNull()
  const seen: OpenTarget[] = []
  openTarget({ kind: 'script', id: 1 }) // no handler yet: nothing happens
  setOpenTarget((t) => seen.push(t))
  openTarget({ kind: 'hook', id: 2 })
  expect(seen).toEqual([{ kind: 'hook', id: 2 }])
})

test('replaceSearch rewrites the query and drops empty values', () => {
  replaceSearch({ tab: 'runs', run: 3, script: undefined, state: '' })
  expect(location.search).toBe('?tab=runs&run=3')
  replaceSearch({})
  expect(location.search).toBe('')
})
