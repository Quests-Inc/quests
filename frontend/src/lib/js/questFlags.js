/** What the journal's quest search understands: status:, flag:, type:, sig:, tag:, cat:, line:. See searchFlags.js. */

const STATUS_OF = { done: 'completed', overdue: 'expired' }

/** @param {{ categories?: any[] }} ctx */
export function questSpec({ categories = [] } = {}) {
  return {
    status: {
      label: 'статус',
      values: [
        { id: 'active', label: 'в работе' },
        { id: 'done', label: 'выполнен', aliases: ['completed'] },
        { id: 'failed', label: 'провален' },
        { id: 'expired', label: 'просрочен', aliases: ['overdue'] },
        { id: 'frozen', label: 'заморожен' },
        { id: 'archived', label: 'в архиве' },
        { id: 'all', label: 'любой, в том числе закрытые' },
      ],
      test: (q, v) => v === 'all' || q.status === (STATUS_OF[v] ?? v),
    },
    flag: {
      label: 'флаг',
      all: true,
      values: [
        { id: 'pinned', label: 'всегда в оверлее', aliases: ['pin'] },
        { id: 'hidden', label: 'никогда в оверлее' },
      ],
      test: (q, v) => (v === 'pinned' ? !!q.pinned : !!q.overlay_hidden),
    },
    type: {
      label: 'вид',
      values: [
        { id: 'automated', label: 'автоматический', aliases: ['auto'] },
        { id: 'manual', label: 'ручной' },
      ],
      test: (q, v) => (v === 'automated' ? !!q.automated : !q.automated),
    },
    sig: {
      label: 'значимость',
      hint: 'epic',
      test: (q, v) => (q.significance || '').toLowerCase().startsWith(v.toLowerCase()),
    },
    tag: {
      label: 'тег',
      hint: 'infra',
      test: (q, v) => (q.tags || []).some((t) => (t.slug || '').toLowerCase() === v.toLowerCase() || (t.label || '').toLowerCase() === v.toLowerCase()),
    },
    cat: {
      label: 'раздел',
      values: categories.map((c) => ({ id: c.slug, label: c.label, aliases: [String(c.id)] })),
      test: (q, v) => (q.category_slug || '').toLowerCase() === v.toLowerCase(),
    },
    line: {
      label: 'квестлайн',
      hint: 'id',
      test: (q, v) => String(q.questline_id) === v || (q.questline_title || '').replace(/\s+/g, '_').toLowerCase() === v.toLowerCase(),
    },
  }
}
