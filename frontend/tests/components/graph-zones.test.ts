import { expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import type { GraphZone } from '@/entities/graph'
import GraphZones from '@/widgets/graph-canvas/ui/GraphZones.svelte'

function pointer(target: Element, type: string, x: number, y: number) {
  target.dispatchEvent(new PointerEvent(type, { bubbles: true, pointerId: 1, button: 0, clientX: x, clientY: y }))
  flushSync()
}

test('зона рисуется в мировых координатах; отмена жеста не сохраняет фигуру', async () => {
  const target = document.createElement('div')
  document.body.append(target)
  const changes: GraphZone[][] = []
  const app = mount(GraphZones, {
    target,
    props: {
      zones: [],
      cam: { x: 10, y: 20, k: 2 },
      drawing: true,
      worldAt: (e: { clientX: number; clientY: number }) => ({ x: (e.clientX - 10) / 2, y: (e.clientY - 20) / 2 }),
      onChange: (z: GraphZone[]) => changes.push(z),
      onFinish: () => {},
    },
  })
  flushSync()
  const surface = target.querySelector('.zones') as HTMLElement
  surface.setPointerCapture = () => {}
  pointer(surface, 'pointerdown', 210, 220)
  pointer(surface, 'pointermove', 10, 20)
  pointer(surface, 'pointerup', 10, 20)
  expect(changes[0][0]).toMatchObject({ x: 0, y: 0, w: 100, h: 100 })
  pointer(surface, 'pointerdown', 10, 20)
  pointer(surface, 'pointermove', 410, 420)
  pointer(surface, 'pointercancel', 410, 420)
  expect(changes).toHaveLength(1)
  await unmount(app)
  target.remove()
})

test('зону можно переместить, растянуть и подписать; некорректный размер не удаляет её', async () => {
  const target = document.createElement('div')
  document.body.append(target)
  const zone: GraphZone = { id: 'a', title: 'Зона', color: '#6a8fbf', x: 10, y: 20, w: 200, h: 100 }
  const changes: GraphZone[][] = []
  const app = mount(GraphZones, {
    target,
    props: {
      zones: [zone],
      cam: { x: 0, y: 0, k: 1 },
      worldAt: (e: { clientX: number; clientY: number }) => ({ x: e.clientX, y: e.clientY }),
      onChange: (z: GraphZone[]) => changes.push(z),
      onFinish: () => {},
    },
  })
  flushSync()
  const header = target.querySelector('.zone__title') as HTMLElement
  const resize = target.querySelector('.zone__resize') as HTMLElement
  header.setPointerCapture = resize.setPointerCapture = () => {}
  pointer(header, 'pointerdown', 10, 20)
  pointer(header, 'pointermove', 60, 90)
  pointer(header, 'pointerup', 60, 90)
  expect(changes[0][0]).toMatchObject({ x: 60, y: 90, w: 200, h: 100 })
  pointer(resize, 'pointerdown', 210, 120)
  pointer(resize, 'pointermove', 260, 200)
  pointer(resize, 'pointerup', 260, 200)
  expect(changes[1][0]).toMatchObject({ x: 10, y: 20, w: 250, h: 180 })
  const input = target.querySelector('[aria-label="Название зоны"]') as HTMLInputElement
  input.value = 'Работа'
  input.dispatchEvent(new Event('change', { bubbles: true }))
  flushSync()
  expect(changes[2][0].title).toBe('Работа')
  const width = target.querySelector('[aria-label="Ширина зоны"]') as HTMLInputElement
  width.value = '0'
  width.dispatchEvent(new Event('change', { bubbles: true }))
  flushSync()
  expect(changes).toHaveLength(3)
  await unmount(app)
  target.remove()
})
