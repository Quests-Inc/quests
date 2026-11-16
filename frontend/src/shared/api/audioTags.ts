import { id3TagLength, parseId3, type Id3Tags } from '@/shared/lib/id3.ts'

/** Covers in the wild often exceed 500 KiB; keep a hard ceiling for the Range fetch. */
const ID3_FETCH_CAP = 2 * 1024 * 1024

/** Fetch the ID3v2 tag of an audio URL (Range), then parse title / artist / cover. */
export async function fetchId3Tags(url: string, signal?: AbortSignal): Promise<Id3Tags> {
  const head = await fetch(url, { headers: { Range: 'bytes=0-9' }, signal })
  const headBuf = new Uint8Array(await head.arrayBuffer())
  const need = Math.min(id3TagLength(headBuf) || 0, ID3_FETCH_CAP)
  if (need < 10) return { title: '', artist: '', cover: null }
  const full =
    need <= headBuf.length
      ? headBuf
      : new Uint8Array(
          await (
            await fetch(url, {
              headers: { Range: `bytes=0-${need - 1}` },
              signal,
            })
          ).arrayBuffer()
        )
  return parseId3(full)
}
