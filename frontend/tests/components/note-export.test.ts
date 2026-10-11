import { expect, test } from 'bun:test'

import { createNoteExport } from '@/pages/notes/model/noteExport.svelte.ts'
import { toasts } from '@/shared/lib/toasts.svelte.ts'

const payload = { id: 3, title: 'Заметка', description: 'текст' }

function make(over: Record<string, unknown> = {}) {
  const log: string[] = []
  const errors: string[] = []
  const exp = createNoteExport({
    downloadMarkdown: (p) => log.push(`md:${p.id}`),
    downloadPdf: async (p) => {
      log.push(`pdf:${p.id}`)
    },
    setError: (m) => errors.push(m),
    ...over,
  })
  return { exp, log, errors }
}

test('Markdown: скачивается сразу, ошибка очищается, ставится тост', async () => {
  const { exp, log, errors } = make()
  await exp.run(payload, 'md')
  expect(log).toEqual(['md:3'])
  expect(errors).toEqual([''])
  expect(toasts.at(-1)?.message).toBe('Markdown сохранён')
  expect(exp.busy).toBe(false)
})

test('PDF: идёт с прогрессом, пока занято — повторный запуск игнорируется', async () => {
  let release: () => void = () => {}
  const { exp, log } = make({
    downloadPdf: (p: any) =>
      new Promise<void>((r) => {
        log2.push(`pdf:${p.id}`)
        release = r
      }),
  })
  const log2: string[] = []
  const first = exp.run(payload, 'pdf')
  expect(exp.busy).toBe(true)
  expect(exp.items[1].label).toBe('PDF…')
  await exp.run(payload, 'pdf')
  expect(log2).toEqual(['pdf:3'])
  release()
  await first
  expect(exp.busy).toBe(false)
  expect(exp.items[1].label).toBe('PDF')
  expect(log).toEqual([])
})

test('ошибки: Markdown — в страницу и тост, PDF — только в страницу (тост прогресса уже показал причину)', async () => {
  const md = make({
    downloadMarkdown: () => {
      throw new Error('сломалось')
    },
  })
  await md.exp.run(payload, 'md')
  expect(md.errors).toEqual(['', 'сломалось'])
  expect(toasts.at(-1)).toMatchObject({ kind: 'error', message: 'сломалось' })
  const pdf = make({
    downloadPdf: async () => {
      throw new Error('нет шрифта')
    },
  })
  await pdf.exp.run(payload, 'pdf')
  expect(pdf.errors).toEqual(['', 'нет шрифта'])
  expect(toasts.some((t) => t.message === 'нет шрифта' && t.kind === 'error')).toBe(true)
})

test('меню: открывается под кнопкой', () => {
  const { exp } = make()
  const button = document.createElement('button')
  button.getBoundingClientRect = () => ({ left: 40, bottom: 100 }) as DOMRect
  const event = { preventDefault() {}, stopPropagation() {}, currentTarget: button } as unknown as MouseEvent
  exp.openMenu(event)
  expect(exp.menuOpen).toBe(true)
  expect([exp.menuX, exp.menuY]).toEqual([40, 104])
  exp.menuOpen = false
  expect(exp.menuOpen).toBe(false)
})
