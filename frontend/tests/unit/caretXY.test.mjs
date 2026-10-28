import { expect, test } from 'bun:test'

import { placePopup } from '../../src/shared/lib/caretXY.ts'

const wrap = { w: 600, h: 400 }
const popup = { w: 384, h: 200 }

test('список под строкой каретки, у левого края каретки', () => {
  expect(placePopup({ left: 50, top: 100, height: 20 }, wrap, popup, { below: 500, above: 100 })).toEqual({
    left: 50,
    top: 124,
    width: 384,
  })
})

test('у правого края список сдвигается внутрь поля и не шире поля', () => {
  expect(placePopup({ left: 590, top: 100, height: 20 }, wrap, popup, { below: 500, above: 100 }).left).toBe(
    600 - 384 - 8
  )
  const narrow = placePopup({ left: 10, top: 0, height: 20 }, { w: 200, h: 100 }, popup, { below: 500, above: 0 })
  expect(narrow.width).toBe(184)
})

test('внизу тесно, а сверху просторнее — список над строкой, и он не закрывает каретку', () => {
  const p = placePopup({ left: 50, top: 350, height: 20 }, wrap, popup, { below: 60, above: 400 })
  expect(p.top).toBeUndefined()
  // Its bottom edge sits 4px above the caret's line: wrap.h - bottom = caret.top - 4.
  expect(wrap.h - (p.bottom ?? 0)).toBe(346)
})

test('тесно и сверху, и снизу — остаётся под строкой', () => {
  expect(placePopup({ left: 0, top: 10, height: 20 }, wrap, popup, { below: 80, above: 40 }).top).toBe(34)
})
