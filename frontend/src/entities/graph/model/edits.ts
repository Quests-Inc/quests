// Правки на карте: какие поля действительно изменились и какое название дать новой заметке.

const sameVal = (a: unknown, b: unknown, key: string) => {
  if ((a == null || a === '') && (b == null || b === '')) return true
  if (key === 'description') return String(a ?? '').trim() === String(b ?? '').trim()
  if (key.endsWith('_at')) return new Date(a as string).getTime() === new Date(b as string).getTime()
  return JSON.stringify(a) === JSON.stringify(b)
}

/** Fields of `payload` (among `keys`) whose value differs from `orig`. */
export function diffFields(orig: Record<string, unknown>, payload: Record<string, unknown>, keys: string[]) {
  const out: Record<string, unknown> = {}
  for (const k of keys) if (k in payload && !sameVal(orig[k], payload[k], k)) out[k] = payload[k]
  return out
}

export const QUEST_KEYS = [
  'title',
  'description',
  'status',
  'significance',
  'pinned',
  'automated',
  'sort_order',
  'category_id',
  'questline_id',
  'deadline_at',
  'duration_seconds',
]
export const STEP_KEYS = [
  'title',
  'description',
  'progress_current',
  'progress_total',
  'check_command',
  'check_interval_seconds',
  'wait_previous',
  'run_mode',
  'run_on',
]

/** What changed in the quest form compared with the quest, tags included. */
export function questEditFields(orig: Record<string, any>, payload: Record<string, any>) {
  const fields = diffFields(orig, payload, QUEST_KEYS)
  const was = (orig.tags ?? []).map((t: { id: number }) => t.id).sort((a: number, b: number) => a - b)
  const now = [...(payload.tag_ids ?? [])].sort((a: number, b: number) => a - b)
  if (JSON.stringify(was) !== JSON.stringify(now)) fields.tag_ids = payload.tag_ids
  return fields
}

export const stepEditFields = (orig: Record<string, unknown>, payload: Record<string, unknown>) =>
  diffFields(orig, payload, STEP_KEYS)

/** What a note edited on its card changed: a non-empty new title and/or new text. */
export function noteEditChanges(e: { title: string; text: string; orig: { title: string; description: string } }) {
  const changed: Record<string, string> = {}
  if (e.title.trim() && e.title.trim() !== e.orig.title) changed.title = e.title.trim()
  if (e.text !== e.orig.description) changed.description = e.text
  return changed
}

const NEW_NOTE = 'Новая заметка'

/** "Новая заметка", then "Новая заметка (2)", (3)… — the first free one among `taken`. */
export function freshTitle(taken: Iterable<unknown>): string {
  const set = new Set(taken)
  if (!set.has(NEW_NOTE)) return NEW_NOTE
  let i = 2
  while (set.has(`${NEW_NOTE} (${i})`)) i += 1
  return `${NEW_NOTE} (${i})`
}
