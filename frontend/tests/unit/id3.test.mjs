import { expect, test } from 'bun:test'

import { id3TagLength, parseId3 } from '../../src/shared/lib/id3.ts'

function u32(n) {
  return [(n >>> 24) & 255, (n >>> 16) & 255, (n >>> 8) & 255, n & 255]
}

function synch(n) {
  return [(n >> 21) & 0x7f, (n >> 14) & 0x7f, (n >> 7) & 0x7f, n & 0x7f]
}

test('parseId3 читает название и автора', () => {
  const title = new TextEncoder().encode('Таверна')
  const artist = new TextEncoder().encode('Бард')
  const frames = [
    0x54,
    0x49,
    0x54,
    0x32,
    ...u32(1 + title.length),
    0,
    0,
    0x03,
    ...title,
    0x54,
    0x50,
    0x45,
    0x31,
    ...u32(1 + artist.length),
    0,
    0,
    0x03,
    ...artist,
  ]
  const size = frames.length
  const data = new Uint8Array([0x49, 0x44, 0x33, 3, 0, 0, ...synch(size), ...frames])
  expect(id3TagLength(data)).toBe(10 + size)
  expect(parseId3(data)).toMatchObject({ title: 'Таверна', artist: 'Бард', cover: null })
  expect(parseId3(new Uint8Array([1, 2, 3]))).toEqual({ title: '', artist: '', cover: null })
})

test('обрезанный APIC не даёт битую обложку', () => {
  const jpeg = new Uint8Array([0xff, 0xd8, 0xff, 0xd9, ...new Uint8Array(40)])
  // text encoding + mime + picture type + empty desc + jpeg
  const body = [0x00, ...new TextEncoder().encode('image/jpeg'), 0x00, 0x03, 0x00, ...jpeg]
  const frames = [0x41, 0x50, 0x49, 0x43, ...u32(body.length), 0, 0, ...body]
  const full = new Uint8Array([0x49, 0x44, 0x33, 3, 0, 0, ...synch(frames.length), ...frames])
  expect(parseId3(full).cover).toBeTruthy()
  expect(parseId3(full.subarray(0, 40)).cover).toBeNull()
})
