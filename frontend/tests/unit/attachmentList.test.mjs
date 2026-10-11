import { expect, test } from 'bun:test'

import {
  ATT_SPEC,
  extOf,
  formatSize,
  groupByOwner,
  isRaster,
  sortAttachments,
} from '../../src/entities/attachment/model/list.ts'
import { parseQuery, passes } from '../../src/shared/lib/searchFlags.ts'

test('formatSize: байты, КБ, МБ', () => {
  expect(formatSize(512)).toBe('512 B')
  expect(formatSize(2048)).toBe('2.0 KB')
  expect(formatSize(3 * 1024 * 1024)).toBe('3.0 MB')
  expect(formatSize(NaN)).toBe('0 B')
})

test('isRaster и extOf', () => {
  expect(isRaster({ content_type_detected: 'image/PNG; charset=x' })).toBe(true)
  expect(isRaster({ content_type_detected: 'image/svg+xml' })).toBe(false)
  expect(extOf({ filename: 'a.tar.gz' })).toBe('GZ')
  expect(extOf({ filename: 'README' })).toBe('')
})

test('sortAttachments: не меняет исходный список, учитывает режим', () => {
  const list = [
    { id: 1, filename: 'б', size_bytes: 10, uploaded_at: '2026-01-01' },
    { id: 2, filename: 'а', size_bytes: 99, uploaded_at: '2026-03-01' },
  ]
  expect(sortAttachments(list, 'new').map((a) => a.id)).toEqual([2, 1])
  expect(sortAttachments(list, 'old').map((a) => a.id)).toEqual([1, 2])
  expect(sortAttachments(list, 'name').map((a) => a.id)).toEqual([2, 1])
  expect(sortAttachments(list, 'size').map((a) => a.id)).toEqual([2, 1])
  expect(list.map((a) => a.id)).toEqual([1, 2])
})

test('groupByOwner: группы по владельцу, порядок по названию', () => {
  const files = [
    { id: 1, owner_type: 'note', owner_id: 5 },
    { id: 2, owner_id: 3 },
    { id: 3, owner_type: 'note', owner_id: 5 },
  ]
  const g = groupByOwner(files, (a) => (a.owner_type === 'note' ? 'Заметка' : 'Квест'))
  expect(g.map((x) => [x.key, x.files.length])).toEqual([
    ['note:5', 2],
    ['quest:3', 1],
  ])
})

test('ATT_SPEC: фильтры по типу файла и числу версий', () => {
  const a = { filename: 'doc.pdf', content_type_detected: 'application/pdf', revision_count: 3 }
  expect(passes(parseQuery('ftype:pdf', ATT_SPEC), ATT_SPEC, a)).toBe(true)
  expect(passes(parseQuery('ftype:png', ATT_SPEC), ATT_SPEC, a)).toBe(false)
  expect(passes(parseQuery('rev:>2', ATT_SPEC), ATT_SPEC, a)).toBe(true)
  expect(passes(parseQuery('rev:<2', ATT_SPEC), ATT_SPEC, a)).toBe(false)
})
