/** A search snippet is plain text with matches in ⟪ ⟫ — escape it, then turn the marks into <mark>. */
export function snippetHtml(snippet: any) {
  return String(snippet ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
    .replace(/⟪/g, '<mark>')
    .replace(/⟫/g, '</mark>')
}

export const WHERE_LABEL: Record<string, string> = {
  title: 'в названии',
  body: 'в тексте',
  fuzzy: 'похоже на название',
}
