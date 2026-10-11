import { renderPdf } from '@/shared/api/exportPdf.ts'

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
export async function downloadPdf(
  document_: {
    title: string
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
  },
  filename: string
) {
  const blob = await renderPdf(document_, filename)
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}
