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
