import { afterEach, expect, test } from 'bun:test'

const play = HTMLMediaElement.prototype.play
const pause = HTMLMediaElement.prototype.pause
const fetchImpl = globalThis.fetch
HTMLMediaElement.prototype.play = () => Promise.resolve()
HTMLMediaElement.prototype.pause = () => {}
globalThis.fetch = (() => Promise.resolve(new Response(new Uint8Array()))) as typeof fetch

import { flushSync, mount, unmount } from 'svelte'

import { playAttachment, player, setPinned, stopPlayback } from '@/shared/lib/player.svelte.ts'
import AudioPlayer from '@/shared/ui/AudioPlayer.svelte'

let target: HTMLElement | null = null
let app: ReturnType<typeof mount> | null = null

afterEach(() => {
  HTMLMediaElement.prototype.play = play
  HTMLMediaElement.prototype.pause = pause
  globalThis.fetch = fetchImpl
  setPinned(false)
  stopPlayback()
  if (app) unmount(app)
  app = null
  target?.remove()
  target = null
})

test('один трек открывает закладку и помечается как играющий', () => {
  target = document.createElement('div')
  document.body.append(target)
  app = mount(AudioPlayer, { target })
  flushSync()
  expect(target.querySelector('.dock')).toBeNull()
  playAttachment({ id: 7, filename: 'theme.mp3', content_type_detected: 'audio/mpeg' })
  flushSync()
  expect(player.playing).toBe(true)
  expect(target.querySelector('.dock__tab')).toBeTruthy()
  expect(target.querySelector('.dock__title')?.textContent).toBe('theme.mp3')
  playAttachment({ id: 7, filename: 'theme.mp3', content_type_detected: 'audio/mpeg' })
  flushSync()
  expect(player.playing).toBe(false)
  setPinned(true)
  flushSync()
  expect(target.querySelector('.dock--pinned')).toBeTruthy()
  expect(target.querySelector('.dock__tab')).toBeNull()
})

test('закладка показывает время, уход курсора снимает фокус, конец трека убирает плеер', () => {
  target = document.createElement('div')
  document.body.append(target)
  app = mount(AudioPlayer, { target })
  setPinned(false)
  playAttachment({ id: 8, filename: 'theme.mp3', content_type_detected: 'audio/mpeg' })
  flushSync()
  expect(target.querySelector('.dock__tab-time')?.textContent).toBe('0:00/0:00')
  expect(target.querySelector('.dock__note')).toBeTruthy()
  const playBtn = target.querySelector('.dock__play') as HTMLButtonElement
  playBtn.focus()
  expect(document.activeElement).toBe(playBtn)
  target.querySelector('.dock')?.dispatchEvent(new PointerEvent('pointerleave', { bubbles: true }))
  flushSync()
  expect(document.activeElement).not.toBe(playBtn)
  target.querySelector('audio')?.dispatchEvent(new Event('ended'))
  flushSync()
  expect(player.track).toBeNull()
  expect(target.querySelector('.dock')).toBeNull()
})
