import DOMPurify from 'dompurify'
import type { Mermaid } from 'mermaid'

/**
 * ```mermaid fenced blocks: the renderer keeps them as ordinary code blocks, and
 * this pass replaces each with a sanitized SVG. Lazy: `mermaid` is a dynamic
 * import, so the library loads only when a note actually has a diagram.
 */
let mod: Mermaid | null = null
let initializedTheme: 'dark' | 'default' | null = null
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

/** Init (or re-init) for a theme; themes may switch between renders (PDF export wants light on white pages). */
async function mermaid(theme: 'dark' | 'default' = preferredTheme()) {
  if (!mod) {
    mod = (await import('mermaid')).default
    initializedTheme = null
  }
  if (initializedTheme !== theme) {
    mod.initialize({
      startOnLoad: false,
      securityLevel: 'strict',
      theme,
      // The themes' dropShadow is a light-gray halo (rgba(185,185,185,1)) that reads as a white
      // glow around dark nodes: turned off everywhere.
      themeVariables: { dropShadow: 'none' },
      // SVG text labels, not foreignObject HTML: safer to sanitize, same look.
      // Since mermaid 12 this only works at the root level, where it overrides the defaults.
      htmlLabels: false,
    })
    initializedTheme = theme
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
      m = m ?? (await mermaid(preferredTheme()))
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

/** Rasterize one diagram source to PNG bytes for the PDF export (2x, capped at 2048 px);
 * null when the render or the rasterization fails — the PDF then keeps the source. */
export async function renderDiagramPng(
  code: string,
  opts: { scale?: number; theme?: 'dark' | 'default'; background?: string } = {}
): Promise<Uint8Array | null> {
  const id = `md-mermaid-${++counter}`
  try {
    const m = await mermaid(opts.theme ?? preferredTheme())
    const cached = svgCache.get(code)
    const svg = cached ?? (await m.render(id, code)).svg
    if (!cached) {
      if (svgCache.size >= SVG_CACHE_MAX) svgCache.delete(svgCache.keys().next().value as string)
      svgCache.set(code, svg)
    }
    return await svgToPng(sanitizeSvg(svg), opts.scale ?? 2, opts.background)
  } catch {
    // A failed render leaves mermaid's own error element in the body: remove it.
    document.getElementById(`d${id}`)?.remove()
    document.getElementById(id)?.remove()
    return null
  }
}

async function svgToPng(svg: string, scale: number, background?: string): Promise<Uint8Array | null> {
  const vb = /viewBox="([\d.\s]+)"/.exec(svg)
  let w = 0
  let h = 0
  if (vb) {
    const p = vb[1].trim().split(/\s+/)
    w = +p[2]
    h = +p[3]
  }
  if (!w || !h || typeof document === 'undefined') return null
  const url = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml' }))
  try {
    const img = new Image()
    img.src = url
    await img.decode()
    const s = Math.min(scale, 2048 / Math.max(w, h))
    const canvas = document.createElement('canvas')
    canvas.width = Math.max(1, Math.round(w * s))
    canvas.height = Math.max(1, Math.round(h * s))
    const ctx = canvas.getContext('2d')
    if (!ctx) return null
    if (background) {
      ctx.fillStyle = background
      ctx.fillRect(0, 0, canvas.width, canvas.height)
    }
    ctx.drawImage(img, 0, 0, canvas.width, canvas.height)
    const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/png'))
    if (!blob) return null
    return new Uint8Array(await blob.arrayBuffer())
  } finally {
    URL.revokeObjectURL(url)
  }
}
