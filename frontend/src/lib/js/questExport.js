import { renderMarkdown } from './markdown.js'
import { inlineAttachmentImages } from './exportImages.js'
import { listAttachments } from './api.js'
import { statusColor, significanceLabel, periodBadge, quantifiedProgress } from './questFormat.js'

const MARGIN_MM = 15

/**
 * @param {{ id?: number | null, title?: string }} quest
 */
export function questExportFilename(quest, ext = 'pdf') {
  const id = quest?.id != null ? String(quest.id) : 'x'
  const raw = String(quest?.title || 'quest')
    .trim()
    .replace(/\s+/g, '-')
    .replace(/[^\wЀ-ӿ-]+/g, '')
    .slice(0, 48)
  return `quest-${id}${raw ? `-${raw}` : ''}.${ext}`
}

/**
 * Title, description, and steps as a markdown file.
 * @param {any} quest
 */
export function downloadQuestMarkdown(quest) {
  if (!quest) return
  const title = String(quest.title || '').trim() || `quest=${quest.id ?? ''}`
  /** @type {string[]} */
  const parts = [`# ${title}`, '']
  const body = String(quest.description || '').trim()
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

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  })[c])
}

function metaLine(quest) {
  const bits = []
  bits.push(`<span style="color:${statusColor(quest.status)}">${escapeHtml(quest.status || '')}</span>`)
  if (quest.significance && quest.significance !== 'common') bits.push(escapeHtml(significanceLabel(quest)))
  if (quest.category_label) bits.push(escapeHtml(quest.category_label))
  const period = periodBadge(quest)
  if (period) bits.push(escapeHtml(period))
  const frac = quantifiedProgress(quest)
  if (frac) bits.push(escapeHtml(frac))
  return bits.join(' &nbsp;·&nbsp; ')
}

function stepsHtml(quest, opts) {
  const steps = quest.steps || []
  if (!steps.length) return '<p class="quest-pdf__muted">Шагов нет</p>'
  return `<ol class="quest-pdf__steps">${steps
    .map((s) => {
      const cur = Number(s.progress_current) || 0
      const tot = Math.max(1, Number(s.progress_total) || 1)
      const done = Boolean(s.done) || cur >= tot
      const desc = s.description ? renderMarkdown(s.description, opts) : ''
      return `<li class="${done ? 'quest-pdf__step--done' : ''}">
        <div class="quest-pdf__step-row">
          <span class="quest-pdf__step-mark">${done ? '✓' : '○'}</span>
          <span class="quest-pdf__step-title">${escapeHtml(s.title || '')}</span>
          <span class="quest-pdf__step-progress">${cur}/${tot}</span>
        </div>
        ${desc ? `<div class="md quest-pdf__step-desc">${desc}</div>` : ''}
      </li>`
    })
    .join('')}</ol>`
}

function datesHtml(quest, tzLabel) {
  const rows = []
  if (quest.created_at) rows.push(['Создан', quest.created_at])
  if (quest.updated_at) rows.push(['Обновлён', quest.updated_at])
  if (quest.deadline_at) rows.push(['Срок', quest.deadline_at])
  if (!rows.length) return ''
  return `<dl class="quest-pdf__dates">${rows
    .map(
      ([label, val]) =>
        `<div><dt>${escapeHtml(label)}${tzLabel ? ` (${escapeHtml(tzLabel)})` : ''}</dt><dd>${escapeHtml(
          new Date(val).toLocaleString('ru-RU', { dateStyle: 'medium', timeStyle: 'short' }),
        )}</dd></div>`,
    )
    .join('')}</dl>`
}

/**
 * Self-contained HTML document for the quest, styled for print — mirrors
 * noteExport.js's approach (WeasyPrint paginates for real, no pixel math)
 * but composes several blocks instead of one markdown stream: meta line,
 * description, steps (with per-step markdown), attachments, dates.
 * @param {any} quest
 * @param {{ labels?: Record<string, string>, tzLabel?: string, attachments?: any[] }} [opts]
 */
export function buildQuestPdfHtml(quest, opts = {}) {
  const title = String(quest?.title || '').trim() || `quest=${quest?.id ?? ''}`
  const exportedAt = new Date().toLocaleString('ru-RU', { dateStyle: 'medium', timeStyle: 'short' })
  const descHtml = quest?.description ? renderMarkdown(quest.description, opts) : ''
  const attachments = opts.attachments || []

  return `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>${escapeHtml(title)}</title>
<style>
  @page { size: A4; margin: ${MARGIN_MM}mm; }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    background: #fff;
    color: #1a1a1a;
    font-family: Georgia, "Times New Roman", serif;
    font-size: 12pt;
    line-height: 1.55;
  }
  .quest-pdf__title {
    margin: 0 0 0.3em;
    font-family: system-ui, "Segoe UI", sans-serif;
    font-size: 1.75rem;
    font-weight: 700;
    line-height: 1.2;
    color: #111;
  }
  .quest-pdf__meta {
    margin: 0 0 0.35em;
    font-family: system-ui, "Segoe UI", sans-serif;
    font-size: 0.8rem;
    color: #444;
  }
  .quest-pdf__exported {
    margin: 0 0 1.25em;
    font-family: system-ui, "Segoe UI", sans-serif;
    font-size: 0.75rem;
    color: #888;
  }
  .quest-pdf__section {
    margin: 0 0 1.1em;
  }
  .quest-pdf__label {
    margin: 0 0 0.5em;
    font-family: system-ui, "Segoe UI", sans-serif;
    font-size: 0.68rem;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: #888;
    font-weight: 600;
    break-after: avoid-page;
  }
  .quest-pdf__muted { color: #999; }
  .quest-pdf__steps { list-style: none; margin: 0; padding: 0; }
  .quest-pdf__steps li {
    margin: 0 0 0.55em;
    padding-bottom: 0.55em;
    border-bottom: 1px solid #eee;
    break-inside: avoid-page;
  }
  .quest-pdf__steps li:last-child { border-bottom: 0; }
  .quest-pdf__step-row {
    display: flex;
    align-items: baseline;
    gap: 0.5em;
    font-family: system-ui, "Segoe UI", sans-serif;
    font-size: 0.95rem;
  }
  .quest-pdf__step--done .quest-pdf__step-title { color: #888; text-decoration: line-through; }
  .quest-pdf__step-mark { color: #2a8a4a; }
  .quest-pdf__step-title { flex: 1; }
  .quest-pdf__step-progress { color: #888; font-size: 0.8rem; }
  .quest-pdf__step-desc { margin: 0.3em 0 0 1.4em; font-size: 0.92em; }
  .quest-pdf__attach { list-style: none; margin: 0; padding: 0; font-family: system-ui, sans-serif; font-size: 0.9rem; }
  .quest-pdf__attach li { margin: 0 0 0.3em; }
  .quest-pdf__dates {
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 0.4em 1.5em;
    font-family: system-ui, sans-serif;
    font-size: 0.78rem;
    color: #666;
  }
  .quest-pdf__dates div { margin: 0; }
  .quest-pdf__dates dt { font-size: 0.68rem; text-transform: uppercase; letter-spacing: 0.06em; color: #999; }
  .quest-pdf__dates dd { margin: 0; color: #333; }
  .md > :first-child { margin-top: 0; }
  .md > :last-child { margin-bottom: 0; }
  .md p, .md ul, .md ol, .md .md-codeblock, .md table, .md blockquote { margin: 0 0 0.65em; break-inside: avoid-page; }
  .md h1, .md h2, .md h3, .md h4 { margin: 1.1em 0 0.4em; font-family: system-ui, sans-serif; font-weight: 700; color: #111; break-after: avoid-page; }
  .md h1 { font-size: 1.35rem; }
  .md h2 { font-size: 1.2rem; }
  .md h3 { font-size: 1.05rem; }
  .md h4 { font-size: 1rem; }
  .md ul, .md ol { padding-left: 1.35em; }
  .md li { margin: 0.15em 0; break-inside: avoid-page; }
  .md a { color: #1a5fb4; text-decoration: underline; }
  .md blockquote { padding: 0.15em 0 0.15em 0.85em; border-left: 3px solid #ccc; color: #444; }
  .md hr { border: 0; border-top: 1px solid #ddd; margin: 1em 0; }
  .md code { font-family: ui-monospace, "SF Mono", Menlo, monospace; font-size: 0.88em; padding: 0.08em 0.32em; border-radius: 3px; background: #f0f0f0; }
  .md .md-codeblock { position: relative; border-radius: 4px; background: #f4f4f4; border: 1px solid #e0e0e0; }
  .md .md-codeblock[data-lang]::before {
    content: attr(data-lang); position: absolute; top: 0.35em; left: 0.75em;
    font-family: system-ui, sans-serif; font-size: 0.68rem; letter-spacing: 0.04em;
    text-transform: uppercase; color: #888;
  }
  .md pre { margin: 0; padding: 0.75em 0.85em; white-space: pre-wrap; overflow-wrap: anywhere; }
  .md .md-codeblock[data-lang] pre { padding-top: 1.55em; }
  .md pre code { padding: 0; background: transparent; font-size: 0.82rem; line-height: 1.45; color: #222; }
  .md table { width: 100%; border-collapse: collapse; font-size: 0.92em; }
  .md tr { break-inside: avoid-page; }
  .md th, .md td { border: 1px solid #ddd; padding: 0.35em 0.5em; text-align: left; }
  .md th { background: #f6f6f6; }
  .md img { max-width: 100%; height: auto; }
  .md .md-img--center { display: block; margin-left: auto; margin-right: auto; }
  .md .md-img--right { display: block; margin-left: auto; }
</style>
</head>
<body>
  <h1 class="quest-pdf__title">${escapeHtml(title)}</h1>
  <p class="quest-pdf__meta">${metaLine(quest)}</p>
  <p class="quest-pdf__exported">Выгрузка: ${escapeHtml(exportedAt)}</p>

  ${
    descHtml
      ? `<div class="quest-pdf__section md">${descHtml}</div>`
      : ''
  }

  <div class="quest-pdf__section">
    <p class="quest-pdf__label">Шаги</p>
    ${stepsHtml(quest, opts)}
  </div>

  ${
    attachments.length
      ? `<div class="quest-pdf__section">
          <p class="quest-pdf__label">Вложения</p>
          <ul class="quest-pdf__attach">${attachments
            .map((a) => `<li>${escapeHtml(a.filename || a.name || `#${a.id}`)}</li>`)
            .join('')}</ul>
        </div>`
      : ''
  }

  ${datesHtml(quest, opts.tzLabel)}
</body>
</html>`
}

/**
 * Renders the quest to PDF server-side (WeasyPrint), composing the
 * detail view's blocks (meta, description, steps, attachments, dates)
 * rather than treating the quest as one markdown stream.
 * @param {any} quest
 * @param {{ labels?: Record<string, string>, tzLabel?: string }} [opts]
 */
export async function downloadQuestPdf(quest, opts = {}) {
  if (!quest) return
  let attachments = []
  try {
    attachments = (await listAttachments('quest', quest.id, { stat: false })).filter((a) => !a.hidden)
  } catch {
    // attachments are a nice-to-have in the export; don't block on them
  }
  const html = await inlineAttachmentImages(buildQuestPdfHtml(quest, { ...opts, attachments }))
  const filename = questExportFilename(quest)

  const resp = await fetch('/api/export/pdf', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ html, filename }),
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
