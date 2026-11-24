import type { PdfOptions } from './pdfOptions.ts'

/**
 * PDF export: the page describes the document (title, markdown, steps…) and the
 * server lays it out (Go, internal/pdfdoc) — no HTML, no styles, no images in the
 * request: attachment images are fetched by the server under the caller's own session.
 */

export function exportedAtLabel(): string {
  return `Выгрузка: ${new Date().toLocaleString('ru-RU', { dateStyle: 'medium', timeStyle: 'short' })}`
}

/**
 * POST the document and download the PDF it comes back as.
 * @param document pdfdoc.Document
 */
export type PdfDocument = {
  title: string
  options?: PdfOptions
  book?: boolean
  reference_base?: string
  chapters?: { note_id: number; title: string; depth: number; markdown: string }[]
  meta?: { text: string; color?: string }[]
  footnote: string
  labels: Record<string, string>
  blocks:
    | (
        | {
            type: string
            markdown: string
            label?: undefined
            steps?: undefined
            empty?: undefined
            items?: undefined
            facts?: undefined
          }
        | {
            type: string
            label: string
            steps: any
            empty: string
            markdown?: undefined
            items?: undefined
            facts?: undefined
          }
        | {
            type: string
            label: string
            items: string[]
            markdown?: undefined
            steps?: undefined
            empty?: undefined
            facts?: undefined
          }
        | {
            type: string
            facts: { label: string; value: string }[]
            markdown?: undefined
            label?: undefined
            steps?: undefined
            empty?: undefined
            items?: undefined
          }
      )[]
    | { type: string; markdown: string; empty: string }[]
}

export async function downloadPdf(
  document_: PdfDocument,
  filename: string,
  options: {
    configure?: boolean
    buildBook?: () => Promise<PdfDocument>
    attachmentOwner?: { kind: 'note' | 'quest'; id: number }
  } = {}
) {
  const { exportPdfDocument } = await import('./pdfSettings.svelte.ts')
  return exportPdfDocument(
    { reference_base: window.location.origin + window.location.pathname, ...document_ },
    filename,
    options
  )
}
