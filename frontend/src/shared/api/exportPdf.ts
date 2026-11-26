// Сервер раскладывает документ в PDF (Go, internal/pdfdoc): отправляем описание документа, получаем файл.
import { renderDiagramPng } from '@/shared/lib/markdownMermaid.ts'

// Клиент рисует каждую ```mermaid-диаграмму в PNG и присылает вместе с документом;
// сервер матчит по обрезанному исходнику и встраивает растр (лимиты — в pdfdoc).
const MAX_PDF_DIAGRAMS = 40
const MAX_PDF_DIAGRAM_BYTES = 2 * 1024 * 1024
const MAX_PDF_DIAGRAMS_BYTES = 8 * 1024 * 1024
const MERMAID_FENCE = /```mermaid[^\n]*\n([\s\S]*?)```/g

interface PdfDiagram {
  source: string
  png: string // base64
}

function harvestMermaid(markdown: string, acc: Set<string>) {
  for (const m of markdown.matchAll(MERMAID_FENCE)) {
    const source = m[1].replace(/\r\n/g, '\n').trim()
    if (source) acc.add(source)
  }
}

function gatherMarkdown(doc: unknown, acc: Set<string>) {
  const d = doc as {
    blocks?: Array<{ markdown?: unknown; steps?: Array<{ markdown?: unknown }> }>
    chapters?: unknown[]
  }
  for (const b of d?.blocks ?? []) {
    if (typeof b?.markdown === 'string') harvestMermaid(b.markdown, acc)
    for (const st of b?.steps ?? []) if (typeof st?.markdown === 'string') harvestMermaid(st.markdown, acc)
  }
  for (const ch of d?.chapters ?? []) gatherMarkdown(ch, acc)
}

function base64(bytes: Uint8Array): string {
  let out = ''
  for (let i = 0; i < bytes.length; i += 0x8000) out += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
  return btoa(out)
}

/** Render the document's mermaid diagrams to PNGs; a failed diagram is simply skipped. */
async function collectDiagrams(doc: unknown): Promise<PdfDiagram[]> {
  const sources = new Set<string>()
  gatherMarkdown(doc, sources)
  if (!sources.size) return []
  const out: PdfDiagram[] = []
  let total = 0
  for (const source of sources) {
    if (out.length >= MAX_PDF_DIAGRAMS || total >= MAX_PDF_DIAGRAMS_BYTES) break
    // PDF pages are white: render light-theme diagrams on a white ground, so the
    // (light) edges and text of the dark theme stay visible.
    const png = await renderDiagramPng(source, { theme: 'default', background: '#ffffff' })
    if (!png || png.byteLength > MAX_PDF_DIAGRAM_BYTES) continue
    total += png.byteLength
    out.push({ source, png: base64(png) })
  }
  return out
}

/** POST документа на /api/export/pdf; возвращает PDF как Blob. */
export async function renderPdf(document_: unknown, filename: string): Promise<Blob> {
  const diagrams = await collectDiagrams(document_)
  const doc = diagrams.length ? { ...(document_ as Record<string, unknown>), diagrams } : document_
  const resp = await fetch('/api/export/pdf', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ document: doc, filename }),
  })
  if (!resp.ok) {
    const detail = await resp.text().catch(() => '')
    throw new Error(`PDF render failed (${resp.status}): ${detail.slice(0, 500)}`)
  }
  return resp.blob()
}
