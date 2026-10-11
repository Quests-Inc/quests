import { downloadPdf, exportedAtLabel } from './pdfExport.ts'

export function noteExportFilename(note: { id?: number | null; title?: string }, ext: 'md' | 'pdf') {
  const id = note?.id != null ? String(note.id) : 'x'
  const raw = String(note?.title || 'note')
    .trim()
    .replace(/\s+/g, '-')
    .replace(/[^\wЀ-ӿ-]+/g, '')
    .slice(0, 48)
  return `note-${id}${raw ? `-${raw}` : ''}.${ext}`
}

export function downloadNoteMarkdown(note: { id?: number | null; title?: string; description?: string }) {
  if (!note) return
  const title = String(note.title || '').trim() || `note=${note.id ?? ''}`
  const body = String(note.description || '')
  const text = body.trimStart().startsWith('#') ? body : `# ${title}\n\n${body}`
  const blob = new Blob([text], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = noteExportFilename(note, 'md')
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

/** Renders the note to PDF on the server and downloads the result. */
export async function downloadNotePdf(note: any, opts: { labels?: Record<string, string> } = {}) {
  if (!note) return
  const title = String(note.title || '').trim() || `note=${note.id ?? ''}`
  await downloadPdf(
    {
      title,
      footnote: exportedAtLabel(),
      labels: opts.labels || {},
      blocks: [{ type: 'markdown', markdown: String(note.description || ''), empty: '(пусто)' }],
    },
    noteExportFilename(note, 'pdf')
  )
}
