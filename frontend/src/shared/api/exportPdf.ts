// Сервер раскладывает документ в PDF (Go, internal/pdfdoc): отправляем описание документа, получаем файл.
/** POST документа на /api/export/pdf; возвращает PDF как Blob. */
export async function renderPdf(document_: unknown, filename: string): Promise<Blob> {
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
  return resp.blob()
}
