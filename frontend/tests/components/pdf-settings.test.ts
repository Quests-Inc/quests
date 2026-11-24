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
mock.module('@/shared/api/attachments.ts', () => ({ listAllAttachments: async () => ({}) }))
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
