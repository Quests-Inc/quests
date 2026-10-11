import { expect, test } from 'bun:test'

import { createQuestPdf } from '../../src/widgets/quest-detail/model/pdfExport.svelte.ts'

function make(fail = false) {
  const log = []
  const pdf = createQuestPdf({
    simple: async () => void log.push('simple'),
    rich: async () => {
      if (fail) throw new Error('нет шрифта')
      log.push('rich')
    },
    progress: (id, m) => log.push(['progress', m]),
    done: (id, m, k) => log.push(['done', m, k]),
  })
  return { pdf, log }
}

test('меню запоминает квест и точку; выбор вида запускает нужную выгрузку', async () => {
  const { pdf, log } = make()
  pdf.openMenu({ clientX: 3, clientY: 4 }, { id: 1 })
  expect(pdf.menu).toMatchObject({ open: true, x: 3, y: 4 })
  await pdf.select('simple')
  await pdf.select('rich')
  expect(log).toEqual([
    ['progress', 'Генерация PDF…'],
    'simple',
    ['done', 'PDF сохранён', undefined],
    ['progress', 'Генерация PDF…'],
    'rich',
    ['done', 'PDF сохранён', undefined],
  ])
  pdf.closeMenu()
  expect(pdf.menu.open).toBe(false)
  expect(pdf.busy).toBe(false)
})

test('ошибка выгрузки показывается в тосте и снимает занятость; без квеста ничего', async () => {
  const { pdf, log } = make(true)
  await pdf.exportPdf(null)
  expect(log).toEqual([])
  const orig = console.error
  console.error = () => {}
  await pdf.exportPdf({ id: 1 })
  console.error = orig
  expect(log.at(-1)).toEqual(['done', 'нет шрифта', 'error'])
  expect(pdf.busy).toBe(false)
})
