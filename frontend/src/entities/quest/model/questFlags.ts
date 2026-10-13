/** What the journal's quest search understands: status:, flag:, type:, sig:, tag:, cat:, line:. See searchFlags.js. */

const STATUS_OF: Record<string, string> = { done: 'completed', overdue: 'expired' }

export function questSpec({ categories = [] }: { categories?: any[] } = {}) {
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
      test: (q: { status: any }, v: string) => v === 'all' || q.status === (STATUS_OF[v] ?? v),
    },
    flag: {
      label: 'флаг',
      all: true,
      values: [
        { id: 'pinned', label: 'всегда в оверлее', aliases: ['pin'] },
        { id: 'hidden', label: 'никогда в оверлее' },
      ],
      test: (q: { pinned: any; overlay_hidden: any }, v: string) => (v === 'pinned' ? !!q.pinned : !!q.overlay_hidden),
    },
    type: {
      label: 'вид',
      values: [
        { id: 'automated', label: 'автоматический', aliases: ['auto'] },
        { id: 'manual', label: 'ручной' },
      ],
      test: (q: { automated: any }, v: string) => (v === 'automated' ? !!q.automated : !q.automated),
    },
    sig: {
      label: 'значимость',
      hint: 'epic',
      test: (q: { significance: any }, v: string) => (q.significance || '').toLowerCase().startsWith(v.toLowerCase()),
    },
    tag: {
      label: 'тег',
      hint: 'infra',
      test: (q: { tags: any }, v: string) =>
        (q.tags || []).some(
          (t: { slug: any; label: any }) =>
            (t.slug || '').toLowerCase() === v.toLowerCase() || (t.label || '').toLowerCase() === v.toLowerCase()
        ),
    },
    cat: {
      label: 'раздел',
      values: [
        ...categories.map((c) => ({ id: c.slug, label: c.label, aliases: [String(c.id)] })),
        { id: 'none', label: 'без раздела' },
      ],
      test: (q: { category_slug: any }, v: string) =>
        v.toLowerCase() === 'none' && !q.category_slug
          ? true
          : (q.category_slug || '').toLowerCase() === v.toLowerCase(),
    },
    line: {
      label: 'квестлайн',
      hint: 'id',
      test: (q: { questline_id: any; questline_title: any }, v: string) =>
        String(q.questline_id) === v ||
        (q.questline_title || '').replace(/\s+/g, '_').toLowerCase() === v.toLowerCase(),
    },
  }
}
