import { expect, test } from 'bun:test'

import { createTemplateActions } from '../../src/features/templates/model/templateActions.ts'

function make(fail = '', preview = null, emitResult = null) {
  const log = []
  const g =
    (n) =>
    async (...a) => {
      if (fail === n) throw new Error('сбой ' + n)
      log.push([n, ...a])
      return n === 'emit' && emitResult ? emitResult : { title: 'Квест' }
    }
  const act = createTemplateActions({
    api: {
      update: g('update'),
      copy: g('copy'),
      emit: g('emit'),
      test: async (id) => ({ ...(preview ?? { outcome: 'miss' }), id }),
    },
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

test('эмит динамического шаблона ставится в очередь: ответ — не квест и не ошибка, квест придёт событием', async () => {
  const a = make('', null, { emitted: false, outcome: 'queued', detail: 'Выпуск поставлен в очередь' })
  await a.act.emit({ id: 5, title: 'Почта' })
  expect(a.log).toContainEqual(['notify', 'Выпуск поставлен в очередь', 'success'])
  expect(a.log.some((x) => x[0] === 'changed')).toBe(false)
})

test('проверка выпуска: квест с шагами, пусто, сбой', async () => {
  const quest = make('', { outcome: 'quest', title: 'Почта', steps: [{ title: 'а' }, { title: 'б' }] })
  await quest.act.test({ id: 5, script_id: 13 })
  expect(quest.log.at(-1)).toEqual(['notify', 'Проверка: «Почта», шагов 2: а; б. Квест не создан.', 'success'])
  const miss = make('', { outcome: 'miss' })
  await miss.act.test({ id: 5 })
  expect(miss.log.at(-1)).toEqual(['notify', 'Проверка: скрипт ничего не вернул, квеста сейчас нет.', 'success'])
  const fail = make('', { outcome: 'failed', error: 'нет MAIL_HOST' })
  await fail.act.test({ id: 5 })
  expect(fail.log.at(-1)).toEqual(['notify', 'Проверка не удалась: нет MAIL_HOST', 'error'])
})
