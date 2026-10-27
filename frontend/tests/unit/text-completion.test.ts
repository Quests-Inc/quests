import { expect, test } from 'bun:test'

import { completeText, scriptInsertion, textCompletionToken } from '../../src/shared/lib/textCompletion.ts'
test('дополнение переменной сохраняет текст справа и не дублирует закрывающие скобки', () => {
  const text = 'HP {{h}} конец'
  const token = textCompletionToken(text, 6)!
  expect(completeText(text, token, '{{hp}}')).toEqual({ text: 'HP {{hp}} конец', caret: 9 })
  expect(textCompletionToken('script=погод', 12)?.kind).toBe('script')
  expect(textCompletionToken('описание', 8)).toBeNull()
})
test('параметры вставки: запись и раннер исключают автоисполнение', () => {
  expect(scriptInsertion(4, '2к6', { auto: true, hide: true, set: 'hp', on: 'laptop' })).toBe(
    'script=4{2к6}?hide&set=hp&on=laptop'
  )
  expect(scriptInsertion(4, '1', { auto: true, hide: false, set: '', on: '' })).toBe('script=4{1}?auto')
  expect(() => scriptInsertion(4, 'a\nb', { auto: false, hide: false, set: '', on: '' })).toThrow()
})
