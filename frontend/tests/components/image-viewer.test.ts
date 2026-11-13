import { afterEach, expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import { attachmentImage, imageViewer, showImages } from '@/shared/lib/imageViewer.svelte.ts'
import ImageViewer from '@/shared/ui/ImageViewer.svelte'
let clean: (() => void)[] = []
afterEach(() => {
  imageViewer.open = false
  flushSync()
  clean.forEach((f) => f())
  clean = []
})
function render() {
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(ImageViewer, { target })
  clean.push(() => {
    unmount(app)
    target.remove()
  })
  return target
}
test('галерея: исходный размер по запросу, навигация, Escape и возврат фокуса', () => {
  const opener = document.createElement('button')
  document.body.append(opener)
  opener.focus()
  clean.push(() => opener.remove())
  const root = render()
  const images = [attachmentImage({ id: 1, filename: 'one.png' }), attachmentImage({ id: 2, filename: 'two.png' })]
  flushSync(() => showImages(images, images[0].src))
  expect(root.querySelector('[role=dialog]')).not.toBeNull()
  expect(root.querySelector('img')?.getAttribute('src')).toBe('/api/attachments/1/image?preview=1')
  flushSync(() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight' })))
  expect(root.textContent).toContain('two.png')
  expect(imageViewer.index).toBe(1)
  flushSync(() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })))
  expect(root.querySelector('[role=dialog]')).toBeNull()
  expect(document.activeElement).toBe(opener)
})
test('ошибка загрузки и повтор, уникальные изображения', () => {
  const root = render()
  const a = attachmentImage({ id: 3, revision: 2 })
  flushSync(() => showImages([a, a], a.src))
  expect(imageViewer.images).toHaveLength(1)
  flushSync(() => root.querySelector('img')!.dispatchEvent(new Event('error')))
  expect(root.textContent).toContain('Не удалось загрузить')
  flushSync(() =>
    Array.from(root.querySelectorAll('button'))
      .find((b) => b.textContent === 'Повторить')!
      .click()
  )
  expect(root.textContent).toContain('Загрузка…')
})

test('касания: pinch увеличивает, перетаскивание двигает, тап по изображению не закрывает', () => {
  const oldObserver = globalThis.ResizeObserver
  globalThis.ResizeObserver = class {
    constructor(private callback: ResizeObserverCallback) {}
    observe() {
      this.callback(
        [{ contentRect: { width: 500, height: 500 } }] as ResizeObserverEntry[],
        this as unknown as ResizeObserver
      )
    }
    unobserve() {}
    disconnect() {}
  } as typeof ResizeObserver
  clean.push(() => (globalThis.ResizeObserver = oldObserver))
  const root = render()
  const image = attachmentImage({ id: 8 })
  flushSync(() => showImages([image], image.src))
  const img = root.querySelector('img')!
  Object.defineProperties(img, { naturalWidth: { value: 1000 }, naturalHeight: { value: 1000 } })
  flushSync(() => img.dispatchEvent(new Event('load')))
  const pointer = (type: string, id: number, x: number, y: number) =>
    flushSync(() =>
      img.dispatchEvent(
        new PointerEvent(type, {
          pointerId: id,
          pointerType: 'touch',
          clientX: x,
          clientY: y,
          button: 0,
          bubbles: true,
        })
      )
    )
  pointer('pointerdown', 1, 200, 200)
  pointer('pointerdown', 2, 400, 200)
  pointer('pointermove', 2, 600, 200)
  expect(img.style.transform).toContain('scale(2)')
  pointer('pointerup', 2, 600, 200)
  pointer('pointerup', 1, 200, 200)
  pointer('pointerdown', 3, 200, 200)
  pointer('pointermove', 3, 100, 200)
  pointer('pointerup', 3, 100, 200)
  expect(img.style.transform).toContain('translate(-100px,0px)')
  pointer('pointerdown', 4, 200, 200)
  pointer('pointerup', 4, 200, 200)
  expect(imageViewer.open).toBe(true)
})
