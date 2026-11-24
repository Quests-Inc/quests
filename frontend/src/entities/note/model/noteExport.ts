import { getNote, listNotes } from '@/shared/api/notes.ts'
import { downloadPdf, exportedAtLabel, type PdfDocument } from '@/shared/lib/pdfExport.ts'
import { bakeAutoScripts } from '@/shared/lib/scriptBake.ts'

import { compareNotes } from './tree.ts'

export function noteExportFilename(note: { id?: number | null; title?: string }, ext: 'md' | 'pdf') {
  const id = note?.id != null ? String(note.id) : 'x'
  const raw = String(note?.title || 'note')
    .trim()
    .replace(/\s+/g, '-')
    .replace(/[^\wЀ-ӿ-]+/g, '')
    .slice(0, 48)
  return `note-${id}${raw ? `-${raw}` : ''}.${ext}`
}

function noteHost(note: { id?: number | null }) {
  const id = note?.id
  return typeof id === 'number' && id > 0 ? { kind: 'note', id } : undefined
}

export async function downloadNoteMarkdown(note: { id?: number | null; title?: string; description?: string }) {
  if (!note) return
  const title = String(note.title || '').trim() || `note=${note.id ?? ''}`
  const body = await bakeAutoScripts(String(note.description || ''), noteHost(note))
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
export async function downloadNotePdf(
  note: any,
  opts: { labels?: Record<string, string>; book?: boolean; configure?: boolean } = {}
) {
  if (!note) return
  const title = String(note.title || '').trim() || `note=${note.id ?? ''}`
  if (opts.book) {
    const document = await buildNoteBook(note, opts.labels)
    return downloadPdf(document, noteExportFilename(note, 'pdf').replace('note-', 'book-'))
  }
  const markdown = await bakeAutoScripts(String(note.description || ''), noteHost(note))
  return downloadPdf(
    {
      title,
      footnote: exportedAtLabel(),
      labels: opts.labels || {},
      blocks: [
        {
          type: 'markdown',
          markdown,
          empty: '(пусто)',
        },
      ],
    },
    noteExportFilename(note, 'pdf'),
    {
      configure: opts.configure,
      attachmentOwner: { kind: 'note', id: note.id },
      buildBook: () => buildNoteBook(note, opts.labels, markdown),
    }
  )
}

/** Unfiltered, cycle-safe preorder using exactly the same sibling order as the note tree. */
export function noteBookOrder<T extends { id: number; parent_id?: number | null }>(notes: T[], rootId: number) {
  const children = new Map<number, T[]>()
  for (const note of notes) {
    if (note.parent_id == null) continue
    const siblings = children.get(note.parent_id) ?? []
    siblings.push(note)
    children.set(note.parent_id, siblings)
  }
  for (const siblings of children.values()) siblings.sort((a, b) => compareNotes(a, b, (n) => children.has(n.id)))
  const result: { id: number; depth: number }[] = []
  const seen = new Set<number>()
  const stack = [{ id: rootId, depth: 0 }]
  while (stack.length) {
    const item = stack.pop()!
    if (seen.has(item.id)) continue
    seen.add(item.id)
    result.push(item)
    for (const child of [...(children.get(item.id) ?? [])].reverse())
      stack.push({ id: child.id, depth: item.depth + 1 })
  }
  return result
}

/** Fetch full bodies, not catalog snippets; bake each note in its own inherited environment. */
export async function buildNoteBook(
  root: { id: number; title: string; description: string },
  labels: Record<string, string> = {},
  rootMarkdown?: string
): Promise<PdfDocument> {
  const catalog = await listNotes({ catalog: true })
  const order = noteBookOrder(catalog, root.id)
  if (order.length > 1000) throw new Error('Книга содержит больше 1000 заметок. Выберите меньший раздел.')
  const chapters: { note_id: number; title: string; depth: number; markdown: string }[] = []
  const bookLabels = { ...labels }
  // Sequential reads bound work and ensure each auto insertion is evaluated only once per note.
  for (const item of order) {
    const note = item.id === root.id ? root : await getNote(item.id)
    const title = String(note.title || `note=${item.id}`)
    bookLabels[`note:${item.id}`] = title
    chapters.push({
      note_id: item.id,
      title,
      depth: item.depth,
      markdown:
        item.id === root.id && rootMarkdown !== undefined
          ? rootMarkdown
          : await bakeAutoScripts(String(note.description || ''), { kind: 'note', id: item.id }),
    })
  }
  return {
    title: root.title,
    footnote: exportedAtLabel(),
    labels: bookLabels,
    book: true,
    reference_base: window.location.origin + window.location.pathname,
    chapters,
    blocks: [],
  }
}
