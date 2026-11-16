/** The tags a player can show. Missing frames stay empty. */
export type Id3Tags = { title: string; artist: string; cover: Blob | null }

const empty = (): Id3Tags => ({ title: '', artist: '', cover: null })

function synchsafe(b: Uint8Array, i: number) {
  return ((b[i] & 0x7f) << 21) | ((b[i + 1] & 0x7f) << 14) | ((b[i + 2] & 0x7f) << 7) | (b[i + 3] & 0x7f)
}

/** Bytes from the start of the file that hold the ID3v2 header + tag, or 0 if absent. */
export function id3TagLength(data: Uint8Array): number {
  if (data.length < 10 || data[0] !== 0x49 || data[1] !== 0x44 || data[2] !== 0x33) return 0
  const ver = data[3]
  if (ver !== 3 && ver !== 4) return 0
  return 10 + synchsafe(data, 6)
}

function u32(b: Uint8Array, i: number) {
  return (b[i] << 24) | (b[i + 1] << 16) | (b[i + 2] << 8) | b[i + 3]
}

function textOf(frame: Uint8Array) {
  if (!frame.length) return ''
  const enc = frame[0]
  const raw = frame.subarray(1)
  let text = ''
  if (enc === 0) text = new TextDecoder('latin1').decode(raw)
  else if (enc === 3) text = new TextDecoder('utf-8').decode(raw)
  else text = new TextDecoder('utf-16').decode(raw)
  return text.replace(/\0+$/g, '').trim()
}

function cstr(frame: Uint8Array, i: number) {
  let end = i
  while (end < frame.length && frame[end] !== 0) end++
  return { text: new TextDecoder('latin1').decode(frame.subarray(i, end)), next: end + 1 }
}

function apic(frame: Uint8Array): Blob | null {
  if (frame.length < 4) return null
  let i = 1
  const mime = cstr(frame, i)
  i = mime.next
  if (i >= frame.length) return null
  i += 1
  const descEnc = frame[0]
  if (descEnc === 1 || descEnc === 2) {
    while (i + 1 < frame.length && !(frame[i] === 0 && frame[i + 1] === 0)) i++
    i += 2
  } else {
    while (i < frame.length && frame[i] !== 0) i++
    i += 1
  }
  if (i >= frame.length) return null
  const type = mime.text || 'image/jpeg'
  const bytes = frame.subarray(i)
  const copy = new ArrayBuffer(bytes.byteLength)
  new Uint8Array(copy).set(bytes)
  return new Blob([copy], { type })
}

/** ID3v2.3/2.4 title, artist and the first picture, from the start of a file. */
export function parseId3(data: Uint8Array): Id3Tags {
  if (data.length < 10 || data[0] !== 0x49 || data[1] !== 0x44 || data[2] !== 0x33) return empty()
  const ver = data[3]
  if (ver !== 3 && ver !== 4) return empty()
  let i = 10
  if (data[5] & 0x40) {
    const ext = ver === 4 ? synchsafe(data, 10) : u32(data, 10)
    i += ext
  }
  const end = Math.min(data.length, 10 + synchsafe(data, 6))
  const tags = empty()
  while (i + 10 <= end) {
    const id = String.fromCharCode(data[i], data[i + 1], data[i + 2], data[i + 3])
    if (id === '\0\0\0\0') break
    const size = ver === 4 ? synchsafe(data, i + 4) : u32(data, i + 4)
    if (size < 0) break
    const bodyEnd = i + 10 + size
    // A truncated Range yields a broken JPEG (top strip, white below) — skip incomplete frames.
    if (bodyEnd > end) break
    const body = data.subarray(i + 10, bodyEnd)
    if (id === 'TIT2' && !tags.title) tags.title = textOf(body)
    else if (id === 'TPE1' && !tags.artist) tags.artist = textOf(body)
    else if (id === 'APIC' && !tags.cover) tags.cover = apic(body)
    i = bodyEnd
  }
  return tags
}
