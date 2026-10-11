import { expect, test } from 'bun:test'

import { createTemplateActions } from '../../src/features/templates/model/templateActions.ts'

function make(fail = '') {
  const log = []
  const g =
    (n) =>
    async (...a) => {
      if (fail === n) throw new Error('сбой ' + n)
      log.push([n, ...a])
      return { title: 'Квест' }
    }
  const act = createTemplateActions({
    api: { update: g('update'), copy: g('copy'), emit: g('emit') },
    refresh: async () => void log.push(['refresh']),
    onChanged: () => log.push(['changed']),
    setError: (m) => log.push(['error', m]),
    notify: (m, k) => log.push(['notify', m, k]),
    copyText: g('clip'),
  })
  return { act, log }
}

test('включение переключает enabled, затем перечитывает список', async () => {
  const { act, log } = make()
  await act.toggleEnabled({ id: 1, enabled: true })
  expect(log).toEqual([['update', 1, { enabled: false }], ['changed'], ['refresh']])
})

test('дубликат и ошибка дубликата', async () => {
  const a = make()
  await a.act.duplicate({ id: 2 })
  expect(a.log).toEqual([['copy', 2], ['changed'], ['refresh']])
  const b = make('copy')
  await b.act.duplicate({ id: 2 })
  expect(b.log).toEqual([['error', 'сбой copy']])
})

test('эмит: успех показывает название квеста, ошибка — в строку ошибки и тост', async () => {
  const a = make()
  await a.act.emit({ id: 3, title: 'Шаблон' })
  expect(a.log).toContainEqual(['notify', 'Эмит: «Квест»', 'success'])
  const b = make('emit')
  await b.act.emit({ id: 3 })
  expect(b.log).toEqual([
    ['error', ''],
    ['error', 'сбой emit'],
    ['notify', 'сбой emit', 'error'],
  ])
})

test('copyRef копирует template=id', async () => {
  const { act, log } = make()
  await act.copyRef({ id: 4 })
  expect(log).toEqual([
    ['clip', 'template=4'],
    ['notify', 'Скопировано template=4', 'success'],
  ])
})
