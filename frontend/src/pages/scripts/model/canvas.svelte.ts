// Поле редактора: камера и жесты берём у карты (createGraphGestures), здесь — то, чего у карты нет:
// ноды скрипта как карточки, связь протягиванием от выхода ко входу, клавиши, миникарта и «вписать».
import { fitCamera, minimapModel } from '@/entities/graph'
import { NODE_W, type ScriptNodeView, viewNodes } from '@/entities/script'
import type { End } from '@/shared/api/scriptgraph.gen.ts'
import { createGraphGestures } from '@/widgets/graph-canvas'

import type { ScriptEditor } from './editor.svelte.ts'

type Pending = { from: End; cursor: { x: number; y: number }; start: { x: number; y: number }; moved: boolean }

export function createCanvas(ed: ScriptEditor) {
  let el = $state<HTMLElement | null>(null)
  let views = $state<ScriptNodeView[]>([])
  let selectMode = $state(false)
  let pending = $state<Pending | null>(null)
  let size = $state({ w: 0, h: 0 })
  let fitted: number | null = null
  let bgDown: { x: number; y: number } | null = null
  let added = 0

  // Where the gestures report that nodes were dragged: written back to the graph as one step.
  const saved = new Map<string, { x: number; y: number }>()
  saved.set = (key, pos) => {
    const v = views.find((n) => n.key === key)
    if (v) ed.moved({ id: v.id, x: pos.x, y: pos.y, w: v.w, h: v.h })
    return saved
  }

  const gest = createGraphGestures({
    getStage: () => el,
    nodes: () => views as never,
    nodeMap: () => new Map(views.map((n) => [n.key, n])) as never,
    touchView: () => {},
    isLinking: () => false,
    finishLink: () => {},
    setLinkCursor: () => {},
    selectMode: () => selectMode,
    getPicked: () => ed.picked,
    setPicked: (s) => {
      ed.picked = s
    },
    getSaved: () => saved,
    select: (n) => ed.select((n as unknown as ScriptNodeView).id),
  })

  /** Rebuild the node views from the graph (call from an effect: it reads the graph and the ports). */
  function sync() {
    views = viewNodes(ed.graph, ed.portsOf, ed.meta)
    if (el && ed.scriptId !== fitted && views.length && size.w) {
      fitted = ed.scriptId
      fit()
    }
  }

  function watchSize() {
    const node = el
    if (!node) return
    const read = () => {
      size = { w: node.clientWidth, h: node.clientHeight }
    }
    read()
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(read)
    ro.observe(node)
    return () => ro.disconnect()
  }

  function fit() {
    const cam = fitCamera(views, size)
    if (cam) gest.cam = cam
  }

  const view = (id: number) => views.find((n) => n.id === id)

  function startLink(from: End, e: PointerEvent) {
    e.stopPropagation()
    e.preventDefault()
    pending = { from, cursor: gest.worldAt(e), start: { x: e.clientX, y: e.clientY }, moved: false }
    el?.setPointerCapture(e.pointerId)
  }

  async function finishLink(to: End) {
    const p = pending
    pending = null
    if (p) await ed.link(p.from, to)
  }

  function onBgDown(e: PointerEvent) {
    bgDown = (e.target as Element).closest('[data-gkey]') ? null : { x: e.clientX, y: e.clientY }
    gest.onBgDown(e)
  }

  function onMove(e: PointerEvent) {
    gest.onMove(e)
    const p = pending
    if (p)
      pending = {
        ...p,
        cursor: gest.worldAt(e),
        moved: p.moved || Math.hypot(e.clientX - p.start.x, e.clientY - p.start.y) > 4,
      }
  }

  async function onUp(e: PointerEvent) {
    const p = pending
    if (p) {
      const target = document.elementFromPoint(e.clientX, e.clientY)?.closest('[data-in]')
      if (target instanceof HTMLElement && target.dataset.node) {
        await finishLink({ node: Number(target.dataset.node), port: target.dataset.in as string })
      } else if (p.moved) {
        pending = null
      } // a plain click on an output keeps the link pending until an input is clicked
    }
    // A click on the bare field (not a drag) drops the selection and an unfinished link.
    if (bgDown && Math.hypot(e.clientX - bgDown.x, e.clientY - bgDown.y) < 4 && !target(e)) {
      ed.select(null)
      ed.picked = new Set()
      pending = null
    }
    bgDown = null
    gest.onUp(e)
  }

  // A drop that landed on a port is not a click on the field.
  const target = (e: PointerEvent) => document.elementFromPoint(e.clientX, e.clientY)?.closest('[data-in], [data-out]')

  function onKey(e: KeyboardEvent) {
    const t = e.target as Element
    if (t.closest('input, textarea, select')) return
    const mod = e.ctrlKey || e.metaKey
    if (e.key === 'Delete' || e.key === 'Backspace') {
      e.preventDefault()
      void ed.remove()
    } else if (e.key === 'Escape') {
      pending = null
      selectMode = false
    } else if (mod && e.key.toLowerCase() === 'z') {
      e.preventDefault()
      if (e.shiftKey) ed.redo()
      else ed.undo()
    } else if (mod && e.key.toLowerCase() === 'y') {
      e.preventDefault()
      ed.redo()
    } else if (e.key.toLowerCase() === 'v' && !mod) {
      selectMode = !selectMode
    }
  }

  /** Put a new node near the middle of what is on screen. */
  function addAtCenter(type: string) {
    const r = el?.getBoundingClientRect()
    const c = r ? gest.worldAt({ clientX: r.left + r.width / 2, clientY: r.top + r.height / 2 }) : { x: 120, y: 120 }
    const jitter = (added++ % 6) * 26
    return ed.add(type, c.x - NODE_W / 2 + jitter, c.y - 50 + jitter)
  }

  /** Middle of the minimap becomes the middle of the field. */
  function miniGo(e: { currentTarget: HTMLElement; clientX: number; clientY: number }) {
    const m = mini
    if (!m) return
    const r = e.currentTarget.getBoundingClientRect()
    const wx = (e.clientX - r.left) / m.sc + m.x0
    const wy = (e.clientY - r.top) / m.sc + m.y0
    gest.cam = { ...gest.cam, x: size.w / 2 - wx * gest.cam.k, y: size.h / 2 - wy * gest.cam.k }
  }

  const mini = $derived(minimapModel({ nodes: views as never, ready: views.length > 0, cam: gest.cam, stage: size }))

  return {
    gest,
    get el() {
      return el
    },
    set el(v: HTMLElement | null) {
      el = v
    },
    get views() {
      return views
    },
    get pending() {
      return pending
    },
    get selectMode() {
      return selectMode
    },
    get mini() {
      return mini
    },
    toggleSelect: () => {
      selectMode = !selectMode
    },
    view,
    sync,
    watchSize,
    fit,
    startLink,
    finishLink,
    onBgDown,
    onMove,
    onUp,
    onKey,
    addAtCenter,
    miniGo,
  }
}

export type Canvas = ReturnType<typeof createCanvas>
