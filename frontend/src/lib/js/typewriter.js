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
function wordChunks(text) {
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
export function animateText(text, opts) {
  const {
    mode = 'reveal',
    granularity = 'char',
    stepMs = granularity === 'char' ? 22 : 55,
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

  return new Promise((resolve) => {
    let i = 0
    let acc = mode === 'erase' ? full : ''
    /** @type {number | null} */
    let timer = null

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
    tick()
  })
}
