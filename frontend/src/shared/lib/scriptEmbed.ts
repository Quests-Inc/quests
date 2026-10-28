// `script=N{arg}` in a text: a link that computes on click. `script=N{arg}?auto` computes when the
// text is shown. A click throws again. Nothing is stored (ADR-0003 §6b).
import { rollValue } from '@/shared/api/client.ts'
import { evalScriptText } from '@/shared/api/scripts.ts'

import { labelParts, MAX_AUTO_SCRIPTS, parseScriptHref } from './scriptRef.ts'
import { hostKey, touchValues } from './valueLive.svelte.ts'

function valueHost(node: HTMLElement) {
  const raw = node.dataset.valueHost
  if (!raw) return undefined
  const cut = raw.indexOf(':')
  const id = Number(raw.slice(cut + 1))
  if (cut < 1 || !Number.isInteger(id) || id <= 0) return undefined
  return { kind: raw.slice(0, cut), id }
}

/** `label` is the formula shown with its values as chips, then `tail`; without a label `text` is shown as is. */
type Row = { text: string; label?: string; tail?: string; err: boolean; title: string }

type Parsed = { id: number; arg: string; auto: boolean; hide: boolean; set: string; on: string }

function cacheKey(node: HTMLElement, parsed: Parsed) {
  return `${node.dataset.valueHost || ''}\u0000${node.dataset.valueSig || ''}\u0000${parsed.id}\u0000${parsed.arg}\u0000${parsed.hide ? '1' : '0'}`
}

function rowFor(parsed: Parsed, value: string | null, errTitle = ''): Row {
  if (errTitle) return { text: parsed.hide ? '?' : `${parsed.arg} — ошибка`, err: true, title: errTitle }
  if (value == null) return { text: parsed.hide ? '?' : parsed.arg, err: false, title: '' }
  const title = parsed.auto || parsed.hide ? 'Пересчитать' : ''
  if (parsed.hide) return { text: value, err: false, title }
  return { text: `${parsed.arg} = ${value}`, label: parsed.arg, tail: ` = ${value}`, err: false, title }
}

/** Svelte action: makes the embedded-script links inside a rendered text live. */
export function scriptEmbed(node: HTMLElement, initial: Record<string, string> = {}) {
  let names = initial
  let rev = 0
  const cache = new Map<string, Row>()
  let timer = 0
  let armed = false

  const remember = (key: string, row: Row) => {
    if (cache.size > 200) cache.delete(cache.keys().next().value as string)
    cache.set(key, row)
  }

  /** The formula with each named value as a chip, like a `{{name}}` in prose; the chip keeps the name for hover and edit. */
  const fillLabel = (a: HTMLAnchorElement, label: string, tail: string) => {
    const kids: Node[] = labelParts(label, names).map((part) => {
      if (!part.name) return document.createTextNode(part.text)
      const chip = document.createElement('span')
      chip.className = 'md-value'
      chip.dataset.value = part.name
      chip.textContent = part.text
      return chip
    })
    if (tail) kids.push(document.createTextNode(tail))
    a.replaceChildren(...kids)
    a.dataset.lrev = String(rev)
  }

  const apply = (a: HTMLAnchorElement, row: Row) => {
    if (!node.contains(a)) return
    if (row.label != null) fillLabel(a, row.label, row.tail ?? '')
    else a.textContent = row.text
    a.classList.toggle('md-script--err', row.err)
    if (row.title) a.title = row.title
    else a.removeAttribute('title')
  }

  const evalOne = async (a: HTMLAnchorElement, parsed: Parsed) => {
    if (a.dataset.busy) return
    const key = cacheKey(node, parsed)
    a.dataset.busy = '1'
    a.classList.add('md-script--busy')
    try {
      const host = valueHost(node)
      let value: string
      if (parsed.set && host && !parsed.auto) {
        await rollValue(host.kind, host.id, parsed.set, { script: parsed.id, arg: parsed.arg })
        // The result lives in the variable (its chip reads again); the formula stays as it was. Writing « = 8» into it too
        // would widen the link until the text is redrawn, and stay there when the new value equals the old one.
        touchValues(hostKey(host.kind, host.id))
        return
      } else {
        value = await evalScriptText(parsed.id, { _: parsed.arg }, host, parsed.on)
      }
      const out = { value }
      const row = rowFor(parsed, out.value)
      if (parsed.auto) remember(key, row)
      apply(a, row)
    } catch (err) {
      const row = rowFor(parsed, null, err instanceof Error ? err.message : String(err))
      if (parsed.auto) remember(key, row)
      apply(a, row)
    } finally {
      delete a.dataset.busy
      a.classList.remove('md-script--busy')
    }
  }

  const anchors = () => [...node.querySelectorAll<HTMLAnchorElement>('a[href^="?script="]')]

  /** Cache hits paint at once. Links still waiting for a roll are returned. */
  const paint = () => {
    const wait: HTMLAnchorElement[] = []
    let used = 0
    for (const a of anchors()) {
      a.classList.add('md-script')
      const parsed = parseScriptHref(a.getAttribute('href') || '')
      if (parsed?.hide && !parsed.auto && !a.title) a.title = 'Посчитать'
      if (parsed?.set && !a.title) a.title = `Посчитать и записать в ${parsed.set}`
      if (parsed?.on && !a.title) a.title = `Выполнить на раннере ${parsed.on}`
      if (parsed && !parsed.hide && !a.dataset.settled && a.dataset.lrev !== String(rev)) fillLabel(a, parsed.arg, '')
      if (!parsed?.auto) continue
      if (a.dataset.settled) {
        used++
        continue
      }
      if (used >= MAX_AUTO_SCRIPTS) continue
      const hit = cache.get(cacheKey(node, parsed))
      if (hit) {
        used++
        a.dataset.settled = '1'
        apply(a, hit)
        continue
      }
      wait.push(a)
    }
    return wait
  }

  const run = async (wait: HTMLAnchorElement[]) => {
    let used = anchors().filter((a) => a.dataset.settled && parseScriptHref(a.getAttribute('href') || '')?.auto).length
    const todo: HTMLAnchorElement[] = []
    for (const a of wait) {
      if (!node.contains(a) || a.dataset.settled) continue
      const parsed = parseScriptHref(a.getAttribute('href') || '')
      if (!parsed?.auto || used >= MAX_AUTO_SCRIPTS) continue
      used++
      a.dataset.settled = '1'
      todo.push(a)
    }
    let next = 0
    async function worker() {
      while (next < todo.length) {
        const a = todo[next++]
        const parsed = parseScriptHref(a.getAttribute('href') || '')
        if (parsed) await evalOne(a, parsed)
      }
    }
    await Promise.all(Array.from({ length: Math.min(4, todo.length) }, () => worker()))
  }

  let pendingKey = ''
  const mo = new MutationObserver(() => kick())
  const kick = () => {
    mo.disconnect()
    const wait = paint()
    mo.observe(node, { childList: true, subtree: true })
    const key = wait.map((a) => a.getAttribute('href')).join('\n')
    if (!wait.length) {
      window.clearTimeout(timer)
      pendingKey = ''
      return
    }
    // An icon or a class change repeats the same links: keep the roll already scheduled.
    if (key === pendingKey) return
    pendingKey = key
    window.clearTimeout(timer)
    const delay = armed ? 200 : 0
    armed = true
    timer = window.setTimeout(() => {
      pendingKey = ''
      void run(wait)
    }, delay)
  }
  kick()

  const onClick = (e: MouseEvent) => {
    const a = e.target instanceof Element ? (e.target.closest('a.md-script') as HTMLAnchorElement | null) : null
    if (!a || !node.contains(a)) return
    e.preventDefault()
    e.stopPropagation()
    const parsed = parseScriptHref(a.getAttribute('href') || '')
    if (!parsed || a.dataset.busy) return
    a.dataset.settled = '1'
    void evalOne(a, parsed)
  }
  node.addEventListener('click', onClick, true)
  return {
    update(next: Record<string, string> = {}) {
      names = next
      rev++
      kick()
    },
    destroy() {
      mo.disconnect()
      window.clearTimeout(timer)
      node.removeEventListener('click', onClick, true)
    },
  }
}
