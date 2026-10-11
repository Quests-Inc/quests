// Подписи для ссылок вида kind:id: как показывать упоминание квеста, заметки, шага, шаблона или вложения.

type Titled = { id: number; title?: string }

export function buildRefLabels({
  notes,
  quests,
  questlines,
  templates,
  attachments,
}: {
  notes: Titled[]
  quests: (Titled & { steps?: Titled[] })[]
  questlines: Titled[]
  templates: Titled[]
  attachments: { id: number; filename?: string }[]
}): Record<string, string> {
  const m: Record<string, string> = {}
  for (const n of notes) m[`note:${n.id}`] = n.title || `note=${n.id}`
  for (const q of quests) {
    m[`quest:${q.id}`] = q.title || `quest=${q.id}`
    for (const s of q.steps || []) {
      m[`step:${s.id}`] = s.title || `step=${s.id}`
    }
  }
  for (const l of questlines) m[`questline:${l.id}`] = l.title || `questline=${l.id}`
  for (const t of templates) m[`template:${t.id}`] = t.title || `template=${t.id}`
  for (const a of attachments) {
    m[`attachment:${a.id}`] = a.filename || `attachment=${a.id}`
  }
  return m
}
