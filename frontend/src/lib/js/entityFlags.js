import { parseFlagQuery, passesFlags } from './searchFlags.js'

/** Flags for pickers that search quests, notes and questlines together (map centre, «insert a link»).
 *  An item is { type: 'quest' | 'note' | 'questline', x: <the entity> }. */
export const ENTITY_FLAGS = {
  quest: { label: 'квесты', test: (i) => i.type === 'quest' },
  note: { label: 'заметки', test: (i) => i.type === 'note' },
  questline: { label: 'квестлайны', test: (i) => i.type === 'questline' },
  outdated: { label: 'заметка устарела', test: (i) => i.type === 'note' && !!i.x.outdated, aliases: ['stale'] },
  pinned: { label: 'закреплено', test: (i) => !!i.x.pinned, aliases: ['pin'] },
  active: { label: 'квест в работе', test: (i) => i.type === 'quest' && i.x.status === 'active' },
  done: { label: 'квест выполнен', test: (i) => i.type === 'quest' && i.x.status === 'completed', aliases: ['completed'] },
}

/** Quests, notes and questlines matching the words and `flag:` filters of `raw` (title or `type=id`). */
export function findEntities(raw, { questlines = [], quests = [], notes = [], types = ['questline', 'quest', 'note'], limit = 50 }) {
  const parsed = parseFlagQuery(raw, ENTITY_FLAGS)
  const q = parsed.text.toLowerCase()
  if (!q && !parsed.active) return { parsed, items: [] }
  const items = []
  const scan = (type, list) => {
    if (!types.includes(type)) return
    for (const x of list) {
      const item = { type, x }
      if (!passesFlags(parsed, ENTITY_FLAGS, item)) continue
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
