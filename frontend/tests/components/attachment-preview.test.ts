import { afterEach, expect, mock, test } from 'bun:test'

import { mount, unmount } from 'svelte'

import type { AttachmentView } from '@/shared/api/knowledge_http.gen.ts'

const calls: number[] = []
let fail = false
mock.module('@/shared/api/attachments.ts', () => ({
  getAttachmentPreview: async (id: number) => {
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
