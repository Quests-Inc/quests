import { expect, test } from 'bun:test'

import { readZones } from '../../src/entities/graph/model/zones.ts'

test('zones reject invalid geometry and duplicate ids; normalize titles and colours', () => {
  const z = { id: 'a', title: 'Учёба', color: '#6a8fbf', x: -20, y: 30, w: 400, h: 240 }
  expect(readZones([z, z, { ...z, id: 'bad', w: NaN }, { ...z, id: 'small', h: 0 }])).toEqual([z])
  expect(readZones([{ ...z, color: 'url(unsafe)' }])[0].color).toBe('#6a8fbf')
  expect(readZones(null)).toEqual([])
})
