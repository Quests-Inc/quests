/** Copy plain text; works on HTTP (non-secure) where Clipboard API is missing. */
export async function copyText(text) {
  const value = String(text ?? '')
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value)
    return
  }
  const el = document.createElement('textarea')
  el.value = value
  el.setAttribute('readonly', '')
  el.style.position = 'fixed'
  el.style.left = '-9999px'
  el.style.top = '0'
  document.body.appendChild(el)
  el.focus()
  el.select()
  el.setSelectionRange(0, value.length)
  try {
    if (!document.execCommand('copy')) {
      throw new Error('copy failed')
    }
  } finally {
    document.body.removeChild(el)
  }
}
