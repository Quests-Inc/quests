/**
 * PDF export: the page describes the document (title, markdown, steps…) and the
 * server lays it out (Go, internal/pdfdoc) — no HTML, no styles, no images in the
 * request: attachment images are fetched by the server under the caller's own session.
 */

/** @returns {string} */
export function exportedAtLabel() {
  return `Выгрузка: ${new Date().toLocaleString('ru-RU', { dateStyle: 'medium', timeStyle: 'short' })}`
}

/**
 * POST the document and download the PDF it comes back as.
 * @param {object} document pdfdoc.Document
 * @param {string} filename
 */
export async function downloadPdf(document_, filename) {
  const resp = await fetch('/api/export/pdf', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ document: document_, filename }),
  })
  if (!resp.ok) {
    const detail = await resp.text().catch(() => '')
    throw new Error(`PDF render failed (${resp.status}): ${detail.slice(0, 500)}`)
  }
  const blob = await resp.blob()
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
