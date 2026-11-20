/** A `note=12#key` click records the piece; opening the note then puts it in the address. */

const KEY = '[a-z][a-z0-9_-]{0,63}'
const HASH_RE = new RegExp(`^#${KEY}(?:/${KEY})?$`)

let pending: { id: number; fragment: string } | null = null

export function validNoteHash(hash: string) {
  return HASH_RE.test(hash)
}

export function setPendingFragment(id: number, fragment: string) {
  if (id > 0 && validNoteHash(`#${fragment}`)) pending = { id, fragment }
}

/** The fragment for this note, if the click that opened it asked for one. */
export function takePendingFragment(id: number) {
  if (pending?.id !== id) return ''
  const fragment = pending.fragment
  pending = null
  return fragment
}
