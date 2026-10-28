import { expect, test } from 'bun:test'

import { highlightMarkdown } from '../../src/shared/lib/mdHighlight.ts'

/** What the layer shows is exactly the text, with only wrappers added: the field and the layer must line up. */
const plain = (html) =>
  html
    .replace(/<[^>]+>/g, '')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&')

const cls = (src) => [...highlightMarkdown(src).matchAll(/class="mdh-([a-z]+)"/g)].map((m) => m[1])

test('текст не меняется ни на символ, только обёртки; в конце пробел', () => {
  const src =
    '# Заголовок\n\n*a* и **b** и ***c***, `код`, [л](http://x), script=7{2к8+2}?set=hp, {{hp}}, quest=5\n| a | b |\n|---|---|\n> цитата <b>&\n- пункт\n```js\n**не жирный**\n```\n'
  expect(plain(highlightMarkdown(src))).toBe(src + ' ')
})

test('разметка markdown красится по мере ввода: заголовок, пары, код, ссылки', () => {
  expect(cls('# Волк')).toContain('heading')
  expect(cls('*a* **b** ***c***')).toEqual(['italic', 'bold', 'bolditalic'])
  expect(cls('`x` [t](u) ![i](u) ~~z~~')).toEqual(['code', 'link', 'image', 'strike'])
  expect(cls('> q')).toContain('quote')
  expect(cls('- a')).toContain('mark')
})

test('наши вставки: скрипт с флагами, переменная, ссылка на сущность', () => {
  expect(cls('script=7{2к8+2}?set=hp')).toEqual(['script'])
  expect(cls('script=7{к20+bite_hit}')).toEqual(['script'])
  expect(cls('{{hp}} и note=12')).toEqual(['value', 'ref'])
  expect(cls('**{{hp}}**')).toEqual(['bold', 'value'])
})

test('незакрытая пара и середина слова не красятся; код-блок не разбирается', () => {
  expect(cls('**ещё не закрыто')).toEqual([])
  expect(cls('snake_case_name и 2*3*4')).toEqual([])
  expect(cls('```\n**x**\n```')).toEqual(['code', 'code', 'code'])
})
