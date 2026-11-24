import { expect, test, mock } from 'bun:test'

import { mount, unmount, flushSync } from 'svelte'

import { defaultPdfOptions } from '@/shared/lib/pdfOptions.ts'
mock.module('@/shared/ui/PdfPreview.svelte', () => ({ default: () => ({}) }))
let renders = 0
mock.module('@/shared/api/exportPdf.ts', () => ({
  renderPdf: async () => {
    renders++
    return new Blob(['%PDF-test'], { type: 'application/pdf' })
  },
}))
const uploads: { kind: string; id: number; name: string }[] = []
mock.module('@/shared/api/attachments.ts', () => ({
  listAllAttachments: async () => ({}),
  uploadAttachment: async (kind: string, id: number, file: File) => {
    uploads.push({ kind, id, name: file.name })
    return { id: 99, filename: file.name, content_type_detected: 'image/png' }
  },
}))
const { default: Dialog } = await import('@/shared/ui/PdfSettingsDialog.svelte')
const { readPdfPreferences, writePdfPreferences, pdfSettings } = await import('@/shared/lib/pdfSettings.svelte.ts')
const preferences = () => ({
  quick: { id: 'standard', name: 'Стандартный', options: { ...defaultPdfOptions } },
  custom: [],
})
test('preferences recover safely and remain separated by user/workspace key', () => {
  const a = preferences()
  a.quick.options.font_size = 14
  writePdfPreferences('user-a:workspace-1', a)
  expect(readPdfPreferences('user-a:workspace-1').quick.options.font_size).toBe(14)
  expect(readPdfPreferences('user-a:workspace-2').quick.options.font_size).toBe(10)
  localStorage.setItem('broken', '{')
  expect(readPdfPreferences('broken').quick.options.font_size).toBe(10)
  localStorage.setItem('invalid', JSON.stringify({ quick: { id: 'bad', name: 'Bad', options: { font_size: -3 } } }))
  expect(readPdfPreferences('invalid').quick.options.font_size).toBe(10)
})
test('preview and download share the same PDF; one-off settings do not change quick export', async () => {
  const originalCreate = URL.createObjectURL,
    originalRevoke = URL.revokeObjectURL,
    originalClick = HTMLAnchorElement.prototype.click
  URL.createObjectURL = () => 'about:blank'
  URL.revokeObjectURL = () => {}
  HTMLAnchorElement.prototype.click = () => {}
  const target = document.createElement('div')
  document.body.appendChild(target)
  const session = {
    document: { title: 'Test', footnote: '', labels: {}, blocks: [] },
    filename: 'test.pdf',
    storageKey: 'one-off',
    preferences: preferences(),
    resolve: () => {},
  }
  pdfSettings.session = session
  const component = mount(Dialog, { target, props: { session } })
  try {
    flushSync()
    const button = (text: string) => [...target.querySelectorAll('button')].find((b) => b.textContent?.trim() === text)!
    button('Обновить предпросмотр').click()
    for (let i = 0; i < 10; i++) {
      await Promise.resolve()
      flushSync()
    }
    expect(renders).toBe(1)
    expect(target.textContent).toContain('Предпросмотр актуален')
    button('Скачать PDF').click()
    for (let i = 0; i < 10; i++) {
      await Promise.resolve()
      flushSync()
    }
    expect(renders).toBe(1)
    expect(localStorage.getItem('one-off')).toBeNull()
    expect(pdfSettings.session).toBeNull()
  } finally {
    await unmount(component)
    target.remove()
    URL.createObjectURL = originalCreate
    URL.revokeObjectURL = originalRevoke
    HTMLAnchorElement.prototype.click = originalClick
  }
})

test('background upload belongs to the exported note and is selected immediately', async () => {
  const target = document.createElement('div')
  document.body.appendChild(target)
  const session = {
    document: { title: 'Root note', footnote: '', labels: {}, blocks: [] },
    filename: 'note.pdf',
    storageKey: 'upload-test',
    preferences: preferences(),
    attachmentOwner: { kind: 'note' as const, id: 15 },
    resolve: () => {},
  }
  const component = mount(Dialog, { target, props: { session } })
  try {
    flushSync()
    expect(target.textContent).toContain('Изображение будет загружено вложением к этой заметке.')
    const input = target.querySelector('input[type=file]') as HTMLInputElement
    Object.defineProperty(input, 'files', {
      value: [new File(['image'], 'my-background.png', { type: 'image/png' })],
      configurable: true,
    })
    input.dispatchEvent(new Event('change', { bubbles: true }))
    for (let i = 0; i < 10; i++) {
      await Promise.resolve()
      flushSync()
    }
    expect(uploads.at(-1)).toEqual({ kind: 'note', id: 15, name: 'my-background.png' })
    const selector = target.querySelector('#pdf-background-image') as HTMLSelectElement
    expect(selector.value).toBe('99')
    expect(selector.textContent).toContain('my-background.png')
    expect(localStorage.getItem('upload-test')).toBeNull()
  } finally {
    await unmount(component)
    target.remove()
  }
})
