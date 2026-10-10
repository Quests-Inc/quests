import { listAttachments } from './api.js'
import { downloadPdf } from './pdfExport.ts'
import { significanceLabel, periodBadge, quantifiedProgress } from './questFormat.ts'
import { buildQuestPdfDocument } from './questPdfDocument.ts'

/**
 * @param {{ id?: number | null, title?: string }} quest
 */
export function questExportFilename(quest, ext = 'pdf') {
  const id = quest?.id != null ? String(quest.id) : 'x'
  const raw = String(quest?.title || 'quest')
    .trim()
    .replace(/\s+/g, '-')
    .replace(/[^\wЀ-ӿ-]+/g, '')
    .slice(0, 48)
  return `quest-${id}${raw ? `-${raw}` : ''}.${ext}`
}

/**
 * Title, description, and steps as a markdown file.
 * @param {any} quest
 */
export function downloadQuestMarkdown(quest) {
  if (!quest) return
  const title = String(quest.title || '').trim() || `quest=${quest.id ?? ''}`
  /** @type {string[]} */
  const parts = [`# ${title}`, '']
  const body = String(quest.description || '').trim()
  if (body) parts.push(body, '')
  const steps = Array.isArray(quest.steps) ? quest.steps : []
  if (steps.length) {
    parts.push('## Шаги', '')
    for (const step of steps) {
      const total = Math.max(0, Number(step.progress_total) || 0)
      const current = Math.max(0, Number(step.progress_current) || 0)
      const done = total > 0 && current >= total
      const extra = total > 1 ? ` (${current}/${total})` : ''
      const label = String(step.title || '').trim() || `step=${step.id}`
      parts.push(`- [${done ? 'x' : ' '}] ${label}${extra}`)
    }
    parts.push('')
  }
  const blob = new Blob([parts.join('\n')], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = questExportFilename(quest, 'md')
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

/**
 * Renders the quest to PDF on the server.
 * @param {any} quest
 * @param {{ labels?: Record<string, string>, tzLabel?: string }} [opts]
 */
export async function downloadQuestPdf(quest, opts = {}) {
  if (!quest) return
  let attachments = []
  try {
    attachments = (await listAttachments('quest', quest.id, { stat: false })).filter((a) => !a.hidden)
  } catch {
    // attachments are a nice-to-have in the export; don't block on them
  }
  await downloadPdf(
    buildQuestPdfDocument(quest, { ...opts, attachments }, { significanceLabel, periodBadge, quantifiedProgress }),
    questExportFilename(quest)
  )
}
