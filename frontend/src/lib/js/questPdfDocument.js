import { exportedAtLabel } from './pdfExport.ts'

/** Print colours of the status chip (the app's CSS variables mean nothing to the server). */
const STATUS_COLORS = {
  active: '#2f7d4f',
  frozen: '#2b6fb3',
  expired: '#b0701a',
  completed: '#2f7d4f',
  failed: '#b3382c',
  archived: '#8a8a8a',
}

/**
 * @typedef {{ significanceLabel: (q: any) => string, periodBadge: (q: any) => string | null, quantifiedProgress: (q: any) => string | null }} QuestFormat
 */

/** @param {any} quest @param {QuestFormat} fmt */
function metaChips(quest, fmt) {
  /** @type {{ text: string, color?: string }[]} */
  const chips = [{ text: String(quest.status || ''), color: STATUS_COLORS[quest.status] }]
  if (quest.significance && quest.significance !== 'common') chips.push({ text: fmt.significanceLabel(quest) })
  if (quest.category_label) chips.push({ text: String(quest.category_label) })
  const period = fmt.periodBadge(quest)
  if (period) chips.push({ text: period })
  const frac = fmt.quantifiedProgress(quest)
  if (frac) chips.push({ text: frac })
  return chips.filter((c) => c.text)
}

function stepRows(quest) {
  return (quest.steps || []).map((s) => {
    const cur = Number(s.progress_current) || 0
    const tot = Math.max(1, Number(s.progress_total) || 1)
    return {
      title: String(s.title || ''),
      progress: `${cur}/${tot}`,
      done: Boolean(s.done) || cur >= tot,
      markdown: String(s.description || ''),
    }
  })
}

function dateFacts(quest, tzLabel) {
  const fmt = (v) => new Date(v).toLocaleString('ru-RU', { dateStyle: 'medium', timeStyle: 'short' })
  const suffix = tzLabel ? ` (${tzLabel})` : ''
  return [
    ['Создан', quest.created_at],
    ['Обновлён', quest.updated_at],
    ['Срок', quest.deadline_at],
  ]
    .filter(([, v]) => v)
    .map(([label, v]) => ({ label: `${label}${suffix}`, value: fmt(v) }))
}

/**
 * The quest as a document for the server: meta line, description, steps (each with its own
 * markdown), attachments and dates — the detail view's blocks, not one markdown stream.
 * @param {any} quest
 * @param {{ labels?: Record<string, string>, tzLabel?: string, attachments?: any[] }} opts
 * @param {QuestFormat} fmt label helpers (kept out of this module: they pull in the API client)
 */
export function buildQuestPdfDocument(quest, opts, fmt) {
  const title = String(quest?.title || '').trim() || `quest=${quest?.id ?? ''}`
  const attachments = opts.attachments || []
  return {
    title,
    meta: metaChips(quest, fmt),
    footnote: exportedAtLabel(),
    labels: opts.labels || {},
    blocks: [
      { type: 'markdown', markdown: String(quest?.description || '') },
      { type: 'steps', label: 'Шаги', steps: stepRows(quest), empty: 'Шагов нет' },
      {
        type: 'list',
        label: 'Вложения',
        items: attachments.map((a) => String(a.filename || a.name || `#${a.id}`)),
      },
      { type: 'facts', facts: dateFacts(quest, opts.tzLabel) },
    ],
  }
}
