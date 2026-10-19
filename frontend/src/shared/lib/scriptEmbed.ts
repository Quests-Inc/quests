// `script=N{arg}` in a text: a link that computes on click. The script (Args → … → Result, server nodes only) is called with the
// text in braces; its value shows after the arg ("3к8 = 19"), and a click throws again. Nothing is stored (ADR-0003 §6b).
import { evalScript } from '@/shared/api/scripts.ts'

const SCRIPT_HREF = /^\?script=(\d+)&_=(.*)$/

/** Svelte action: makes the embedded-script links inside a rendered text live. */
export function scriptEmbed(node: HTMLElement) {
  const mark = () => {
    for (const a of node.querySelectorAll<HTMLAnchorElement>('a[href^="?script="]')) a.classList.add('md-script')
  }
  mark()
  const mo = new MutationObserver(() => {
    mo.disconnect()
    mark()
    mo.observe(node, { childList: true, subtree: true })
  })
  mo.observe(node, { childList: true, subtree: true })

  const onClick = async (e: MouseEvent) => {
    const a = e.target instanceof Element ? (e.target.closest('a.md-script') as HTMLAnchorElement | null) : null
    if (!a || !node.contains(a)) return
    e.preventDefault()
    e.stopPropagation()
    const m = SCRIPT_HREF.exec(a.getAttribute('href') || '')
    if (!m || a.dataset.busy) return
    const arg = decodeURIComponent(m[2])
    a.dataset.busy = '1'
    a.classList.add('md-script--busy')
    try {
      const out = await evalScript(Number(m[1]), { _: arg })
      a.textContent = `${arg} = ${out.value}`
      a.classList.remove('md-script--err')
      a.removeAttribute('title')
    } catch (err) {
      a.textContent = `${arg} — ошибка`
      a.classList.add('md-script--err')
      a.title = err instanceof Error ? err.message : String(err)
    } finally {
      delete a.dataset.busy
      a.classList.remove('md-script--busy')
    }
  }
  node.addEventListener('click', onClick, true)
  return {
    destroy() {
      mo.disconnect()
      node.removeEventListener('click', onClick, true)
    },
  }
}
