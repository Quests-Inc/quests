import { parseQuery, passes } from '@/shared/lib/searchFlags.ts'

/** What pickers that search quests, notes and questlines together understand (map centre, «insert a link»).
 *  An item is { type: 'quest' | 'note' | 'questline', x: <the entity> }. */
export const ENTITY_SPEC = {
  type: {
    label: 'вид',
    values: [
      { id: 'quest', label: 'квесты' },
      { id: 'note', label: 'заметки' },
      { id: 'questline', label: 'квестлайны' },
    ],
    test: (i: { type: any }, v: any) => i.type === v,
  },
  flag: {
    label: 'флаг',
    all: true,
    values: [
      { id: 'outdated', label: 'заметка устарела', aliases: ['stale'] },
      { id: 'pinned', label: 'закреплено', aliases: ['pin'] },
    ],
    test: (i: { type: string; x: { outdated: any; pinned: any } }, v: string) =>
      v === 'outdated' ? i.type === 'note' && !!i.x.outdated : !!i.x.pinned,
  },
  status: {
    label: 'статус квеста',
    values: [
      { id: 'active', label: 'в работе' },
      { id: 'done', label: 'выполнен', aliases: ['completed'] },
      { id: 'failed', label: 'провален' },
      { id: 'frozen', label: 'заморожен' },
    ],
    test: (i: { type: string; x: { status: any } }, v: string) =>
      i.type === 'quest' && i.x.status === (v === 'done' ? 'completed' : v),
  },
}

/** Quests, notes and questlines matching the words and filters of `raw` (title or `type=id`). */
export function findEntities(
  raw: any,
  { questlines = [], quests = [], notes = [], types = ['questline', 'quest', 'note'], limit = 50 }: any
) {
  const parsed = parseQuery(raw, ENTITY_SPEC)
  const q = parsed.text.toLowerCase()
  if (!q && !parsed.active) return { parsed, items: [] }
  const items: { type: any; id: any; title: any }[] = []
  const scan = (type: string, list: any[]) => {
    if (!types.includes(type)) return
    for (const x of list) {
      const item = { type, x }
      if (!passes(parsed, ENTITY_SPEC, item)) continue
      if (q && !((x.title || '').toLowerCase().includes(q) || `${type}=${x.id}` === q)) continue
      items.push({ type, id: x.id, title: x.title })
      if (items.length >= limit) return
    }
  }
  scan('questline', questlines)
  scan('quest', quests)
  scan('note', notes)
  return { parsed, items: items.slice(0, limit) }
}
