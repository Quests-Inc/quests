import DOMPurify from 'dompurify'
import type { Mermaid } from 'mermaid'

/**
 * ```mermaid fenced blocks: the renderer keeps them as ordinary code blocks, and
 * this pass replaces each with a sanitized SVG. Lazy: `mermaid` is a dynamic
 * import, so the library loads only when a note actually has a diagram.
 */
let mod: Mermaid | null = null
let initialized = false
let counter = 0
/** Rendered diagrams by source, so a re-render (saving a tick, value chips) reinstates them
 * instantly instead of blanking the layout and re-drawing after the fact. */
const svgCache = new Map<string, string>()
const SVG_CACHE_MAX = 40

function preferredTheme(): 'dark' | 'default' {
  try {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'default'
  } catch {
    return 'default'
  }
}

async function mermaid() {
  if (!mod) {
    mod = (await import('mermaid')).default
    initialized = false
  }
  if (!initialized) {
    mod.initialize({
      startOnLoad: false,
      securityLevel: 'strict',
      theme: preferredTheme(),
      // SVG text labels, not foreignObject HTML: safer to sanitize, same look.
      // Since mermaid 12 this only works at the root level, where it overrides the defaults.
      htmlLabels: false,
    })
    initialized = true
  }
  return mod
}

function sanitizeSvg(svg: string) {
  // Mermaid puts its theme in an id-scoped `<style>` (#md-mermaid-N …) inside the svg:
  // without it the shapes and labels lose their fills and turn black-on-black.
  return DOMPurify.sanitize(svg, { USE_PROFILES: { svg: true, svgFilters: true }, ADD_TAGS: ['style'] })
}

function injectSvg(block: HTMLElement, svg: string) {
  const holder = document.createElement('div')
  holder.className = 'md-mermaid'
  holder.innerHTML = sanitizeSvg(svg)
  const pre = block.querySelector('pre')
  if (pre) pre.style.display = 'none'
  block.prepend(holder)
  block.dataset.mdMermaid = 'done'
}

/** Replace every fresh `mermaid` code block under `root` with a diagram; a failed render keeps the source. */
export async function renderMermaidDiagrams(root: HTMLElement): Promise<void> {
  const blocks = Array.from(root.querySelectorAll<HTMLElement>('.md-codeblock[data-lang="mermaid"]'))
  if (!blocks.length) return
  let m: Mermaid | null = null
  for (const block of blocks) {
    if (block.dataset.mdMermaid) continue
    block.dataset.mdMermaid = 'pending'
    const code = block.querySelector('pre')?.textContent ?? ''
    const cached = svgCache.get(code)
    if (cached) {
      injectSvg(block, cached)
      continue
    }
    const id = `md-mermaid-${++counter}`
    try {
      m = m ?? (await mermaid())
      const { svg } = await m.render(id, code)
      if (svgCache.size >= SVG_CACHE_MAX) svgCache.delete(svgCache.keys().next().value as string)
      svgCache.set(code, svg)
      injectSvg(block, svg)
    } catch {
      // A failed render leaves mermaid's own error element in the body: remove it and keep the source.
      document.getElementById(`d${id}`)?.remove()
      document.getElementById(id)?.remove()
      block.dataset.mdMermaid = 'error'
    }
  }
}
