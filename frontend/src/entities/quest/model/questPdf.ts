import { downloadPdf } from '@/shared/lib/pdfExport.ts'

export function buildQuestPdfPayload(quest: any): {
  title: string
  exportedAt: string
  progress: string
  steps: { title: string; progress: string; done: boolean }[]
} {
  const exportedAt = new Date().toLocaleString('ru-RU', {
    dateStyle: 'medium',
    timeStyle: 'short',
  })
  const steps = (quest?.steps || []).map(
    (s: { progress_current: any; progress_total: any; title: any; id: any; done: any }) => {
      const cur = Number(s.progress_current) || 0
      const tot = Math.max(1, Number(s.progress_total) || 1)
      return {
        title: String(s.title || `step=${s.id}`),
        progress: `${cur}/${tot}`,
        done: Boolean(s.done) || cur >= tot,
      }
    }
  )
  return {
    title: String(quest?.title || `quest=${quest?.id ?? ''}`),
    exportedAt,
    progress: `${steps.filter((s: { done: any }) => s.done).length}/${steps.length || 0}`,
    steps,
  }
}

export async function downloadQuestPdfSimple(quest: any) {
  if (!quest) return
  const p = buildQuestPdfPayload(quest)
  const name = String(quest.title || 'quest')
    .trim()
    .replace(/\s+/g, '-')
    .replace(/[^\wЀ-ӿ-]+/g, '')
    .slice(0, 48)
  return downloadPdf(
    {
      title: p.title,
      footnote: `Выгрузка: ${p.exportedAt}`,
      labels: {},
      meta: [{ text: p.progress }],
      blocks: [{ type: 'steps', label: 'Шаги', steps: p.steps, empty: 'Шагов нет' }],
    },
    `quest-${quest.id ?? 'x'}${name ? `-${name}` : ''}.pdf`
  )
}
