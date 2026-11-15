import { mapSourceOffsetToRendered, rangeFromTextOffset } from './mdCaretMap.ts'

type Options = { enabled: boolean; source: string }
export function middleSourceOffset(lines: { y: number; offset: number }[], middle: number) {
  let lo = 0,
    hi = lines.length
  while (lo < hi) {
    const mid = (lo + hi) >>> 1
    if (lines[mid].y <= middle) lo = mid + 1
    else hi = mid
  }
  return lines[Math.max(0, lo - 1)]?.offset ?? 0
}

export function syncAnchorFraction(passed: number, remaining: number, visible: number) {
  const edge = Math.max(1, visible)
  return (1 - Math.min(1, remaining / edge) + Math.min(1, passed / edge)) / 2
}

export function followScrollPosition(current: number, target: number, elapsedMs: number) {
  return current + (target - current) * (1 - Math.exp(-Math.max(0, elapsedMs) / 110))
}

/** The raw text/page owns scrolling; the preview follows the middle visible line. */
export function syncNoteColumns(node: HTMLElement, initial: Options) {
  let options = initial,
    frame = 0,
    signature = '',
    lines: { y: number; offset: number }[] = []
  const page = node.closest<HTMLElement>('.notes__page')
  let motion = 0,
    last = 0,
    destination = 0
  function follow(preview: HTMLElement, target: number) {
    destination = Math.max(0, Math.min(target, preview.scrollHeight - preview.clientHeight))
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      preview.scrollTop = destination
      return
    }
    if (motion) return
    last = performance.now()
    const step = (now: number) => {
      motion = 0
      if (!options.enabled || !preview.isConnected) return
      const distance = destination - preview.scrollTop
      if (Math.abs(distance) < 1) {
        preview.scrollTop = destination
        return
      }
      const next = followScrollPosition(preview.scrollTop, destination, Math.min(50, now - last))
      const before = preview.scrollTop
      preview.scrollTop = next
      last = now
      if (Math.abs(preview.scrollTop - before) < 0.1) {
        preview.scrollTop = destination
        return
      }
      motion = requestAnimationFrame(step)
    }
    motion = requestAnimationFrame(step)
  }
  function measure(ta: HTMLTextAreaElement) {
    const cs = getComputedStyle(ta)
    const key = `${options.source}\0${ta.clientWidth}\0${cs.font}\0${cs.lineHeight}`
    if (key === signature) return
    signature = key
    const mirror = document.createElement('div')
    mirror.style.cssText =
      'position:absolute;visibility:hidden;left:-99999px;top:0;white-space:pre-wrap;overflow-wrap:break-word;box-sizing:border-box;'
    for (const prop of [
      'font',
      'lineHeight',
      'letterSpacing',
      'tabSize',
      'paddingTop',
      'paddingRight',
      'paddingBottom',
      'paddingLeft',
      'borderTopWidth',
      'borderLeftWidth',
      'borderRightWidth',
      'borderBottomWidth',
      'borderStyle',
    ] as const)
      mirror.style[prop] = cs[prop]
    mirror.style.width = `${ta.offsetWidth}px`
    const spans: { element: HTMLElement; offset: number }[] = []
    for (const match of options.source.matchAll(/\s+|\S+/gu)) {
      if (/^\s/.test(match[0])) mirror.append(document.createTextNode(match[0]))
      else {
        const span = document.createElement('span')
        span.textContent = match[0]
        mirror.append(span)
        spans.push({ element: span, offset: match.index })
      }
    }
    document.body.append(mirror)
    lines = []
    try {
      for (const { element, offset } of spans) {
        const y = element.offsetTop
        if (lines.at(-1)?.y !== y) lines.push({ y, offset })
      }
    } finally {
      mirror.remove()
    }
  }
  function sync() {
    frame = 0
    if (!options.enabled || !page) return
    const ta = node.querySelector<HTMLTextAreaElement>('textarea'),
      preview = node.querySelector<HTMLElement>('.notes__preview'),
      md = preview?.querySelector<HTMLElement>('.md')
    if (!ta || !preview || !md) return
    measure(ta)
    if (!lines.length) return
    const a = ta.getBoundingClientRect(),
      p = page.getBoundingClientRect()
    const top = Math.max(a.top, p.top),
      bottom = Math.min(a.bottom, p.bottom)
    if (bottom <= top) return
    const visible = bottom - top
    const passed = Math.max(0, top - a.top + ta.scrollTop)
    const remaining = Math.max(0, a.bottom - bottom)
    const fraction = syncAnchorFraction(passed, remaining, visible)
    if (passed <= 1) {
      follow(preview, 0)
      return
    }
    if (remaining <= 1) {
      follow(preview, preview.scrollHeight)
      return
    }
    const middle = top + visible * fraction - a.top + ta.scrollTop
    const offset = middleSourceOffset(lines, middle)
    const rendered = md.textContent ?? ''
    let target = mapSourceOffsetToRendered(options.source, rendered, offset)
    while (target < rendered.length && /\s/.test(rendered[target])) target++
    const range = rangeFromTextOffset(md, target)
    if (!range) return
    if (
      range.startContainer.nodeType === Node.TEXT_NODE &&
      range.startOffset < (range.startContainer.textContent?.length ?? 0)
    )
      range.setEnd(range.startContainer, range.startOffset + 1)
    const rect = range.getBoundingClientRect(),
      box = preview.getBoundingClientRect()
    if (!rect.height) return
    const previewTop = Math.max(box.top, p.top),
      previewBottom = Math.min(box.bottom, p.bottom)
    follow(preview, preview.scrollTop + rect.top - (previewTop + (previewBottom - previewTop) * fraction))
  }
  function schedule() {
    if (!frame) frame = requestAnimationFrame(sync)
  }
  page?.addEventListener('scroll', schedule, { passive: true })
  const sourceScroll = (e: Event) => {
    if (!(e.target instanceof Element && e.target.closest('.notes__preview'))) schedule()
  }
  node.addEventListener('scroll', sourceScroll, true)
  const observer = new ResizeObserver(schedule)
  observer.observe(node)
  const changes = new MutationObserver(schedule)
  changes.observe(node, { childList: true, subtree: true })
  schedule()
  return {
    update(next: Options) {
      options = next
      schedule()
    },
    destroy() {
      if (motion) cancelAnimationFrame(motion)
      if (frame) cancelAnimationFrame(frame)
      page?.removeEventListener('scroll', schedule)
      node.removeEventListener('scroll', sourceScroll, true)
      observer.disconnect()
      changes.disconnect()
    },
  }
}
