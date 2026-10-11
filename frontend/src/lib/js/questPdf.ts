import { jsPDF } from 'jspdf'

const PAGE_W = 210
const PAGE_H = 297
const MARGIN = 16
const CONTENT_W = PAGE_W - MARGIN * 2
const FONT_PX = 14
const HEAD_H = 8
const STEP_H = 7

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

function pdfFilename(quest: { id: null; title: any }) {
  const id = quest?.id != null ? String(quest.id) : 'x'
  const raw = String(quest?.title || 'quest')
    .trim()
    .replace(/\s+/g, '-')
    .replace(/[^\w\u0400-\u04FF-]+/g, '')
    .slice(0, 48)
  return `quest-${id}${raw ? `-${raw}` : ''}.pdf`
}

/** jsPDF core fonts lack Cyrillic — draw via canvas using system UI fonts. */
function wrapLines(ctx: CanvasRenderingContext2D, text: string, maxWidth: number, font: string) {
  ctx.font = font
  const words = String(text || '')
    .split(/\s+/)
    .filter(Boolean)
  if (!words.length) return ['']
  const lines = []
  let cur = words[0]
  for (let i = 1; i < words.length; i++) {
    const trial = `${cur} ${words[i]}`
    if (ctx.measureText(trial).width <= maxWidth) {
      cur = trial
    } else {
      lines.push(cur)
      cur = words[i]
    }
  }
  lines.push(cur)
  return lines
}

/**
 * Download a simple quest PDF: title with progress at the end of the same
 * line, export date at the end of the next line, then the step list.
 */
export async function downloadQuestPdfSimple(quest: any) {
  if (!quest) return
  const payload = buildQuestPdfPayload(quest)
  const doc = new jsPDF({ unit: 'mm', format: 'a4' })
  const scale = 3
  const cssW = CONTENT_W * scale * (96 / 25.4)
  const cssPageH = (PAGE_H - MARGIN * 2) * scale * (96 / 25.4)

  const canvas = document.createElement('canvas')
  canvas.width = Math.ceil(cssW)
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('canvas unavailable')

  const font = `400 ${Math.round(FONT_PX * scale * 0.75)}px system-ui, "Segoe UI", sans-serif`
  const boldFont = `600 ${Math.round(FONT_PX * scale * 0.75)}px system-ui, "Segoe UI", sans-serif`
  const pxPerMm = scale * (96 / 25.4)
  const headPx = HEAD_H * pxPerMm
  const stepPx = STEP_H * pxPerMm

  const blocks: { left?: string; right?: string; font: string; h: number; color: string }[] = []
  blocks.push({ left: payload.title, right: payload.progress, font: boldFont, h: headPx, color: '#1a1a1a' })
  blocks.push({ right: `Выгрузка: ${payload.exportedAt}`, font, h: headPx, color: '#666666' })

  if (!payload.steps.length) {
    blocks.push({ left: 'Шагов нет', font, h: stepPx, color: '#444444' })
  } else {
    payload.steps.forEach((s, i) => {
      const mark = s.done ? '✓' : '○'
      const line = `${mark}  ${i + 1}. ${s.title}  (${s.progress})`
      for (const wrapped of wrapLines(ctx, line, cssW, font)) {
        blocks.push({ left: wrapped, font, h: stepPx, color: s.done ? '#2a6b3c' : '#1a1a1a' })
      }
    })
  }

  let pageBlocks: { left?: string; right?: string; font: string; h: number; color: string }[] = []
  let used = 0
  let first = true

  const flush = () => {
    if (!pageBlocks.length) return
    const h = Math.max(1, Math.ceil(used))
    canvas.height = h
    ctx.fillStyle = '#ffffff'
    ctx.fillRect(0, 0, canvas.width, h)
    ctx.textBaseline = 'top'
    let y = 0
    for (const b of pageBlocks) {
      ctx.font = b.font
      ctx.fillStyle = b.color
      if (b.left) {
        ctx.textAlign = 'left'
        ctx.fillText(b.left, 0, y)
      }
      if (b.right) {
        ctx.textAlign = 'right'
        ctx.fillText(b.right, canvas.width, y)
      }
      y += b.h
    }
    if (!first) doc.addPage()
    first = false
    const imgH = h / pxPerMm
    doc.addImage(canvas.toDataURL('image/png'), 'PNG', MARGIN, MARGIN, CONTENT_W, Math.min(imgH, PAGE_H - MARGIN * 2))
    pageBlocks = []
    used = 0
  }

  for (const b of blocks) {
    if (used + b.h > cssPageH && pageBlocks.length) flush()
    pageBlocks.push(b)
    used += b.h
  }
  flush()

  doc.save(pdfFilename(quest))
}
