/** Reveal/erase text progressively — used for the "typed by an automated
 * quest" effect in QuestDetail. Two speeds: char-by-char for short plain
 * text (title), word-by-word for markdown source (description) — typing
 * markdown one raw character at a time flashes broken syntax mid-token
 * (`**bo` before the closing `**` lands), word chunks mostly land on
 * markdown token boundaries and avoid that. */

function prefersReducedMotion() {
  try {
    return window.matchMedia('(prefers-reduced-motion: reduce)').matches
  } catch {
    return false
  }
}

/** Split into chunks that re-concatenate exactly to the input: each chunk is
 * one non-space run plus any trailing whitespace, so revealing them in order
 * looks like words appearing left-to-right rather than dumping whole lines. */
function wordChunks(text: string) {
  return text.match(/\S+\s*|\s+/g) || []
}

/**
 * @param {string} text
 * @param {{
 *   mode?: 'reveal' | 'erase',
 *   granularity?: 'char' | 'word',
 *   stepMs?: number,
 *   onUpdate: (partial: string) => void,
 *   signal?: AbortSignal,
 * }} opts
 * @returns {Promise<void>} resolves when done or aborted
 */
export function animateText(
  text: string,
  opts: {
    mode?: 'reveal' | 'erase'
    granularity?: 'char' | 'word'
    stepMs?: number
    onUpdate: (partial: string) => void
    signal?: AbortSignal
  }
): Promise<void> {
  const {
    mode = 'reveal',
    granularity = 'char',
    // Reveal is 30% slower than erase: typing in reads better unhurried.
    stepMs = (granularity === 'char' ? 22 : 28) * (mode === 'reveal' ? 1.3 : 1),
    onUpdate,
    signal,
  } = opts
  const full = String(text ?? '')

  if (!full || prefersReducedMotion() || signal?.aborted) {
    onUpdate(mode === 'reveal' ? full : '')
    return Promise.resolve()
  }

  const units = granularity === 'char' ? Array.from(full) : wordChunks(full)
  const order = mode === 'erase' ? [...units].reverse() : units

  return new Promise<void>((resolve) => {
    let i = 0
    let acc = mode === 'erase' ? full : ''
    let timer: ReturnType<typeof setTimeout> | null = null

    function done() {
      if (timer != null) clearTimeout(timer)
      signal?.removeEventListener('abort', onAbort)
      resolve()
    }
    function onAbort() {
      onUpdate(mode === 'reveal' ? full : '')
      done()
    }
    function tick() {
      if (signal?.aborted) return
      if (i >= order.length) {
        onUpdate(mode === 'reveal' ? full : '')
        done()
        return
      }
      const unit = order[i]
      i += 1
      if (mode === 'reveal') {
        acc += unit
      } else {
        acc = acc.slice(0, -unit.length)
      }
      onUpdate(acc)
      timer = window.setTimeout(tick, stepMs)
    }

    signal?.addEventListener('abort', onAbort, { once: true })
    // Defer the first frame too (never call onUpdate synchronously): a
    // caller may invoke animateText from inside a Svelte $effect, and a
    // synchronous state write there re-enters the same effect mid-run
    // (effect_update_depth_exceeded) — see quest=231.
    timer = window.setTimeout(tick, stepMs)
  })
}

const TAIL_CHARS = 10
const DRAIN_STEP_MS = 36
let tailFadeUid = 0

/** Svelte action: while `active`, the last few characters of the element's
 * text fade in (older ones more opaque than newer), and when `active` drops
 * the tail finishes fading out instead of snapping to full ink. Uses the CSS
 * Custom Highlight API, so the DOM — which Svelte owns via {@html} — is never
 * touched; browsers without it simply get no fade.
 * @param {HTMLElement} node
 * @param {boolean} active */
export function tailFade(node: HTMLElement, active: boolean) {
  if (typeof CSS === 'undefined' || !CSS.highlights || typeof Highlight === 'undefined') {
    return {}
  }
  const uid = (tailFadeUid += 1)
  const names = Array.from({ length: TAIL_CHARS }, (_, d) => `tf${uid}-${d}`)
  const style = document.createElement('style')
  document.head.appendChild(style)
  const observer = new MutationObserver(() => paint())
  let on = false
  let shift = 0
  /** @type {number | null} */
  let drain: number | null = null

  function clear() {
    for (const n of names) CSS.highlights.delete(n)
    style.textContent = ''
  }

  function paint() {
    if (!on && drain == null) return
    const m = getComputedStyle(node).color.match(/[\d.]+/g) || ['128', '128', '128']
    const [r, g, b] = m
    const rules = []
    const ranges = names.map(() => [])
    const texts: Text[] = []
    const walker = document.createTreeWalker(node, NodeFilter.SHOW_TEXT)
    for (let t = walker.nextNode(); t; t = walker.nextNode()) texts.push(t as Text)
    let d = 0
    for (let i = texts.length - 1; i >= 0 && d < TAIL_CHARS; i -= 1) {
      const data = texts[i].data
      for (let j = data.length - 1; j >= 0 && d < TAIL_CHARS; j -= 1) {
        if (/\s/.test(data[j])) continue
        const range = new Range()
        range.setStart(texts[i], j)
        range.setEnd(texts[i], j + 1)
        ranges[d].push(range)
        d += 1
      }
    }
    for (let k = 0; k < TAIL_CHARS; k += 1) {
      const alpha = Math.min(1, (k + shift + 1) / (TAIL_CHARS + 1))
      if (ranges[k].length && alpha < 1) {
        CSS.highlights.set(names[k], new Highlight(...ranges[k]))
        rules.push(`::highlight(${names[k]}){color:rgba(${r},${g},${b},${alpha})}`)
      } else {
        CSS.highlights.delete(names[k])
      }
    }
    style.textContent = rules.join('\n')
  }

  function stopDrain() {
    if (drain != null) clearInterval(drain)
    drain = null
  }

  function set(value: boolean) {
    const next = !!value
    if (next === on) return
    on = next
    stopDrain()
    if (on) {
      shift = 0
      observer.observe(node, { childList: true, subtree: true, characterData: true })
      paint()
    } else {
      observer.disconnect()
      drain = window.setInterval(() => {
        shift += 1
        if (shift > TAIL_CHARS) {
          stopDrain()
          clear()
          return
        }
        paint()
      }, DRAIN_STEP_MS)
    }
  }

  set(active)
  return {
    update: set,
    destroy() {
      stopDrain()
      observer.disconnect()
      clear()
      style.remove()
    },
  }
}
