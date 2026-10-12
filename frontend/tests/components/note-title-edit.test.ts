import { expect, test } from 'bun:test'

import { createTitleEdit } from '@/pages/notes/model/noteTitleEdit.svelte.ts'

function make(selected: number | null = 1) {
  const saved: string[] = []
  const log: string[] = []
  const input = {
    focus: () => log.push('focus'),
    select: () => log.push('select'),
    blur: () => log.push('blur'),
  } as unknown as HTMLInputElement
  const t = createTitleEdit({
    selectedId: () => selected,
    title: () => 'Заметка',
    save: async (v) => void saved.push(v),
    input: () => input,
  })
  const key = (k: string) => {
    const e = { key: k, preventDefault: () => log.push('prevent'), stopPropagation: () => log.push('stop') }
    t.onKeydown(e)
  }
  return { t, saved, log, key }
}

test('start: поле открывается с текущим названием и берёт фокус; без заметки ничего', async () => {
  const { t, log } = make()
  await t.start()
  expect([t.editing, t.draft]).toEqual([true, 'Заметка'])
  expect(log).toEqual(['focus', 'select'])
  const none = make(null)
  await none.t.start()
  expect(none.t.editing).toBe(false)
})

test('commit: правка уходит на сохранение один раз, режим закрывается', async () => {
  const { t, saved } = make()
  await t.start()
  t.draft = 'Новое'
  await t.commit()
  await t.commit()
  expect(saved).toEqual(['Новое'])
  expect(t.editing).toBe(false)
})

test('Esc отменяет: следующий blur ничего не сохраняет; Enter снимает фокус (сохранит blur)', async () => {
  const { t, saved, log, key } = make()
  await t.start()
  t.draft = 'Не надо'
  key('Escape')
  expect(t.editing).toBe(false)
  await t.commit()
  expect(saved).toEqual([])
  expect(log).toContain('stop')
  await t.start()
  log.length = 0
  key('Enter')
  expect(log).toEqual(['prevent', 'blur'])
  expect(t.editing).toBe(true)
})

test('cancel: смена заметки закрывает правку без сохранения', async () => {
  const { t, saved } = make()
  await t.start()
  t.cancel()
  await t.commit()
  expect(saved).toEqual([])
})
