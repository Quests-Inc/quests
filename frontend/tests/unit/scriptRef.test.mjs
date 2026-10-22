import { expect, test } from 'bun:test'

import { escapeMdInline, parseScriptHref, scriptEmbedRe, scriptFlags, scriptHref, scriptLabel } from '../../src/shared/lib/scriptRef.ts'

test('?auto and ?hide are parameters, not the start of a longer word', () => {
  const re = scriptEmbedRe()
  const hit = re.exec('script=12{8+pb+str}?auto.')
  expect(hit?.[1]).toBe('12')
  expect(hit?.[2]).toBe('8+pb+str')
  expect(hit?.[3]).toBe('?auto')
  expect(scriptEmbedRe().exec('script=12{8+pb+str}?auto=1')?.[3]).toBe('?auto=1')
  expect(scriptEmbedRe().exec('script=12{8+pb+str}?hide&auto')?.[3]).toBe('?hide&auto')
  expect(scriptEmbedRe().exec('script=12{8+pb+str}?auto&hide')?.[3]).toBe('?auto&hide')
  expect(scriptEmbedRe().exec('script=12{8+pb+str}?automatic')?.[3]).toBeUndefined()
  expect(scriptEmbedRe().exec('script=12{8+pb+str}?hidden')?.[3]).toBeUndefined()
  expect(scriptEmbedRe().exec('script=12{8+pb+str}')?.[3]).toBeUndefined()
  expect(scriptFlags('?auto&hide')).toEqual({ auto: true, hide: true, set: '' })
  expect(scriptFlags('?hide&auto=1')).toEqual({ auto: true, hide: true, set: '' })
  expect(scriptFlags('?hide')).toEqual({ auto: false, hide: true, set: '' })
  expect(scriptFlags('?automatic')).toEqual({ auto: false, hide: false, set: '' })
  expect(scriptLabel('8+pb+str', { hide: true })).toBe('?')
  expect(scriptLabel('8+pb+str', {})).toBe('8+pb+str')
})

test('the link carries the argument and the flags', () => {
  expect(scriptHref('12', '8+pb+str')).toBe('?script=12&_=8%2Bpb%2Bstr')
  expect(scriptHref('12', '8+pb+str', { auto: true })).toBe('?script=12&_=8%2Bpb%2Bstr&auto=1')
  expect(scriptHref('12', '8+pb+str', { auto: true, hide: true })).toBe('?script=12&_=8%2Bpb%2Bstr&auto=1&hide=1')
  expect(parseScriptHref('?script=12&_=8%2Bpb%2Bstr&auto=1&hide=1')).toEqual({
    id: 12,
    arg: '8+pb+str',
    auto: true,
    hide: true,
    set: '',
  })
  expect(parseScriptHref('?script=12&_=8%2Bpb%2Bstr')?.hide).toBe(false)
  expect(parseScriptHref('?note=3')).toBeNull()
})

test('markdown emphasis characters in a formula are escaped', () => {
  expect(escapeMdInline('2*con')).toBe('2\\*con')
  expect(escapeMdInline('a_b')).toBe('a\\_b')
})

test('?set=name: a click writes the value, and it beats ?auto', async () => {
  const { scriptEmbedRe, scriptFlags, scriptHref, parseScriptHref } = await import('../../src/shared/lib/scriptRef.ts')
  const m = [...'script=7{2к8+2}?set=hp'.matchAll(scriptEmbedRe())][0]
  expect(m[3]).toBe('?set=hp')
  expect(scriptFlags(m[3])).toEqual({ auto: false, hide: false, set: 'hp' })
  expect(scriptFlags('?auto&set=hp').auto).toBe(false)
  expect(scriptFlags('?hide&auto&set=hp')).toEqual({ auto: false, hide: true, set: 'hp' })
  const href = scriptHref('7', '2к8+2', { set: 'hp' })
  expect(parseScriptHref(href)).toEqual({ id: 7, arg: '2к8+2', auto: false, hide: false, set: 'hp' })
  expect(scriptFlags('?set=Hp').set).toBe('')
})
