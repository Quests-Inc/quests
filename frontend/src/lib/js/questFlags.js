/** Filters for the journal's quest search (`flag:…`, `sig:…`, `tag:…`); see searchFlags.js. */

export const QUEST_FLAGS = {
  active: { label: 'в работе', test: (q) => q.status === 'active' },
  done: { label: 'выполнен', test: (q) => q.status === 'completed', aliases: ['completed'] },
  failed: { label: 'провален', test: (q) => q.status === 'failed' },
  overdue: { label: 'просрочен', test: (q) => q.status === 'expired', aliases: ['expired'] },
  frozen: { label: 'заморожен', test: (q) => q.status === 'frozen' },
  archived: { label: 'в архиве', test: (q) => q.status === 'archived' },
  pinned: { label: 'всегда в оверлее', test: (q) => !!q.pinned, aliases: ['pin'] },
  'overlay-hidden': { label: 'никогда в оверлее', test: (q) => !!q.overlay_hidden },
  automated: { label: 'автоматический', test: (q) => !!q.automated },
  deadline: { label: 'есть срок', test: (q) => !!q.deadline_at },
  steps: { label: 'есть шаги', test: (q) => (q.steps_total ?? q.steps?.length ?? 0) > 0 },
  check: { label: 'есть автопроверка', test: (q) => (q.steps || []).some((s) => s.check_command) },
}

export const QUEST_KEYS = {
  sig: {
    label: 'значимость',
    test: (q, v) => (q.significance || '').toLowerCase().startsWith(v.toLowerCase()),
  },
  tag: {
    label: 'тег',
    test: (q, v) => (q.tags || []).some((t) => (t.slug || '').toLowerCase() === v.toLowerCase() || (t.label || '').toLowerCase() === v.toLowerCase()),
  },
}

/** Statuses a flag filter asks for explicitly: they must show even when «Показывать завершённые» is off. */
export function asksClosedStatus(parsed) {
  return parsed.include.some((id) => id === 'done' || id === 'failed' || id === 'archived')
}
