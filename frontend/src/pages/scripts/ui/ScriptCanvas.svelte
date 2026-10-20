<script lang="ts">
  import { GraphMinimap, GraphTools } from '@/entities/graph'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
  import { anchorOf, customIdOf, KIND_ORDER, KIND_TITLES, nodeKey, portColor, type ScriptNodeView, wirePath } from '@/entities/script'
  import type { End } from '@/shared/api/scriptgraph.gen.ts'
  import type { Canvas } from '../model/canvas.svelte.ts'
  import type { ScriptEditor } from '../model/editor.svelte.ts'
  import ScriptNodeBox from './ScriptNodeBox.svelte'

  /**
   * The editor's field. The camera, panning, zoom, dragging and the selection frame are the map's own
   * (`createGraphGestures`); a script node is one more kind of card for them. A wire is drawn from a port's
   * anchor to a port's anchor, so what it joins is exact; dragging from an output to an input makes one.
   */
  let {
    ed,
    cv,
    onNewCode,
    onBake,
    onEditCustom,
    samples = [],
    scripts = [],
    onInsert,
  }: {
    ed: ScriptEditor
    cv: Canvas
    onNewCode?: () => void
    onBake?: () => void
    onEditCustom?: (type: string) => void
    /** whole graphs that can be put in as they are (the former samples, other scripts) */
    samples?: { id: string; title: string }[]
    scripts?: { id: number; title: string }[]
    onInsert?: (kind: 'sample' | 'script', id: string | number, at: { clientX: number; clientY: number }) => void
  } = $props()

  $effect(() => {
    cv.sync()
  })
  $effect(() => cv.watchSize())

  // Right click: on a card, what can be done to it (to all the picked ones, when it is one of several); on the bare field, what can be
  // added: «Добавить ноду» → a group of the catalogue → a node; «Добавить граф» → whole samples or scripts, unfolded.
  let menu = $state<{ open: boolean; x: number; y: number; node: number | null }>({ open: false, x: 0, y: 0, node: null })
  const pickedIds = $derived([...ed.picked].map((k) => Number(k.slice(1))))
  const target = $derived(menu.node != null && ed.picked.has(nodeKey(menu.node)) && ed.picked.size > 1 ? pickedIds : menu.node != null ? [menu.node] : [])

  const addItems = $derived([
    {
      id: 'add',
      label: 'Добавить ноду',
      children: KIND_ORDER.map((kind) => ({
        id: `g:${kind}`,
        label: KIND_TITLES[kind] ?? kind,
        children: ed.infos.filter((i) => i.kind === kind).map((i) => ({ id: `n:${i.name}`, label: i.title })),
      })).filter((g) => g.children.length),
    },
    ...(onInsert && (samples.length || scripts.length)
      ? [
          {
            id: 'graph',
            label: 'Добавить граф',
            children: [
              ...(samples.length
                ? [{ id: 'gs', label: 'Образцы', children: samples.map((x) => ({ id: `ins:sample:${x.id}`, label: x.title })) }]
                : []),
              ...(scripts.length
                ? [
                    {
                      id: 'gk',
                      label: 'Скрипты',
                      children: scripts.map((x) => ({ id: `ins:script:${x.id}`, label: x.title })),
                      search: scripts.map((x) => ({ id: `ins:script:${x.id}`, label: x.title })),
                    },
                  ]
                : []),
            ],
          },
        ]
      : []),
    ...(onNewCode || (onBake && ed.picked.size > 1) ? [{ id: 'sep', sep: true }] : []),
    ...(onNewCode ? [{ id: 'code', label: 'Новая нода из кода…' }] : []),
    ...(onBake && ed.picked.size > 1 ? [{ id: 'bake', label: `Запечь выделенное (${ed.picked.size})…` }] : []),
    { id: 'sep-view', sep: true },
    { id: 'select-mode', label: cv.selectMode ? 'Выйти из режима выделения' : 'Режим выделения' },
    { id: 'fit', label: 'Вписать в экран' },
  ])

  const nodeItems = $derived.by(() => {
    if (target.length > 1)
      return [{ id: 'clear-picked', label: 'Снять выделение' }, { id: 'sep', sep: true }, { id: 'del', label: `Удалить выделенные (${target.length})`, danger: true }]
    const n = ed.graph.nodes.find((x) => x.id === menu.node)
    const custom = n ? customIdOf(n.type) != null : false
    return [
      ...(custom && onEditCustom ? [{ id: 'edit', label: 'Править…' }] : []),
      ...(custom ? [{ id: 'unfold', label: 'Развернуть в ноды' }] : []),
      ...(onBake ? [{ id: 'bake1', label: 'Запечь в ноду…' }] : []),
      ...(custom || onBake ? [{ id: 'sep', sep: true }] : []),
      { id: 'del', label: 'Удалить', danger: true },
    ]
  })
  const menuItems = $derived(menu.node != null ? nodeItems : addItems)

  function onContext(e: MouseEvent) {
    e.preventDefault()
    cv.cancelLink()
    const key = (e.target as Element).closest('[data-gkey]')?.getAttribute('data-gkey')
    const node = key ? Number(key.slice(1)) : null
    // A card that is not among the picked ones is what the menu is about; the picked ones stay for the field's own menu.
    if (node != null && !ed.picked.has(key as string)) {
      ed.picked = new Set()
      ed.select(node)
    }
    menu = { open: true, x: e.clientX, y: e.clientY, node }
  }

  async function onMenuSelect(id: string) {
    const at = { clientX: menu.x, clientY: menu.y }
    if (id === 'select-mode') cv.toggleSelect()
    else if (id === 'fit') cv.fit()
    else if (id === 'clear-picked') ed.picked = new Set()
    else if (id === 'code') onNewCode?.()
    else if (id === 'bake') onBake?.()
    else if (id.startsWith('n:')) void cv.addAtPoint(id.slice(2), at)
    else if (id.startsWith('ins:')) {
      const [, kind, ref] = id.split(':')
      onInsert?.(kind as 'sample' | 'script', kind === 'script' ? Number(ref) : ref, at)
    } else if (menu.node != null) {
      const node = menu.node
      const type = ed.graph.nodes.find((x) => x.id === node)?.type ?? ''
      if (id === 'del') await ed.removeNodes(target)
      else if (id === 'edit') onEditCustom?.(type)
      else if (id === 'unfold') {
        try {
          await ed.unfold(node)
        } catch (err) {
          ed.error = err instanceof Error ? err.message : String(err)
        }
      } else if (id === 'bake1') {
        ed.picked = new Set([nodeKey(node)])
        onBake?.()
      }
    }
  }

  let hint = $derived(cv.pending ? 'Выберите вход, куда вести связь · Esc или клик по полю — отмена' : '')

  function wires() {
    const out: { d: string; color: string; index: number; selected: boolean; at: { x: number; y: number } }[] = []
    ed.graph.edges.forEach((e, index) => {
      const a = cv.view(e.from.node)
      const b = cv.view(e.to.node)
      if (!a || !b) return
      const type = ed.portsOf(e.from.node)?.outputs.find((p) => p.name === e.from.port)?.type ?? 'any'
      const p = anchorOf(a, ed.portsOf(e.from.node), 'out', e.from.port)
      const q = anchorOf(b, ed.portsOf(e.to.node), 'in', e.to.port)
      out.push({
        d: wirePath(p, q),
        color: portColor(type),
        index,
        selected: ed.selectedEdge === index,
        at: { x: (p.x + q.x) / 2, y: (p.y + q.y) / 2 },
      })
    })
    return out
  }

  function pendingPath() {
    const p = cv.pending
    if (!p) return ''
    const a = cv.view(p.from.node)
    if (!a) return ''
    return wirePath(anchorOf(a, ed.portsOf(p.from.node), 'out', p.from.port), p.cursor)
  }

  function onPort(side: 'in' | 'out', n: ScriptNodeView, port: string, e: PointerEvent | MouseEvent) {
    const end: End = { node: n.id, port }
    if (side === 'out') cv.startLink(end, e as PointerEvent)
    else void cv.finishLink(end)
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="scv"
  class:scv--full={cv.full}
  role="application"
  aria-label="Поле скрипта"
  tabindex="-1"
  bind:this={cv.el}
  onpointerdowncapture={cv.gest.onTouchDown}
  onpointerdown={cv.onBgDown}
  onpointermove={cv.onMove}
  onpointerup={cv.onUp}
  onpointercancel={cv.onUp}
  onwheel={cv.onWheel}
  onkeydown={cv.onKey}
  oncontextmenu={onContext}
>
  {#if hint}<p class="scv__hint">{hint}</p>{/if}
  {#if !ed.graph.nodes.length}<p class="scv__empty">Пусто. Добавьте ноды из палитры слева.</p>{/if}

  <div class="scv__world" style:transform="translate({cv.gest.cam.x}px, {cv.gest.cam.y}px) scale({cv.gest.cam.k})">
    <svg class="scv__wires" aria-hidden="true">
      {#each wires() as w (w.index)}
        <path d={w.d} fill="none" stroke={w.color} stroke-width={w.selected ? 3.2 : 2} opacity={w.selected ? 1 : 0.85} />
        <path
          d={w.d}
          fill="none"
          stroke="transparent"
          stroke-width="14"
          class="scv__hit"
          onpointerdown={(e) => {
            e.stopPropagation()
            ed.selectEdge(w.index)
            cv.el?.focus()
          }}
        />
        {#if w.selected}<circle cx={w.at.x} cy={w.at.y} r="4" fill={w.color} />{/if}
      {/each}
      {#if cv.pending}
        <path d={pendingPath()} fill="none" stroke="#c9a227" stroke-width="2.4" stroke-dasharray="6 4" />
      {/if}
    </svg>
    {#each cv.views as n (n.key)}
      <ScriptNodeBox
        {n}
        ports={ed.portsOf(n.id)}
        selected={ed.selectedNode === n.id}
        picked={ed.picked.has(n.key)}
        problems={ed.problemNodes.get(n.id) ?? []}
        pendingFrom={cv.pending?.from ?? null}
        onDown={(e) => cv.gest.onCardDown(e, n)}
        {onPort}
      />
    {/each}
  </div>

  {#if cv.gest.marquee}
    <div
      class="scv__marquee"
      style:left="{Math.min(cv.gest.marquee.x0, cv.gest.marquee.x1)}px"
      style:top="{Math.min(cv.gest.marquee.y0, cv.gest.marquee.y1)}px"
      style:width="{Math.abs(cv.gest.marquee.x1 - cv.gest.marquee.x0)}px"
      style:height="{Math.abs(cv.gest.marquee.y1 - cv.gest.marquee.y0)}px"
    ></div>
  {/if}

  {#if cv.mini}
    <GraphMinimap mini={cv.mini} onGo={cv.miniGo} />
  {/if}

  <GraphTools
    selectMode={cv.selectMode}
    full={cv.full}
    onToggleSelect={cv.toggleSelect}
    onFit={cv.fit}
    onToggleFull={cv.toggleFull}
    onUndo={ed.undo}
    onRedo={ed.redo}
    canUndo={ed.canUndo}
    canRedo={ed.canRedo}
  />
</div>

<ContextMenu open={menu.open} x={menu.x} y={menu.y} items={menuItems} onSelect={(id) => void onMenuSelect(id)} onClose={() => (menu.open = false)} />

<style>
  .scv {
    position: relative;
    flex: 1 1 auto;
    min-width: 0;
    min-height: 18rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--pill-radius, 12px);
    background: var(--pill-bg, #0f0f0f);
    overflow: hidden;
    cursor: grab;
    touch-action: none;
    user-select: none;
    outline: none;
  }

  .scv__world {
    position: absolute;
    left: 0;
    top: 0;
    width: 0;
    height: 0;
    transform-origin: 0 0;
  }

  .scv__wires {
    position: absolute;
    left: 0;
    top: 0;
    width: 1px;
    height: 1px;
    overflow: visible;
    pointer-events: none;
  }

  .scv__hit {
    pointer-events: stroke;
    cursor: pointer;
  }

  .scv__hint,
  .scv__empty {
    position: absolute;
    z-index: 4;
    margin: 0;
    font-size: var(--text-sm, 0.875rem);
    pointer-events: none;
  }

  .scv__hint {
    top: 0.6rem;
    left: 50%;
    transform: translateX(-50%);
    padding: 0.35rem 0.9rem;
    border: 1px solid var(--color-accent, #c9a227);
    border-radius: 999px;
    background: var(--color-bg-raised, #1a1a1a);
  }

  .scv__empty {
    inset: 0;
    display: grid;
    place-items: center;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .scv__marquee {
    position: absolute;
    z-index: 7;
    border: 1px solid #5aa9ff;
    background: color-mix(in srgb, #5aa9ff 14%, transparent);
    pointer-events: none;
  }

  .scv--full {
    position: fixed;
    inset: 0;
    z-index: 40;
    min-height: 0;
    border: none;
    border-radius: 0;
  }
</style>
