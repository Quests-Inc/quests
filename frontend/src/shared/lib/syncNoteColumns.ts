import { rangeFromTextOffset } from './mdCaretMap.ts'
import { orderedTextOffset } from './orderedTextMap.ts'

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

export function sourceLineAnchors(lines: { y: number; offset: number }[], y: number) {
  let lo = 0,
    hi = lines.length
  while (lo < hi) {
    const mid = (lo + hi) >>> 1
    if (lines[mid].y <= y) lo = mid + 1
    else hi = mid
  }
  const before = lines[Math.max(0, lo - 1)],
    after = lines[Math.min(lines.length - 1, lo)]
  return {
    before,
    after,
    fraction: before && after ? Math.max(0, Math.min(1, (y - before.y) / Math.max(1, after.y - before.y))) : 0,
  }
}

export function syncAnchorFraction(passed: number, remaining: number, visible: number) {
  const edge = Math.max(1, visible)
  return (1 - Math.min(1, remaining / edge) + Math.min(1, passed / edge)) / 2
}

export function followScrollPosition(current: number, target: number, elapsedMs: number) {
  return current + (target - current) * (1 - Math.exp(-Math.max(0, elapsedMs) / 110))
}

/** The raw text/page owns scrolling; the preview follows the middle visible line. */

/** textContent of `root` without svg <style> blobs (see the caller in syncNoteColumns). */
export function alignableText(root: HTMLElement): string {
  let out = ''
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      return node.parentElement?.closest('style') ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT
    },
  })
  let n: Node | null
  while ((n = walker.nextNode())) out += n.textContent ?? ''
  return out
}

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
    // On a narrow screen the columns stack; there is no adjacent preview to sync.
    if (preview.getBoundingClientRect().left < ta.getBoundingClientRect().right - 2) return
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
    measure(ta)
    if (!lines.length) return
    const middle = top + visible * fraction - a.top + ta.scrollTop
    const anchors = sourceLineAnchors(lines, middle)
    // Mermaid bakes its theme into a <style> inside every svg; its CSS words sit mid-document
    // in textContent and derail the monotonic word alignment, so diagrams contribute their
    // labels but not their stylesheet.
    const rendered = alignableText(md)
    const box = preview.getBoundingClientRect()
    // Source spans of ```mermaid fences, in order; the Nth fence maps to the Nth diagram block.
    const fences: { start: number; end: number }[] = []
    {
      const re = /^[ \t]*```[ \t]*mermaid[^\n]*\n[\s\S]*?\n[ \t]*```[ \t]*$/gim
      let m: RegExpExecArray | null
      while ((m = re.exec(options.source))) fences.push({ start: m.index, end: m.index + m[0].length })
    }
    const mermaidBlocks = Array.from(md.querySelectorAll<HTMLElement>('.md-codeblock[data-lang="mermaid"]'))
    // Source spans of <details>…</details> blocks, in order: every source line of a hidden
    // block targets the block's header line, so the preview locks onto it without drift.
    const detailSpans: { start: number; end: number }[] = []
    {
      const re = /^[ \t]*<details\b[^>]*>[^\n]*\n[\s\S]*?\n[ \t]*<\/details>[ \t]*$/gim
      let m: RegExpExecArray | null
      while ((m = re.exec(options.source))) detailSpans.push({ start: m.index, end: m.index + m[0].length })
    }
    const detailEls = Array.from(md.querySelectorAll<HTMLDetailsElement>('details'))
    // A closed <details> keeps its content laid out but unpainted: anchor at the plate itself.
    function closedDetailsY(from: Element | null): number | null {
      for (let up = from; up && up !== md; up = up.parentElement) {
        if (up.tagName === 'DETAILS' && !(up as HTMLDetailsElement).open) {
          const r = up.getBoundingClientRect()
          if (r.height) return preview.scrollTop + r.top - box.top
        }
      }
      return null
    }
    function renderedY(offset: number) {
      // A hidden block's whole source span — its markers, summary line and content — aims at
      // the block header (summary); an open block falls through to normal line mapping.
      const span = detailSpans.find((s) => offset >= s.start && offset <= s.end)
      const plate = span ? detailEls[detailSpans.indexOf(span)] : undefined
      if (span && plate && !plate.open) {
        const head = plate.querySelector(':scope > summary')
        const r = (head ?? plate).getBoundingClientRect()
        if (r.height) return preview.scrollTop + r.top - box.top
      }
      // A mermaid fence maps proportionally onto the rendered diagram: scrolling through its
      // source lines sweeps the picture instead of sticking to one edge.
      const fence = fences.find((f) => offset >= f.start && offset <= f.end)
      const block = fence ? mermaidBlocks[fences.indexOf(fence)] : undefined
      if (fence && block) {
        const hidden = closedDetailsY(block)
        if (hidden != null) return hidden
        const r = block.getBoundingClientRect()
        if (r.height) {
          const f = Math.max(0, Math.min(1, (offset - fence.start) / Math.max(1, fence.end - fence.start)))
          return preview.scrollTop + r.top - box.top + r.height * f
        }
        return null
      }
      let target = orderedTextOffset(options.source, rendered, offset, true, true)
      if (target == null) return null
      for (let tries = 0; tries < 64 && target < rendered.length; target++, tries++) {
        if (/\s/.test(rendered[target])) continue
        const range = rangeFromTextOffset(md, target)
        if (!range) return null
        if (
          range.startContainer.nodeType === Node.TEXT_NODE &&
          range.startOffset < (range.startContainer.textContent?.length ?? 0)
        )
          range.setEnd(range.startContainer, range.startOffset + 1)
        const el =
          range.startContainer.nodeType === Node.ELEMENT_NODE
            ? (range.startContainer as Element)
            : range.startContainer.parentElement
        const hidden = closedDetailsY(el)
        if (hidden != null) return hidden
        const rect = range.getBoundingClientRect()
        if (rect.height) return preview.scrollTop + rect.top - box.top
      }
      return null
    }
    const y0 = renderedY(anchors.before.offset),
      y1 = renderedY(anchors.after.offset)
    if (y0 == null && y1 == null) return
    const from = y0 ?? y1!,
      to = Math.max(from, y1 ?? from)
    const position = from + (to - from) * anchors.fraction
    const previewTop = Math.max(box.top, p.top),
      previewBottom = Math.min(box.bottom, p.bottom)
    follow(preview, position - (previewTop - box.top + (previewBottom - previewTop) * fraction))
  }
  function schedule() {
    if (!frame) frame = requestAnimationFrame(sync)
  }
  page?.addEventListener('scroll', schedule, { passive: true })
  const sourceScroll = (e: Event) => {
    if (!(e.target instanceof Element && e.target.closest('.notes__preview'))) schedule()
  }
  node.addEventListener('scroll', sourceScroll, true)
  const resized = schedule
  const fontsChanged = () => {
    signature = ''
    schedule()
  }
  window.addEventListener('resize', resized)
  document.fonts?.addEventListener('loadingdone', fontsChanged)
  const observer = new ResizeObserver(resized)
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
      window.removeEventListener('resize', resized)
      document.fonts?.removeEventListener('loadingdone', fontsChanged)
      observer.disconnect()
      changes.disconnect()
    },
  }
}
