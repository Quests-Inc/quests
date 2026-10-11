import { afterEach, expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

const real = await import('@/shared/api/client.ts')
mock.module('@/shared/api/client.ts', () => ({ ...real, getQuestLog: async () => ({ items: [] }) }))
const { default: AttachmentPanel } = await import('@/pages/attachments/ui/AttachmentPanel.svelte')

let cleanup: Array<() => void> = []
afterEach(() => {
  cleanup.forEach((fn) => fn())
  cleanup = []
})

const att = {
  id: 5,
  filename: 'схема.png',
  hidden: false,
  size_bytes: 2048,
  content_type_detected: 'image/png',
  revision: 2,
  revision_count: 2,
  uploaded_at: '2026-03-01T10:00:00Z',
  owner_id: 1,
  scan_status: 'clean',
}

function open(extra: Record<string, unknown> = {}) {
  const calls: string[] = []
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(AttachmentPanel, {
    target,
    props: {
      a: att,
      revs: [
        { revision: 2, is_current: true, uploaded_at: '2026-03-01T10:00:00Z', size_bytes: 2048 },
        { revision: 1, is_current: false, uploaded_at: '2026-02-01T10:00:00Z', size_bytes: 1024 },
      ],
      revsBusy: false,
      ownerKindLabel: 'Квест',
      ownerLabel: 'Мой квест',
      onClose: () => calls.push('close'),
      onDownload: (rev?: number | null) => calls.push(`download:${rev ?? ''}`),
      onToggleHidden: () => calls.push('hide'),
      onPickVersion: () => calls.push('version'),
      onDelete: () => calls.push('delete'),
      onOpenOwner: () => calls.push('owner'),
      onSaveComment: () => calls.push('comment'),
      onMakeCurrent: (rev: number) => calls.push(`current:${rev}`),
      onDeleteRevision: (rev: number) => calls.push(`delrev:${rev}`),
      onCopyRef: () => calls.push('copy'),
      ...extra,
    },
  })
  cleanup.push(() => {
    unmount(app)
    target.remove()
  })
  return { target, calls }
}

const button = (root: Element, text: string) =>
  [...root.querySelectorAll('button')].find(
    (b) => b.textContent?.includes(text) || b.title?.includes(text)
  ) as HTMLElement

test('панель вложения: сведения и список версий', () => {
  const { target } = open()
  expect(target.textContent).toContain('схема.png')
  expect(target.textContent).toContain('attachment=5')
  expect(target.textContent).toContain('Квест · Мой квест')
  expect(target.textContent).toContain('2.0 KB')
  expect(target.querySelectorAll('.attpage__rev')).toHaveLength(2)
  expect(target.querySelector('.attpage__rev--cur')?.textContent).toContain('текущая')
})

test('панель вложения: кнопки зовут свои обработчики', () => {
  const { target, calls } = open()
  button(target, 'Скачать').click()
  button(target, 'Скрыть').click()
  button(target, 'Новая версия').click()
  button(target, 'Удалить').click()
  button(target, 'Мой квест').click()
  button(target, 'Скачать версию').click()
  button(target, 'Сделать текущей').click()
  button(target, 'Удалить версию').click()
  button(target, 'Копировать').click()
  flushSync()
  expect(calls).toEqual([
    'download:',
    'hide',
    'version',
    'delete',
    'owner',
    'download:2',
    'current:1',
    'delrev:1',
    'copy',
  ])
})

test('панель вложения: скрытое вложение предлагает «Вернуть», пока грузятся версии — заглушка', () => {
  const { target } = open({ a: { ...att, hidden: true }, revs: [], revsBusy: true })
  expect(target.textContent).toContain('Вернуть')
  expect(target.textContent).toContain('скрыто')
  expect(target.textContent).toContain('загрузка…')
})
