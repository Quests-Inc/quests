import { expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import Harness from './GraphImageHarness.svelte'
test('preview starts small, increases on demand and does not reload a lower quality while zooming out', async () => {
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(Harness, { target })
  flushSync()
  const src = () => target.querySelector('img')?.getAttribute('src')
  expect(src()).toContain('thumb=128')
  const input = target.querySelector('input')!
  const resize = async (value: number) => {
    input.value = String(value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
    flushSync()
    await new Promise((r) => setTimeout(r, 180))
    flushSync()
  }
  await resize(600)
  expect(src()).toContain('thumb=1024')
  await resize(70)
  expect(src()).toContain('thumb=1024')
  await resize(3000)
  expect(src()).not.toContain('thumb=')
  await unmount(app)
  target.remove()
})

test('a raster card has a real initial height before the first image is decoded', async () => {
  const { default: Card } = await import('@/widgets/graph-canvas/ui/GraphCard.svelte')
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(Card, {
    target,
    props: {
      n: {
        key: 'attachment:42',
        id: 42,
        type: 'attachment',
        title: 'Image.png',
        mime: 'image/png',
        x: 0,
        y: 0,
        w: 0,
        h: 0,
        links: 0,
        depth: 0,
      },
      raster: true,
      ready: false,
      onCardDown: () => {},
      onCtx: () => {},
      onStartEdit: () => {},
      onFinishEdit: () => {},
      onOpenMore: () => {},
      onShowChanges: () => {},
    },
  })
  flushSync()
  const card = target.querySelector<HTMLElement>('.gcard')!
  expect(parseFloat(card.style.height)).toBeGreaterThan(50)
  expect(card.querySelector('img')).not.toBeNull()
  await unmount(app)
  target.remove()
})
