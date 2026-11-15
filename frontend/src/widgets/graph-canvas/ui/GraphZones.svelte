<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import { centerPanel } from '@/shared/lib/centerPanel.ts'
  import { readZones, type GraphZone } from '@/entities/graph'
  let { zones, cam, drawing = false, worldAt, onChange, onFinish, tools }: {
    tools?: import('svelte').Snippet
    zones: GraphZone[]; cam: { x: number; y: number; k: number }; drawing?: boolean
    worldAt: (e: { clientX: number; clientY: number }) => { x: number; y: number }
    onChange: (zones: GraphZone[]) => void; onFinish: () => void
  } = $props()
  let selected = $state<string | null>(null)
  let editor = $state<HTMLDivElement | null>(null)
  onMount(() => {
    const dismiss = (e: Event) => { if (e.type === 'click' && e.target instanceof Element && e.target.closest('.zones')) return; if (selected && editor && !e.composedPath().includes(editor)) selected = null }
    document.addEventListener('pointerdown', dismiss, true)
    document.addEventListener('click', dismiss, true)
    return () => { document.removeEventListener('pointerdown', dismiss, true); document.removeEventListener('click', dismiss, true) }
  })
  let drag = $state<{ kind: 'draw' | 'move' | 'resize'; start: { x: number; y: number }; original: GraphZone; pointer: number } | null>(null)
  let preview = $state<GraphZone | null>(null)
  const shown = $derived(preview ? [...zones.filter((z) => z.id !== preview!.id), preview] : zones)
  const current = $derived(zones.find((z) => z.id === selected))

  function start(e: PointerEvent, zone?: GraphZone, kind: 'draw' | 'move' | 'resize' = 'draw') {
    if (e.button !== 0 || (kind === 'draw' && !drawing)) return
    e.preventDefault(); e.stopPropagation()
    const point = worldAt(e)
    const original = zone ? { ...zone } : { id: crypto.randomUUID(), title: 'Новая зона', color: '#6a8fbf', x: point.x, y: point.y, w: 0, h: 0 }
    selected = original.id
    drag = { kind, start: point, original, pointer: e.pointerId }
    preview = { ...original }
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  }
  function move(e: PointerEvent) {
    if (!drag || e.pointerId !== drag.pointer) return
    e.stopPropagation()
    const point = worldAt(e), dx = point.x - drag.start.x, dy = point.y - drag.start.y
    const z = drag.original
    preview = drag.kind === 'draw' ? { ...z, x: Math.min(z.x, point.x), y: Math.min(z.y, point.y), w: Math.abs(dx), h: Math.abs(dy) }
      : drag.kind === 'move' ? { ...z, x: z.x + dx, y: z.y + dy }
      : { ...z, w: Math.max(40, z.w + dx), h: Math.max(40, z.h + dy) }
  }
  function finish(e: PointerEvent) {
    if (!drag || e.pointerId !== drag.pointer) return
    e.stopPropagation()
    if (e.type !== 'pointercancel' && preview && preview.w >= 40 && preview.h >= 40) onChange(readZones([...zones.filter((z) => z.id !== preview!.id), preview]))
    drag = null; preview = null
    onFinish()
  }
  function change(patch: Partial<GraphZone>) {
    if (!current) return
    const next = readZones([{ ...current, ...patch }])[0]
    if (next) onChange(zones.map((z) => z.id === current.id ? next : z))
  }
  function onKey(e: KeyboardEvent) {
    if (e.key !== 'Escape' || (!drawing && !selected && !drag)) return
    drag = null; preview = null; selected = null; onFinish()
  }
</script>

<svelte:window onkeydown={onKey} />
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="zones" class:zones--drawing={drawing} onpointerdown={(e) => start(e)} onpointermove={move} onpointerup={finish} onpointercancel={finish} oncontextmenu={(e) => { if (drawing) { e.preventDefault(); e.stopPropagation(); onFinish() } }}>
  <div class="zones__world" style:transform="translate({cam.x}px, {cam.y}px) scale({cam.k})">
    {#each shown as z (z.id)}
      <div class="zone" class:zone--selected={selected === z.id} style:left="{z.x}px" style:top="{z.y}px" style:width="{z.w}px" style:height="{z.h}px" style:--zone-color={z.color}>
        <button type="button" class="zone__title" aria-label={`Зона: ${z.title}`} title="Потяните, чтобы переместить зону" onpointerdown={(e) => start(e, z, 'move')} onclick={() => selected = z.id}>{z.title}</button>
        <button type="button" class="zone__resize" aria-label={`Изменить размер зоны: ${z.title}`} onpointerdown={(e) => start(e, z, 'resize')}>◢</button>
      </div>
    {/each}
  </div>
</div>
{#if drawing}
  <p class="zones__hint">Нарисуйте область на карте · Esc или ПКМ — отмена</p>
{/if}
{#if current && !drawing}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="zones__editor" bind:this={editor} use:centerPanel={{ center: cam.x + ((preview?.x ?? current.x) + (preview?.w ?? current.w) / 2) * cam.k }} style:top="{cam.y + ((preview?.y ?? current.y) + (preview?.h ?? current.h)) * cam.k + 8}px" role="group" aria-label="Настройки зоны" onpointerdown={(e) => e.stopPropagation()} oncontextmenu={(e) => e.stopPropagation()}>
    <input aria-label="Название зоны" maxlength="120" value={current.title} onchange={(e) => change({ title: e.currentTarget.value })} />
    <input type="color" aria-label="Цвет зоны" value={current.color} onchange={(e) => change({ color: e.currentTarget.value })} />
    <label><input type="number" min="40" max="100000" aria-label="Ширина зоны" value={Math.round(current.w)} onchange={(e) => change({ w: Number(e.currentTarget.value) })} /></label>
    <span aria-hidden="true">×</span><label><input type="number" min="40" max="100000" aria-label="Высота зоны" value={Math.round(current.h)} onchange={(e) => change({ h: Number(e.currentTarget.value) })} /></label>
    <button type="button" class="zones__delete" aria-label="Удалить зону" title="Удалить зону" onclick={() => { onChange(zones.filter((z) => z.id !== selected)); selected = null }}><Icon name="delete" size={16} /></button>
  </div>
{/if}
<div class="zones__bottom">{@render tools?.()}</div>

<style>
  .zones { position: absolute; inset: 0; pointer-events: none; }
  .zones--drawing { z-index: 12; pointer-events: auto; cursor: crosshair; }
  .zones__world { position: absolute; left: 0; top: 0; width: 0; height: 0; transform-origin: 0 0; }
  .zone { position: absolute; box-sizing: border-box; border: 2px dashed var(--zone-color); border-radius: 12px; background: color-mix(in srgb, var(--zone-color) 8%, transparent); }
  .zone--selected { border-style: solid; }
  .zone__title { position: absolute; left: 10px; top: -16px; max-width: calc(100% - 20px); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 4px 9px; border: 1px solid var(--zone-color); border-radius: 6px; background: var(--color-bg-raised, #1a1a1a); color: var(--zone-color); font: inherit; cursor: move; pointer-events: auto; touch-action: none; }
  .zone__resize { position: absolute; right: 0; bottom: 0; border: 0; background: transparent; color: var(--zone-color); cursor: nwse-resize; pointer-events: auto; touch-action: none; }
  .zones--drawing .zone__title, .zones--drawing .zone__resize { pointer-events: none; }
  .zones__hint { position: absolute; z-index: 13; top: 8px; left: 50%; transform: translateX(-50%); margin: 0; padding: 8px 12px; border-radius: 6px; background: var(--color-bg-raised, #1a1a1a); pointer-events: none; font-size: var(--text-sm); }
  .zones__bottom { position: absolute; z-index: 14; left: .6rem; right: .6rem; bottom: .6rem; display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: .6rem; pointer-events: none; }
  .zones__bottom > :global(*) { pointer-events: auto; }
  .zones__bottom > :global(.gtools) { margin-left: auto; }
  .zones__editor { position: absolute; z-index: 14; min-height: 2rem; flex: 0 1 auto; width: max-content; max-width: calc(100% - 16px); box-sizing: border-box; display: flex; flex-wrap: wrap; align-items: center; gap: 6px; padding: 2px; background: var(--color-bg-raised, #1a1a1a); border: 0; border-radius: 8px; font-size: var(--text-xs); }
  .zones__editor input { min-width: 0; height: 28px; box-sizing: border-box; padding: 2px 4px; font: inherit; background: transparent; color: inherit; border: 1px solid var(--color-border, #333); border-radius: 4px; }
  .zones__editor input[type=number] { width: 4rem; }
  .zones__editor input[type=color] { appearance: none; -webkit-appearance: none; flex: none; width: 22px; height: 22px; padding: 0; border: 0; border-radius: 50%; background: transparent; cursor: pointer; overflow: hidden; }
  .zones__editor input:not([type]) { width: 10rem; }
  .zones__editor label { display: inline-flex; align-items: center; gap: 3px; }
  .zones__editor button { height: 28px; box-sizing: border-box; padding: 0 .5rem; font: inherit; }
  .zones__editor input[type=color]::-webkit-color-swatch-wrapper { padding: 0; }
  .zones__editor input[type=color]::-webkit-color-swatch { border: 0; border-radius: 50%; }
  .zones__editor input[type=color]::-moz-color-swatch { border: 0; border-radius: 50%; }
  .zones__editor .zones__delete { display: grid; place-items: center; flex: none; width: 28px; padding: 0; border: 0; border-radius: 4px; background: transparent; color: var(--color-danger, #b54a3a); cursor: pointer; }
  .zones__delete:hover { background: color-mix(in srgb, var(--color-danger, #b54a3a) 12%, transparent); }
</style>
