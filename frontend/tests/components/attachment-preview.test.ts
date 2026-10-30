import { afterEach, expect, mock, test } from 'bun:test'

import { mount, unmount } from 'svelte'

import type { AttachmentView } from '@/shared/api/knowledge_http.gen.ts'

const calls: number[] = []
const signals: AbortSignal[] = []
let fail = false
mock.module('@/shared/api/attachments.ts', () => ({
  getAttachmentPreview: async (id: number, _revision: number, signal: AbortSignal) => {
    signals.push(signal)
    calls.push(id)
    if (fail) throw new Error('unavailable')
    return { kind: 'text', text: '<script>alert(1)</script>\nНачало файла' }
  },
}))
const { default: AttachmentPreview } = await import('@/entities/attachment/ui/AttachmentPreview.svelte')
const originalObserver = globalThis.IntersectionObserver
let reveal: () => void
class Observer {
  constructor(callback: IntersectionObserverCallback) {
    reveal = () =>
      callback([{ isIntersecting: true } as IntersectionObserverEntry], this as unknown as IntersectionObserver)
  }
  observe() {}
  disconnect() {}
}
let app: ReturnType<typeof mount>
let target: HTMLDivElement
const a: AttachmentView = {
  id: 1,
  filename: 'file.txt',
  owner_type: 'note',
  owner_id: 1,
  size_bytes: 12,
  content_type_declared: 'text/plain',
  content_type_detected: 'text/plain',
  comment: '',
  scan_status: 'clean',
  revision: 1,
  revision_count: 1,
  hidden: false,
}
const tick = () => new Promise((resolve) => setTimeout(resolve, 0))
afterEach(() => {
  unmount(app)
  target.remove()
  calls.length = 0
  signals.length = 0
  fail = false
  globalThis.IntersectionObserver = originalObserver
})
async function open(filename = 'file.txt') {
  globalThis.IntersectionObserver = Observer as unknown as typeof IntersectionObserver
  target = document.createElement('div')
  document.body.append(target)
  app = mount(AttachmentPreview, { target, props: { a: { ...a, filename } } })
  await tick()
}
test('preview loads only near the viewport and keeps markup as plain text', async () => {
  await open()
  expect(calls).toEqual([])
  reveal()
  await tick()
  await tick()
  expect(calls).toEqual([1])
  expect(target.querySelector('pre')?.textContent).toContain('<script>alert(1)</script>')
  expect(target.querySelector('script')).toBeNull()
})
test('unavailable preview falls back to its file icon', async () => {
  fail = true
  await open()
  reveal()
  await tick()
  await tick()
  expect(target.querySelector('pre')).toBeNull()
  expect(target.querySelector('.file-preview__icon')?.textContent).toContain('TXT')
})

test('HTML preview is highlighted without becoming executable markup', async () => {
  await open('file.html')
  reveal()
  await tick()
  await tick()
  expect(target.querySelector('pre .hljs-tag')).not.toBeNull()
  expect(target.querySelector('pre')?.textContent).toContain('<script>alert(1)</script>')
  expect(target.querySelector('script')).toBeNull()
})

for (const filename of ['file.pdf', 'file.docx', 'file.xlsx', 'file.pptx', 'file.zip']) {
  test(`${filename} waits for explicit consent even with a text MIME`, async () => {
    await open(filename)
    reveal()
    await tick()
    expect(calls).toEqual([])
    let stopped = false
    const button = target.querySelector('button')!
    expect(button.textContent).toContain('Показать превью')
    const event = new MouseEvent('click', { bubbles: true })
    const stop = event.stopPropagation.bind(event)
    event.stopPropagation = () => {
      stopped = true
      stop()
    }
    button.dispatchEvent(event)
    await tick()
    await tick()
    expect(calls).toEqual([1])
    expect(stopped).toBe(true)
    expect(target.querySelector('pre')).not.toBeNull()
    reveal()
    await tick()
    expect(calls).toEqual([1])
  })
}

test('failed manual preview settles without repeated automatic requests', async () => {
  fail = true
  await open('file.pdf')
  target.querySelector('button')!.click()
  await tick()
  await tick()
  expect(target.querySelector('button')?.textContent).toContain('Превью недоступно')
  reveal()
  await tick()
  expect(calls).toEqual([1])
})

test('switching revisions aborts stale work and requires a fresh click', async () => {
  const { default: Harness } = await import('./AttachmentPreviewHarness.svelte')
  globalThis.IntersectionObserver = Observer as unknown as typeof IntersectionObserver
  target = document.createElement('div')
  document.body.append(target)
  const file = { ...a, filename: 'file.pdf' }
  const harness = mount(Harness, { target, props: { initial: file } })
  app = harness
  await tick()
  target.querySelector('button')!.click()
  await tick()
  await tick()
  expect(calls).toEqual([1])
  harness.change({ ...file, revision: 2 })
  await tick()
  expect(signals[0].aborted).toBe(true)
  expect(target.querySelector('pre')).toBeNull()
  expect(target.querySelector('button')?.textContent).toContain('Показать превью')
  expect(calls).toEqual([1])
  harness.change(file)
  await tick()
  expect(calls).toEqual([1])
  target.querySelector('button')!.click()
  await tick()
  expect(calls).toEqual([1, 1])
})
