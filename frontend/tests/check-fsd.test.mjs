import { expect, test } from 'bun:test'

import { checkFsd } from '../scripts/check-fsd.mjs'

const fixture = (name) => new URL(`./fixtures/${name}`, import.meta.url).pathname

test('корректная раскладка проходит', () => {
  expect(checkFsd(fixture('fsd-good'))).toEqual([])
})

test('нарушения находятся, в том числе когда целью служит .svelte', () => {
  const v = checkFsd(fixture('fsd-bad')).join('\n')
  expect(v).toContain(
    'entities/quest/ui/up.ts → ../../../features/edit-quest: слой «entities» не может импортировать вышележащий «features»'
  )
  expect(v).toContain('entities/quest/ui/side.ts → ../../note: слайсы слоя «entities» друг друга не знают')
  expect(v).toContain(
    'features/edit-quest/ui/deep.ts → @/entities/quest/ui/model: в слайс «entities/quest» заходят только через index.ts'
  )
  expect(v).toContain(
    'pages/journal/Page.svelte → @/widgets/header/Header.svelte: в слайс «widgets/header» заходят только через index.ts'
  )
  expect(v).toContain(
    'features/edit-quest/ui/FromPage.svelte → ../../../pages/journal/Page.svelte: слой «features» не может импортировать вышележащий «pages»'
  )
})

test('нет каталога слоёв — нечего проверять', () => {
  expect(checkFsd(fixture('absent'))).toEqual([])
})
