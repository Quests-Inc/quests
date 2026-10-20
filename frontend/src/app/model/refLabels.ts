import { plainExcerpt, type RefPreview } from '@/shared/lib/refPreview.ts'

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

type Iconed = { id: number; icon?: string | null; icon_url?: string | null; color?: string | null }

/** Иконки ссылок по ключу kind:id: у заметки и квестлайна свои, квест показывает иконку своего квестлайна. */
export function buildRefIcons({
  notes,
  quests,
  questlines,
}: {
  notes: Iconed[]
  quests: (Iconed & {
    questline_icon?: string | null
    questline_icon_url?: string | null
    questline_color?: string | null
  })[]
  questlines: Iconed[]
}): Record<string, { icon?: string | null; url?: string | null; color?: string | null }> {
  const m: Record<string, { icon?: string | null; url?: string | null; color?: string | null }> = {}
  for (const n of notes) m[`note:${n.id}`] = { icon: n.icon, url: n.icon_url, color: n.color }
  for (const l of questlines) m[`questline:${l.id}`] = { icon: l.icon, url: l.icon_url, color: l.color }
  for (const q of quests)
    m[`quest:${q.id}`] = { icon: q.questline_icon, url: q.questline_icon_url, color: q.questline_color }
  return m
}

type PNote = {
  id: number
  title?: string
  description?: string
  pinned?: boolean
  is_private?: boolean
  is_category?: boolean
  outdated?: boolean
}
type PStep = { title?: string; done?: boolean }
type PQuest = {
  id: number
  title?: string
  status?: string
  progress_label?: string
  questline_id?: number | null
  questline_title?: string
  deadline_at?: string
  steps?: PStep[]
}
type PLine = { id: number; title?: string; description?: string }

/** Карточка-превью по kind:id из каталогов в памяти (без запросов). Приватная заметка текст не отдаёт. */
export function buildRefPreview({
  notes,
  quests,
  questlines,
}: {
  notes: PNote[]
  quests: PQuest[]
  questlines: PLine[]
}) {
  return (kind: string, id: number): RefPreview | null => {
    if (kind === 'note') {
      const n = notes.find((x) => x.id === id)
      if (!n) return null
      const meta = [n.pinned && 'закреплена', n.is_category && 'категория', n.outdated && 'устарела'].filter(
        Boolean
      ) as string[]
      if (n.is_private) return { kind, type: 'Заметка', title: n.title || `note=${id}`, meta: ['приватная'] }
      return { kind, type: 'Заметка', title: n.title || `note=${id}`, meta, body: plainExcerpt(n.description || '') }
    }
    if (kind === 'quest') {
      const q = quests.find((x) => x.id === id)
      if (!q) return null
      const next = (q.steps || []).find((s) => !s.done)
      const meta = [
        q.status,
        q.progress_label,
        q.questline_title,
        q.deadline_at && `до ${q.deadline_at.slice(0, 10)}`,
      ].filter(Boolean) as string[]
      return {
        kind,
        type: 'Квест',
        title: q.title || `quest=${id}`,
        meta,
        body: next?.title ? `Дальше: ${next.title}` : undefined,
      }
    }
    if (kind === 'questline') {
      const l = questlines.find((x) => x.id === id)
      if (!l) return null
      const open = quests.filter((q) => q.questline_id === id && q.status === 'active').length
      return {
        kind,
        type: 'Квестлайн',
        title: l.title || `questline=${id}`,
        meta: [`активных квестов: ${open}`],
        body: plainExcerpt(l.description || '', 160),
      }
    }
    return null
  }
}
