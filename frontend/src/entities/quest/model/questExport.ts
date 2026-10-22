import { listAttachments } from '@/shared/api/client.ts'
import type { AttachmentView } from '@/shared/api/knowledge_http.gen.ts'
import { downloadPdf } from '@/shared/lib/pdfExport.ts'
import { bakeAutoScripts } from '@/shared/lib/scriptBake.ts'

import { significanceLabel, periodBadge, quantifiedProgress } from './questFormat.ts'
import { buildQuestPdfDocument } from './questPdfDocument.ts'

export function questExportFilename(quest: { id?: number | null; title?: string }, ext = 'pdf') {
  const id = quest?.id != null ? String(quest.id) : 'x'
  const raw = String(quest?.title || 'quest')
    .trim()
    .replace(/\s+/g, '-')
    .replace(/[^\wЀ-ӿ-]+/g, '')
    .slice(0, 48)
  return `quest-${id}${raw ? `-${raw}` : ''}.${ext}`
}

function ownerHost(kind: string, id: unknown) {
  return typeof id === 'number' && id > 0 ? { kind, id } : undefined
}

/** `?auto` embeddings become their values. Each block keeps the owner the formula reads. */
async function withBakedScripts(quest: any) {
  const description = await bakeAutoScripts(String(quest.description || ''), ownerHost('quest', quest.id))
  const steps = []
  for (const step of quest.steps || []) {
    steps.push({
      ...step,
      description: await bakeAutoScripts(String(step?.description || ''), ownerHost('step', step?.id)),
    })
  }
  return { ...quest, description, steps }
}

/** Title, description, and steps as a markdown file. */
export async function downloadQuestMarkdown(quest: any) {
  if (!quest) return
  const title = String(quest.title || '').trim() || `quest=${quest.id ?? ''}`
  const parts: string[] = [`# ${title}`, '']
  const body = (await bakeAutoScripts(String(quest.description || ''), ownerHost('quest', quest.id))).trim()
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

/** Renders the quest to PDF on the server. */
export async function downloadQuestPdf(quest: any, opts: { labels?: Record<string, string>; tzLabel?: string } = {}) {
  if (!quest) return
  quest = await withBakedScripts(quest)
  let attachments: AttachmentView[] = []
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
