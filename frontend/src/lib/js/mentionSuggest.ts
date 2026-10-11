type Kind = 'questline' | 'quest' | 'step' | 'note' | 'attachment' | 'template'
type Mention = { kind: Kind; id: number; title: string; hint?: string; label: string }
type Titled = { id: number; title?: string | null }
type MentionSources = {
  quests?: (Titled & { steps?: Titled[] })[]
  questlines?: Titled[]
  notes?: Titled[]
  attachments?: { id: number; filename?: string; comment?: string | null }[]
  templates?: Titled[]
}

const KIND_LABEL: Record<Kind, string> = {
  questline: 'квестлайн',
  quest: 'квест',
  step: 'шаг',
  note: 'заметка',
  attachment: 'файл',
  template: 'шаблон',
}

/**
 * Candidates for `@query` autocomplete — matches already-loaded
 * quests/questlines/notes/attachments/templates (and nested steps) by title substring.
 * Ranked: title starts with query > title contains query,
 * questlines/quests before their nested steps within each tier.
 */
export function matchMentions(
  query: string,
  { quests = [], questlines = [], notes = [], attachments = [], templates = [] }: MentionSources = {},
  limit = 8
): Mention[] {
  const q = query.trim().toLowerCase()
  if (!q) return []

  const starts: Mention[] = []
  const contains: Mention[] = []

  function push(kind: Kind, id: number, title: string, hint?: string) {
    const t = title.toLowerCase()
    const idx = t.indexOf(q)
    if (idx < 0) return
    const item = { kind, id, title, hint, label: KIND_LABEL[kind] }
    ;(idx === 0 ? starts : contains).push(item)
  }

  for (const note of notes) push('note', note.id, note.title || '')
  for (const line of questlines) push('questline', line.id, line.title || '')
  for (const tpl of templates) push('template', tpl.id, tpl.title || '')
  for (const att of attachments) {
    const name = att.filename || `attachment=${att.id}`
    const hint = att.comment || undefined
    push('attachment', att.id, name, hint)
    if (att.comment) push('attachment', att.id, att.comment, name)
  }
  for (const quest of quests) {
    push('quest', quest.id, quest.title || '')
    for (const step of quest.steps || []) {
      push('step', step.id, step.title || '', quest.title)
    }
  }

  // Dedupe attachment hits (filename + comment) keeping first/best rank.
  const seen = new Set()
  const ranked = [...starts, ...contains].filter((item) => {
    const key = `${item.kind}:${item.id}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
  return ranked.slice(0, limit)
}

/** `@partial` token ending at `caret` in `text`, or null if caret isn't inside one. */
export function activeMentionToken(text: any, caret: number) {
  const upto = text.slice(0, caret)
  const m = upto.match(/@([^\s@]*)$/)
  if (!m) return null
  return { query: m[1], start: caret - m[0].length }
}

/** Replace the `@query` token with `kind=id ` (space after, cursor lands there). */
export function applyMention(text: any, token: { start: number; query: any }, item: { kind: any; id: any }) {
  const tagEnd = token.start + 1 + token.query.length
  const before = text.slice(0, token.start)
  const after = text.slice(tagEnd)
  const tag = `${item.kind}=${item.id}`
  return { text: `${before}${tag} ${after}`, caret: before.length + tag.length + 1 }
}
